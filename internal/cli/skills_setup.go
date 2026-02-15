package cli

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed ../../skills
var embeddedSkills embed.FS

//go:embed ../../templates
var embeddedTemplates embed.FS

// SetupCoreSkills installs core skills to ~/.evoclaw/skills/
func SetupCoreSkills(agentName, agentRole string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	evoclawDir := filepath.Join(homeDir, ".evoclaw")
	skillsDir := filepath.Join(evoclawDir, "skills")

	// Create skills directory
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("create skills directory: %w", err)
	}

	coreSkills := []string{
		"tiered-memory",
		"intelligent-router",
		"agent-self-governance",
	}

	fmt.Println()
	fmt.Println("📦 Installing core skills...")

	for _, skill := range coreSkills {
		sourcePath := filepath.Join("skills", skill)
		destPath := filepath.Join(skillsDir, skill)

		// Read embedded skill directory
		entries, err := embeddedSkills.ReadDir(sourcePath)
		if err != nil {
			fmt.Printf("   ⚠️  Skill %s not found in embedded files, skipping\\n", skill)
			continue
		}

		// Create destination directory
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return fmt.Errorf("create skill directory %s: %w", skill, err)
		}

		// Copy all files recursively
		if err := copyEmbeddedDir(embeddedSkills, sourcePath, destPath); err != nil {
			return fmt.Errorf("copy skill %s: %w", skill, err)
		}

		// Run install.sh if it exists
		installScript := filepath.Join(destPath, "install.sh")
		if _, err := os.Stat(installScript); err == nil {
			// Make executable
			if err := os.Chmod(installScript, 0755); err != nil {
				fmt.Printf("   ⚠️  Could not make install.sh executable for %s: %v\\n", skill, err)
			}

			// Run installation
			cmd := exec.Command("bash", installScript)
			cmd.Dir = destPath
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("AGENT_NAME=%s", agentName),
				fmt.Sprintf("AGENT_ROLE=%s", agentRole),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf("   ⚠️  Installation script failed for %s: %v\\n", skill, err)
				if len(output) > 0 {
					fmt.Printf("   Output: %s\\n", string(output))
				}
			} else {
				fmt.Printf("   ✅ %s installed\\n", skill)
			}
		} else {
			fmt.Printf("   ✅ %s copied (no install script)\\n", skill)
		}
	}

	return nil
}

// GenerateAgentFiles creates SOUL.md and AGENTS.md from templates
func GenerateAgentFiles(agentName, agentRole string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	evoclawDir := filepath.Join(homeDir, ".evoclaw")

	fmt.Println()
	fmt.Println("📝 Generating agent files...")

	// Generate SOUL.md
	soulTemplate, err := embeddedTemplates.ReadFile("templates/SOUL.md.template")
	if err != nil {
		return fmt.Errorf("read SOUL.md template: %w", err)
	}

	soulContent := string(soulTemplate)
	soulContent = strings.ReplaceAll(soulContent, "{{AGENT_NAME}}", agentName)
	soulContent = strings.ReplaceAll(soulContent, "{{AGENT_ROLE}}", agentRole)

	soulPath := filepath.Join(evoclawDir, "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(soulContent), 0644); err != nil {
		return fmt.Errorf("write SOUL.md: %w", err)
	}
	fmt.Printf("   ✅ SOUL.md created at %s\\n", soulPath)

	// Generate AGENTS.md
	agentsTemplate, err := embeddedTemplates.ReadFile("templates/AGENTS.md.template")
	if err != nil {
		return fmt.Errorf("read AGENTS.md template: %w", err)
	}

	agentsPath := filepath.Join(evoclawDir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, agentsTemplate, 0644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}
	fmt.Printf("   ✅ AGENTS.md created at %s\\n", agentsPath)

	return nil
}

// copyEmbeddedDir recursively copies an embedded directory to the filesystem
func copyEmbeddedDir(fsys embed.FS, src, dest string) error {
	entries, err := fsys.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			// Recurse
			if err := copyEmbeddedDir(fsys, srcPath, destPath); err != nil {
				return err
			}
		} else {
			// Copy file
			data, err := fsys.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}
