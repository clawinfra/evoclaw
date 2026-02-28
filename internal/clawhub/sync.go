package clawhub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// SyncOptions configures the behaviour of SyncSkills.
type SyncOptions struct {
	// Filter restricts which skills are synced.
	Filter SkillFilter
	// Overwrite controls whether existing skill dirs are overwritten.
	// Default: false (skip already-present skills).
	Overwrite bool
	// Logger receives sync progress messages. Defaults to slog.Default().
	Logger *slog.Logger
}

// SyncSkills pulls the latest skills from ClawHub and writes them to localDir.
// Each skill is written to a subdirectory named after the skill ID:
//
//	localDir/
//	  weather-v2/
//	    SKILL.md
//	    skill-meta.json
//	    <other files from Skill.Files>
//
// Returns the number of skills synced and any error encountered.
func (c *Client) SyncSkills(ctx context.Context, localDir string, opts SyncOptions) (int, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return 0, fmt.Errorf("clawhub sync: create local dir %q: %w", localDir, err)
	}

	skills, err := c.ListSkills(ctx, opts.Filter)
	if err != nil {
		return 0, fmt.Errorf("clawhub sync: list skills: %w", err)
	}

	logger.Info("clawhub sync: found skills", "count", len(skills))

	synced := 0
	for _, meta := range skills {
		skillDir := filepath.Join(localDir, meta.ID)

		if !opts.Overwrite {
			if _, err := os.Stat(skillDir); err == nil {
				logger.Debug("clawhub sync: skipping existing skill", "id", meta.ID)
				continue
			}
		}

		skill, err := c.GetSkill(ctx, meta.ID)
		if err != nil {
			logger.Error("clawhub sync: failed to fetch skill", "id", meta.ID, "err", err)
			continue
		}

		if err := writeSkill(skillDir, skill); err != nil {
			logger.Error("clawhub sync: failed to write skill", "id", meta.ID, "err", err)
			continue
		}

		logger.Info("clawhub sync: synced skill", "id", meta.ID, "version", meta.Version)
		synced++
	}

	return synced, nil
}

// writeSkill writes all skill files to the target directory.
func writeSkill(skillDir string, skill *Skill) error {
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	// Write individual files from skill bundle
	for relPath, content := range skill.Files {
		destPath := filepath.Join(skillDir, filepath.Clean(relPath))
		// Security: ensure the path doesn't escape the skill dir
		if !isSubPath(skillDir, destPath) {
			return fmt.Errorf("skill file path escapes skill dir: %s", relPath)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create parent dir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", relPath, err)
		}
	}

	// If no SKILL.md in files but Readme is set, write it
	if _, ok := skill.Files["SKILL.md"]; !ok && skill.Readme != "" {
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill.Readme), 0o644); err != nil {
			return fmt.Errorf("write SKILL.md: %w", err)
		}
	}

	// Always write a skill-meta.json with the SkillMeta
	metaData, err := json.MarshalIndent(skill.SkillMeta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skill meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill-meta.json"), metaData, 0o644); err != nil {
		return fmt.Errorf("write skill-meta.json: %w", err)
	}

	return nil
}

// isSubPath returns true if target is under base.
func isSubPath(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && len(rel) >= 1 && rel[0] != '.'
}
