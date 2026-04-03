use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, List, ListItem, Paragraph, Wrap},
    Frame,
};

use crate::app::{App, FilterField, ViewState};
use crate::types::colour_for_ip;

// ── Colours ─────────────────────────────────────────────

const TEAL: Color = Color::Rgb(78, 201, 176);
const HEADER_BG: Color = Color::Rgb(51, 51, 51);
const DIM: Color = Color::Rgb(85, 85, 85);
const RED: Color = Color::Rgb(255, 107, 107);
const YELLOW: Color = Color::Rgb(220, 220, 170);
const BLUE: Color = Color::Rgb(86, 156, 214);
const TEXT: Color = Color::Rgb(212, 212, 212);

// ── Column widths ───────────────────────────���───────────

const COL_TIME: u16 = 19;
const COL_IP: u16 = 16;
const COL_CC: u16 = 4;
const COL_METHOD: u16 = 7;
const COL_STATUS: u16 = 6;
const COL_REASON: u16 = 14;
const COL_SERVER: u16 = 28;

fn calc_flex_widths(total_width: u16) -> (u16, u16) {
    let fixed = COL_TIME + COL_IP + COL_CC + COL_METHOD + COL_STATUS + COL_REASON + COL_SERVER + 8;
    let remaining = (total_width as i32 - fixed as i32).max(20) as u16;
    let url_w = remaining * 60 / 100;
    let ua_w = remaining - url_w;
    (url_w, ua_w)
}

fn truncate(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else if max_len <= 3 {
        s[..max_len].to_string()
    } else {
        format!("{}...", &s[..max_len - 3])
    }
}

fn pad_right(s: &str, width: usize) -> String {
    if s.len() >= width {
        s[..width].to_string()
    } else {
        format!("{}{}", s, " ".repeat(width - s.len()))
    }
}

fn format_time(dt: chrono::DateTime<chrono::Local>) -> String {
    dt.format("%Y-%m-%d %H:%M:%S").to_string()
}

fn parsed_ua(raw: &str) -> String {
    if raw.is_empty() || raw == "-" {
        return "-".to_string();
    }
    // Simple UA parsing: extract key info from the user agent string
    let raw_lower = raw.to_lowercase();
    let mut parts = Vec::new();

    // Browser detection
    if raw_lower.contains("chrome") && !raw_lower.contains("chromium") && !raw_lower.contains("edg") {
        parts.push("Chrome");
    } else if raw_lower.contains("firefox") {
        parts.push("Firefox");
    } else if raw_lower.contains("safari") && !raw_lower.contains("chrome") {
        parts.push("Safari");
    } else if raw_lower.contains("edg") {
        parts.push("Edge");
    } else if raw_lower.contains("curl") {
        parts.push("curl");
    } else if raw_lower.contains("python") {
        parts.push("Python");
    } else if raw_lower.contains("go-http") {
        parts.push("Go");
    }

    // OS detection
    if raw_lower.contains("windows") {
        parts.push("Windows");
    } else if raw_lower.contains("mac os") || raw_lower.contains("macos") {
        parts.push("macOS");
    } else if raw_lower.contains("linux") {
        parts.push("Linux");
    } else if raw_lower.contains("android") {
        parts.push("Android");
    } else if raw_lower.contains("iphone") || raw_lower.contains("ipad") {
        parts.push("iOS");
    }

    // Type detection
    if raw_lower.contains("bot") || raw_lower.contains("crawl") || raw_lower.contains("spider") {
        parts.push("Bot");
    } else if raw_lower.contains("mobile") {
        parts.push("Mobile");
    }

    if parts.is_empty() {
        truncate(raw, 30)
    } else {
        parts.join(" / ")
    }
}

// ── Main draw ─────────────────────���─────────────────────

pub fn draw(f: &mut Frame, app: &mut App) {
    let size = f.area();
    app.width = size.width;
    app.height = size.height;

    if app.loading {
        let text = Paragraph::new("  Loading reports from Redis...")
            .style(Style::default().fg(TEAL));
        f.render_widget(text, size);
        return;
    }

    match app.current_view {
        ViewState::ReportsList => draw_reports_list(f, app, size),
        ViewState::ReportDetail => draw_detail(f, app, size, "Block Detail"),
        ViewState::BansList => draw_bans_list(f, app, size),
        ViewState::BanDetail => draw_detail(f, app, size, "Ban Detail"),
    }

    // Overlay modals
    if app.filter_open {
        draw_filter_modal(f, app, size);
    }
    if app.exclude_modal_open {
        draw_exclude_modal(f, app, size);
    }
}

// ── Title bar ────────────────────���──────────────────────

fn render_title_bar(f: &mut Frame, area: Rect, title: &str, context: &str) {
    let style = Style::default().fg(Color::Rgb(26, 26, 26)).bg(TEAL);
    let title_style = style.add_modifier(Modifier::BOLD);

    let left = format!(" {} ", title);
    let right = format!(" {} ", context);
    let gap = area.width as usize - left.len().min(area.width as usize) - right.len().min(area.width as usize);

    let line = Line::from(vec![
        Span::styled(left, title_style),
        Span::styled(" ".repeat(gap.max(0)), style),
        Span::styled(right, style),
    ]);

    f.render_widget(Paragraph::new(line), area);
}

// ── Reports list ─────────���──────────────────────────────

fn draw_reports_list(f: &mut Frame, app: &mut App, area: Rect) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(1), // title
            Constraint::Length(1), // header
            Constraint::Min(1),   // list
            Constraint::Length(1), // status
            Constraint::Length(1), // help
        ])
        .split(area);

    // Title bar
    render_title_bar(f, chunks[0], "BW Monitor", "Live View");

    // Column header
    let (url_w, ua_w) = calc_flex_widths(area.width);
    let header_text = format!(
        "{} {} {} {} {} {} {} {} {}",
        pad_right("Time", COL_TIME as usize),
        pad_right("IP", COL_IP as usize),
        pad_right("CC", COL_CC as usize),
        pad_right("Method", COL_METHOD as usize),
        pad_right("Status", COL_STATUS as usize),
        pad_right("Reason", COL_REASON as usize),
        pad_right("Server", COL_SERVER as usize),
        pad_right("URL", url_w as usize),
        pad_right("User Agent", ua_w as usize),
    );
    let header = Paragraph::new(header_text)
        .style(Style::default().fg(Color::White).bg(HEADER_BG).add_modifier(Modifier::BOLD));
    f.render_widget(header, chunks[1]);

    // Report rows
    let data_rows = chunks[2].height as usize;
    let items: Vec<ListItem> = app
        .filtered_idx
        .iter()
        .enumerate()
        .skip(app.reports_scroll)
        .take(data_rows)
        .map(|(i, &idx)| {
            let r = &app.all_reports[idx];
            let ip_color = colour_for_ip(&r.ip);
            let row = format!(
                "{} {} {} {} {} {} {} {} {}",
                pad_right(&format_time(r.time()), COL_TIME as usize),
                pad_right(&r.ip, COL_IP as usize),
                pad_right(&r.country, COL_CC as usize),
                pad_right(&r.method, COL_METHOD as usize),
                pad_right(&format!("{}", r.status), COL_STATUS as usize),
                pad_right(&truncate(&r.reason, COL_REASON as usize), COL_REASON as usize),
                pad_right(&truncate(&r.server_name, COL_SERVER as usize), COL_SERVER as usize),
                pad_right(&truncate(&r.url, url_w as usize), url_w as usize),
                pad_right(&truncate(&parsed_ua(&r.user_agent), ua_w as usize), ua_w as usize),
            );

            let style = if i == app.reports_cursor {
                Style::default()
                    .fg(ip_color)
                    .bg(HEADER_BG)
                    .add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(ip_color)
            };

            ListItem::new(Line::from(Span::styled(row, style)))
        })
        .collect();

    let list = List::new(items);
    f.render_widget(list, chunks[2]);

    // Status bar
    render_reports_status(f, app, chunks[3]);

    // Help bar
    let help = "[1] Reports  [2] Bans  [Space] Pause  [Enter] Detail  [f] Filter  [c] Clear  [x] Exclude IP  [X] Excludes  [q] Quit";
    let help_widget = Paragraph::new(help).style(Style::default().fg(DIM));
    f.render_widget(help_widget, chunks[4]);
}

fn render_reports_status(f: &mut Frame, app: &App, area: Rect) {
    let mut parts: Vec<Span> = Vec::new();

    if app.paused {
        parts.push(Span::styled("[PAUSED]", Style::default().fg(RED).add_modifier(Modifier::BOLD)));
    } else {
        parts.push(Span::styled("[LIVE]", Style::default().fg(TEAL).add_modifier(Modifier::BOLD)));
    }

    parts.push(Span::styled(
        format!("  Showing {}/{}  ", app.filtered_idx.len(), app.total_reports),
        Style::default().fg(DIM),
    ));

    if app.filter.active {
        parts.push(Span::styled(
            format!("Filter: {}  ", app.filter.summary()),
            Style::default().fg(YELLOW),
        ));
    }

    if app.excludes.count() > 0 {
        parts.push(Span::styled(
            format!("{} IP(s) excluded  ", app.excludes.count()),
            Style::default().fg(DIM),
        ));
    }

    if let Some(ref err) = app.last_error {
        parts.push(Span::styled(
            format!("Err: {}", err),
            Style::default().fg(RED),
        ));
    }

    let status = Paragraph::new(Line::from(parts))
        .style(Style::default().bg(Color::Rgb(26, 26, 26)));
    f.render_widget(status, area);
}

// ── Bans list ──────────────────────────���────────────────

fn draw_bans_list(f: &mut Frame, app: &mut App, area: Rect) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(1), // title
            Constraint::Length(1), // header
            Constraint::Min(1),   // list
            Constraint::Length(1), // status
            Constraint::Length(1), // help
        ])
        .split(area);

    render_title_bar(f, chunks[0], "BW Monitor", "Active Bans");

    // Column header
    let header_text = format!(
        "{} {} {} {} {} {} {}",
        pad_right("IP", 16),
        pad_right("CC", 4),
        pad_right("Service", 30),
        pad_right("Reason", 14),
        pad_right("Banned At", 19),
        pad_right("Expires In", 12),
        pad_right("Events", 8),
    );
    let header = Paragraph::new(header_text)
        .style(Style::default().fg(Color::White).bg(HEADER_BG).add_modifier(Modifier::BOLD));
    f.render_widget(header, chunks[1]);

    // Ban rows
    let data_rows = chunks[2].height as usize;
    let items: Vec<ListItem> = app
        .filtered_ban_idx
        .iter()
        .enumerate()
        .skip(app.bans_scroll)
        .take(data_rows)
        .map(|(i, &idx)| {
            let ban = &app.bans[idx];
            let ip_color = colour_for_ip(&ban.ip);

            let expires_in = if ban.permanent {
                "permanent".to_string()
            } else {
                let secs = ban.ttl.as_secs();
                let h = secs / 3600;
                let m = (secs % 3600) / 60;
                format!("{}h {}m", h, m)
            };

            let row = format!(
                "{} {} {} {} {} {} {}",
                pad_right(&ban.ip, 16),
                pad_right(&ban.country, 4),
                pad_right(&truncate(&ban.service, 30), 30),
                pad_right(&truncate(&ban.reason, 14), 14),
                pad_right(&format_time(ban.time()), 19),
                pad_right(&expires_in, 12),
                pad_right(&format!("{}", ban.events.len()), 8),
            );

            let style = if i == app.bans_cursor {
                Style::default()
                    .fg(ip_color)
                    .bg(HEADER_BG)
                    .add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(ip_color)
            };

            ListItem::new(Line::from(Span::styled(row, style)))
        })
        .collect();

    let list = List::new(items);
    f.render_widget(list, chunks[2]);

    // Status bar
    let mut parts: Vec<Span> = Vec::new();
    parts.push(Span::styled(
        format!("Showing {}/{} ban(s)  ", app.filtered_ban_idx.len(), app.bans.len()),
        Style::default().fg(DIM),
    ));
    if app.filter.active {
        parts.push(Span::styled(
            format!("Filter: {}  ", app.filter.summary()),
            Style::default().fg(YELLOW),
        ));
    }
    if app.excludes.count() > 0 {
        parts.push(Span::styled(
            format!("{} IP(s) excluded  ", app.excludes.count()),
            Style::default().fg(DIM),
        ));
    }
    if let Some(ref err) = app.last_error {
        parts.push(Span::styled(format!("Err: {}", err), Style::default().fg(RED)));
    }
    let status = Paragraph::new(Line::from(parts)).style(Style::default().bg(Color::Rgb(26, 26, 26)));
    f.render_widget(status, chunks[3]);

    // Help bar
    let help = "[1] Reports  [2] Bans  [Enter] Detail  [f] Filter  [c] Clear  [x] Exclude IP  [X] Excludes  [r] Refresh  [q] Quit";
    let help_widget = Paragraph::new(help).style(Style::default().fg(DIM));
    f.render_widget(help_widget, chunks[4]);
}

// ── Detail view ─────────────────────────────���───────────

fn draw_detail(f: &mut Frame, app: &mut App, area: Rect, context_label: &str) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(1), // title
            Constraint::Min(1),   // content
            Constraint::Length(1), // help
        ])
        .split(area);

    render_title_bar(f, chunks[0], "BW Monitor", context_label);

    let lines = match app.current_view {
        ViewState::ReportDetail => build_report_detail_lines(app),
        ViewState::BanDetail => build_ban_detail_lines(app),
        _ => vec![],
    };

    app.detail_content_height = lines.len();
    let visible_height = chunks[1].height as usize;

    // Apply scroll
    let visible_lines: Vec<Line> = lines
        .into_iter()
        .skip(app.detail_scroll)
        .take(visible_height)
        .collect();

    let detail = Paragraph::new(visible_lines).wrap(Wrap { trim: false });
    f.render_widget(detail, chunks[1]);

    let help = "[Esc] Back  [Up/Down] Scroll  [PgUp/PgDn] Page";
    let help_widget = Paragraph::new(help).style(Style::default().fg(DIM));
    f.render_widget(help_widget, chunks[2]);
}

fn field_line(label: &str, value: impl Into<String>) -> Line<'static> {
    Line::from(vec![
        Span::styled(
            pad_right(label, 16),
            Style::default().fg(BLUE).add_modifier(Modifier::BOLD),
        ),
        Span::styled(value.into(), Style::default().fg(TEXT)),
    ])
}

fn build_report_detail_lines(app: &App) -> Vec<Line<'static>> {
    let idx = match app.detail_report_idx {
        Some(i) => i,
        None => return vec![],
    };
    let r = &app.all_reports[idx];
    let mut lines: Vec<Line<'static>> = Vec::new();

    lines.push(Line::from(""));
    lines.push(field_line("Request ID:", &r.id));
    lines.push(field_line("Date/Time:", r.time().to_rfc3339()));
    lines.push(field_line("IP Address:", &r.ip));

    // DNS
    if let Some(names) = app.dns.cache.get(&r.ip) {
        lines.push(field_line("rDNS:", names.join(", ")));
    } else if app.dns.looking_up.as_deref() == Some(&r.ip) {
        lines.push(field_line("rDNS:", "(looking up...)"));
    }

    lines.push(field_line("Country:", &r.country));
    lines.push(field_line("Method:", &r.method));
    lines.push(field_line("URL:", &r.url));
    lines.push(field_line("Status:", format!("{}", r.status)));
    lines.push(field_line("Reason:", &r.reason));
    lines.push(field_line("Server:", &r.server_name));
    lines.push(field_line("Security Mode:", &r.security_mode));
    lines.push(field_line("User Agent:", &r.user_agent));
    lines.push(field_line("Parsed UA:", parsed_ua(&r.user_agent)));

    if !r.bad_behavior_details.is_empty() {
        lines.push(Line::from(""));
        lines.push(Line::from(Span::styled(
            "  Bad Behavior History",
            Style::default().fg(TEAL).add_modifier(Modifier::BOLD),
        )));
        lines.push(Line::from(""));

        for (i, d) in r.bad_behavior_details.iter().enumerate() {
            lines.push(Line::from(Span::styled(
                format!("  Event {}:", i + 1),
                Style::default().fg(BLUE).add_modifier(Modifier::BOLD),
            )));
            lines.push(Line::from(format!("    Date:       {}", d.time().to_rfc3339())));
            lines.push(Line::from(format!("    URL:        {}", d.url)));
            lines.push(Line::from(format!("    Method:     {}", d.method)));
            lines.push(Line::from(format!("    Status:     {}", d.status)));
            lines.push(Line::from(format!("    Ban Time:   {}s", d.ban_time)));
            lines.push(Line::from(format!("    Ban Scope:  {}", d.ban_scope)));
            lines.push(Line::from(format!("    Threshold:  {}", d.threshold)));
            lines.push(Line::from(format!("    Count Time: {}s", d.count_time)));
            lines.push(Line::from(""));
        }
    }

    lines
}

fn build_ban_detail_lines(app: &App) -> Vec<Line<'static>> {
    let idx = match app.detail_ban_idx {
        Some(i) => i,
        None => return vec![],
    };
    let ban = &app.bans[idx];
    let mut lines: Vec<Line<'static>> = Vec::new();

    lines.push(Line::from(""));
    lines.push(field_line("IP Address:", &ban.ip));

    // DNS
    if let Some(names) = app.dns.cache.get(&ban.ip) {
        lines.push(field_line("rDNS:", names.join(", ")));
    } else if app.dns.looking_up.as_deref() == Some(&ban.ip) {
        lines.push(field_line("rDNS:", "(looking up...)"));
    }

    lines.push(field_line("Country:", &ban.country));
    lines.push(field_line("Service:", &ban.service));
    lines.push(field_line("Reason:", &ban.reason));
    lines.push(field_line("Ban Scope:", &ban.ban_scope));
    lines.push(field_line("Banned At:", ban.time().to_rfc3339()));

    if ban.permanent {
        lines.push(field_line("Expires:", "Never (permanent)"));
    } else {
        let secs = ban.ttl.as_secs();
        let h = secs / 3600;
        let m = (secs % 3600) / 60;
        lines.push(field_line("Expires In:", format!("{}h {}m", h, m)));
        lines.push(field_line("Expires At:", ban.expires_at().to_rfc3339()));
    }

    lines.push(field_line(
        "Events:",
        format!("{} requests led to this ban", ban.events.len()),
    ));

    if !ban.events.is_empty() {
        lines.push(Line::from(""));
        lines.push(Line::from(Span::styled(
            "  Events Leading to Ban",
            Style::default().fg(TEAL).add_modifier(Modifier::BOLD),
        )));
        lines.push(Line::from(""));

        for (i, e) in ban.events.iter().enumerate() {
            lines.push(Line::from(Span::styled(
                format!(
                    "  [{}] {}  {} {}  -> {}",
                    i + 1,
                    e.time().format("%H:%M:%S"),
                    e.method,
                    e.url,
                    e.status,
                ),
                Style::default().fg(YELLOW).add_modifier(Modifier::BOLD),
            )));
        }

        if let Some(first) = ban.events.first() {
            lines.push(Line::from(""));
            lines.push(Line::from(Span::styled(
                format!(
                    "  Ban triggered after {} requests in {}s (threshold: {})",
                    ban.events.len(),
                    first.count_time,
                    first.threshold,
                ),
                Style::default().fg(DIM),
            )));
        }
    }

    lines
}

// ── Filter modal ──────────────────────────���─────────────

fn centered_rect(percent_x: u16, height: u16, area: Rect) -> Rect {
    let popup_width = area.width * percent_x / 100;
    let x = (area.width.saturating_sub(popup_width)) / 2;
    let y = (area.height.saturating_sub(height)) / 2;
    Rect::new(
        area.x + x,
        area.y + y,
        popup_width.min(66),
        height.min(area.height),
    )
}

fn draw_filter_modal(f: &mut Frame, app: &App, area: Rect) {
    let height = (FilterField::count() as u16) * 2 + 6;
    let modal_area = centered_rect(50, height, area);

    f.render_widget(Clear, modal_area);

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(BLUE))
        .title(" Filter Reports ")
        .title_style(Style::default().fg(TEAL).add_modifier(Modifier::BOLD));

    let inner = block.inner(modal_area);
    f.render_widget(block, modal_area);

    let mut lines: Vec<Line> = Vec::new();

    for i in 0..FilterField::count() {
        let field = FilterField::from_index(i);
        let prefix = if i == app.filter_focus { " > " } else { "   " };
        let label = field.label();
        let value = &app.filter_inputs[i];

        let display_value = if value.is_empty() && i != app.filter_focus {
            Span::styled(field.placeholder(), Style::default().fg(DIM))
        } else if i == app.filter_focus {
            Span::styled(format!("{}_", value), Style::default().fg(Color::White))
        } else {
            Span::styled(value.to_string(), Style::default().fg(TEXT))
        };

        lines.push(Line::from(vec![
            Span::styled(prefix, Style::default().fg(TEAL)),
            Span::styled(
                pad_right(label, 10),
                Style::default().fg(BLUE).add_modifier(Modifier::BOLD),
            ),
            display_value,
        ]));
        lines.push(Line::from(""));
    }

    lines.push(Line::from(Span::styled(
        "  [Tab] Next field  [Enter] Apply  [Esc] Cancel",
        Style::default().fg(DIM),
    )));

    let content = Paragraph::new(lines);
    f.render_widget(content, inner);
}

// ── Exclude modal ───────────────────────────────────────

fn draw_exclude_modal(f: &mut Frame, app: &App, area: Rect) {
    let ips = app.excludes.list();
    let height = (ips.len() as u16 + 6).min(area.height);
    let modal_area = centered_rect(50, height, area);

    f.render_widget(Clear, modal_area);

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(BLUE))
        .title(" Excluded IPs ")
        .title_style(Style::default().fg(TEAL).add_modifier(Modifier::BOLD));

    let inner = block.inner(modal_area);
    f.render_widget(block, modal_area);

    let mut lines: Vec<Line> = Vec::new();

    if ips.is_empty() {
        lines.push(Line::from(Span::styled(
            "  No excluded IPs",
            Style::default().fg(DIM),
        )));
    } else {
        for (i, ip) in ips.iter().enumerate() {
            let prefix = if i == app.exclude_modal_cursor {
                "> "
            } else {
                "  "
            };
            let ip_color = colour_for_ip(ip);
            let style = if i == app.exclude_modal_cursor {
                Style::default()
                    .fg(ip_color)
                    .bg(HEADER_BG)
                    .add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(ip_color)
            };
            lines.push(Line::from(vec![
                Span::raw(prefix),
                Span::styled(pad_right(ip, 40), style),
            ]));
        }
    }

    lines.push(Line::from(""));
    lines.push(Line::from(Span::styled(
        "  [Del] Remove  [Esc] Close",
        Style::default().fg(DIM),
    )));

    let content = Paragraph::new(lines);
    f.render_widget(content, inner);
}
