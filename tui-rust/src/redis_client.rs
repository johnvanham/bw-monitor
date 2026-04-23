use anyhow::Result;
use std::time::Duration;

use crate::types::{Ban, BlockReport};

pub struct RedisClient {
    conn: redis::aio::MultiplexedConnection,
    highwater: i64,
}

impl RedisClient {
    pub async fn new(url: &str) -> Result<Self> {
        let client = redis::Client::open(url)?;
        let conn = client.get_multiplexed_async_connection().await?;
        Ok(RedisClient {
            conn,
            highwater: 0,
        })
    }

    /// Load the most recent reports from Redis.
    /// Reports are returned newest-first for display.
    pub async fn load_initial(&mut self, max_entries: usize) -> Result<Vec<BlockReport>> {
        let total: i64 = redis::cmd("LLEN")
            .arg("requests")
            .query_async(&mut self.conn)
            .await?;

        let start = if max_entries > 0 {
            (total - max_entries as i64).max(0)
        } else {
            0
        };

        let mut all_reports = Vec::new();
        let batch_size: i64 = 200;

        let mut pos = start;
        while pos < total {
            let batch_end = (pos + batch_size - 1).min(total - 1);
            let vals: Vec<String> = redis::cmd("LRANGE")
                .arg("requests")
                .arg(pos)
                .arg(batch_end)
                .query_async(&mut self.conn)
                .await?;

            for val in vals {
                if let Ok(mut report) = serde_json::from_str::<BlockReport>(&val) {
                    report.parse_data();
                    all_reports.push(report);
                }
            }

            pos = batch_end + 1;
        }

        // Reverse so newest is first
        all_reports.reverse();
        self.highwater = total;
        Ok(all_reports)
    }

    /// Fetch any new reports appended since the last poll.
    /// Returns new reports newest-first.
    pub async fn poll_new(&mut self) -> Result<Vec<BlockReport>> {
        let total: i64 = redis::cmd("LLEN")
            .arg("requests")
            .query_async(&mut self.conn)
            .await?;

        if total <= self.highwater {
            return Ok(Vec::new());
        }

        let vals: Vec<String> = redis::cmd("LRANGE")
            .arg("requests")
            .arg(self.highwater)
            .arg(-1_i64)
            .query_async(&mut self.conn)
            .await?;

        let mut reports = Vec::new();
        for val in vals {
            if let Ok(mut report) = serde_json::from_str::<BlockReport>(&val) {
                report.parse_data();
                reports.push(report);
            }
        }

        reports.reverse();
        self.highwater = total;
        Ok(reports)
    }

    /// Fetch all active bans from Redis.
    pub async fn load_bans(&mut self) -> Result<Vec<Ban>> {
        let keys: Vec<String> = redis::cmd("KEYS")
            .arg("bans_*")
            .query_async(&mut self.conn)
            .await?;

        let mut bans = Vec::new();
        for key in keys {
            if let Ok(ban) = self.load_ban(&key).await {
                bans.push(ban);
            }
        }

        // Sort newest first
        bans.sort_by(|a, b| b.date_unix.partial_cmp(&a.date_unix).unwrap_or(std::cmp::Ordering::Equal));
        Ok(bans)
    }

    async fn load_ban(&mut self, key: &str) -> Result<Ban> {
        let val: String = redis::cmd("GET")
            .arg(key)
            .query_async(&mut self.conn)
            .await?;

        let ttl_secs: i64 = redis::cmd("TTL")
            .arg(key)
            .query_async(&mut self.conn)
            .await?;

        let mut ban: Ban = serde_json::from_str(&val)?;
        ban.key = key.to_string();
        ban.ip = Ban::parse_ip_from_key(key);
        ban.ttl = if ttl_secs > 0 {
            Duration::from_secs(ttl_secs as u64)
        } else {
            Duration::ZERO
        };
        ban.parse_data();
        Ok(ban)
    }
}
