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

    eprintln!("Connecting to Kubernetes...");
    let k8s_client = k8s::Client::new(&cli.namespace).await?;

    eprintln!("Finding Redis pod...");
    let redis_pod = k8s_client.find_redis_pod().await?;
    eprintln!("Found Redis pod: {}", redis_pod);

    eprintln!("Starting port-forward to {}...", redis_pod);
    let local_port = k8s_client.start_port_forward(&redis_pod, 6379).await?;

    eprintln!("Connecting to Redis via port-forward (localhost:{})...", local_port);
    let redis = redis_client::RedisClient::new(&format!("redis://127.0.0.1:{}", local_port)).await?;

    eprintln!("Connected. Loading reports...");

    // Setup terminal
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    // Create app
    let mut app = app::App::new(redis, cli.max_entries);

    // Initial load
    app.load_initial().await;

    // Main loop
    let tick_rate = Duration::from_millis(100);
    let poll_interval = Duration::from_secs(2);
    let mut last_poll = std::time::Instant::now();
    let mut bans_loaded = false;
    let mut needs_bans_load = false;

    loop {
        terminal.draw(|f| ui::draw(f, &mut app))?;

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

                    // Check if switching to bans view or requesting refresh
                    let was_bans = matches!(app.current_view, app::ViewState::BansList);
                    let was_r = key.code == KeyCode::Char('r');

                    if app.handle_key(key.code, key.modifiers) {
                        break; // quit requested
                    }

                    // Load bans when switching to bans view for the first time, or on refresh
                    let now_bans = matches!(app.current_view, app::ViewState::BansList);
                    if (now_bans && !was_bans && !bans_loaded) || (now_bans && was_r) {
                        needs_bans_load = true;
                    }
                }
            }
        }

        // Load bans if needed
        if needs_bans_load {
            app.load_bans().await;
            bans_loaded = true;
            needs_bans_load = false;
        }

        // Poll for new reports
        if !app.paused && last_poll.elapsed() >= poll_interval {
            app.poll_new().await;
            last_poll = std::time::Instant::now();
        }
    }

    // Restore terminal
    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    terminal.show_cursor()?;

    Ok(())
}
