---
name: model-health-registry
description: Circuit breaker pattern for intelligent model fallback. Track model health, automatically route around failures, and persist state across restarts.
version: 1.0.0
---

# Model Health Registry

## Quick Setup

**New to this feature?** Start here:

1. **Configure in `evoclaw.json`:**
   ```json
   {
     "models": {
       "health": {
         "persistPath": "~/.evoclaw/model_health.json",
         "failureThreshold": 3,
         "cooldownMinutes": 5
       }
     }
   }
   ```

2. **Restart EvoClaw** — health registry initializes automatically

3. **Monitor degraded models:**
   ```bash
   curl http://localhost:8420/api/health/models
   ```

**No code changes required** — health registry integrates automatically with the intelligent router.

---

## Overview

The Model Health Registry implements a **circuit breaker pattern** for LLM model selection. It tracks failures, automatically degrades unhealthy models, and routes requests to healthy fallbacks. This prevents cascading failures when models experience outages, rate limits, or quota exhaustion.

**Key benefits:**
- ✅ **Automatic failure detection** — Models degraded after configurable failure threshold
- ✅ **Intelligent fallback** — Routes to healthy models from your fallback chain
- ✅ **Persistent state** — Survives restarts via disk persistence
- ✅ **Auto-recovery** — Re-tests degraded models after cooldown period
- ✅ **Observability** — Track model health, error types, and success rates

## Model States

Each model tracked by the health registry can be in one of three states:

| State | Description | Routing Behavior |
|-------|-------------|------------------|
| **healthy** | Model is operating normally | Preferred for routing |
| **degraded** | Model has exceeded failure threshold | Skipped unless all fallbacks are also degraded |
| **unknown** | New model with no history | Treated as healthy |

**State transitions:**
```
unknown ──[success]──▶ healthy ──[N consecutive failures]──▶ degraded
                                        │
                                        └──[cooldown expires]──▶ healthy
```

## Configuration

### Configuration Options

Add to `evoclaw.json` under `models.health`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `persistPath` | string | `~/.evoclaw/model_health.json` | File path for health state persistence |
| `failureThreshold` | int | `3` | Consecutive failures before degrading a model |
| `cooldownMinutes` | int | `5` | Minutes before retrying a degraded model |

### Example Configuration

```json
{
  "models": {
    "providers": {
      "anthropic": {
        "baseUrl": "https://api.anthropic.com",
        "apiKey": "sk-ant-...",
        "models": [
          {
            "id": "claude-sonnet-4-20250514",
            "name": "Claude Sonnet 4",
            "contextWindow": 200000
          }
        ]
      },
      "openai": {
        "baseUrl": "https://api.openai.com/v1",
        "apiKey": "sk-...",
        "models": [
          {
            "id": "gpt-4o-mini",
            "name": "GPT-4o Mini",
            "contextWindow": 128000
          }
        ]
      }
    },
    "routing": {
      "simple": "gpt-4o-mini",
      "medium": "gpt-4o-mini",
      "complex": "claude-sonnet-4-20250514",
      "critical": "claude-opus-4-20250514"
    },
    "health": {
      "persistPath": "~/.evoclaw/model_health.json",
      "failureThreshold": 3,
      "cooldownMinutes": 5
    }
  }
}
```

### Behavioral Settings

**Failure Threshold (default: 3)**
- **Lower values (1-2):** More aggressive degradation — quick to route around problems
- **Higher values (5-10):** More tolerant — allows transient failures before degrading

**Cooldown Period (default: 5 minutes)**
- **Shorter (1-2 min):** Faster recovery — retries degraded models more frequently
- **Longer (10-30 min):** More stable — gives models more time to recover from outages

**Recommendation:** Start with defaults, adjust based on:
- Your model provider's outage patterns
- Cost sensitivity (more retries = more API costs)
- Availability requirements

## API Reference

### Health Registry Methods

The health registry is accessed via the orchestrator:

```go
// Get the health registry
healthReg := orchestrator.GetHealthRegistry()

// Record a successful API call
healthReg.RecordSuccess("claude-sonnet-4-20250514")

// Record a failed API call with error classification
errType := router.ClassifyError(err) // "rate_limited", "timeout", etc.
healthReg.RecordFailure("claude-sonnet-4-20250514", errType)

// Check if a model is healthy
healthy := healthReg.IsHealthy("claude-sonnet-4-20250514")

// Get best healthy model from preferred + fallbacks
selected := healthReg.GetHealthyModel(
    "claude-sonnet-4-20250514",
    []string{"gpt-4o-mini", "ollama-qwen-32b"},
)
```

### Health Status Methods

```go
// Get status for all models
status := healthReg.GetStatus()
// Returns: map[string]*ModelHealth

// Get status for a specific model
modelStatus, exists := healthReg.GetModelStatus("claude-sonnet-4-20250514")
if exists {
    fmt.Printf("Success rate: %.2f%%\n", modelStatus.SuccessRate * 100)
}

// Get list of currently degraded models
degraded := healthReg.DegradedModels()
// Returns: []string

// Manually reset a model to healthy state
healthReg.ResetModel("claude-sonnet-4-20250514")
```

### Persistence Methods

```go
// Persist health state to disk
err := healthReg.Persist()
// Called automatically every 5 minutes, but can be manual if needed

// Load is called automatically on initialization
// No manual load required
```

## Model Health Structure

Each model's health is tracked with the following metrics:

```go
type ModelHealth struct {
    State               ModelState     // "healthy", "degraded", "unknown"
    ConsecutiveFailures int            // Current failure streak
    LastFailure         *time.Time     // Timestamp of last failure
    LastSuccess         *time.Time     // Timestamp of last success
    DegradedAt          *time.Time     // When model entered degraded state
    TotalRequests       int64          // Total API calls
    TotalFailures       int64          // Total failed calls
    SuccessRate         float64        // (TotalRequests - TotalFailures) / TotalRequests
    ErrorTypes          map[string]int // Count by error type
    LastErrorType       string         // Most recent error classification
}
```

### Example Health Status

```json
{
  "claude-sonnet-4-20250514": {
    "state": "healthy",
    "consecutive_failures": 0,
    "last_success": "2026-02-15T11:25:30Z",
    "last_failure": null,
    "degraded_at": null,
    "total_requests": 152,
    "total_failures": 3,
    "success_rate": 0.980,
    "error_types": {
      "rate_limited": 2,
      "timeout": 1
    },
    "last_error_type": ""
  },
  "gpt-4o-mini": {
    "state": "degraded",
    "consecutive_failures": 5,
    "last_success": "2026-02-15T10:15:22Z",
    "last_failure": "2026-02-15T11:20:45Z",
    "degraded_at": "2026-02-15T11:18:30Z",
    "total_requests": 89,
    "total_failures": 8,
    "success_rate": 0.910,
    "error_types": {
      "quota_exhausted": 8
    },
    "last_error_type": "quota_exhausted"
  }
}
```

## Error Type Classification

Errors are automatically classified into types for tracking and analysis:

| Error Type | Description | Common Causes |
|------------|-------------|---------------|
| `quota_exhausted` | API quota or token limit reached | Monthly spend limit, token cap |
| `rate_limited` | Rate limit exceeded (HTTP 429) | Too many requests/minute |
| `timeout` | Request timeout | Slow model, network issues |
| `server_error` | HTTP 5xx errors from provider | Provider outage |
| `auth_error` | Authentication failure (HTTP 401/403) | Invalid API key, permissions |
| `model_not_found` | Model doesn't exist (HTTP 404) | Deprecated model, typo in ID |
| `context_too_long` | Prompt exceeds context window | Long conversation, large files |
| `unknown` | Unclassified error | Edge cases, new error patterns |

### Classification Logic

```go
// Automatic error classification
errType := router.ClassifyError(err)

// Example outputs:
// "rate limit exceeded" → "rate_limited"
// "quota exceeded for this billing period" → "quota_exhausted"
// "context length exceeded" → "context_too_long"
// "deadline exceeded" → "timeout"
// "500 Internal Server Error" → "server_error"
// "401 Unauthorized" → "auth_error"
```

**Pattern matching:** The classifier searches for known error patterns in error messages. Unknown errors are classified as `unknown`.

## Integration with Orchestrator

The health registry integrates with the orchestrator's model selection:

### Automatic Model Selection

When selecting a model for a request:

```go
// 1. Intelligent router selects preferred model based on task complexity
preferred := router.SelectModel(prompt) // e.g., "claude-sonnet-4-20250514"

// 2. Health registry provides fallback list from config
fallbacks := []string{
    cfg.Models.Routing.Simple,    // e.g., "gpt-4o-mini"
    cfg.Models.Routing.Complex,   // e.g., "claude-opus-4-20250514"
}

// 3. Health registry selects best healthy model
selected := healthReg.GetHealthyModel(preferred, fallbacks)

// 4. If preferred is degraded, first healthy fallback is used
// If all are degraded, model with highest success rate is used
```

### Fallback Strategy

**Scenario 1: Preferred is healthy**
```json
Preferred: "claude-sonnet-4" (healthy) → Used
Fallbacks: ["gpt-4o-mini" (degraded), "claude-opus" (healthy)]
→ Result: "claude-sonnet-4"
```

**Scenario 2: Preferred is degraded, fallbacks available**
```json
Preferred: "claude-sonnet-4" (degraded)
Fallbacks: ["gpt-4o-mini" (healthy), "claude-opus" (healthy)]
→ Result: "gpt-4o-mini" (first healthy fallback)
```

**Scenario 3: All models degraded**
```json
Preferred: "claude-sonnet-4" (degraded, 85% success rate)
Fallbacks: ["gpt-4o-mini" (degraded, 70% success rate), "claude-opus" (degraded, 90% success rate)]
→ Result: "claude-opus" (highest success rate)
```

### Recording Results

After each API call:

```go
// Success case
response, err := provider.Call(model, prompt)
if err == nil {
    healthReg.RecordSuccess(model)
}

// Failure case
if err != nil {
    errType := router.ClassifyError(err)
    healthReg.RecordFailure(model, errType)
}
```

**Automatic recording:** The orchestrator automatically records results for all model calls. No manual tracking needed.

## Example Usage

### Basic Health Monitoring

```bash
# Check health status of all models
curl http://localhost:8420/api/health/models

# Response example:
{
  "claude-sonnet-4-20250514": {
    "state": "healthy",
    "success_rate": 0.98
  },
  "gpt-4o-mini": {
    "state": "degraded",
    "success_rate": 0.91,
    "last_error_type": "quota_exhausted"
  }
}
```

### Manual Model Reset

If a model has recovered but is still in cooldown:

```go
// Force reset to healthy state
healthReg.ResetModel("gpt-4o-mini")
// Next request will immediately use this model
```

### Observing Degraded Models

```go
// Get list of degraded models
degraded := healthReg.DegradedModels()
for _, modelID := range degraded {
    status, _ := healthReg.GetModelStatus(modelID)
    fmt.Printf("Model %s degraded at %v, error: %s\n",
        modelID, status.DegradedAt, status.LastErrorType)
}
```

### Custom Fallback Logic

```go
// Build custom fallback chain for specific use case
preferred := "claude-sonnet-4"
fallbacks := []string{
    "gpt-4o-mini",           // Mid-tier backup
    "ollama-qwen-32b",       // Local backup
    "claude-opus-4",         // Premium last resort
}

selected := healthReg.GetHealthyModel(preferred, fallbacks)
```

## Monitoring & Observability

### Health Status API

EvoClaw exposes health metrics via HTTP API:

```bash
# Get all model health status
curl http://localhost:8420/api/health/models

# Get specific model status
curl http://localhost:8420/api/health/models/claude-sonnet-4-20250514

# Get degraded models only
curl http://localhost:8420/api/health/models?state=degraded
```

### Monitoring Degraded Models

**Key metrics to watch:**

1. **Degraded model count:** Number of models in degraded state
2. **Consecutive failures:** Current failure streak per model
3. **Success rate:** Rolling success rate (should stay > 90%)
4. **Error type distribution:** Which errors are most common

**Example monitoring query:**

```bash
# Check for degraded models every 5 minutes
watch -n 300 'curl -s http://localhost:8420/api/health/models?state=degraded | jq ". | length"'
```

### Logging

Health registry logs important events:

```
INFO  health registry initialized
  persist_path=/home/user/.evoclaw/model_health.json
  failure_threshold=3
  cooldown=5m0s

WARN  model degraded
  model=gpt-4o-mini
  consecutive_failures=3
  error_type=quota_exhausted

INFO  model recovered
  model=claude-sonnet-4
  previous_state=degraded
  downtime=4m23s

INFO  using fallback model
  preferred=claude-sonnet-4
  fallback=gpt-4o-mini
```

### Persistence File Format

The health state is persisted as JSON:

```json
{
  "models": {
    "claude-sonnet-4-20250514": {
      "state": "healthy",
      "consecutive_failures": 0,
      "total_requests": 152,
      "total_failures": 3,
      "success_rate": 0.980,
      "error_types": {
        "rate_limited": 2,
        "timeout": 1
      }
    }
  },
  "last_updated": "2026-02-15T11:30:00Z",
  "version": "1.0"
}
```

**Location:** `~/.evoclaw/model_health.json` (default)

## Best Practices

### 1. Set Appropriate Failure Thresholds

**For critical systems:** Lower threshold (1-2)
- Degrades models quickly on first failure
- Routes to backups immediately
- Higher operational costs but better availability

**For cost-sensitive systems:** Higher threshold (5-10)
- Allows transient failures before degrading
- Reduces unnecessary fallback usage
- Some failed requests but lower overall cost

### 2. Configure Cooldown Based on Provider Behavior

**Cloud providers with frequent outages:** Shorter cooldown (1-3 min)
- Quick recovery to healthy models
- Faster return to normal routing

**Self-hosted/local models:** Longer cooldown (10-30 min)
- More time for manual intervention
- Avoids rapid retry loops on persistent issues

### 3. Monitor Error Type Distribution

```go
status, _ := healthReg.GetModelStatus("model-id")
for errType, count := range status.ErrorTypes {
    fmt.Printf("%s: %d occurrences\n", errType, count)
}
```

**Use insights to:**
- Detect quota exhaustion early → Increase quota or switch providers
- Identify rate limiting → Spread load across more models
- Spot auth errors → Check API key rotation
- Find timeout patterns → Increase timeouts or switch providers

### 4. Regular Health Audits

**Weekly audit:**
```bash
# Export health report
curl -s http://localhost:8420/api/health/models | jq '.' > health_report.json

# Check for models with low success rate
jq 'to_entries[] | select(.value.success_rate < 0.9) | .key' health_report.json
```

**Monthly cleanup:**
- Remove old models from config
- Reset persist file if corrupted
- Adjust thresholds based on metrics

### 5. Combine with Intelligent Router

The health registry works best when combined with intelligent routing:

```go
// Intelligent router selects based on task complexity
decision := router.Route(prompt)
preferred := decision.Model

// Health registry ensures selected model is healthy
selected := healthReg.GetHealthyModel(preferred, fallbacks)
```

**Benefits:**
- Simple tasks use cheap models (cost optimization)
- Complex tasks use premium models (quality)
- Health registry ensures reliability (availability)

## Troubleshooting

### Problem: Models Never Recover from Degraded State

**Symptoms:** Model stays degraded even after provider recovers

**Solutions:**
1. Check cooldown period — may be too long
2. Verify API key quota — may still be exhausted
3. Manually reset model: `healthReg.ResetModel(modelID)`
4. Check provider status page — outage may be ongoing

### Problem: Too Many False Positives (Healthy Models Degraded)

**Symptoms:** Models degraded after just 1-2 transient failures

**Solutions:**
1. Increase `failureThreshold` to 5-10
2. Check if errors are transient (network blips, rate limit spikes)
3. Review error classification — may be misclassifying errors

### Problem: High Latency on Model Selection

**Symptoms:** Slow model selection with many models tracked

**Solutions:**
1. Health registry uses RWMutex — reads are concurrent
2. Persist operations are async — don't block routing
3. If still slow, check disk I/O on `persistPath`

### Problem: Persist File Corruption

**Symptoms:** Health registry fails to load on startup

**Solutions:**
1. Delete `~/.evoclaw/model_health.json` and restart EvoClaw
2. Health registry will start fresh (loses history, but safe)
3. Check disk space — persist writes may fail if disk full

## Configuration Reference

### Full Configuration Example

```json
{
  "models": {
    "providers": {
      "anthropic": {
        "baseUrl": "https://api.anthropic.com",
        "apiKey": "sk-ant-xxx",
        "models": [
          {
            "id": "claude-sonnet-4-20250514",
            "name": "Claude Sonnet 4",
            "contextWindow": 200000
          },
          {
            "id": "claude-opus-4-20250514",
            "name": "Claude Opus 4",
            "contextWindow": 200000
          }
        ]
      },
      "openai": {
        "baseUrl": "https://api.openai.com/v1",
        "apiKey": "sk-xxx",
        "models": [
          {
            "id": "gpt-4o-mini",
            "name": "GPT-4o Mini",
            "contextWindow": 128000
          }
        ]
      },
      "ollama": {
        "baseUrl": "http://localhost:11434",
        "apiKey": "",
        "models": [
          {
            "id": "qwen2.5:32b",
            "name": "Qwen 2.5 32B",
            "contextWindow": 32768
          }
        ]
      }
    },
    "routing": {
      "simple": "gpt-4o-mini",
      "medium": "gpt-4o-mini",
      "complex": "claude-sonnet-4-20250514",
      "critical": "claude-opus-4-20250514",
      "reasoning": "deepseek-r1-32b"
    },
    "health": {
      "persistPath": "~/.evoclaw/model_health.json",
      "failureThreshold": 3,
      "cooldownMinutes": 5
    }
  }
}
```

## See Also

- **[Intelligent Router](INTELLIGENT-ROUTER.md)** — Task-based model selection
- **[Configuration Guide](../README.md)** — Full EvoClaw configuration
- **[API Reference](../README.md#api-endpoints)** — HTTP API documentation

---

*Last updated: 2026-02-15*
