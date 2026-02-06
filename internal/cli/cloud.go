// Package cli provides CLI command implementations for EvoClaw.
package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloud"
)

// CloudCLI handles the `evoclaw cloud` subcommands.
type CloudCLI struct {
	apiURL     string
	httpClient *http.Client
}

// NewCloudCLI creates a new cloud CLI handler.
func NewCloudCLI(apiURL string) *CloudCLI {
	return &CloudCLI{
		apiURL: strings.TrimRight(apiURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewCloudCLIWithClient creates a CloudCLI with a custom HTTP client (for testing).
func NewCloudCLIWithClient(apiURL string, client *http.Client) *CloudCLI {
	return &CloudCLI{
		apiURL:     strings.TrimRight(apiURL, "/"),
		httpClient: client,
	}
}

// Run executes the cloud subcommand based on args.
// Returns exit code.
func (c *CloudCLI) Run(args []string) int {
	if len(args) == 0 {
		c.printUsage()
		return 1
	}

	switch args[0] {
	case "spawn":
		return c.runSpawn(args[1:])
	case "list":
		return c.runList()
	case "kill":
		return c.runKill(args[1:])
	case "logs":
		return c.runLogs(args[1:])
	case "costs":
		return c.runCosts()
	case "help", "--help", "-h":
		c.printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown cloud command: %s\n", args[0])
		c.printUsage()
		return 1
	}
}

// printUsage displays cloud subcommand help.
func (c *CloudCLI) printUsage() {
	fmt.Println(`Usage: evoclaw cloud <command> [options]

Manage E2B cloud agent sandboxes.

Commands:
  spawn     Spawn a new cloud agent
  list      List running cloud agents
  kill      Terminate a cloud agent
  logs      View cloud agent logs
  costs     Show E2B credit usage

Examples:
  evoclaw cloud spawn --template evoclaw-agent --config agent.toml
  evoclaw cloud list
  evoclaw cloud kill sb-abc123
  evoclaw cloud logs sb-abc123
  evoclaw cloud costs`)
}

// runSpawn handles `evoclaw cloud spawn`.
func (c *CloudCLI) runSpawn(args []string) int {
	fs := flag.NewFlagSet("cloud spawn", flag.ContinueOnError)
	template := fs.String("template", "evoclaw-agent", "E2B template name")
	agentID := fs.String("id", "", "Agent ID (auto-generated if empty)")
	agentType := fs.String("type", "trader", "Agent type: trader, monitor, sensor")
	configFile := fs.String("config", "", "Agent config file path")
	timeout := fs.Int("timeout", 300, "Sandbox timeout in seconds")
	genome := fs.String("genome", "", "JSON strategy genome")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	config := cloud.AgentConfig{
		TemplateID: *template,
		AgentID:    *agentID,
		AgentType:  *agentType,
		TimeoutSec: *timeout,
		Genome:     *genome,
	}

	// Read config file if provided
	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
			return 1
		}
		config.EnvVars = map[string]string{
			"EVOCLAW_CONFIG": string(data),
		}
	}

	body, _ := json.Marshal(config)
	resp, err := c.httpClient.Post(c.apiURL+"/api/cloud/spawn", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (HTTP %d): %s\n", resp.StatusCode, string(errBody))
		return 1
	}

	var sandbox cloud.Sandbox
	json.NewDecoder(resp.Body).Decode(&sandbox)

	fmt.Printf("✅ Cloud agent spawned\n")
	fmt.Printf("   Sandbox ID:  %s\n", sandbox.SandboxID)
	fmt.Printf("   Agent ID:    %s\n", sandbox.AgentID)
	fmt.Printf("   Template:    %s\n", sandbox.TemplateID)
	fmt.Printf("   Expires:     %s\n", sandbox.EndsAt.Format(time.RFC3339))
	return 0
}

// runList handles `evoclaw cloud list`.
func (c *CloudCLI) runList() int {
	resp, err := c.httpClient.Get(c.apiURL + "/api/cloud")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (HTTP %d): %s\n", resp.StatusCode, string(errBody))
		return 1
	}

	var sandboxes []cloud.Sandbox
	json.NewDecoder(resp.Body).Decode(&sandboxes)

	if len(sandboxes) == 0 {
		fmt.Println("No cloud agents running.")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SANDBOX ID\tAGENT ID\tTEMPLATE\tSTATE\tSTARTED\tEXPIRES")
	for _, s := range sandboxes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.SandboxID,
			s.AgentID,
			s.TemplateID,
			s.State,
			s.StartedAt.Format("15:04:05"),
			s.EndsAt.Format("15:04:05"),
		)
	}
	w.Flush()

	fmt.Printf("\n%d cloud agent(s) running.\n", len(sandboxes))
	return 0
}

// runKill handles `evoclaw cloud kill <sandbox-id>`.
func (c *CloudCLI) runKill(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: evoclaw cloud kill <sandbox-id>\n")
		return 1
	}

	sandboxID := args[0]
	req, _ := http.NewRequest(http.MethodDelete, c.apiURL+"/api/cloud/"+sandboxID, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (HTTP %d): %s\n", resp.StatusCode, string(errBody))
		return 1
	}

	fmt.Printf("✅ Agent %s killed.\n", sandboxID)
	return 0
}

// runLogs handles `evoclaw cloud logs <sandbox-id>`.
func (c *CloudCLI) runLogs(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: evoclaw cloud logs <sandbox-id>\n")
		return 1
	}

	sandboxID := args[0]
	resp, err := c.httpClient.Get(c.apiURL + "/api/cloud/" + sandboxID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (HTTP %d): %s\n", resp.StatusCode, string(errBody))
		return 1
	}

	var status cloud.Status
	json.NewDecoder(resp.Body).Decode(&status)

	fmt.Printf("📋 Agent Status: %s\n", sandboxID)
	fmt.Printf("   Agent ID:    %s\n", status.AgentID)
	fmt.Printf("   State:       %s\n", status.State)
	fmt.Printf("   Healthy:     %v\n", status.Healthy)
	fmt.Printf("   Uptime:      %ds\n", status.UptimeSec)
	fmt.Printf("   Expires:     %s\n", status.EndsAt.Format(time.RFC3339))
	return 0
}

// runCosts handles `evoclaw cloud costs`.
func (c *CloudCLI) runCosts() int {
	resp, err := c.httpClient.Get(c.apiURL + "/api/cloud/costs")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (HTTP %d): %s\n", resp.StatusCode, string(errBody))
		return 1
	}

	var costs cloud.CostSnapshot
	json.NewDecoder(resp.Body).Decode(&costs)

	fmt.Println("💰 E2B Credit Usage")
	fmt.Printf("   Active sandboxes:  %d\n", costs.ActiveSandboxes)
	fmt.Printf("   Total sandboxes:   %d\n", costs.TotalSandboxes)
	fmt.Printf("   Total uptime:      %s\n", formatDuration(time.Duration(costs.TotalUptimeSec)*time.Second))
	fmt.Printf("   Estimated cost:    $%.4f\n", costs.EstimatedCostUSD)
	fmt.Printf("   Budget:            $%.2f\n", costs.BudgetUSD)
	fmt.Printf("   Remaining:         $%.4f\n", costs.BudgetRemaining)

	if costs.BudgetRemaining < costs.BudgetUSD*0.1 {
		fmt.Println("   ⚠️  Budget running low!")
	}

	return 0
}

// formatDuration formats a duration as human-readable string.
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
