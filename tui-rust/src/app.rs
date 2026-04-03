use crossterm::event::{KeyCode, KeyModifiers};
use tokio::sync::mpsc;

use crate::redis_client::RedisClient;
use crate::types::{Ban, BlockReport, DnsCache, ExcludeList, Filter};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ViewState {
    ReportsList,
    ReportDetail,
    BansList,
    BanDetail,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FilterField {
    Ip = 0,
    Country = 1,
    Server = 2,
    DateFrom = 3,
    DateTo = 4,
}

impl FilterField {
    pub fn count() -> usize {
        5
    }

    pub fn from_index(i: usize) -> Self {
        match i {
            0 => Self::Ip,
            1 => Self::Country,
            2 => Self::Server,
            3 => Self::DateFrom,
            4 => Self::DateTo,
            _ => Self::Ip,
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Ip => "IP:",
            Self::Country => "Country:",
            Self::Server => "Server:",
            Self::DateFrom => "From:",
            Self::DateTo => "To:",
        }
    }

    pub fn placeholder(self) -> &'static str {
        match self {
            Self::Ip => "e.g. 192.168.1",
            Self::Country => "e.g. GB or GB,IN,US",
            Self::Server => "e.g. www.example.com",
            Self::DateFrom => "YYYY-MM-DD HH:MM",
            Self::DateTo => "YYYY-MM-DD HH:MM",
        }
    }
}

pub struct App {
    // Data
    pub all_reports: Vec<BlockReport>,
    pub filtered_idx: Vec<usize>,
    pub total_reports: usize,
    pub redis: RedisClient,

    // View state
    pub current_view: ViewState,
    pub detail_report_idx: Option<usize>,
    pub detail_ban_idx: Option<usize>,

    // Reports list
    pub reports_cursor: usize,
    pub reports_scroll: usize,
    pub following: bool,

    // Bans
    pub bans: Vec<Ban>,
    pub filtered_ban_idx: Vec<usize>,
    pub bans_cursor: usize,
    pub bans_scroll: usize,

    // Detail scroll
    pub detail_scroll: usize,
    pub detail_content_height: usize,

    // Filter modal
    pub filter_open: bool,
    pub filter_focus: usize,
    pub filter_inputs: [String; 5],
    pub filter: Filter,

    // Stream control
    pub paused: bool,
    pub pending_reports: Vec<BlockReport>,

    // Dimensions
    pub width: u16,
    pub height: u16,

    // Errors
    pub last_error: Option<String>,

    // Loading
    pub loading: bool,
    pub max_entries: usize,

    // DNS
    pub dns: DnsCache,
    pub dns_tx: mpsc::UnboundedSender<(String, Vec<String>)>,
    pub dns_rx: mpsc::UnboundedReceiver<(String, Vec<String>)>,

    // Excludes
    pub excludes: ExcludeList,
    pub exclude_modal_open: bool,
    pub exclude_modal_cursor: usize,
}

impl App {
    pub fn new(redis: RedisClient, max_entries: usize) -> Self {
        let mut filter = Filter::default();
        filter.load();
        let (dns_tx, dns_rx) = mpsc::unbounded_channel();
        App {
            all_reports: Vec::new(),
            filtered_idx: Vec::new(),
            total_reports: 0,
            redis,
            current_view: ViewState::ReportsList,
            detail_report_idx: None,
            detail_ban_idx: None,
            reports_cursor: 0,
            reports_scroll: 0,
            following: true,
            bans: Vec::new(),
            filtered_ban_idx: Vec::new(),
            bans_cursor: 0,
            bans_scroll: 0,
            detail_scroll: 0,
            detail_content_height: 0,
            filter_open: false,
            filter_focus: 0,
            filter_inputs: Default::default(),
            filter,
            paused: false,
            pending_reports: Vec::new(),
            width: 0,
            height: 0,
            last_error: None,
            loading: true,
            max_entries,
            dns: DnsCache::default(),
            dns_tx,
            dns_rx,
            excludes: ExcludeList::new(),
            exclude_modal_open: false,
            exclude_modal_cursor: 0,
        }
    }

    /// Number of visible data rows (height minus title, header, status, help bars).
    pub fn data_rows(&self) -> usize {
        let rows = self.height as i32 - 4;
        rows.max(1) as usize
    }

    pub async fn load_initial(&mut self) {
        match self.redis.load_initial(self.max_entries).await {
            Ok(reports) => {
                self.total_reports = reports.len();
                self.all_reports = reports;
                self.loading = false;
                self.refilter();
                self.scroll_to_newest();
            }
            Err(e) => {
                self.loading = false;
                self.last_error = Some(format!("Load error: {}", e));
            }
        }
    }

    pub async fn poll_new(&mut self) {
        match self.redis.poll_new().await {
            Ok(reports) => {
                if !reports.is_empty() {
                    if self.paused {
                        let mut pending = reports;
                        pending.append(&mut self.pending_reports);
                        self.pending_reports = pending;
                    } else {
                        let old_filtered_len = self.filtered_idx.len();
                        let mut new_reports = reports;
                        new_reports.append(&mut self.all_reports);
                        self.all_reports = new_reports;
                        self.total_reports = self.all_reports.len();
                        self.refilter();
                        let new_visible = self.filtered_idx.len().saturating_sub(old_filtered_len);
                        if self.following {
                            self.scroll_to_newest();
                        } else if new_visible > 0 {
                            self.reports_cursor += new_visible;
                        }
                    }
                    self.last_error = None;
                }
            }
            Err(e) => {
                self.last_error = Some(format!("Poll error: {}", e));
            }
        }
    }

    pub async fn load_bans(&mut self) {
        match self.redis.load_bans().await {
            Ok(bans) => {
                self.bans = bans;
                self.refilter_bans();
                self.last_error = None;
            }
            Err(e) => {
                self.last_error = Some(format!("Bans error: {}", e));
            }
        }
    }

    fn scroll_to_newest(&mut self) {
        self.reports_cursor = 0;
        self.reports_scroll = 0;
    }

    pub fn refilter(&mut self) {
        if !self.filter.active && self.excludes.count() == 0 {
            self.filtered_idx = (0..self.all_reports.len()).collect();
        } else {
            self.filtered_idx = (0..self.all_reports.len())
                .filter(|&i| {
                    let r = &self.all_reports[i];
                    !self.excludes.contains(&r.ip)
                        && (!self.filter.active || self.filter.matches_report(r))
                })
                .collect();
        }
        self.refilter_bans();
    }

    pub fn refilter_bans(&mut self) {
        if !self.filter.active && self.excludes.count() == 0 {
            self.filtered_ban_idx = (0..self.bans.len()).collect();
        } else {
            self.filtered_ban_idx = (0..self.bans.len())
                .filter(|&i| {
                    let b = &self.bans[i];
                    !self.excludes.contains(&b.ip)
                        && (!self.filter.active || self.filter.matches_ban(b))
                })
                .collect();
        }
    }

    fn sync_scroll(&mut self) {
        let data_rows = self.data_rows();
        if self.reports_cursor < self.reports_scroll {
            self.reports_scroll = self.reports_cursor;
        } else if self.reports_cursor >= self.reports_scroll + data_rows {
            self.reports_scroll = self.reports_cursor - data_rows + 1;
        }
    }

    fn sync_bans_scroll(&mut self) {
        let data_rows = self.data_rows();
        if self.bans_cursor < self.bans_scroll {
            self.bans_scroll = self.bans_cursor;
        } else if self.bans_cursor >= self.bans_scroll + data_rows {
            self.bans_scroll = self.bans_cursor - data_rows + 1;
        }
    }

    fn open_filter(&mut self) {
        self.filter_open = true;
        self.filter_focus = 0;
        self.filter_inputs[0] = self.filter.ip.clone();
        self.filter_inputs[1] = self.filter.country.clone();
        self.filter_inputs[2] = self.filter.server.clone();
        self.filter_inputs[3] = self
            .filter
            .date_from
            .map(|d| d.format("%Y-%m-%d %H:%M").to_string())
            .unwrap_or_default();
        self.filter_inputs[4] = self
            .filter
            .date_to
            .map(|d| d.format("%Y-%m-%d %H:%M").to_string())
            .unwrap_or_default();
    }

    fn apply_filter(&mut self) {
        self.filter.ip = self.filter_inputs[0].trim().to_string();
        self.filter.country = self.filter_inputs[1].trim().to_string();
        self.filter.server = self.filter_inputs[2].trim().to_string();
        self.filter.date_from =
            chrono::NaiveDateTime::parse_from_str(self.filter_inputs[3].trim(), "%Y-%m-%d %H:%M")
                .ok();
        self.filter.date_to =
            chrono::NaiveDateTime::parse_from_str(self.filter_inputs[4].trim(), "%Y-%m-%d %H:%M")
                .ok();
        self.filter.set_active();
        self.filter.save();
        self.refilter();
        self.reports_cursor = 0;
        self.reports_scroll = 0;
        self.bans_cursor = 0;
        self.bans_scroll = 0;
        self.filter_open = false;
    }

    /// Handle a key event. Returns true if the app should quit.
    pub fn handle_key(&mut self, code: KeyCode, modifiers: KeyModifiers) -> bool {
        // Exclude modal
        if self.exclude_modal_open {
            return self.handle_exclude_modal_key(code);
        }

        // Filter modal
        if self.filter_open {
            return self.handle_filter_key(code);
        }

        match self.current_view {
            ViewState::ReportsList => self.handle_reports_list_key(code, modifiers),
            ViewState::ReportDetail => self.handle_detail_key(code),
            ViewState::BansList => self.handle_bans_list_key(code, modifiers),
            ViewState::BanDetail => self.handle_detail_key(code),
        }
    }

    fn handle_reports_list_key(&mut self, code: KeyCode, _modifiers: KeyModifiers) -> bool {
        match code {
            KeyCode::Char('q') => return true,
            KeyCode::Char(' ') => {
                self.paused = !self.paused;
                if !self.paused && !self.pending_reports.is_empty() {
                    let mut pending = std::mem::take(&mut self.pending_reports);
                    pending.append(&mut self.all_reports);
                    self.all_reports = pending;
                    self.total_reports = self.all_reports.len();
                    self.refilter();
                }
            }
            KeyCode::Up | KeyCode::Char('k') => {
                if self.reports_cursor > 0 {
                    self.reports_cursor -= 1;
                    self.following = false;
                    self.sync_scroll();
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if self.reports_cursor + 1 < self.filtered_idx.len() {
                    self.reports_cursor += 1;
                    self.following = false;
                    self.sync_scroll();
                }
            }
            KeyCode::PageUp => {
                let rows = self.data_rows();
                self.reports_cursor = self.reports_cursor.saturating_sub(rows);
                self.following = false;
                self.sync_scroll();
            }
            KeyCode::PageDown => {
                let rows = self.data_rows();
                self.reports_cursor =
                    (self.reports_cursor + rows).min(self.filtered_idx.len().saturating_sub(1));
                self.following = false;
                self.sync_scroll();
            }
            KeyCode::Home => {
                self.following = true;
                self.scroll_to_newest();
            }
            KeyCode::End => {
                self.reports_cursor = self.filtered_idx.len().saturating_sub(1);
                self.following = false;
                self.sync_scroll();
            }
            KeyCode::Enter => {
                if self.reports_cursor < self.filtered_idx.len() {
                    let idx = self.filtered_idx[self.reports_cursor];
                    self.detail_report_idx = Some(idx);
                    self.current_view = ViewState::ReportDetail;
                    self.detail_scroll = 0;

                    // Trigger DNS lookup
                    let ip = self.all_reports[idx].ip.clone();
                    self.trigger_dns_lookup(&ip);
                }
            }
            KeyCode::Char('f') => self.open_filter(),
            KeyCode::Char('c') => {
                self.filter.clear();
                self.filter.delete();
                self.refilter();
                self.reports_cursor = 0;
                self.reports_scroll = 0;
            }
            KeyCode::Char('x') => {
                if self.reports_cursor < self.filtered_idx.len() {
                    let idx = self.filtered_idx[self.reports_cursor];
                    let ip = self.all_reports[idx].ip.clone();
                    self.excludes.add(&ip);
                    self.refilter();
                    if self.reports_cursor >= self.filtered_idx.len() {
                        self.reports_cursor = self.filtered_idx.len().saturating_sub(1);
                    }
                    self.sync_scroll();
                }
            }
            KeyCode::Char('X') => {
                self.exclude_modal_open = true;
                self.exclude_modal_cursor = 0;
            }
            KeyCode::Char('2') => {
                self.current_view = ViewState::BansList;
                self.bans_cursor = 0;
                self.bans_scroll = 0;
                // Bans will be loaded in the main loop
            }
            _ => {}
        }
        false
    }

    fn handle_bans_list_key(&mut self, code: KeyCode, _modifiers: KeyModifiers) -> bool {
        match code {
            KeyCode::Char('q') => return true,
            KeyCode::Up | KeyCode::Char('k') => {
                if self.bans_cursor > 0 {
                    self.bans_cursor -= 1;
                    self.sync_bans_scroll();
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if self.bans_cursor + 1 < self.filtered_ban_idx.len() {
                    self.bans_cursor += 1;
                    self.sync_bans_scroll();
                }
            }
            KeyCode::PageUp => {
                let rows = self.data_rows();
                self.bans_cursor = self.bans_cursor.saturating_sub(rows);
                self.sync_bans_scroll();
            }
            KeyCode::PageDown => {
                let rows = self.data_rows();
                self.bans_cursor =
                    (self.bans_cursor + rows).min(self.filtered_ban_idx.len().saturating_sub(1));
                self.sync_bans_scroll();
            }
            KeyCode::Enter => {
                if self.bans_cursor < self.filtered_ban_idx.len() {
                    let idx = self.filtered_ban_idx[self.bans_cursor];
                    self.detail_ban_idx = Some(idx);
                    self.current_view = ViewState::BanDetail;
                    self.detail_scroll = 0;

                    let ip = self.bans[idx].ip.clone();
                    self.trigger_dns_lookup(&ip);
                }
            }
            KeyCode::Char('f') => self.open_filter(),
            KeyCode::Char('c') => {
                self.filter.clear();
                self.filter.delete();
                self.refilter_bans();
                self.bans_cursor = 0;
                self.bans_scroll = 0;
            }
            KeyCode::Char('x') => {
                if self.bans_cursor < self.filtered_ban_idx.len() {
                    let idx = self.filtered_ban_idx[self.bans_cursor];
                    let ip = self.bans[idx].ip.clone();
                    self.excludes.add(&ip);
                    self.refilter_bans();
                    if self.bans_cursor >= self.filtered_ban_idx.len() {
                        self.bans_cursor = self.filtered_ban_idx.len().saturating_sub(1);
                    }
                    self.sync_bans_scroll();
                }
            }
            KeyCode::Char('X') => {
                self.exclude_modal_open = true;
                self.exclude_modal_cursor = 0;
            }
            KeyCode::Char('r') => {
                // Bans reload will happen in main loop
            }
            KeyCode::Char('1') => {
                self.current_view = ViewState::ReportsList;
                self.sync_scroll();
            }
            _ => {}
        }
        false
    }

    fn handle_detail_key(&mut self, code: KeyCode) -> bool {
        match code {
            KeyCode::Esc | KeyCode::Char('q') => {
                match self.current_view {
                    ViewState::ReportDetail => {
                        self.current_view = ViewState::ReportsList;
                        self.detail_report_idx = None;
                    }
                    ViewState::BanDetail => {
                        self.current_view = ViewState::BansList;
                        self.detail_ban_idx = None;
                    }
                    _ => {}
                }
            }
            KeyCode::Up | KeyCode::Char('k') => {
                self.detail_scroll = self.detail_scroll.saturating_sub(1);
            }
            KeyCode::Down | KeyCode::Char('j') => {
                let max = self.detail_content_height.saturating_sub(
                    self.height.saturating_sub(2) as usize, // minus title and help bar
                );
                if self.detail_scroll < max {
                    self.detail_scroll += 1;
                }
            }
            KeyCode::PageUp => {
                let page = self.height.saturating_sub(4) as usize;
                self.detail_scroll = self.detail_scroll.saturating_sub(page);
            }
            KeyCode::PageDown => {
                let page = self.height.saturating_sub(4) as usize;
                let max = self.detail_content_height.saturating_sub(
                    self.height.saturating_sub(2) as usize,
                );
                self.detail_scroll = (self.detail_scroll + page).min(max);
            }
            _ => {}
        }
        false
    }

    fn handle_filter_key(&mut self, code: KeyCode) -> bool {
        match code {
            KeyCode::Esc => {
                self.filter_open = false;
            }
            KeyCode::Tab | KeyCode::Down => {
                self.filter_focus = (self.filter_focus + 1) % FilterField::count();
            }
            KeyCode::BackTab | KeyCode::Up => {
                self.filter_focus =
                    (self.filter_focus + FilterField::count() - 1) % FilterField::count();
            }
            KeyCode::Enter => {
                self.apply_filter();
            }
            KeyCode::Char(c) => {
                self.filter_inputs[self.filter_focus].push(c);
            }
            KeyCode::Backspace => {
                self.filter_inputs[self.filter_focus].pop();
            }
            _ => {}
        }
        false
    }

    fn handle_exclude_modal_key(&mut self, code: KeyCode) -> bool {
        let count = self.excludes.count();
        match code {
            KeyCode::Esc => {
                self.exclude_modal_open = false;
            }
            KeyCode::Up | KeyCode::Char('k') => {
                if self.exclude_modal_cursor > 0 {
                    self.exclude_modal_cursor -= 1;
                }
            }
            KeyCode::Down | KeyCode::Char('j') => {
                if count > 0 && self.exclude_modal_cursor < count - 1 {
                    self.exclude_modal_cursor += 1;
                }
            }
            KeyCode::Delete | KeyCode::Backspace => {
                let ips = self.excludes.list();
                if !ips.is_empty() && self.exclude_modal_cursor < ips.len() {
                    let ip = ips[self.exclude_modal_cursor].to_string();
                    self.excludes.remove(&ip);
                    let new_count = self.excludes.count();
                    if self.exclude_modal_cursor >= new_count && new_count > 0 {
                        self.exclude_modal_cursor = new_count - 1;
                    }
                    self.refilter();
                    if self.excludes.count() == 0 {
                        self.exclude_modal_open = false;
                    }
                }
            }
            _ => {}
        }
        false
    }

    fn trigger_dns_lookup(&mut self, ip: &str) {
        if self.dns.cache.contains_key(ip) {
            return;
        }
        self.dns.looking_up = Some(ip.to_string());
        self.dns.cache.insert(ip.to_string(), vec!["(looking up...)".to_string()]);

        let ip_owned = ip.to_string();
        let tx = self.dns_tx.clone();
        tokio::spawn(async move {
            // Reverse DNS lookup: reverse the IP octets and query PTR
            let parts: Vec<&str> = ip_owned.split('.').collect();
            let reversed: String = parts.iter().rev().cloned().collect::<Vec<_>>().join(".");
            let query = format!("{}.in-addr.arpa", reversed);

            let names = match tokio::net::lookup_host(format!("{}:0", query)).await {
                Ok(_addrs) => {
                    // Use std reverse lookup as fallback
                    match tokio::task::spawn_blocking({
                        let ip = ip_owned.clone();
                        move || {
                            use std::net::ToSocketAddrs;
                            let addr = format!("{}:0", ip);
                            if let Ok(mut addrs) = addr.to_socket_addrs() {
                                if let Some(addr) = addrs.next() {
                                    if let Ok(host) = dns_lookup::lookup_addr(&addr.ip()) {
                                        return vec![host];
                                    }
                                }
                            }
                            vec!["(no rDNS)".to_string()]
                        }
                    }).await {
                        Ok(names) => names,
                        Err(_) => vec!["(lookup failed)".to_string()],
                    }
                }
                Err(_) => {
                    // Try direct reverse lookup
                    match tokio::task::spawn_blocking({
                        let ip = ip_owned.clone();
                        move || {
                            use std::net::ToSocketAddrs;
                            let addr = format!("{}:0", ip);
                            if let Ok(mut addrs) = addr.to_socket_addrs() {
                                if let Some(addr) = addrs.next() {
                                    if let Ok(host) = dns_lookup::lookup_addr(&addr.ip()) {
                                        return vec![host];
                                    }
                                }
                            }
                            vec!["(no rDNS)".to_string()]
                        }
                    }).await {
                        Ok(names) => names,
                        Err(_) => vec!["(lookup failed)".to_string()],
                    }
                }
            };

            let _ = tx.send((ip_owned, names));
        });
    }

    /// Drain completed DNS results from the channel.
    pub fn drain_dns_results(&mut self) {
        while let Ok((ip, names)) = self.dns_rx.try_recv() {
            self.dns.cache.insert(ip.clone(), names);
            if self.dns.looking_up.as_deref() == Some(&ip) {
                self.dns.looking_up = None;
            }
        }
    }
}
