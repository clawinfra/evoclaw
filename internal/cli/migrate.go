package cli

import (
	"flag"
	"fmt"

	"github.com/clawinfra/evoclaw/internal/migrate"
)

// MigrateCommand handles the `evoclaw migrate` subcommand.
func MigrateCommand(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: evoclaw migrate openclaw [--source DIR] [--target DIR] [--dry-run]")
		fmt.Println("\nSupported sources: openclaw")
		return 1
	}

	switch args[0] {
	case "openclaw":
		return migrateOpenClaw(args[1:])
	default:
		fmt.Printf("Unknown migration source: %s (supported: openclaw)\n", args[0])
		return 1
	}
}

func migrateOpenClaw(args []string) int {
	fs := flag.NewFlagSet("migrate openclaw", flag.ContinueOnError)
	source := fs.String("source", "", "OpenClaw home directory (default ~/.openclaw)")
	target := fs.String("target", "", "EvoClaw home directory (default ~/.evoclaw)")
	dryRun := fs.Bool("dry-run", false, "Show what would be migrated without writing")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	opts := migrate.Options{
		Source: *source,
		Target: *target,
		DryRun: *dryRun,
	}

	if *dryRun {
		fmt.Println("🔍 Dry run mode — no files will be written")
		fmt.Println()
	}

	result, err := migrate.OpenClaw(opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	fmt.Println("📦 OpenClaw → EvoClaw Migration Results")
	fmt.Println("========================================")

	if len(result.Memory) > 0 {
		fmt.Printf("\n🧠 Memory files: %d\n", len(result.Memory))
		for _, m := range result.Memory {
			fmt.Printf("  • %s\n", m)
		}
	}

	if len(result.Identity) > 0 {
		fmt.Printf("\n🪪 Identity fields: %d\n", len(result.Identity))
		for _, i := range result.Identity {
			fmt.Printf("  • %s\n", i)
		}
	}

	if len(result.Skills) > 0 {
		fmt.Printf("\n🔧 Skills: %d\n", len(result.Skills))
		for _, s := range result.Skills {
			fmt.Printf("  • %s\n", s)
		}
	}

	if len(result.Config) > 0 {
		fmt.Printf("\n⚙️  Config: %d\n", len(result.Config))
		for _, c := range result.Config {
			fmt.Printf("  • %s\n", c)
		}
	}

	if len(result.Cron) > 0 {
		fmt.Printf("\n⏰ Cron jobs: %d\n", len(result.Cron))
		for _, c := range result.Cron {
			fmt.Printf("  • %s\n", c)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\n⚠️  Warnings: %d\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	}

	if *dryRun {
		fmt.Println("\n✅ Dry run complete. Run without --dry-run to apply.")
	} else {
		fmt.Println("\n✅ Migration complete!")
	}

	return 0
}
