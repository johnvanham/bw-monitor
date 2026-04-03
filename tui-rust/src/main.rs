mod app;
mod k8s;
mod redis_client;
mod types;
mod ui;

use anyhow::Result;
use clap::Parser;
use crossterm::{
    event::{self, Event, KeyCode, KeyEventKind, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use ratatui::prelude::*;
use std::io;
use std::time::Duration;

#[derive(Parser)]
#[command(name = "bw-monitor", about = "BunkerWeb security monitor TUI")]
struct Cli {
    /// Kubernetes namespace for BunkerWeb
    #[arg(long, default_value = "bunkerweb")]
    namespace: String,

    /// Maximum number of initial reports to load (0 = all)
    #[arg(long, default_value_t = 10000)]
    max_entries: usize,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    // Try to connect — collect errors to show in TUI if they fail
    let result = connect(&cli).await;

    // Setup terminal
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let exit_result = match result {
        Ok(redis) => {
            let mut app = app::App::new(redis, cli.max_entries);
            app.load_initial().await;
            run_app(&mut terminal, &mut app).await
        }
        Err(e) => {
            // Show startup error in a TUI modal
            let dummy_redis =
                match redis_client::RedisClient::new("redis://127.0.0.1:0").await {
                    Ok(r) => r,
                    Err(_) => {
                        // Can't even create a dummy client — fall back to plain error
                        cleanup_terminal(&mut terminal)?;
                        return Err(e);
                    }
                };
            let mut app = app::App::new(dummy_redis, 0);
            app.loading = false;
            app.show_error(format!("Startup error:\n\n{:#}", e), true);
            run_app(&mut terminal, &mut app).await
        }
    };

    cleanup_terminal(&mut terminal)?;
    exit_result
}

async fn connect(cli: &Cli) -> Result<redis_client::RedisClient> {
    eprintln!("Connecting to Kubernetes...");
    let k8s_client = k8s::Client::new(&cli.namespace).await?;

    eprintln!("Finding Redis pod...");
    let redis_pod = k8s_client.find_redis_pod().await?;
    eprintln!("Found Redis pod: {}", redis_pod);

    eprintln!("Starting port-forward to {}...", redis_pod);
    let local_port = k8s_client.start_port_forward(&redis_pod, 6379).await?;

    eprintln!("Connecting to Redis via port-forward (localhost:{})...", local_port);
    let redis =
        redis_client::RedisClient::new(&format!("redis://127.0.0.1:{}", local_port)).await?;

    eprintln!("Connected.");
    Ok(redis)
}

async fn run_app(
    terminal: &mut Terminal<CrosstermBackend<io::Stdout>>,
    app: &mut app::App,
) -> Result<()> {
    let tick_rate = Duration::from_millis(100);
    let poll_interval = Duration::from_secs(2);
    let mut last_poll = std::time::Instant::now();
    let mut bans_loaded = false;
    let mut needs_bans_load = false;

    loop {
        terminal.draw(|f| ui::draw(f, app))?;

        // Drain any completed DNS lookups
        app.drain_dns_results();

        if event::poll(tick_rate)? {
            if let Event::Key(key) = event::read()? {
                if key.kind == KeyEventKind::Press {
                    if key.code == KeyCode::Char('c')
                        && key.modifiers.contains(KeyModifiers::CONTROL)
                    {
                        break;
                    }

                    let was_bans = matches!(app.current_view, app::ViewState::BansList);
                    let was_r = key.code == KeyCode::Char('r');

                    if app.handle_key(key.code, key.modifiers) {
                        break;
                    }

                    let now_bans = matches!(app.current_view, app::ViewState::BansList);
                    if (now_bans && !was_bans && !bans_loaded) || (now_bans && was_r) {
                        needs_bans_load = true;
                    }
                }
            }
        }

        if needs_bans_load {
            app.load_bans().await;
            bans_loaded = true;
            needs_bans_load = false;
        }

        if !app.paused && last_poll.elapsed() >= poll_interval {
            app.poll_new().await;
            last_poll = std::time::Instant::now();
        }
    }

    Ok(())
}

fn cleanup_terminal(terminal: &mut Terminal<CrosstermBackend<io::Stdout>>) -> Result<()> {
    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    terminal.show_cursor()?;
    Ok(())
}
