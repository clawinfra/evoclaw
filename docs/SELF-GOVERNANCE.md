# Self-Governance Protocols

Four protocols that prevent agent failure modes: losing context, false completion claims, persona drift, and wasteful spending.

## Overview

Self-governance enables autonomous agents to maintain reliability, consistency, and cost-effectiveness without human supervision. These protocols are implemented in both the EvoClaw orchestrator (Go) and edge agents (Rust).

**The four protocols:**
1. **WAL (Write-Ahead Log)** — Prevent context loss through session compaction
2. **VBR (Verify Before Reporting)** — Prevent false completion claims
3. **ADL (Anti-Divergence Limit)** — Prevent persona drift
4. **VFM (Value-For-Money)** — Prevent wasteful spending on low-value tasks

## 1. WAL (Write-Ahead Log)

**Rule: Write before you respond.** If something is worth remembering, WAL it first.

### Purpose

Conversation compaction can lose critical context (user corrections, key decisions, state changes). WAL ensures important information survives compaction by logging it before responding.

### Triggers

| Trigger | Action Type | Example |
|---------|------------|---------|
| User corrects you | `correction` | "No, use Podman not Docker" |
| Key decision | `decision` | "Using CogVideoX-2B for text-to-video" |
| Important analysis | `analysis` | "WAL patterns should be core infra not skills" |
| State change | `state_change` | "GPU server SSH key auth configured" |

### Implementation

**Go (Orchestrator):**
```go
// internal/memory/wal.go
func (w *WAL) Append(agentID string, actionType string, content string) error
func (w *WAL) BufferAdd(agentID string, actionType string, content string) error
func (w *WAL) FlushBuffer(agentID string) error
func (w *WAL) Replay(agentID string) ([]WALEntry, error)
func (w *WAL) MarkApplied(agentID string, entryID string) error
func (w *WAL) Status(agentID string) (*WALStatus, error)
func (w *WAL) Prune(agentID string, keep int) error
```

**Rust (Edge Agent):**
```rust
// edge-agent/src/wal.rs
pub fn append(agent_id: &str, action_type: &str, content: &str) -> Result<()>
pub fn buffer_add(agent_id: &str, action_type: &str, content: &str) -> Result<()>
pub fn flush_buffer(agent_id: &str) -> Result<()>
pub fn replay(agent_id: &str) -> Result<Vec<WALEntry>>
pub fn mark_applied(agent_id: &str, entry_id: &str) -> Result<()>
```

### Integration Points

- **Session start** → Replay unapplied entries to recover lost context
- **User correction** → Append BEFORE responding
- **Pre-compaction flush** → Flush buffer, then write daily memory
- **During conversation** → Buffer-add for less critical items

### Storage Format

WAL entries stored in `~/.evoclaw/wal/{agent_id}.jsonl`:

```json
{"id":"wal_20260213_143022_abc123","timestamp":"2026-02-13T14:30:22Z","agent_id":"alex-hub","action_type":"correction","content":"Use Podman not Docker","applied":false}
{"id":"wal_20260213_143145_def456","timestamp":"2026-02-13T14:31:45Z","agent_id":"alex-hub","action_type":"decision","content":"Using AnimateDiff for video generation","applied":true}
```

## 2. VBR (Verify Before Reporting)

**Rule: Don't say "done" until verified.** Run a check before claiming completion.

### Purpose

Prevent "done but not done" situations where agents claim task completion without verifying the output.

### Verification Types

```bash
# File exists
vbr.check(task_id, "file_exists", "/path/to/output.py")

# File recently modified
vbr.check(task_id, "file_changed", "/path/to/file.go")

# Command succeeds
vbr.check(task_id, "command", "cd /tmp/repo && go test ./...")

# Git pushed
vbr.check(task_id, "git_pushed", "/tmp/repo")
```

### Implementation

**Go (Orchestrator):**
```go
// internal/verification/vbr.go
func (v *VBR) Check(taskID string, checkType string, target string) (bool, error)
func (v *VBR) Log(agentID string, taskID string, passed bool, message string) error
func (v *VBR) Stats(agentID string) (*VBRStats, error)
```

**Rust (Edge Agent):**
```rust
// edge-agent/src/vbr.rs
pub fn check(task_id: &str, check_type: &str, target: &str) -> Result<bool>
pub fn log(agent_id: &str, task_id: &str, passed: bool, message: &str) -> Result<()>
pub fn stats(agent_id: &str) -> Result<VBRStats>
```

### When to VBR

- After code changes → `check("command", "go test ./...")`
- After file creation → `check("file_exists", "/path")`
- After git push → `check("git_pushed", "/repo")`
- After sub-agent task → verify claimed output exists

### Storage Format

VBR logs stored in `~/.evoclaw/vbr/{agent_id}.jsonl`:

```json
{"timestamp":"2026-02-13T14:35:12Z","agent_id":"alex-hub","task_id":"task_123","passed":true,"message":"All tests pass","check_type":"command"}
```

## 3. ADL (Anti-Divergence Limit)

**Rule: Stay true to your persona.** Track behavioral drift from SOUL.md.

### Purpose

Autonomous agents can drift from their defined personality over time. ADL detects and quantifies this drift.

### Anti-Patterns (Negative Signals)

- **Sycophancy** — "Great question!", "I'd be happy to help!"
- **Passivity** — "Would you like me to", "Shall I", "Let me know if"
- **Hedging** — "I think maybe", "It might be possible"
- **Verbosity** — Response length exceeding expected bounds

### Persona Signals (Positive Signals)

- **Direct** — "Done", "Fixed", "Ship", "Built"
- **Opinionated** — "I'd argue", "Better to", "The right call"
- **Action-oriented** — "Spawning", "On it", "Kicking off"

### Implementation

**Go (Orchestrator):**
```go
// internal/adl/detector.go
func (d *ADL) Analyze(text string) *Analysis
func (d *ADL) Log(agentID string, signalType string, excerpt string) error
func (d *ADL) Score(agentID string) (float64, error)
func (d *ADL) Check(agentID string, threshold float64) (bool, error)
func (d *ADL) Reset(agentID string) error
```

**Rust (Edge Agent):**
```rust
// edge-agent/src/adl.rs
pub fn analyze(text: &str) -> Analysis
pub fn log(agent_id: &str, signal_type: &str, excerpt: &str) -> Result<()>
pub fn score(agent_id: &str) -> Result<f64>
pub fn check(agent_id: &str, threshold: f64) -> Result<bool>
pub fn reset(agent_id: &str) -> Result<()>
```

### Divergence Scoring

**Score calculation:**
```
score = (anti_patterns_count - persona_signals_count) / total_responses
```

- **0.0** = Fully aligned with persona
- **1.0** = Fully drifted from persona
- **Typical threshold**: 0.7 (trigger recalibration)

### Storage Format

ADL logs stored in `~/.evoclaw/adl/{agent_id}.jsonl`:

```json
{"timestamp":"2026-02-13T14:40:00Z","agent_id":"alex-hub","signal_type":"anti_sycophancy","excerpt":"Great question!","positive":false}
{"timestamp":"2026-02-13T14:42:15Z","agent_id":"alex-hub","signal_type":"persona_direct","excerpt":"Done","positive":true}
```

## 4. VFM (Value-For-Money)

**Rule: Track cost vs value.** Don't burn premium tokens on budget tasks.

### Purpose

Prevent wasteful spending by tracking task costs and outcomes, identifying when cheaper models would have sufficed.

### Task → Tier Guidelines

| Task Type | Recommended Tier | Example Models |
|-----------|-----------------|----------------|
| Monitoring, formatting, summarization | Budget | GLM-4.7, DeepSeek, Haiku |
| Code generation, debugging, creative | Standard | Sonnet, Gemini Pro |
| Architecture, complex analysis | Premium | Opus, Sonnet+thinking |

### Implementation

**Go (Orchestrator):**
```go
// internal/vfm/tracker.go
func (v *VFM) Log(agentID, task, model string, tokens int, cost, value float64) error
func (v *VFM) Score(agentID string) (*VFMScore, error)
func (v *VFM) Report(agentID string) (*VFMReport, error)
func (v *VFM) Suggest(agentID string) ([]Suggestion, error)
```

**Rust (Edge Agent):**
```rust
// edge-agent/src/vfm.rs
pub fn log(agent_id: &str, task: &str, model: &str, tokens: u32, cost: f64, value: f64) -> Result<()>
pub fn score(agent_id: &str) -> Result<VFMScore>
pub fn report(agent_id: &str) -> Result<VFMReport>
pub fn suggest(agent_id: &str) -> Result<Vec<Suggestion>>
```

### Value Scoring

**Value is subjective but can be approximated:**
- 1.0 = Perfect completion, no retries
- 0.8 = Completed with minor issues
- 0.5 = Partial completion, needed rework
- 0.0 = Failed, wasted tokens

**VFM calculation:**
```
vfm_score = value / normalized_cost
```

High VFM = good value for money  
Low VFM = overpaid for result

### When to Check VFM

- After spawning sub-agents → log cost and outcome
- During heartbeat → run suggest() for optimization tips
- Weekly review → run report() for cost breakdown

### Storage Format

VFM logs stored in `~/.evoclaw/vfm/{agent_id}.jsonl`:

```json
{"timestamp":"2026-02-13T14:50:00Z","agent_id":"alex-hub","task":"monitoring","model":"glm-4.7","tokens":37000,"cost":0.03,"value":0.8}
{"timestamp":"2026-02-13T15:00:00Z","agent_id":"alex-hub","task":"architecture","model":"claude-opus-4","tokens":120000,"cost":2.40,"value":1.0}
```

## Integration with EvoClaw

### Orchestrator Integration

Self-governance protocols are core infrastructure in the orchestrator:

```go
// internal/orchestrator/orchestrator.go
type Orchestrator struct {
    wal    *memory.WAL
    vbr    *verification.VBR
    adl    *adl.Detector
    vfm    *vfm.Tracker
    // ...
}

func (o *Orchestrator) HandleMessage(msg *Message) error {
    // WAL: Log important information before processing
    if msg.IsCorrection() {
        o.wal.Append(msg.AgentID, "correction", msg.Content)
    }
    
    // ADL: Analyze response for persona drift
    analysis := o.adl.Analyze(response)
    if analysis.Score > 0.7 {
        o.logger.Warn("High persona drift detected", "score", analysis.Score)
    }
    
    // VBR: Verify task completion
    if msg.ClaimsDone() {
        verified := o.vbr.Check(msg.TaskID, "command", msg.VerificationCmd)
        if !verified {
            return errors.New("verification failed")
        }
    }
    
    // VFM: Track costs
    o.vfm.Log(msg.AgentID, msg.Task, msg.Model, msg.Tokens, msg.Cost, msg.Value)
    
    return nil
}
```

### Edge Agent Integration

Edge agents implement the same protocols in Rust:

```rust
// edge-agent/src/agent.rs
pub struct EdgeAgent {
    wal: WAL,
    vbr: VBR,
    adl: ADL,
    vfm: VFM,
    // ...
}

impl EdgeAgent {
    pub async fn handle_message(&mut self, msg: Message) -> Result<()> {
        // WAL: Log corrections
        if msg.is_correction() {
            self.wal.append(&msg.agent_id, "correction", &msg.content)?;
        }
        
        // VBR: Verify completion
        if msg.claims_done() {
            let verified = self.vbr.check(&msg.task_id, "command", &msg.verification_cmd)?;
            if !verified {
                return Err(anyhow!("verification failed"));
            }
        }
        
        Ok(())
    }
}
```

### Cloud Sync

All self-governance data syncs to Turso (cloud SQLite) for multi-agent coordination:

**Schema:**
```sql
CREATE TABLE wal_entries (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    action_type TEXT NOT NULL,
    content TEXT NOT NULL,
    applied INTEGER DEFAULT 0
);

CREATE TABLE vbr_logs (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    task_id TEXT NOT NULL,
    passed INTEGER NOT NULL,
    message TEXT
);

CREATE TABLE adl_signals (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    signal_type TEXT NOT NULL,
    excerpt TEXT,
    positive INTEGER NOT NULL
);

CREATE TABLE vfm_logs (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    task TEXT NOT NULL,
    model TEXT NOT NULL,
    tokens INTEGER NOT NULL,
    cost REAL NOT NULL,
    value REAL NOT NULL
);
```

## Usage Examples

### Example 1: WAL on User Correction

**Scenario:** User corrects agent about preferred tool

```go
// User says: "Use Podman not Docker"
msg := &Message{
    AgentID: "alex-hub",
    Content: "Use Podman not Docker",
    Type: MessageTypeCorrection,
}

// WAL BEFORE responding
err := orchestrator.wal.Append("alex-hub", "correction", msg.Content)
if err != nil {
    return err
}

// Now respond acknowledging the correction
response := "Got it, using Podman instead of Docker from now on."
```

### Example 2: VBR on Task Completion

**Scenario:** Sub-agent claims to have built and tested a feature

```go
// Sub-agent claims: "Feature complete, all tests passing"
taskID := "feature_auth_jwt"
repoPath := "/tmp/evoclaw-repo"

// VBR: Verify tests actually pass
passed, err := orchestrator.vbr.Check(taskID, "command", 
    fmt.Sprintf("cd %s && go test ./...", repoPath))
if err != nil {
    return fmt.Errorf("verification command failed: %w", err)
}

if !passed {
    // Log the failure
    orchestrator.vbr.Log("alex-hub", taskID, false, "Tests failed despite claim")
    return errors.New("task verification failed: tests do not pass")
}

// Log success
orchestrator.vbr.Log("alex-hub", taskID, true, "Tests verified passing")
```

### Example 3: ADL Persona Check

**Scenario:** Weekly persona drift check during maintenance

```go
// Calculate divergence score
score, err := orchestrator.adl.Score("alex-hub")
if err != nil {
    return err
}

threshold := 0.7
if score > threshold {
    // High drift detected — alert user
    orchestrator.logger.Warn("Persona drift detected",
        "agent", "alex-hub",
        "score", score,
        "threshold", threshold)
    
    // Optionally reset and recalibrate
    orchestrator.adl.Reset("alex-hub")
}
```

### Example 4: VFM Cost Optimization

**Scenario:** Weekly cost review suggests cheaper models

```go
// Generate VFM report
report, err := orchestrator.vfm.Report("alex-hub")
if err != nil {
    return err
}

// Get optimization suggestions
suggestions, err := orchestrator.vfm.Suggest("alex-hub")
if err != nil {
    return err
}

// Example suggestion:
// "Task 'monitoring' used claude-opus-4 ($2.40) but could use glm-4.7 ($0.03) for 98.75% savings"

for _, s := range suggestions {
    orchestrator.logger.Info("VFM suggestion",
        "task", s.Task,
        "current_model", s.CurrentModel,
        "suggested_model", s.SuggestedModel,
        "savings", s.Savings)
}
```

## Maintenance

### Daily
- WAL replay at session start (automatic)
- VBR verification on task completion (automatic)
- ADL analysis on each response (automatic)

### Weekly
- Review VFM report for cost optimization opportunities
- Check ADL score for persona drift (threshold 0.7)
- Prune old WAL entries (keep last 50-100)

### Monthly
- Deep VFM analysis: identify model selection patterns
- ADL recalibration if drift detected
- Archive old self-governance logs

## See Also

- [TIERED-MEMORY.md](TIERED-MEMORY.md) — Memory architecture complementing WAL
- [INTELLIGENT-ROUTER.md](INTELLIGENT-ROUTER.md) — Model selection supporting VFM
- [EVOLUTION.md](EVOLUTION.md) — How self-governance enables evolution
- [ROADMAP.md](ROADMAP.md) — Future self-governance enhancements
