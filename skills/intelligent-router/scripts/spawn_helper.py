#!/usr/bin/env python3
"""
Intelligent Router - Spawn Helper

Automatically classifies tasks and spawns sub-agents with optimal model selection.

Usage:
    from spawn_helper import spawn_with_routing
    
    spawn_with_routing(
        task="Fix authentication bug in login.py",
        label="auth-fix"
    )
    
Or standalone:
    python spawn_helper.py "task description"
"""

import sys
import json
import subprocess
from pathlib import Path

SCRIPT_DIR = Path(__file__).parent
CONFIG_FILE = SCRIPT_DIR.parent / "config.json"


def load_config():
    """Load router configuration."""
    if not CONFIG_FILE.exists():
        raise FileNotFoundError(f"Router config not found: {CONFIG_FILE}")
    
    with open(CONFIG_FILE) as f:
        return json.load(f)


def classify_task(task_description):
    """Classify task and return tier + recommended model."""
    result = subprocess.run(
        ["python3", str(SCRIPT_DIR / "router.py"), "classify", task_description],
        capture_output=True,
        text=True,
        check=True
    )
    
    # Parse output
    lines = result.stdout.strip().split('\n')
    tier = None
    model_id = None
    
    for line in lines:
        if line.startswith("Classification:"):
            tier = line.split(":", 1)[1].strip()
        elif line.startswith("Recommended Model:"):
            model_id = line.split(":", 1)[1].strip()
    
    return tier, model_id


def get_primary_model(tier, config):
    """Get primary model ID for a given tier."""
    rules = config.get("routing_rules", {})
    tier_rules = rules.get(tier, {})
    
    primary_id = tier_rules.get("primary")
    if primary_id:
        return primary_id
    
    # Fallback: find first model in that tier
    for model in config.get("models", []):
        if model.get("tier") == tier:
            return model["id"]
    
    raise ValueError(f"No model found for tier: {tier}")


def spawn_with_routing(task, label=None, **kwargs):
    """
    Spawn sub-agent with automatic model routing.
    
    Args:
        task (str): Task description
        label (str, optional): Session label
        **kwargs: Additional arguments to pass to sessions_spawn
    
    Returns:
        dict: Classification info (tier, model, confidence)
    """
    config = load_config()
    
    # Classify task
    tier, model_id = classify_task(task)
    
    if not model_id:
        model_id = get_primary_model(tier, config)
    
    # Build spawn command
    print(f"🎯 Classified as {tier} tier → {model_id}")
    print(f"📋 Task: {task[:80]}{'...' if len(task) > 80 else ''}")
    print(f"\n⚠️  This is a helper - you must call sessions_spawn yourself:")
    print(f"    sessions_spawn(")
    print(f"        task=\"{task}\",")
    print(f"        model=\"{model_id}\",")
    if label:
        print(f"        label=\"{label}\",")
    for k, v in kwargs.items():
        if isinstance(v, str):
            print(f"        {k}=\"{v}\",")
        else:
            print(f"        {k}={v},")
    print(f"    )")
    
    return {
        "tier": tier,
        "model": model_id,
        "task": task
    }


def main():
    """CLI entry point."""
    if len(sys.argv) < 2:
        print("Usage: python spawn_helper.py <task_description>")
        sys.exit(1)
    
    task = " ".join(sys.argv[1:])
    result = spawn_with_routing(task)
    
    print(f"\n📊 Result:")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
