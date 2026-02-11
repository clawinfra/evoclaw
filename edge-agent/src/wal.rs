use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum ActionType {
    #[serde(rename = "correction")]
    Correction,
    #[serde(rename = "decision")]
    Decision,
    #[serde(rename = "state_change")]
    StateChange,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Entry {
    pub timestamp: String,
    pub agent_id: String,
    pub action_type: ActionType,
    pub payload: serde_json::Value,
    pub applied: bool,
}

/// Append-only write-ahead log for edge agent state
pub struct WAL {
    path: PathBuf,
    entries: Vec<Entry>,
}

impl WAL {
    pub fn open(dir: &Path) -> Result<Self, Box<dyn std::error::Error>> {
        fs::create_dir_all(dir)?;
        let path = dir.join("wal.json");
        let entries = if path.exists() {
            let data = fs::read_to_string(&path)?;
            serde_json::from_str(&data)?
        } else {
            Vec::new()
        };
        Ok(Self { path, entries })
    }

    pub fn append(
        &mut self,
        agent_id: &str,
        action: ActionType,
        payload: serde_json::Value,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let entry = Entry {
            timestamp: chrono_now(),
            agent_id: agent_id.to_string(),
            action_type: action,
            payload,
            applied: false,
        };
        self.entries.push(entry);
        self.persist()
    }

    pub fn mark_applied(&mut self, index: usize) -> Result<(), Box<dyn std::error::Error>> {
        let entry = self.entries.get_mut(index).ok_or("index out of range")?;
        entry.applied = true;
        self.persist()
    }

    pub fn unapplied(&self) -> Vec<&Entry> {
        self.entries.iter().filter(|e| !e.applied).collect()
    }

    pub fn unapplied_for_agent(&self, agent_id: &str) -> Vec<&Entry> {
        self.entries
            .iter()
            .filter(|e| !e.applied && e.agent_id == agent_id)
            .collect()
    }

    fn persist(&self) -> Result<(), Box<dyn std::error::Error>> {
        let data = serde_json::to_string_pretty(&self.entries)?;
        fs::write(&self.path, data)?;
        Ok(())
    }
}

fn chrono_now() -> String {
    // Simple UTC timestamp without pulling in chrono crate
    let dur = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default();
    format!("{}Z", dur.as_secs())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_wal_append_and_replay() {
        let dir = tempdir().unwrap();
        let mut w = WAL::open(dir.path()).unwrap();

        w.append("a1", ActionType::Correction, serde_json::json!({"k": "v"}))
            .unwrap();
        w.append("a1", ActionType::Decision, serde_json::json!("x"))
            .unwrap();

        assert_eq!(w.unapplied().len(), 2);

        w.mark_applied(0).unwrap();
        assert_eq!(w.unapplied().len(), 1);

        // Reload
        let w2 = WAL::open(dir.path()).unwrap();
        assert_eq!(w2.unapplied().len(), 1);
    }
}
