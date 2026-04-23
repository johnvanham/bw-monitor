use serde::{Deserialize, Serialize};
use std::collections::{BTreeSet, HashMap};
use std::fs;
use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use std::time::Duration;

// ── Block Report ────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BlockReport {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub ip: String,
    #[serde(default, rename = "date")]
    pub date_unix: f64,
    #[serde(default)]
    pub country: String,
    #[serde(default)]
    pub reason: String,
    #[serde(default)]
    pub method: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub status: i32,
    #[serde(default)]
    pub user_agent: String,
    #[serde(default)]
    pub server_name: String,
    #[serde(default)]
    pub security_mode: String,
    #[serde(default)]
    pub synced: bool,
    #[serde(default)]
    pub data: Option<serde_json::Value>,

    /// Parsed from data field
    #[serde(skip)]
    pub bad_behavior_details: Vec<BadBehaviorDetail>,
}

impl BlockReport {
    pub fn time(&self) -> chrono::DateTime<chrono::Local> {
        let secs = self.date_unix as i64;
        let nsecs = ((self.date_unix - secs as f64) * 1e9) as u32;
        chrono::DateTime::from_timestamp(secs, nsecs)
            .unwrap_or_default()
            .with_timezone(&chrono::Local)
    }

    pub fn parse_data(&mut self) {
        if let Some(ref val) = self.data {
            if let Ok(details) = serde_json::from_value::<Vec<BadBehaviorDetail>>(val.clone()) {
                if !details.is_empty() {
                    self.bad_behavior_details = details;
                }
            }
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BadBehaviorDetail {
    #[serde(default)]
    pub ip: String,
    #[serde(default)]
    pub date: f64,
    #[serde(default)]
    pub country: String,
    #[serde(default)]
    pub ban_time: i64,
    #[serde(default)]
    pub ban_scope: String,
    #[serde(default)]
    pub threshold: i64,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub server_name: String,
    #[serde(default)]
    pub method: String,
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub count_time: i64,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub security_mode: String,
}

impl BadBehaviorDetail {
    pub fn time(&self) -> chrono::DateTime<chrono::Local> {
        let secs = self.date as i64;
        let nsecs = ((self.date - secs as f64) * 1e9) as u32;
        chrono::DateTime::from_timestamp(secs, nsecs)
            .unwrap_or_default()
            .with_timezone(&chrono::Local)
    }
}

// ── Ban ─────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Ban {
    #[serde(skip)]
    pub key: String,
    #[serde(skip)]
    pub ip: String,
    #[serde(default)]
    pub service: String,
    #[serde(default)]
    pub reason: String,
    #[serde(default, rename = "date")]
    pub date_unix: f64,
    #[serde(default)]
    pub country: String,
    #[serde(default)]
    pub ban_scope: String,
    #[serde(default)]
    pub permanent: bool,
    #[serde(default)]
    pub reason_data: Option<serde_json::Value>,
    #[serde(skip)]
    pub ttl: Duration,
    #[serde(skip)]
    pub events: Vec<BanEvent>,
}

impl Ban {
    pub fn time(&self) -> chrono::DateTime<chrono::Local> {
        let secs = self.date_unix as i64;
        let nsecs = ((self.date_unix - secs as f64) * 1e9) as u32;
        chrono::DateTime::from_timestamp(secs, nsecs)
            .unwrap_or_default()
            .with_timezone(&chrono::Local)
    }

    pub fn expires_at(&self) -> chrono::DateTime<chrono::Local> {
        chrono::Local::now() + chrono::Duration::from_std(self.ttl).unwrap_or_default()
    }

    pub fn parse_data(&mut self) {
        if let Some(ref val) = self.reason_data {
            if let Ok(events) = serde_json::from_value::<Vec<BanEvent>>(val.clone()) {
                self.events = events;
            }
        }
    }

    pub fn parse_ip_from_key(key: &str) -> String {
        if let Some(pos) = key.find("_ip_") {
            key[pos + 4..].to_string()
        } else {
            key.to_string()
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BanEvent {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub ip: String,
    #[serde(default)]
    pub date: f64,
    #[serde(default)]
    pub country: String,
    #[serde(default)]
    pub method: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub server_name: String,
    #[serde(default)]
    pub security_mode: String,
    #[serde(default)]
    pub ban_scope: String,
    #[serde(default)]
    pub ban_time: i64,
    #[serde(default)]
    pub count_time: i64,
    #[serde(default)]
    pub threshold: i64,
}

impl BanEvent {
    pub fn time(&self) -> chrono::DateTime<chrono::Local> {
        let secs = self.date as i64;
        let nsecs = ((self.date - secs as f64) * 1e9) as u32;
        chrono::DateTime::from_timestamp(secs, nsecs)
            .unwrap_or_default()
            .with_timezone(&chrono::Local)
    }
}

// ── Filter ──────────────────────────────────────────────

#[derive(Debug, Clone, Default)]
pub struct Filter {
    pub ip: String,
    pub country: String,
    pub server: String,
    pub date_from: Option<chrono::NaiveDateTime>,
    pub date_to: Option<chrono::NaiveDateTime>,
    pub active: bool,
}

impl Filter {
    pub fn matches_report(&self, r: &BlockReport) -> bool {
        if !self.ip.is_empty() && !r.ip.contains(&self.ip) {
            return false;
        }
        if !self.country.is_empty() {
            let codes: Vec<&str> = self.country.split(',').map(|c| c.trim()).collect();
            if !codes.iter().any(|c| c.eq_ignore_ascii_case(&r.country)) {
                return false;
            }
        }
        if !self.server.is_empty()
            && !r.server_name.to_lowercase().contains(&self.server.to_lowercase())
        {
            return false;
        }
        if let Some(from) = self.date_from {
            if r.time().naive_local() < from {
                return false;
            }
        }
        if let Some(to) = self.date_to {
            if r.time().naive_local() > to {
                return false;
            }
        }
        true
    }

    pub fn matches_ban(&self, b: &Ban) -> bool {
        if !self.ip.is_empty() && !b.ip.contains(&self.ip) {
            return false;
        }
        if !self.country.is_empty() {
            let codes: Vec<&str> = self.country.split(',').map(|c| c.trim()).collect();
            if !codes.iter().any(|c| c.eq_ignore_ascii_case(&b.country)) {
                return false;
            }
        }
        if !self.server.is_empty()
            && !b.service.to_lowercase().contains(&self.server.to_lowercase())
        {
            return false;
        }
        if let Some(from) = self.date_from {
            if b.time().naive_local() < from {
                return false;
            }
        }
        if let Some(to) = self.date_to {
            if b.time().naive_local() > to {
                return false;
            }
        }
        true
    }

    pub fn set_active(&mut self) {
        self.active = !self.ip.is_empty()
            || !self.country.is_empty()
            || !self.server.is_empty()
            || self.date_from.is_some()
            || self.date_to.is_some();
    }

    pub fn clear(&mut self) {
        self.ip.clear();
        self.country.clear();
        self.server.clear();
        self.date_from = None;
        self.date_to = None;
        self.active = false;
    }

    pub fn summary(&self) -> String {
        let mut parts = Vec::new();
        if !self.ip.is_empty() {
            parts.push(format!("IP:{}", self.ip));
        }
        if !self.country.is_empty() {
            parts.push(format!("CC:{}", self.country));
        }
        if !self.server.is_empty() {
            parts.push(format!("Server:{}", self.server));
        }
        if let Some(from) = self.date_from {
            parts.push(format!("From:{}", from.format("%Y-%m-%d %H:%M")));
        }
        if let Some(to) = self.date_to {
            parts.push(format!("To:{}", to.format("%Y-%m-%d %H:%M")));
        }
        parts.join(" / ")
    }

    fn filter_path() -> PathBuf {
        dirs_path(".bw-monitor-filter")
    }

    pub fn save(&self) {
        let path = Self::filter_path();
        if let Ok(mut f) = fs::File::create(path) {
            if !self.ip.is_empty() {
                let _ = writeln!(f, "ip={}", self.ip);
            }
            if !self.country.is_empty() {
                let _ = writeln!(f, "country={}", self.country);
            }
            if !self.server.is_empty() {
                let _ = writeln!(f, "server={}", self.server);
            }
            if let Some(from) = self.date_from {
                let _ = writeln!(f, "from={}", from.format("%Y-%m-%d %H:%M"));
            }
            if let Some(to) = self.date_to {
                let _ = writeln!(f, "to={}", to.format("%Y-%m-%d %H:%M"));
            }
        }
    }

    pub fn load(&mut self) {
        let path = Self::filter_path();
        if let Ok(file) = fs::File::open(path) {
            let reader = BufReader::new(file);
            for line in reader.lines().map_while(Result::ok) {
                let line = line.trim().to_string();
                if let Some((key, val)) = line.split_once('=') {
                    match key {
                        "ip" => self.ip = val.to_string(),
                        "country" => self.country = val.to_string(),
                        "server" => self.server = val.to_string(),
                        "from" => {
                            self.date_from =
                                chrono::NaiveDateTime::parse_from_str(val, "%Y-%m-%d %H:%M").ok();
                        }
                        "to" => {
                            self.date_to =
                                chrono::NaiveDateTime::parse_from_str(val, "%Y-%m-%d %H:%M").ok();
                        }
                        _ => {}
                    }
                }
            }
            self.set_active();
        }
    }

    pub fn delete(&self) {
        let _ = fs::remove_file(Self::filter_path());
    }
}

// ── Exclude List ────────────────────────────────────────

#[derive(Debug, Clone)]
pub struct ExcludeList {
    pub ips: BTreeSet<String>,
    path: PathBuf,
}

impl ExcludeList {
    pub fn new() -> Self {
        let path = dirs_path(".bw-monitor-excludes");
        let mut el = ExcludeList {
            ips: BTreeSet::new(),
            path,
        };
        el.load();
        el
    }

    pub fn contains(&self, ip: &str) -> bool {
        self.ips.contains(ip)
    }

    pub fn add(&mut self, ip: &str) {
        self.ips.insert(ip.to_string());
        self.save();
    }

    pub fn remove(&mut self, ip: &str) {
        self.ips.remove(ip);
        self.save();
    }

    pub fn list(&self) -> Vec<&str> {
        self.ips.iter().map(|s| s.as_str()).collect()
    }

    pub fn count(&self) -> usize {
        self.ips.len()
    }

    fn load(&mut self) {
        if let Ok(file) = fs::File::open(&self.path) {
            let reader = BufReader::new(file);
            for line in reader.lines().map_while(Result::ok) {
                let ip = line.trim().to_string();
                if !ip.is_empty() && !ip.starts_with('#') {
                    self.ips.insert(ip);
                }
            }
        }
    }

    fn save(&self) {
        if let Ok(mut f) = fs::File::create(&self.path) {
            for ip in &self.ips {
                let _ = writeln!(f, "{}", ip);
            }
        }
    }
}

// ── DNS Cache ───────────────────────────────────────────

#[derive(Debug, Default)]
pub struct DnsCache {
    pub cache: HashMap<String, Vec<String>>,
    pub looking_up: Option<String>,
}

// ── Helpers ─────────────────────────────────────────────

fn dirs_path(filename: &str) -> PathBuf {
    let home = dirs::home_dir().unwrap_or_else(|| PathBuf::from("."));
    home.join(filename)
}

pub fn colour_for_ip(ip: &str) -> Color {
    let hash = fnv1a_32(ip.as_bytes());
    let idx = (hash as usize) % IP_PALETTE.len();
    IP_PALETTE[idx]
}

fn fnv1a_32(data: &[u8]) -> u32 {
    let mut hash: u32 = 2166136261;
    for &byte in data {
        hash ^= byte as u32;
        hash = hash.wrapping_mul(16777619);
    }
    hash
}

pub const IP_PALETTE: [Color; 20] = [
    Color::Rgb(255, 107, 107), // coral red
    Color::Rgb(78, 201, 176),  // teal
    Color::Rgb(220, 220, 170), // warm yellow
    Color::Rgb(86, 156, 214),  // soft blue
    Color::Rgb(197, 134, 192), // purple/magenta
    Color::Rgb(79, 193, 255),  // bright cyan
    Color::Rgb(206, 145, 120), // orange/rust
    Color::Rgb(181, 206, 168), // soft green
    Color::Rgb(215, 186, 125), // gold
    Color::Rgb(156, 220, 254), // ice blue
    Color::Rgb(244, 135, 113), // salmon
    Color::Rgb(126, 200, 227), // sky blue
    Color::Rgb(195, 232, 141), // lime green
    Color::Rgb(247, 140, 108), // bright orange
    Color::Rgb(255, 121, 198), // pink
    Color::Rgb(139, 233, 253), // aqua
    Color::Rgb(80, 250, 123),  // bright green
    Color::Rgb(255, 184, 108), // peach
    Color::Rgb(189, 147, 249), // violet
    Color::Rgb(241, 250, 140), // pale yellow
];

use ratatui::style::Color;
