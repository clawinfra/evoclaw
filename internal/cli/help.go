package cli

import (
	"fmt"
	"os"
)

// commandInfo describes a top-level subcommand.
type commandInfo struct {
	Name    string
	Args    string
	Short   string
	Long    string
	Examples []string
}

var commands = []commandInfo{
	{
		Name:  "start",
		Args:  "[--config <file>]",
		Short: "Start the EvoClaw orchestrator (default action)",
		Long: `Start the EvoClaw orchestrator server.

Loads agents, models, channels, and skills from the config file.
Exposes REST API and web dashboard on the configured port (default :8420).`,
		Examples: []string{
			"evoclaw",
			"evoclaw start",
			"evoclaw start --config /etc/evoclaw/evoclaw.json",
		},
	},
	{
		Name:  "init",
		Args:  "[--dir <path>]",
		Short: "Initialise a new EvoClaw workspace",
		Long: `Create a new EvoClaw workspace with a default config file,
example agent definitions, and recommended directory structure.`,
		Examples: []string{
			"evoclaw init",
			"evoclaw init --dir /opt/evoclaw",
		},
	},
	{
		Name:  "memory",
		Args:  "<store|retrieve|consolidate|status>",
		Short: "Manage the tiered memory system",
		Long: `Interact with the three-tier memory system (hot/warm/cold).

Subcommands:
  store       Store a new memory node
  retrieve    Retrieve nodes by semantic query
  consolidate Run consolidation pass (hot→warm, warm→cold)
  status      Show memory tier statistics`,
		Examples: []string{
			"evoclaw memory status",
			`evoclaw memory store --text "Deploy key: abc123" --category infra --importance 0.9`,
			`evoclaw memory retrieve --query "deploy key"`,
			"evoclaw memory consolidate --tier warm",
		},
	},
	{
		Name:  "schedule",
		Args:  "<list|add|remove|run>",
		Short: "Manage scheduled agent tasks (cron-style)",
		Long: `Create, list, and manage periodic agent tasks.

Subcommands:
  list    Show all scheduled tasks
  add     Add a new scheduled task
  remove  Remove a task by ID
  run     Trigger a task immediately`,
		Examples: []string{
			"evoclaw schedule list",
			`evoclaw schedule add --cron "0 2 * * *" --agent alex-hub --prompt "Nightly summary"`,
			"evoclaw schedule remove --id task-123",
			"evoclaw schedule run --id task-123",
		},
	},
	{
		Name:  "router",
		Args:  "<list|test|stats>",
		Short: "Inspect and test the intelligent model router",
		Long: `Query the model routing layer — list available models,
test routing decisions, or view provider usage statistics.

Subcommands:
  list    List all registered models and providers
  test    Simulate a routing decision for a given prompt
  stats   Show provider usage statistics`,
		Examples: []string{
			"evoclaw router list",
			`evoclaw router test --prompt "Write a Rust function"`,
			"evoclaw router stats",
		},
	},
	{
		Name:  "governance",
		Args:  "<status|log|flush>",
		Short: "Self-governance protocol (WAL, VBR, ADL)",
		Long: `Manage the agent self-governance system:
  - WAL: Write-Ahead Log for decision persistence
  - VBR: Verify-Before-Reporting audit trail
  - ADL: Anti-Divergence Limit monitoring

Subcommands:
  status  Show governance health and pending entries
  log     View recent WAL entries
  flush   Flush pending WAL entries to cold storage`,
		Examples: []string{
			"evoclaw governance status",
			"evoclaw governance log --limit 20",
			"evoclaw governance flush",
		},
	},
	{
		Name:  "chain",
		Args:  "<list|status|balance|send>",
		Short: "Interact with connected blockchains",
		Long: `Manage on-chain operations for connected EVM / Substrate chains.

Subcommands:
  list     List configured chains and connection status
  status   Show chain health (latest block, peers)
  balance  Query account balance
  send     Send a transaction`,
		Examples: []string{
			"evoclaw chain list",
			"evoclaw chain status --chain clawchain",
			"evoclaw chain balance --chain bsc --address 0xABC...",
		},
	},
	{
		Name:  "gateway",
		Args:  "<start|stop|restart|status>",
		Short: "Manage the EvoClaw gateway daemon",
		Long: `Control the long-running EvoClaw gateway process via
PID file management.

Subcommands:
  start    Start the gateway daemon
  stop     Stop the gateway daemon
  restart  Graceful restart (sends SIGHUP)
  status   Show daemon PID and uptime`,
		Examples: []string{
			"evoclaw gateway start",
			"evoclaw gateway status",
			"evoclaw gateway restart",
			"evoclaw gateway stop",
		},
	},
	{
		Name:  "version",
		Short: "Print version and build information",
		Examples: []string{
			"evoclaw version",
			"evoclaw --version",
		},
	},
}

// PrintHelp prints top-level help (evoclaw help).
func PrintHelp(binaryName string) {
	fmt.Fprintf(os.Stdout, `EvoClaw — Self-Evolving Agent Framework
https://github.com/clawinfra/evoclaw

USAGE:
  %s [command] [flags]

COMMANDS:
`, binaryName)

	for _, c := range commands {
		if c.Args != "" {
			fmt.Fprintf(os.Stdout, "  %-12s %-30s %s\n", c.Name, c.Args, c.Short)
		} else {
			fmt.Fprintf(os.Stdout, "  %-12s %-30s %s\n", c.Name, "", c.Short)
		}
	}

	fmt.Fprintf(os.Stdout, `
GLOBAL FLAGS:
  --config <file>   Path to config file (default: evoclaw.json)
  --version         Print version information
  -h, --help        Show this help message

Run '%s help <command>' for detailed help on a specific command.
`, binaryName)
}

// PrintCommandHelp prints help for a specific subcommand.
func PrintCommandHelp(binaryName, cmdName string) {
	for _, c := range commands {
		if c.Name == cmdName {
			fmt.Fprintf(os.Stdout, "COMMAND: %s %s\n\n", binaryName, c.Name)
			if c.Args != "" {
				fmt.Fprintf(os.Stdout, "USAGE:\n  %s %s %s\n\n", binaryName, c.Name, c.Args)
			}
			if c.Long != "" {
				fmt.Fprintf(os.Stdout, "DESCRIPTION:\n  %s\n\n", c.Long)
			}
			if len(c.Examples) > 0 {
				fmt.Fprintln(os.Stdout, "EXAMPLES:")
				for _, ex := range c.Examples {
					fmt.Fprintf(os.Stdout, "  %s\n", ex)
				}
				fmt.Fprintln(os.Stdout)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "Unknown command: %s\n\nRun '%s help' for a list of commands.\n", cmdName, binaryName)
	os.Exit(1)
}

// CommandNames returns all valid command names (used for error messages).
func CommandNames() []string {
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name
	}
	return names
}
