---
name: intelligent-router
description: Intelligent model routing for sub-agent task delegation. Choose the optimal model based on task complexity, cost, and capability requirements. Reduces costs by routing simple tasks to cheaper models while preserving quality for complex work.
version: 2.2.0
---

# Intelligent Router

## Quick Setup

**New to this skill?** Start here:

1. **Copy `config.json.example` to `config.json`** (or customize the included `config.json`)
2. **Edit `config.json`** to add your available models with their costs and tiers
3. **Test the CLI**: `python scripts/router.py classify "your task description"`
4. **Use in your agent**: Reference models from your config when spawning sub-agents

**Example config entry:**
```json
{
  "id": "anthropic/claude-sonnet-4",
  "alias": "Claude Sonnet",
  "tier": "COMPLEX",
  "input_cost_per_m": 3.00,
  "output_cost_per_m": 15.00,
  "capabilities": ["text", "code", "reasoning", "vision"]
}
```

---

## 🆕 What's New in v2.2

**Major enhancements inspired by ClawRouter:**

1. **🧮 REASONING Tier** - New dedicated tier for formal proofs, mathematical derivations, and step-by-step logic
   - Primary: DeepSeek R1 32B ($0.20/$0.20)
   - Fallback: QwQ 32B, DeepSeek Reasoner
   - Confidence threshold: 0.97 (requires strong reasoning signals)

2. **🔁 Automatic Fallback Chains** - Models now have 2-3 fallback options with automatic retry
   - SIMPLE: GLM-4.7 → GLM-4.5-Air → Ollama qwen2.5:32b
   - MEDIUM: DeepSeek V3.2 → Llama 3.3 70B → Ollama
   - COMPLEX: Sonnet 4.5 → Gemini 3 Pro → Nemotron 253B
   - REASONING: R1 32B → QwQ 32B → DeepSeek Reasoner
   - Max 3 attempts per request

3. **🤖 Agentic Task Detection** - Automatically detects multi-step tasks and bumps tier appropriately
   - Keywords: "run", "test", "fix", "deploy", "edit", "build", "create", "implement"
   - Multi-step patterns: "first...then", "step 1", numbered lists
   - Tool presence detection

4. **⚖️ Weighted Scoring (15 Dimensions)** - Replaces simple keyword matching with sophisticated scoring
   - 15 weighted dimensions (reasoning markers, code presence, multi-step patterns, etc.)
   - Confidence formula: `confidence = 1 / (1 + exp(-8 * (score - 0.5)))`
   - More accurate tier classification with confidence scores

**New CLI commands:**
- `python scripts/router.py score "task"` - Show detailed scoring breakdown
- Confidence scores now shown in all classifications

---

## Overview

This skill teaches AI agents how to intelligently route sub-agent tasks to different LLM models based on task complexity, cost, and capability requirements. The goal is to **reduce costs by routing simple tasks to cheaper models while preserving quality for complex work**.

## When to Use This Skill

Use this skill whenever you:
- Spawn sub-agents or delegate tasks to other models
- Need to choose between different LLM options
- Want to optimize costs without sacrificing quality
- Handle tasks with varying complexity levels
- Need to estimate costs before execution

## Core Routing Logic

### 1. Five-Tier Classification System

Tasks are classified into five tiers based on complexity and risk:

| Tier | Description | Example Tasks | Model Characteristics |
|------|-------------|---------------|----------------------|
| **🟢 SIMPLE** | Routine, low-risk operations | Monitoring, status checks, API calls, summarization | Cheapest available, good for repetitive tasks |
| **🟡 MEDIUM** | Moderate complexity work | Code fixes, research, small patches, data analysis | Balanced cost/quality, good general purpose |
| **🟠 COMPLEX** | Multi-component development | Feature builds, debugging, architecture, multi-file changes | High-quality reasoning, excellent code generation |
| **🧮 REASONING** | Formal logic and proofs | Mathematical proofs, theorem derivation, step-by-step verification | Specialized reasoning models with chain-of-thought |
| **🔴 CRITICAL** | High-stakes operations | Security audits, production deploys, financial operations | Best available model, maximum reliability |

### 2. Model Selection Strategy

Configure your available models in `config.json` and assign them to tiers. The router will automatically recommend models based on task classification.

**Configuration pattern:**
```json
{
  "models": [
    {
      "id": "{provider}/{model-name}",
      "alias": "Human-Friendly Name",
      "tier": "SIMPLE|MEDIUM|COMPLEX|CRITICAL",
      "provider": "anthropic|openai|google|local|etc",
      "input_cost_per_m": 0.50,
      "output_cost_per_m": 1.50,
      "context_window": 128000,
      "capabilities": ["text", "code", "reasoning", "vision"],
      "notes": "Optional notes about strengths/limitations"
    }
  ]
}
```

**Tier recommendations:**
- **SIMPLE tier**: Models under $0.50/M input, optimized for speed/cost
- **MEDIUM tier**: Models $0.50-$3.00/M input, good at coding/analysis
- **COMPLEX tier**: Models $3.00-$5.00/M input, excellent reasoning
- **REASONING tier**: Models $0.20-$2.00/M, specialized for mathematical/logical reasoning
- **CRITICAL tier**: Best available models, cost secondary to quality

### 2a. Weighted Scoring System (15 Dimensions)

The router uses a sophisticated 15-dimension weighted scoring system to classify tasks accurately:

**Scoring Dimensions (weights shown):**
1. **Reasoning Markers** (0.18) - Keywords like "prove", "theorem", "derive", "verify", "logic"
2. **Code Presence** (0.15) - Code blocks, function definitions, file extensions
3. **Multi-Step Patterns** (0.12) - "first...then", "step 1", numbered lists
4. **Agentic Task** (0.10) - Action verbs: "run", "test", "fix", "deploy", "build"
5. **Technical Terms** (0.10) - "algorithm", "architecture", "optimization", "security"
6. **Token Count** (0.08) - Estimated complexity based on length
7. **Creative Markers** (0.05) - "creative", "story", "compose", "brainstorm"
8. **Question Complexity** (0.05) - Multiple questions, complex queries
9. **Constraint Count** (0.04) - "must", "require", "only", "exactly"
10. **Imperative Verbs** (0.03) - "analyze", "evaluate", "compare", "assess"
11. **Output Format** (0.03) - Structured output requests (JSON, tables, markdown)
12. **Simple Indicators** (0.02) - "check", "get", "show", "status" (inverted)
13. **Domain Specificity** (0.02) - Acronyms, technical notation
14. **Reference Complexity** (0.02) - Cross-references, contextual dependencies
15. **Negation Complexity** (0.01) - Negations, exceptions, exclusions

**Confidence Calculation:**
```
weighted_score = Σ(dimension_score × weight)
confidence = 1 / (1 + exp(-8 × (weighted_score - 0.5)))
```

This creates a smooth S-curve where:
- Score 0.0 → Confidence ~2%
- Score 0.5 → Confidence ~50%
- Score 1.0 → Confidence ~98%

**View detailed scoring:**
```bash
python scripts/router.py score "prove that sqrt(2) is irrational step by step"
# Shows breakdown of all 15 dimensions and top contributors
```

### 2b. Agentic Task Detection

Tasks are automatically flagged as "agentic" if they involve:
- **Action keywords**: run, test, fix, deploy, edit, build, create, implement
- **Multi-step patterns**: "first...then", "step 1", numbered instructions
- **Tool array presence** (when used programmatically)

**Impact on routing:**
- Agentic tasks are automatically bumped to at least MEDIUM tier
- Models with `"agentic": true` flag are preferred for these tasks
- Simple monitoring tasks won't be incorrectly classified as agentic

**Example:**
```bash
router.py classify "Build a React component with tests, then deploy it"
# → COMPLEX tier (agentic task detected)

router.py classify "Check the server status"
# → SIMPLE tier (not agentic)
```

### 2c. Automatic Fallback Chains

Each tier now has a fallback chain (2-3 models) for automatic retry on failure:

**Configured fallback chains:**
```json
{
  "SIMPLE": {
    "primary": "glm-4.7",
    "fallback_chain": ["glm-4.7-backup", "glm-4.5-air", "ollama-qwen"]
  },
  "MEDIUM": {
    "primary": "deepseek-v3.2",
    "fallback_chain": ["llama-3.3-70b", "ollama-llama"]
  },
  "COMPLEX": {
    "primary": "claude-sonnet-4.5",
    "fallback_chain": ["gemini-3-pro", "nemotron-253b"]
  },
  "REASONING": {
    "primary": "deepseek-r1-32b",
    "fallback_chain": ["qwq-32b", "deepseek-reasoner"]
  },
  "CRITICAL": {
    "primary": "claude-opus-4.6",
    "fallback_chain": ["claude-opus-oauth", "claude-sonnet-4.5"]
  }
}
```

**Usage:**
- Maximum 3 attempts per request (1 primary + 2 fallbacks)
- Automatic retry with next model in chain on failure
- Preserves cost efficiency by trying cheaper options first (within same tier)

### 3. Coding Task Routing (Important!)

**For coding tasks specifically:**

- **Simple code tasks** (lint fixes, small patches, single-file changes)
  - Use MEDIUM tier model as primary coder
  - Consider spawning a SIMPLE tier model as QA reviewer
  - **Cost check**: Only use coder+QA if combined cost < using COMPLEX tier directly

- **Complex code tasks** (multi-file builds, architecture, debugging)
  - Use COMPLEX or CRITICAL tier directly
  - Skip delegation — premium models are more reliable and cost-effective for complex work
  - QA review unnecessary when using top-tier models

**Decision flow for coding:**
```
IF task is simple code (lint, patch, single file):
  → {medium_model} as coder + optional {simple_model} QA
  → Only if (coder + QA cost) < {complex_model} solo

IF task is complex code (multi-file, architecture):
  → {complex_model} or {critical_model} directly
  → Skip delegation, skip QA — the model IS the quality
```

### 4. Usage Pattern

When spawning sub-agents, use the `model` parameter with IDs from your `config.json`:

```python
# Use the router CLI to classify first (optional but recommended)
# $ python scripts/router.py classify "check GitHub notifications"
# → Recommends: SIMPLE tier, {simple_model}

# Simple task — monitoring
sessions_spawn(
    task="Check GitHub notifications and summarize recent activity",
    model="{simple_model}",  # Use your configured SIMPLE tier model ID
    label="github-monitor"
)

# Medium task — code fix
sessions_spawn(
    task="Fix lint errors in utils.js and write changes to disk",
    model="{medium_model}",  # Use your configured MEDIUM tier model ID
    label="lint-fix"
)

# Complex task — feature development
sessions_spawn(
    task="Build authentication system with JWT, middleware, tests",
    model="{complex_model}",  # Use your configured COMPLEX tier model ID
    label="auth-feature"
)

# Critical task — security audit
sessions_spawn(
    task="Security audit of authentication code for vulnerabilities",
    model="{critical_model}",  # Use your configured CRITICAL tier model ID
    label="security-audit"
)
```

### 5. Cost Awareness

Understanding cost structures helps optimize routing decisions:

**Typical cost ranges per million tokens (input/output):**
- **SIMPLE tier**: $0.10-$0.50 / $0.10-$1.50
- **MEDIUM tier**: $0.40-$3.00 / $0.40-$15.00
- **COMPLEX tier**: $3.00-$5.00 / $1.30-$25.00
- **CRITICAL tier**: $5.00+ / $25.00+

**Cost estimation:**
```bash
# Estimate cost before running
python scripts/router.py cost-estimate "build authentication system"
# Output: Tier: COMPLEX, Est. cost: $0.024 USD
```

**Rule of thumb**: 
- High-volume repetitive tasks → cheaper models
- One-off complex critical work → premium models
- When in doubt → estimate cost of both options and compare

### 6. Fallback & Escalation Strategy

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

### 7. Decision Heuristics

Quick classification rules for common patterns (now automated via weighted scoring):

**SIMPLE tier indicators:**
- Keywords: check, monitor, fetch, get, status, list, summarize
- High-frequency operations (heartbeats, polling)
- Well-defined API calls with minimal logic
- Data extraction without analysis
- **Example**: "What is 2+2?"

**MEDIUM tier indicators:**
- Keywords: fix, patch, update, research, analyze, test
- Code changes under ~50 lines
- Single-file modifications
- Research and documentation tasks
- **Example**: "Write a Python function to sort a list"

**COMPLEX tier indicators:**
- Keywords: build, create, architect, debug, design, integrate
- Multi-file changes or new features
- Complex debugging or troubleshooting
- System design and architecture work
- Agentic tasks with multiple steps
- **Example**: "Build a React component with tests, then deploy it"

**REASONING tier indicators** (requires confidence ≥ 0.97):
- Keywords: prove, theorem, derive, formal, verify, logic, step by step
- Mathematical proofs and derivations
- Formal verification tasks
- Step-by-step logical reasoning
- Theorem proving and lemmas
- **Example**: "Prove that sqrt(2) is irrational step by step"

**CRITICAL tier indicators:**
- Keywords: security, production, deploy, financial, audit
- Security-sensitive operations
- Production deployments
- Financial or legal analysis
- High-stakes decision-making
- **Example**: "Security audit of authentication code for vulnerabilities"

**Classification is automatic:** The weighted scoring system analyzes all 15 dimensions. These heuristics are for understanding what triggers each tier.

**When in doubt:** Go one tier up. Under-speccing costs more in retries than over-speccing costs in model quality.

### 8. Extended Thinking Modes

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

```python
# Enable thinking for complex architectural work
sessions_spawn(
    task="Design scalable microservices architecture for payment system",
    model="{complex_model}",
    thinking="on",  # or thinking="budget_tokens:5000"
    label="architecture-design"
)
```

## Advanced Patterns

### Pattern 1: Two-Phase Processing

For large or uncertain tasks, use a cheaper model for initial work, then refine with a better model.

**Note:** Sub-agents are asynchronous — results come back as notifications, not synchronous returns.

```python
# Phase 1: Draft with cheaper model
sessions_spawn(
    task="Draft initial API design document outline",
    model="{simple_model}",
    label="draft-phase"
)

# Wait for draft-phase to complete and write output...

# Phase 2: Refine with capable model (after Phase 1 finishes)
sessions_spawn(
    task="Review and refine the draft at /tmp/api-draft.md, add detailed specs",
    model="{medium_model}",
    label="refine-phase"
)
```

**Savings:** Process bulk content with cheap model, only use expensive model for refinement.

### Pattern 2: Batch Processing

Group multiple similar SIMPLE tasks together to reduce overhead:

```python
# Instead of spawning 10 separate agents:
tasks = [
    "Check server1 status",
    "Check server2 status",
    # ... 10 tasks
]

# Batch them:
sessions_spawn(
    task=f"Run these checks: {', '.join(tasks)}. Report any issues.",
    model="{simple_model}",
    label="batch-monitoring"
)
```

### Pattern 3: Tiered Escalation

Start with MEDIUM tier, escalate to COMPLEX if needed:

```python
# Try MEDIUM first
sessions_spawn(
    task="Debug intermittent test failures in test_auth.py",
    model="{medium_model}",
    label="debug-attempt-1"
)

# If insufficient, escalate:
if debug_failed:
    sessions_spawn(
        task="Deep debug of test_auth.py failures (previous attempt incomplete)",
        model="{complex_model}",
        label="debug-attempt-2"
    )
```

### Pattern 4: Cost-Benefit Analysis

Before routing, consider:

1. **Criticality**: How bad is failure? → Higher criticality = higher tier
2. **Cost delta**: What's the price difference between tiers? → Small delta = lean toward higher tier
3. **Retry costs**: Will failures require retries? → High retry cost = start with higher tier
4. **Time sensitivity**: How urgent is completion? → Urgent = higher tier for speed/reliability

## Using the Router CLI

The included `router.py` script helps with classification and cost estimation:

```bash
# Classify a task and get model recommendation
python scripts/router.py classify "fix authentication bug in login.py"
# Output: Classification: MEDIUM
#         Recommended Model: {medium_model}

# List all configured models
python scripts/router.py models
# Output: Models grouped by tier

# Check configuration health
python scripts/router.py health
# Output: Validates config.json structure

# Estimate task cost
python scripts/router.py cost-estimate "build payment processing system"
# Output: Tier: COMPLEX, Est. tokens: 5000/3000, Cost: $0.060 USD
```

**Integration tip:** Run `router.py classify` before spawning agents to validate your tier selection.

## Configuration Guide

### Setting Up Your Models

1. **Inventory your models**: List all LLM providers and models you have access to
2. **Gather pricing**: Find input/output costs per million tokens from provider docs
3. **Assign tiers**: Map models to SIMPLE/MEDIUM/COMPLEX/CRITICAL based on capability
4. **Document capabilities**: Note what each model can do (vision, function-calling, etc.)
5. **Add notes**: Include limitations or special characteristics

### Example Multi-Provider Config

```json
{
  "models": [
    {
      "id": "local/ollama-qwen-1.5b",
      "alias": "Local Qwen",
      "tier": "SIMPLE",
      "provider": "ollama",
      "input_cost_per_m": 0.00,
      "output_cost_per_m": 0.00,
      "context_window": 32768,
      "capabilities": ["text"],
      "notes": "Free local model, good for testing and simple tasks"
    },
    {
      "id": "openai/gpt-4o-mini",
      "alias": "GPT-4o Mini",
      "tier": "MEDIUM",
      "provider": "openai",
      "input_cost_per_m": 0.15,
      "output_cost_per_m": 0.60,
      "context_window": 128000,
      "capabilities": ["text", "code", "vision"],
      "notes": "Great balance of cost and capability"
    },
    {
      "id": "anthropic/claude-sonnet-4",
      "alias": "Claude Sonnet",
      "tier": "COMPLEX",
      "provider": "anthropic",
      "input_cost_per_m": 3.00,
      "output_cost_per_m": 15.00,
      "context_window": 200000,
      "capabilities": ["text", "code", "reasoning", "vision"],
      "notes": "Excellent for complex coding and analysis"
    },
    {
      "id": "anthropic/claude-opus-4",
      "alias": "Claude Opus",
      "tier": "CRITICAL",
      "provider": "anthropic",
      "input_cost_per_m": 15.00,
      "output_cost_per_m": 75.00,
      "context_window": 200000,
      "capabilities": ["text", "code", "reasoning", "vision", "function-calling"],
      "notes": "Best available for critical operations"
    }
  ]
}
```

### Validation Checklist

- [ ] At least one model per tier (SIMPLE, MEDIUM, COMPLEX, CRITICAL)
- [ ] All models have required fields (id, alias, tier, costs, capabilities)
- [ ] Model IDs match your actual provider/model format
- [ ] Costs are accurate per million tokens
- [ ] Tiers make sense relative to each other (SIMPLE cheaper than CRITICAL)
- [ ] Run `python scripts/router.py health` to validate

## Resources

For additional guidance:

- **[references/model-catalog.md](references/model-catalog.md)** - Guide to evaluating and selecting models for each tier
- **[references/examples.md](references/examples.md)** - Real-world routing patterns and examples
- **[config.json](config.json)** - Your model configuration (customize this!)

## Quick Reference Card

**Classification:**
- **"check/monitor/fetch"** → SIMPLE tier
- **"fix/patch/research"** → MEDIUM tier
- **"build/debug/architect"** → COMPLEX tier
- **"security/production/financial"** → CRITICAL tier

**Coding tasks:**
- Simple code → {medium_model} + optional {simple_model} QA
- Complex code → {complex_model} or {critical_model} directly

**Cost optimization:**
- High-volume tasks → cheaper models
- One-off complex tasks → premium models
- When uncertain → estimate both options

**General rule:**
When in doubt, go one tier up — retries cost more than quality.
