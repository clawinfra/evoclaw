//! JSONL Tree Session Store — inspired by pi's session branching format.
//!
//! Sessions are stored as append-only JSONL files where each entry has an `id` and
//! optional `parent_id`, forming an implicit tree. This enables in-place branching:
//! multiple conversation branches coexist in a single file, and any branch can be
//! reconstructed by walking the `parent_id` chain from leaf to root.
//!
//! Format spec:
//! ```jsonl
//! {"id":"a1","parent_id":null,"role":"user","content":"hello","ts":1700000000}
//! {"id":"a2","parent_id":"a1","role":"assistant","content":"hi","ts":1700000001}
//! ```

#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use uuid::Uuid;

/// A single entry in the JSONL session tree.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionEntry {
    pub id: String,
    #[serde(default)]
    pub parent_id: Option<String>,
    pub role: String,
    pub content: String,
    pub ts: i64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub metadata: Option<serde_json::Value>,
}

/// Append-only JSONL tree session store.
///
/// All mutations append to the file — nothing is overwritten. Branching is achieved
/// by appending new entries that reference an existing entry as `parent_id`.
pub struct SessionStore {
    path: PathBuf,
}

impl SessionStore {
    /// Open (or create) a session file at the given path.
    pub fn new(path: PathBuf) -> Self {
        Self { path }
    }

    /// Generate a new unique entry ID.
    pub fn new_id() -> String {
        Uuid::new_v4().to_string()
    }

    /// Append a single entry to the session file.
    pub fn append(&self, entry: &SessionEntry) -> std::io::Result<()> {
        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&self.path)?;
        let line = serde_json::to_string(entry)
            .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))?;
        writeln!(file, "{}", line)?;
        Ok(())
    }

    /// Load every entry from the session file.
    pub fn load_all(&self) -> std::io::Result<Vec<SessionEntry>> {
        let file = match File::open(&self.path) {
            Ok(f) => f,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(e) => return Err(e),
        };
        let reader = BufReader::new(file);
        let mut entries = Vec::new();
        for line in reader.lines() {
            let line = line?;
            let trimmed = line.trim();
            if trimmed.is_empty() {
                continue;
            }
            let entry: SessionEntry = serde_json::from_str(trimmed)
                .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))?;
            entries.push(entry);
        }
        Ok(entries)
    }

    /// Reconstruct a single branch by walking `parent_id` from the given leaf back
    /// to root, then reversing to get chronological order.
    pub fn load_branch(&self, leaf_id: &str) -> std::io::Result<Vec<SessionEntry>> {
        let all = self.load_all()?;
        let index: HashMap<&str, &SessionEntry> = all.iter().map(|e| (e.id.as_str(), e)).collect();

        let mut branch = Vec::new();
        let mut current_id: Option<&str> = Some(leaf_id);

        while let Some(cid) = current_id {
            if let Some(entry) = index.get(cid) {
                branch.push((*entry).clone());
                current_id = entry.parent_id.as_deref();
            } else {
                break;
            }
        }

        branch.reverse();
        Ok(branch)
    }

    /// Create a branch point from an existing entry. Returns a new ID that can be
    /// used as `parent_id` for subsequent entries on the new branch.
    ///
    /// This doesn't write anything — it just generates the branch ID. The caller
    /// appends entries with `parent_id = branch_from(some_id)`.
    pub fn branch_from(&self, from_id: &str) -> std::io::Result<String> {
        // Verify the source entry exists
        let all = self.load_all()?;
        let exists = all.iter().any(|e| e.id == from_id);
        if !exists {
            return Err(std::io::Error::new(
                std::io::ErrorKind::NotFound,
                format!("entry '{}' not found in session", from_id),
            ));
        }
        // The branch point is just from_id itself — new entries attach to it as parent
        Ok(from_id.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    fn ts() -> i64 {
        1700000000
    }

    #[test]
    fn test_append_and_load() {
        let tmp = NamedTempFile::new().unwrap();
        let store = SessionStore::new(tmp.path().to_path_buf());

        let e1 = SessionEntry {
            id: "a1".into(),
            parent_id: None,
            role: "user".into(),
            content: "hello".into(),
            ts: ts(),
            metadata: None,
        };
        let e2 = SessionEntry {
            id: "a2".into(),
            parent_id: Some("a1".into()),
            role: "assistant".into(),
            content: "hi there".into(),
            ts: ts() + 1,
            metadata: None,
        };

        store.append(&e1).unwrap();
        store.append(&e2).unwrap();

        let all = store.load_all().unwrap();
        assert_eq!(all.len(), 2);
        assert_eq!(all[0].id, "a1");
        assert_eq!(all[1].parent_id, Some("a1".into()));
    }

    #[test]
    fn test_load_branch() {
        let tmp = NamedTempFile::new().unwrap();
        let store = SessionStore::new(tmp.path().to_path_buf());

        // Build a tree:  root -> a -> b (branch 1)
        //                      root -> a -> c (branch 2)
        for entry in &[
            SessionEntry {
                id: "root".into(),
                parent_id: None,
                role: "user".into(),
                content: "start".into(),
                ts: ts(),
                metadata: None,
            },
            SessionEntry {
                id: "a".into(),
                parent_id: Some("root".into()),
                role: "assistant".into(),
                content: "reply".into(),
                ts: ts() + 1,
                metadata: None,
            },
            SessionEntry {
                id: "b".into(),
                parent_id: Some("a".into()),
                role: "user".into(),
                content: "branch1".into(),
                ts: ts() + 2,
                metadata: None,
            },
            SessionEntry {
                id: "c".into(),
                parent_id: Some("a".into()),
                role: "user".into(),
                content: "branch2".into(),
                ts: ts() + 3,
                metadata: None,
            },
        ] {
            store.append(entry).unwrap();
        }

        let branch1 = store.load_branch("b").unwrap();
        assert_eq!(branch1.len(), 3);
        assert_eq!(branch1[0].id, "root");
        assert_eq!(branch1[2].content, "branch1");

        let branch2 = store.load_branch("c").unwrap();
        assert_eq!(branch2.len(), 3);
        assert_eq!(branch2[2].content, "branch2");
    }

    #[test]
    fn test_branch_from() {
        let tmp = NamedTempFile::new().unwrap();
        let store = SessionStore::new(tmp.path().to_path_buf());

        store
            .append(&SessionEntry {
                id: "x".into(),
                parent_id: None,
                role: "user".into(),
                content: "msg".into(),
                ts: ts(),
                metadata: None,
            })
            .unwrap();

        let branch_id = store.branch_from("x").unwrap();
        assert_eq!(branch_id, "x");

        // Non-existent entry should error
        assert!(store.branch_from("nonexistent").is_err());
    }

    #[test]
    fn test_empty_file() {
        let tmp = NamedTempFile::new().unwrap();
        let store = SessionStore::new(tmp.path().to_path_buf());
        let all = store.load_all().unwrap();
        assert!(all.is_empty());
    }
}
