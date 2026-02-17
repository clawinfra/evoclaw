package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// ScheduleCommand handles 'evoclaw schedule' subcommands
func ScheduleCommand(args []string, configPath string) int {
	if len(args) == 0 {
		printScheduleHelp()
		return 1
	}

	subCmd := args[0]
	switch subCmd {
	case "list":
		return scheduleList(args[1:], configPath)
	case "add":
		return scheduleAdd(args[1:], configPath)
	case "remove":
		return scheduleRemove(args[1:], configPath)
	case "run":
		return scheduleRun(args[1:], configPath)
	case "status":
		return scheduleStatus(args[1:], configPath)
	case "help", "--help", "-h":
		printScheduleHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown schedule subcommand: %s\n", subCmd)
		printScheduleHelp()
		return 1
	}
}

func printScheduleHelp() {
	fmt.Print(`Usage: evoclaw schedule <subcommand> [options]

Manage scheduled jobs for periodic tasks.

Subcommands:
  list              List all scheduled jobs
  add               Add a new job (use --config job.json)
  remove <job-id>   Remove a job
  run <job-id>      Trigger a job immediately
  status <job-id>   Show job execution status

Examples:
  # List all jobs
  evoclaw schedule list

  # Add job from file
  evoclaw schedule add --config sensor-job.json

  # Run job immediately (bypass schedule)
  evoclaw schedule run sensor-read

  # Check job status
  evoclaw schedule status sensor-read

Job Configuration (JSON):
{
  "id": "sensor-read",
  "name": "Read Temperature Sensor",
  "enabled": true,
  "schedule": {
    "kind": "interval",
    "intervalMs": 300000
  },
  "action": {
    "kind": "shell",
    "command": "./sensors/read_temp.sh"
  }
}

Schedule Kinds:
  interval   - Run every N milliseconds (intervalMs)
  cron       - Run on cron expression (expr)
  at         - Run daily at specific time (time="HH:MM")

Action Kinds:
  shell   - Run shell command
  agent   - Send message to agent
  mqtt    - Publish to MQTT topic
  http    - Make HTTP request
`)
}

func scheduleList(args []string, configPath string) int {
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	if !cfg.Scheduler.Enabled {
		fmt.Println("Scheduler is disabled in config")
		return 0
	}

	if len(cfg.Scheduler.Jobs) == 0 {
		fmt.Println("No jobs configured")
		return 0
	}

	// Print jobs in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSCHEDULE\tACTION\tENABLED\tRUNS\tERRORS")
	fmt.Fprintln(w, "--\t----\t--------\t------\t-------\t----\t------")

	for _, job := range cfg.Scheduler.Jobs {
		scheduleDesc := formatSchedule(job.Schedule)
		actionDesc := formatAction(job.Action)
		enabled := "yes"
		if !job.Enabled {
			enabled = "no"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t-\t-\n",
			job.ID,
			job.Name,
			scheduleDesc,
			actionDesc,
			enabled)
	}

	w.Flush()
	return 0
}

func scheduleAdd(args []string, configPath string) int {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	jobFile := fs.String("config", "", "Job configuration file (JSON)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	if *jobFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --config required")
		return 1
	}

	// Load job config
	data, err := os.ReadFile(*jobFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading job file: %v\n", err)
		return 1
	}

	var job config.SchedulerJobConfig
	if err := json.Unmarshal(data, &job); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing job JSON: %v\n", err)
		return 1
	}

	// Load evoclaw config
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Enable scheduler if not enabled
	cfg.Scheduler.Enabled = true

	// Check for duplicate ID
	for _, existing := range cfg.Scheduler.Jobs {
		if existing.ID == job.ID {
			fmt.Fprintf(os.Stderr, "Error: Job with ID '%s' already exists\n", job.ID)
			return 1
		}
	}

	// Add job
	cfg.Scheduler.Jobs = append(cfg.Scheduler.Jobs, job)

	// Save config
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Job '%s' added\n", job.ID)
	fmt.Println("  Restart evoclaw for changes to take effect")
	return 0
}

func scheduleRemove(args []string, configPath string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: job ID required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw schedule remove <job-id>")
		return 1
	}

	jobID := args[0]

	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Find and remove job
	found := false
	newJobs := make([]config.SchedulerJobConfig, 0, len(cfg.Scheduler.Jobs))
	for _, job := range cfg.Scheduler.Jobs {
		if job.ID == jobID {
			found = true
			continue
		}
		newJobs = append(newJobs, job)
	}

	if !found {
		fmt.Fprintf(os.Stderr, "Error: Job '%s' not found\n", jobID)
		return 1
	}

	cfg.Scheduler.Jobs = newJobs

	// Save config
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Job '%s' removed\n", jobID)
	fmt.Println("  Restart evoclaw for changes to take effect")
	return 0
}

func scheduleRun(args []string, configPath string) int {
	fmt.Fprintln(os.Stderr, "Error: schedule run requires runtime access")
	fmt.Fprintln(os.Stderr, "This command will be implemented via API in a future release")
	return 1
}

func scheduleStatus(args []string, configPath string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: job ID required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw schedule status <job-id>")
		return 1
	}

	jobID := args[0]

	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Find job
	var targetJob *config.SchedulerJobConfig
	for i := range cfg.Scheduler.Jobs {
		if cfg.Scheduler.Jobs[i].ID == jobID {
			targetJob = &cfg.Scheduler.Jobs[i]
			break
		}
	}

	if targetJob == nil {
		fmt.Fprintf(os.Stderr, "Error: Job '%s' not found\n", jobID)
		return 1
	}

	// Print job details
	fmt.Printf("Job: %s\n", targetJob.Name)
	fmt.Printf("ID: %s\n", targetJob.ID)
	fmt.Printf("Enabled: %v\n", targetJob.Enabled)
	fmt.Printf("Schedule: %s\n", formatSchedule(targetJob.Schedule))
	fmt.Printf("Action: %s\n", formatAction(targetJob.Action))
	fmt.Println()
	fmt.Println("Note: Runtime stats require API access")
	fmt.Println("      Use API endpoint /api/scheduler/jobs/<id> for live status")

	return 0
}

// Helper functions

func formatSchedule(s config.ScheduleConfig) string {
	switch s.Kind {
	case "interval":
		duration := time.Duration(s.IntervalMs) * time.Millisecond
		return fmt.Sprintf("Every %s", duration)
	case "cron":
		return fmt.Sprintf("Cron: %s", s.Expr)
	case "at":
		return fmt.Sprintf("Daily at %s", s.Time)
	default:
		return s.Kind
	}
}

func formatAction(a config.ActionConfig) string {
	switch a.Kind {
	case "shell":
		return fmt.Sprintf("Shell: %s", a.Command)
	case "agent":
		return fmt.Sprintf("Agent: %s", a.AgentID)
	case "mqtt":
		return fmt.Sprintf("MQTT: %s", a.Topic)
	case "http":
		return fmt.Sprintf("HTTP: %s %s", a.Method, a.URL)
	default:
		return a.Kind
	}
}
