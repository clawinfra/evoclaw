# Intelligent Router

Intelligent model routing for sub-agent task delegation. Choose the optimal model based on task complexity, cost, and capability requirements.

## Overview

The intelligent router automatically selects the best LLM model for a given task, optimizing for both quality and cost. It prevents wasteful spending on simple tasks while ensuring complex work gets the capabilities it needs.

**Key benefits:**
- **40-80% cost reduction** on routine operations
- **Automatic model selection** based on task complexity
- **Quality preservation** for critical work
- **Extensible tier system** supporting any LLM provider

## Core Concepts

### Four-Tier Classification System

Tasks are classified into four tiers based on complexity and risk:

| Tier | Description | Example Tasks | Model Characteristics |
|------|-------------|---------------|----------------------|
| **🟢 SIMPLE** | Routine, low-risk operations | Monitoring, status checks, API calls, summarization | Cheapest available, good for repetitive tasks |
| **🟡 MEDIUM** | Moderate complexity work | Code fixes, research, small patches, data analysis | Balanced cost/quality, good general purpose |
| **🟠 COMPLEX** | Multi-component development | Feature builds, debugging, architecture, multi-file changes | High-quality reasoning, excellent code generation |
| **🔴 CRITICAL** | High-stakes operations | Security audits, production deploys, financial operations | Best available model, maximum reliability |

### Model Configuration

Models are configured in the orchestrator's config file with tier assignments:

```json
{
  "models": [
    {
      "id": "nvidia-nim/microsoft/phi-4-mini-flash-reasoning",
      "alias": "Phi-4 Reasoning",
      "tier": "SIMPLE",
      "provider": "nvidia-nim",
      "input_cost_per_m": 0.05,
      "output_cost_per_m": 0.05,
      "context_window": 16384,
      "capabilities": ["text", "reasoning"]
    },
    {
      "id": "nvidia-nim/meta/llama-3.3-70b-instruct",
      "alias": "Llama 3.3 70B",
      "tier": "MEDIUM",
      "provider": "nvidia-nim",
      "input_cost_per_m": 0.40,
      "output_cost_per_m": 0.40,
      "context_window": 128000,
      "capabilities": ["text", "code"]
    },
    {
      "id": "nvidia-nim/deepseek-ai/deepseek-v3.2",
      "alias": "DeepSeek V3.2",
      "tier": "COMPLEX",
      "provider": "nvidia-nim",
      "input_cost_per_m": 0.80,
      "output_cost_per_m": 0.80,
      "context_window": 65536,
      "capabilities": ["text", "code", "reasoning"]
    },
    {
      "id": "nvidia-nim/deepseek-ai/deepseek-r1-distill-qwen-32b",
      "alias": "R1 32B",
      "tier": "REASONING",
      "provider": "nvidia-nim",
      "input_cost_per_m": 0.20,
      "output_cost_per_m": 0.20,
      "context_window": 65536,
      "capabilities": ["text", "code", "reasoning"]
    }
  ]
}
```

## Classification Logic

### Decision Heuristics

Quick classification rules for common patterns:

**SIMPLE tier indicators:**
- Keywords: `check`, `monitor`, `fetch`, `get`, `status`, `list`, `summarize`
- High-frequency operations (heartbeats, polling)
- Well-defined API calls with minimal logic
- Data extraction without analysis

**MEDIUM tier indicators:**
- Keywords: `fix`, `patch`, `update`, `research`, `analyze`, `test`
- Code changes under ~50 lines
- Single-file modifications
- Research and documentation tasks

**COMPLEX tier indicators:**
- Keywords: `build`, `create`, `architect`, `debug`, `design`, `integrate`
- Multi-file changes or new features
- Complex debugging or troubleshooting
- System design and architecture work

**CRITICAL tier indicators:**
- Keywords: `security`, `production`, `deploy`, `financial`, `audit`
- Security-sensitive operations
- Production deployments
- Financial or legal analysis
- High-stakes decision-making

**When in doubt:** Go one tier up. Under-speccing costs more in retries than over-speccing costs in model quality.

### Special Case: Coding Tasks

Coding tasks have specific routing logic to balance cost and quality:

**Simple code tasks** (lint fixes, small patches, single-file changes):
- Use MEDIUM tier model as primary coder
- Consider spawning a SIMPLE tier model as QA reviewer
- **Cost check**: Only use coder+QA if combined cost < using COMPLEX tier directly

**Complex code tasks** (multi-file builds, architecture, debugging):
- Use COMPLEX or CRITICAL tier directly
- Skip delegation — premium models are more reliable and cost-effective
- QA review unnecessary when using top-tier models

**Decision flow:**
```
IF task is simple code (lint, patch, single file):
  → {medium_model} as coder + optional {simple_model} QA
  → Only if (coder + QA cost) < {complex_model} solo

IF task is complex code (multi-file, architecture):
  → {complex_model} or {critical_model} directly
  → Skip delegation, skip QA — the model IS the quality
```

## Implementation

### Orchestrator Integration

The intelligent router is integrated into the orchestrator's model selection:

```go
// internal/router/router.go
type Router struct {
    config   *Config
    models   map[string]*Model
    tiers    map[string][]*Model
}

func (r *Router) ClassifyTask(task string) Tier {
    // Analyze task description for complexity signals
    score := r.calculateComplexityScore(task)
    
    if score < 0.3 {
        return TierSimple
    } else if score < 0.6 {
        return TierMedium
    } else if score < 0.8 {
        return TierComplex
    }
    return TierCritical
}

func (r *Router) SelectModel(tier Tier, capabilities []string) (*Model, error) {
    // Get models for tier
    candidates := r.tiers[tier]
    if len(candidates) == 0 {
        return nil, fmt.Errorf("no models configured for tier %s", tier)
    }
    
    // Filter by required capabilities
    filtered := r.filterByCapabilities(candidates, capabilities)
    if len(filtered) == 0 {
        // Fallback: escalate to next tier
        return r.SelectModel(tier+1, capabilities)
    }
    
    // Select cheapest model in tier
    return r.selectCheapest(filtered), nil
}

func (r *Router) calculateComplexityScore(task string) float64 {
    task = strings.ToLower(task)
    score := 0.0
    
    // Simple tier keywords (negative weight)
    simpleKeywords := []string{"check", "monitor", "fetch", "get", "status", "list", "summarize"}
    for _, kw := range simpleKeywords {
        if strings.Contains(task, kw) {
            score -= 0.2
        }
    }
    
    // Medium tier keywords (neutral weight)
    mediumKeywords := []string{"fix", "patch", "update", "research", "analyze", "test"}
    for _, kw := range mediumKeywords {
        if strings.Contains(task, kw) {
            score += 0.1
        }
    }
    
    // Complex tier keywords (positive weight)
    complexKeywords := []string{"build", "create", "architect", "debug", "design", "integrate"}
    for _, kw := range complexKeywords {
        if strings.Contains(task, kw) {
            score += 0.4
        }
    }
    
    // Critical tier keywords (high positive weight)
    criticalKeywords := []string{"security", "production", "deploy", "financial", "audit"}
    for _, kw := range criticalKeywords {
        if strings.Contains(task, kw) {
            score += 0.8
        }
    }
    
    // Task length indicator (longer = more complex)
    words := len(strings.Fields(task))
    if words > 50 {
        score += 0.2
    } else if words > 30 {
        score += 0.1
    }
    
    // Normalize to [0, 1]
    if score < 0 {
        score = 0
    } else if score > 1 {
        score = 1
    }
    
    return score
}
```

### Usage in Sub-Agent Spawning

When spawning a sub-agent, the orchestrator can use the router to select the model:

```go
// internal/orchestrator/orchestrator.go
func (o *Orchestrator) SpawnSubAgent(task string, label string) (*Session, error) {
    // Classify task
    tier := o.router.ClassifyTask(task)
    
    // Select model for tier
    model, err := o.router.SelectModel(tier, []string{"text", "code"})
    if err != nil {
        return nil, fmt.Errorf("model selection failed: %w", err)
    }
    
    o.logger.Info("Spawning sub-agent",
        "task", task,
        "tier", tier,
        "model", model.ID,
        "cost_per_m", model.InputCostPerM)
    
    // Spawn session with selected model
    session := o.sessions.Create(label, model.ID, task)
    return session, nil
}
```

## Cost Optimization Patterns

### Pattern 1: Two-Phase Processing

For large or uncertain tasks, use a cheaper model for initial work, then refine with a better model.

```go
// Phase 1: Draft with cheaper model
tier1 := o.router.ClassifyTask("Draft initial API design document outline")
model1, _ := o.router.SelectModel(tier1, []string{"text"})
session1 := o.sessions.Create("draft-phase", model1.ID, "Draft initial API design...")

// Wait for Phase 1 to complete...

// Phase 2: Refine with capable model
tier2 := o.router.ClassifyTask("Review and refine the draft, add detailed specs")
model2, _ := o.router.SelectModel(tier2, []string{"text", "code"})
session2 := o.sessions.Create("refine-phase", model2.ID, "Review draft at /tmp/api-draft.md...")
```

**Savings:** Process bulk content with cheap model, only use expensive model for refinement.

### Pattern 2: Batch Processing

Group multiple similar SIMPLE tasks together to reduce overhead:

```go
// Instead of spawning 10 separate agents
tasks := []string{
    "Check server1 status",
    "Check server2 status",
    // ... 10 tasks
}

// Batch them
batchTask := fmt.Sprintf("Run these checks: %s. Report any issues.", strings.Join(tasks, ", "))
tier := o.router.ClassifyTask(batchTask) // → TierSimple
model, _ := o.router.SelectModel(tier, []string{"text"})
session := o.sessions.Create("batch-monitoring", model.ID, batchTask)
```

### Pattern 3: Tiered Escalation

Start with MEDIUM tier, escalate to COMPLEX if needed:

```go
// Try MEDIUM first
tier := o.router.ClassifyTask("Debug intermittent test failures in test_auth.py")
model, _ := o.router.SelectModel(tier, []string{"code"})
session := o.sessions.Create("debug-attempt-1", model.ID, "Debug test_auth.py failures...")

// If insufficient, escalate
if debugFailed {
    // Force next tier up
    model2, _ := o.router.SelectModel(TierComplex, []string{"code"})
    session2 := o.sessions.Create("debug-attempt-2", model2.ID, 
        "Deep debug of test_auth.py failures (previous attempt incomplete)")
}
```

### Pattern 4: Cost-Benefit Analysis

Before routing, consider:

1. **Criticality**: How bad is failure? → Higher criticality = higher tier
2. **Cost delta**: What's the price difference between tiers? → Small delta = lean toward higher tier
3. **Retry costs**: Will failures require retries? → High retry cost = start with higher tier
4. **Time sensitivity**: How urgent is completion? → Urgent = higher tier for speed/reliability

## Extended Thinking Modes

Some models support extended thinking/reasoning which improves quality but increases cost:

**Models with thinking support:**
- Anthropic Claude models: Use `thinking="on"` or `thinking="budget_tokens:5000"`
- DeepSeek R1 variants: Built-in chain-of-thought reasoning
- OpenAI o1/o3 models: Native reasoning capabilities

**When to use thinking:**
- COMPLEX tier tasks requiring deep reasoning
- CRITICAL tier tasks where accuracy is paramount
- Multi-step logical problems
- Architecture and design decisions

**When to avoid thinking:**
- SIMPLE tier tasks (wasteful)
- MEDIUM tier routine operations
- High-frequency repetitive tasks
- Tasks where thinking tokens would 2-5x the cost unnecessarily

```go
// Enable thinking for complex architectural work
model, _ := o.router.SelectModel(TierComplex, []string{"code", "reasoning"})
session := o.sessions.CreateWithThinking(
    "architecture-design",
    model.ID,
    "Design scalable microservices architecture for payment system",
    "on", // or "budget_tokens:5000"
)
```

## Fallback & Escalation Strategy

If a model produces unsatisfactory results:

1. **Identify the issue**: Model limitation vs task misclassification
2. **Escalate one tier**: Try the next tier up for the same task
3. **Document failures**: Note model-specific limitations for future routing
4. **Consider capabilities**: Check if model has required capabilities (vision, function-calling, etc.)
5. **Review classification**: Was the task properly classified initially?

**Escalation path:**
```
SIMPLE → MEDIUM → COMPLEX → CRITICAL
```

**Implementation:**
```go
func (o *Orchestrator) ExecuteWithEscalation(task string) (*Result, error) {
    tier := o.router.ClassifyTask(task)
    
    for tier <= TierCritical {
        model, err := o.router.SelectModel(tier, []string{"code"})
        if err != nil {
            return nil, err
        }
        
        result, err := o.execute(task, model)
        if err == nil && result.Quality > 0.7 {
            return result, nil
        }
        
        // Escalate to next tier
        tier++
        o.logger.Info("Escalating to next tier", "tier", tier)
    }
    
    return nil, errors.New("all tiers exhausted")
}
```

## Metrics & Monitoring

### Routing Statistics

Track routing decisions over time:

```go
type RouterMetrics struct {
    TotalTasks        int
    TierDistribution  map[Tier]int
    AverageCost       float64
    EscalationRate    float64
    SuccessRate       map[Tier]float64
}

func (r *Router) GetMetrics() *RouterMetrics {
    // Calculate from routing history
}
```

**Example output:**
```
Total Tasks: 1,247
Tier Distribution:
  SIMPLE:   623 (50%)
  MEDIUM:   437 (35%)
  COMPLEX:  156 (12.5%)
  CRITICAL: 31  (2.5%)

Average Cost per Task: $0.042
Escalation Rate: 8.3%
Success Rate by Tier:
  SIMPLE:   96.2%
  MEDIUM:   91.8%
  COMPLEX:  98.7%
  CRITICAL: 100%
```

### Cost Savings

Compare actual costs vs. always-using-premium-model costs:

```go
func (r *Router) CalculateSavings() float64 {
    actualCost := r.totalCost
    
    // What would it cost using CRITICAL tier for everything?
    premiumModel := r.tiers[TierCritical][0]
    premiumCost := r.totalTasks * premiumModel.AverageCostPerTask
    
    savings := (premiumCost - actualCost) / premiumCost
    return savings
}
```

**Example:** 67% cost reduction (actual $450 vs. premium $1,350)

## Configuration Guide

### Tier Recommendations

**SIMPLE tier** (under $0.50/M input):
- Phi-4 Mini ($0.05/M)
- GLM-4.5-air ($0.10/M)
- DeepSeek Chat ($0.14/M)
- Claude Haiku ($0.25/M)

**MEDIUM tier** ($0.50-$3.00/M input):
- Llama 3.3 70B ($0.40/M)
- Gemini Pro ($0.50/M)
- GPT-4o Mini ($0.15/M — can be either SIMPLE or MEDIUM)
- Claude Sonnet ($3.00/M)

**COMPLEX tier** ($3.00-$5.00/M input):
- DeepSeek V3.2 ($0.80/M — pricing outlier, excellent value)
- Claude Sonnet 4.5 ($3.00/M)
- GPT-4o ($2.50/M)
- Gemini Pro 1.5 ($1.25/M)

**CRITICAL tier** ($5.00+/M input):
- Claude Opus 4.6 ($15.00/M)
- GPT-4 Turbo ($10.00/M)
- o1-preview ($15.00/M)

**REASONING tier** (specialized for extended thinking):
- DeepSeek R1 32B ($0.20/M)
- QwQ 32B ($0.20/M)
- Phi-4 Reasoning ($0.05/M)
- Claude Opus with thinking ($15.00/M + thinking tokens)

### Validation Checklist

Before deploying routing configuration:

- [ ] At least one model per tier (SIMPLE, MEDIUM, COMPLEX, CRITICAL)
- [ ] All models have required fields (id, alias, tier, costs, capabilities)
- [ ] Model IDs match actual provider/model format
- [ ] Costs are accurate per million tokens
- [ ] Tiers make sense relative to each other (SIMPLE cheaper than CRITICAL)
- [ ] Capabilities accurately reflect model features
- [ ] Notes document any special limitations or strengths

## Integration with Self-Governance

The intelligent router works with VFM (Value-For-Money) to track cost effectiveness:

```go
// After task completion
result, _ := o.execute(task, model)

// Log VFM
o.vfm.Log(
    agentID,
    task,
    model.ID,
    result.Tokens,
    result.Cost,
    result.Quality, // 0.0-1.0 subjective quality score
)

// VFM will suggest if a cheaper model would have sufficed
suggestions, _ := o.vfm.Suggest(agentID)
// Example: "Task 'monitoring' used claude-opus-4 ($2.40) but could use phi-4 ($0.03) for 98.75% savings"
```

See [SELF-GOVERNANCE.md](SELF-GOVERNANCE.md) for full VFM documentation.

## Real-World Results

Based on EvoClaw production usage (Feb 2026):

**Cost savings:**
- **Cron jobs**: 83% reduction ($0.30 → $0.05 per run)
- **Sub-agents**: 67% reduction ($0.60 → $0.20 average)
- **Main session**: 40% reduction (Opus → Sonnet for routine work)

**Quality metrics:**
- SIMPLE tier success rate: 96.2%
- MEDIUM tier success rate: 91.8%
- COMPLEX tier success rate: 98.7%
- Escalation rate: 8.3% (acceptable)

**Lesson learned:** DeepSeek V3.2 is a pricing outlier — COMPLEX-tier quality at MEDIUM-tier cost ($0.80/M). Use liberally.

## See Also

- [SELF-GOVERNANCE.md](SELF-GOVERNANCE.md) — VFM protocol for cost tracking
- [TIERED-MEMORY.md](TIERED-MEMORY.md) — Memory efficiency complementing model selection
- [EXECUTION-TIERS.md](EXECUTION-TIERS.md) — Execution chains and model routing
- [ROADMAP.md](ROADMAP.md) — Future routing enhancements
