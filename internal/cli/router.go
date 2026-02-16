package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RouterCommand handles 'evoclaw router' subcommands
func RouterCommand(args []string, configPath string) int {
	if len(args) == 0 {
		printRouterHelp()
		return 1
	}

	subCmd := args[0]
	switch subCmd {
	case "classify":
		return routerClassify(args[1:])
	case "recommend":
		return routerRecommend(args[1:])
	case "models":
		return routerModels(args[1:], configPath)
	case "help", "--help", "-h":
		printRouterHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown router subcommand: %s\n", subCmd)
		printRouterHelp()
		return 1
	}
}

func printRouterHelp() {
	fmt.Println(`Usage: evoclaw router <subcommand> [options]

Intelligent model routing for cost-optimized task delegation.
Classifies tasks by complexity and recommends appropriate models.

Subcommands:
  classify <task>        Classify task complexity
  recommend <task>       Get model recommendation
  models                 List available models by tier

Examples:
  # Classify task complexity
  evoclaw router classify "fix authentication bug in user login"

  # Get model recommendation
  evoclaw router recommend "design scalable microservices architecture"

  # List models by tier
  evoclaw router models

Task Tiers:
  SIMPLE    - Basic checks, monitoring, simple queries
  MEDIUM    - Bug fixes, code reviews, straightforward features
  COMPLEX   - Architecture design, multi-component features
  REASONING - Formal proofs, logic puzzles, mathematical reasoning
  CRITICAL  - Security audits, production deployments, financial ops
`)
}

func routerClassify(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task description required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw router classify \"task description\"")
		return 1
	}

	task := args[0]

	// Find router.py script
	skillDir := filepath.Join(os.Getenv("HOME"), ".evoclaw", "skills", "intelligent-router")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		// Try system skills dir
		skillDir = "/usr/local/share/evoclaw/skills/intelligent-router"
	}

	routerScript := filepath.Join(skillDir, "scripts", "router.py")
	if _, err := os.Stat(routerScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: router.py not found at %s\n", routerScript)
		fmt.Fprintln(os.Stderr, "Install intelligent-router skill first")
		return 1
	}

	// Run classification
	cmd := exec.Command("python3", routerScript, "classify", task)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running classifier: %v\n%s\n", err, string(output))
		return 1
	}

	// Parse result
	var result struct {
		Task       string  `json:"task"`
		Tier       string  `json:"tier"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing result: %v\n", err)
		return 1
	}

	// Display result
	fmt.Printf("Tier: %s (%.0f%% confidence)\n", result.Tier, result.Confidence*100)
	if result.Reasoning != "" {
		fmt.Printf("Reasoning: %s\n", result.Reasoning)
	}

	return 0
}

func routerRecommend(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: task description required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw router recommend \"task description\"")
		return 1
	}

	task := args[0]

	// Find router.py script
	skillDir := filepath.Join(os.Getenv("HOME"), ".evoclaw", "skills", "intelligent-router")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		skillDir = "/usr/local/share/evoclaw/skills/intelligent-router"
	}

	routerScript := filepath.Join(skillDir, "scripts", "router.py")
	if _, err := os.Stat(routerScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: router.py not found at %s\n", routerScript)
		return 1
	}

	// Run recommendation
	cmd := exec.Command("python3", routerScript, "recommend", task)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting recommendation: %v\n%s\n", err, string(output))
		return 1
	}

	// Parse result
	var result struct {
		Task          string   `json:"task"`
		Tier          string   `json:"tier"`
		PrimaryModel  string   `json:"primary_model"`
		FallbackChain []string `json:"fallback_chain"`
		EstimatedCost string   `json:"estimated_cost"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing result: %v\n", err)
		return 1
	}

	// Display result
	fmt.Printf("Task Tier: %s\n", result.Tier)
	fmt.Printf("Recommended Model: %s\n", result.PrimaryModel)
	if len(result.FallbackChain) > 0 {
		fmt.Printf("Fallback Chain: %v\n", result.FallbackChain)
	}
	if result.EstimatedCost != "" {
		fmt.Printf("Estimated Cost: %s\n", result.EstimatedCost)
	}

	return 0
}

func routerModels(args []string, configPath string) int {
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Load router config
	skillDir := filepath.Join(os.Getenv("HOME"), ".evoclaw", "skills", "intelligent-router")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		skillDir = "/usr/local/share/evoclaw/skills/intelligent-router"
	}

	configFile := filepath.Join(skillDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading router config: %v\n", err)
		return 1
	}

	var routerConfig struct {
		ModelCatalog struct {
			Simple    []string `json:"simple"`
			Medium    []string `json:"medium"`
			Complex   []string `json:"complex"`
			Reasoning []string `json:"reasoning"`
			Critical  []string `json:"critical"`
		} `json:"model_catalog"`
	}

	if err := json.Unmarshal(data, &routerConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing router config: %v\n", err)
		return 1
	}

	// Check which models are available in evoclaw config
	availableModels := make(map[string]bool)
	for _, provider := range cfg.Models.Providers {
		for _, model := range provider.Models {
			availableModels[model.ID] = true
		}
	}

	// Display models by tier
	printTier := func(tier string, models []string) {
		fmt.Printf("\n%s:\n", tier)
		for _, model := range models {
			status := "❌"
			if availableModels[model] {
				status = "✅"
			}
			fmt.Printf("  %s %s\n", status, model)
		}
	}

	fmt.Println("Model Catalog (✅ = available in config):")
	printTier("SIMPLE", routerConfig.ModelCatalog.Simple)
	printTier("MEDIUM", routerConfig.ModelCatalog.Medium)
	printTier("COMPLEX", routerConfig.ModelCatalog.Complex)
	printTier("REASONING", routerConfig.ModelCatalog.Reasoning)
	printTier("CRITICAL", routerConfig.ModelCatalog.Critical)

	return 0
}
