package cli

import (
	"context"
	"fmt"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/logger"
	"github.com/clawinfra/evoclaw/internal/updater"
	"github.com/spf13/cobra"
)

func newUpdateCmd(cfg *config.Config, log *logger.Logger, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install updates",
		Long:  `Check for the latest EvoClaw version and optionally install it.`,
	}

	cmd.AddCommand(newUpdateCheckCmd(cfg, log, version))
	cmd.AddCommand(newUpdateInstallCmd(cfg, log, version))
	cmd.AddCommand(newUpdateEnableCmd(cfg, log))
	cmd.AddCommand(newUpdateDisableCmd(cfg, log))

	return cmd
}

func newUpdateCheckCmd(cfg *config.Config, log *logger.Logger, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := updater.New(cfg, log, version)

			log.Info("checking for updates", "current", version)

			release, updateAvailable, err := u.CheckForUpdates(ctx)
			if err != nil {
				return fmt.Errorf("check for updates: %w", err)
			}

			if !updateAvailable {
				log.Info("you are running the latest version", "version", version)
				return nil
			}

			log.Info("update available",
				"current", version,
				"latest", release.TagName,
				"name", release.Name)

			fmt.Printf("\n%s\n\n", release.Body)
			fmt.Println("Run 'evoclaw update install' to update")

			return nil
		},
	}
}

func newUpdateInstallCmd(cfg *config.Config, log *logger.Logger, version string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the latest update",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := updater.New(cfg, log, version)

			log.Info("checking for updates", "current", version)

			release, updateAvailable, err := u.CheckForUpdates(ctx)
			if err != nil {
				return fmt.Errorf("check for updates: %w", err)
			}

			if !updateAvailable && !force {
				log.Info("you are running the latest version", "version", version)
				return nil
			}

			log.Info("installing update", "version", release.TagName)

			if err := u.DownloadAndInstall(ctx, release); err != nil {
				return fmt.Errorf("install update: %w", err)
			}

			log.Info("update installed successfully", "version", release.TagName)
			fmt.Println("\n🎉 Update complete! Restart EvoClaw to use the new version:")
			fmt.Println("  sudo systemctl restart evoclaw  (Linux systemd)")
			fmt.Println("  launchctl kickstart -k gui/$UID/com.evoclaw.agent  (macOS)")
			fmt.Println("  evoclaw gateway restart  (manual mode)")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall even if up to date")

	return cmd
}

func newUpdateEnableCmd(cfg *config.Config, log *logger.Logger) *cobra.Command {
	var autoInstall bool

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable automatic update checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Updates == nil {
				cfg.Updates = &config.UpdatesConfig{}
			}

			cfg.Updates.Enabled = true
			cfg.Updates.AutoInstall = autoInstall

			// Save config
			configPath := getConfigPath()
			if err := cfg.Save(configPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if autoInstall {
				log.Info("automatic updates enabled (with auto-install)")
			} else {
				log.Info("automatic update checks enabled (notify only)")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&autoInstall, "auto-install", false, "Automatically install updates")

	return cmd
}

func newUpdateDisableCmd(cfg *config.Config, log *logger.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable automatic update checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Updates == nil {
				cfg.Updates = &config.UpdatesConfig{}
			}

			cfg.Updates.Enabled = false
			cfg.Updates.AutoInstall = false

			// Save config
			configPath := getConfigPath()
			if err := cfg.Save(configPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			log.Info("automatic updates disabled")

			return nil
		},
	}
}
