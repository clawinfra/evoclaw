package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GovernanceCommand handles 'evoclaw governance' subcommands
func GovernanceCommand(args []string, configPath string) int {
	if len(args) == 0 {
		printGovernanceHelp()
		return 1
	}

	subCmd := args[0]
	switch subCmd {
	case "wal":
		return governanceWAL(args[1:])
	case "vbr":
		return governanceVBR(args[1:])
	case "adl":
		return governanceADL(args[1:])
	case "vfm":
		return governanceVFM(args[1:])
	case "status":
		return governanceStatus(args[1:])
	case "help", "--help", "-h":
		printGovernanceHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown governance subcommand: %s\n", subCmd)
		printGovernanceHelp()
		return 1
	}
}

func printGovernanceHelp() {
	fmt.Print(`Usage: evoclaw governance <subcommand> [options]

Self-governance protocols for agent reliability and consistency.

Subcommands:
  wal <action>           Write-Ahead Log operations
  vbr <action>           Verify Before Reporting
  adl <action>           Anti-Divergence Limit (persona consistency)
  vfm <action>           Value-For-Money tracking
  status                 Show governance status

WAL (Write-Ahead Log):
  wal append --type correction --text "..."     Log correction
  wal append --type decision --text "..."       Log decision
  wal replay                                     Replay unapplied entries
  wal flush                                      Flush working buffer

VBR (Verify Before Reporting):
  vbr check --type file-exists --target path    Verify file exists
  vbr check --type process-running --target pid Verify process running
  vbr log --task-id <id> --passed true          Log verification result

ADL (Anti-Divergence Limit):
  adl load-baseline --soul-path SOUL.md         Load persona baseline
  adl check-drift --text "current behavior"     Check for drift
  adl report                                     Show drift history

VFM (Value-For-Money):
  vfm track --model <model> --input <n> --output <n> --cost <usd>
  vfm check-budget                               Check budget status
  vfm report                                     Show cost stats

Examples:
  # Log a user correction
  evoclaw governance wal append --type correction --text "Use uv not pip"

  # Verify task completion
  evoclaw governance vbr check --type file-exists --target /tmp/output.json

  # Check persona drift
  evoclaw governance adl check-drift --text "I prefer quick fixes over proper solutions"

  # Track API cost
  evoclaw governance vfm track --model gpt-4 --input 1000 --output 500 --cost 0.05
`)
}

func governanceWAL(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: WAL action required (append, replay, flush)")
		return 1
	}

	action := args[0]
	skillDir := getGovernanceSkillDir()
	walScript := filepath.Join(skillDir, "scripts", "wal.py")

	if _, err := os.Stat(walScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: wal.py not found at %s\n", walScript)
		return 1
	}

	switch action {
	case "append":
		fs := flag.NewFlagSet("append", flag.ExitOnError)
		entryType := fs.String("type", "", "Entry type (correction, decision, analysis)")
		text := fs.String("text", "", "Entry text")
		_ = fs.Parse(args[1:])

		if *entryType == "" || *text == "" {
			fmt.Fprintln(os.Stderr, "Error: --type and --text required")
			return 1
		}

		cmd := exec.Command("python3", walScript, "append", *entryType, *text)
		return runCommand(cmd)

	case "replay":
		cmd := exec.Command("python3", walScript, "replay")
		return runCommand(cmd)

	case "flush":
		cmd := exec.Command("python3", walScript, "flush")
		return runCommand(cmd)

	default:
		fmt.Fprintf(os.Stderr, "Unknown WAL action: %s\n", action)
		return 1
	}
}

func governanceVBR(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: VBR action required (check, log)")
		return 1
	}

	action := args[0]
	skillDir := getGovernanceSkillDir()
	vbrScript := filepath.Join(skillDir, "scripts", "vbr.py")

	if _, err := os.Stat(vbrScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: vbr.py not found at %s\n", vbrScript)
		return 1
	}

	switch action {
	case "check":
		fs := flag.NewFlagSet("check", flag.ExitOnError)
		checkType := fs.String("type", "", "Check type (file-exists, process-running, http-status)")
		target := fs.String("target", "", "Check target")
		_ = fs.Parse(args[1:])

		if *checkType == "" || *target == "" {
			fmt.Fprintln(os.Stderr, "Error: --type and --target required")
			return 1
		}

		cmd := exec.Command("python3", vbrScript, "check", *checkType, *target)
		return runCommand(cmd)

	case "log":
		fs := flag.NewFlagSet("log", flag.ExitOnError)
		taskID := fs.String("task-id", "", "Task ID")
		passed := fs.Bool("passed", false, "Verification passed")
		notes := fs.String("notes", "", "Optional notes")
		_ = fs.Parse(args[1:])

		if *taskID == "" {
			fmt.Fprintln(os.Stderr, "Error: --task-id required")
			return 1
		}

		passedStr := "false"
		if *passed {
			passedStr = "true"
		}

		cmd := exec.Command("python3", vbrScript, "log", *taskID, passedStr, *notes)
		return runCommand(cmd)

	default:
		fmt.Fprintf(os.Stderr, "Unknown VBR action: %s\n", action)
		return 1
	}
}

func governanceADL(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: ADL action required (load-baseline, check-drift, report)")
		return 1
	}

	action := args[0]
	skillDir := getGovernanceSkillDir()
	adlScript := filepath.Join(skillDir, "scripts", "adl.py")

	if _, err := os.Stat(adlScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: adl.py not found at %s\n", adlScript)
		return 1
	}

	switch action {
	case "load-baseline":
		fs := flag.NewFlagSet("load-baseline", flag.ExitOnError)
		soulPath := fs.String("soul-path", "SOUL.md", "Path to SOUL.md")
		_ = fs.Parse(args[1:])

		cmd := exec.Command("python3", adlScript, "load-baseline", *soulPath)
		return runCommand(cmd)

	case "check-drift":
		fs := flag.NewFlagSet("check-drift", flag.ExitOnError)
		text := fs.String("text", "", "Current behavior text")
		_ = fs.Parse(args[1:])

		if *text == "" {
			fmt.Fprintln(os.Stderr, "Error: --text required")
			return 1
		}

		cmd := exec.Command("python3", adlScript, "check-drift", *text)
		return runCommand(cmd)

	case "report":
		cmd := exec.Command("python3", adlScript, "report")
		return runCommand(cmd)

	default:
		fmt.Fprintf(os.Stderr, "Unknown ADL action: %s\n", action)
		return 1
	}
}

func governanceVFM(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: VFM action required (track, check-budget, report)")
		return 1
	}

	action := args[0]
	skillDir := getGovernanceSkillDir()
	vfmScript := filepath.Join(skillDir, "scripts", "vfm.py")

	if _, err := os.Stat(vfmScript); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: vfm.py not found at %s\n", vfmScript)
		return 1
	}

	switch action {
	case "track":
		fs := flag.NewFlagSet("track", flag.ExitOnError)
		model := fs.String("model", "", "Model name")
		inputTokens := fs.Int("input", 0, "Input tokens")
		outputTokens := fs.Int("output", 0, "Output tokens")
		cost := fs.Float64("cost", 0, "Cost in USD")
		_ = fs.Parse(args[1:])

		if *model == "" || *inputTokens == 0 || *outputTokens == 0 {
			fmt.Fprintln(os.Stderr, "Error: --model, --input, --output required")
			return 1
		}

		cmd := exec.Command("python3", vfmScript, "track",
			*model,
			fmt.Sprintf("%d", *inputTokens),
			fmt.Sprintf("%d", *outputTokens),
			fmt.Sprintf("%.4f", *cost))
		return runCommand(cmd)

	case "check-budget":
		cmd := exec.Command("python3", vfmScript, "check-budget")
		return runCommand(cmd)

	case "report":
		cmd := exec.Command("python3", vfmScript, "report")
		return runCommand(cmd)

	default:
		fmt.Fprintf(os.Stderr, "Unknown VFM action: %s\n", action)
		return 1
	}
}

func governanceStatus(args []string) int {
	skillDir := getGovernanceSkillDir()

	// Collect status from all protocols
	status := make(map[string]interface{})

	// WAL status
	walScript := filepath.Join(skillDir, "scripts", "wal.py")
	if output, err := exec.Command("python3", walScript, "status").Output(); err == nil {
		var walStatus map[string]interface{}
		if err := json.Unmarshal(output, &walStatus); err == nil {
			status["wal"] = walStatus
		}
	}

	// VBR status
	vbrScript := filepath.Join(skillDir, "scripts", "vbr.py")
	if output, err := exec.Command("python3", vbrScript, "status").Output(); err == nil {
		var vbrStatus map[string]interface{}
		if err := json.Unmarshal(output, &vbrStatus); err == nil {
			status["vbr"] = vbrStatus
		}
	}

	// ADL status
	adlScript := filepath.Join(skillDir, "scripts", "adl.py")
	if output, err := exec.Command("python3", adlScript, "status").Output(); err == nil {
		var adlStatus map[string]interface{}
		if err := json.Unmarshal(output, &adlStatus); err == nil {
			status["adl"] = adlStatus
		}
	}

	// VFM status
	vfmScript := filepath.Join(skillDir, "scripts", "vfm.py")
	if output, err := exec.Command("python3", vfmScript, "status").Output(); err == nil {
		var vfmStatus map[string]interface{}
		if err := json.Unmarshal(output, &vfmStatus); err == nil {
			status["vfm"] = vfmStatus
		}
	}

	// Pretty print
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(status); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding status: %v\n", err)
		return 1
	}

	return 0
}

func getGovernanceSkillDir() string {
	skillDir := filepath.Join(os.Getenv("HOME"), ".evoclaw", "skills", "agent-self-governance")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		skillDir = "/usr/local/share/evoclaw/skills/agent-self-governance"
	}
	return skillDir
}

func runCommand(cmd *exec.Cmd) int {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 1
	}
	return 0
}
