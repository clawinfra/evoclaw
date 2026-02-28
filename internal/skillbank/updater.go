package skillbank

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	emaAlpha        = 0.1 // exponential moving average factor for success_rate
	archiveFileName = "archived_skills.jsonl"
)

// SkillUpdater implements recursive skill evolution: it distills new skills from
// uncovered failure trajectories, prunes stale skills, and tracks confidence.
type SkillUpdater struct {
	distiller  Distiller
	store      Store
	archiveDir string // directory for archived_skills.jsonl; defaults to current dir
}

// NewSkillUpdater creates a new SkillUpdater.
// archiveDir controls where archived_skills.jsonl is written; pass "" for the current directory.
func NewSkillUpdater(distiller Distiller, store Store, archiveDir string) *SkillUpdater {
	if archiveDir == "" {
		archiveDir = "."
	}
	return &SkillUpdater{
		distiller:  distiller,
		store:      store,
		archiveDir: archiveDir,
	}
}

// Update finds failure trajectories not covered by existing skills, distills new skills
// from them, and persists them to the store. Returns the newly added skills.
func (u *SkillUpdater) Update(ctx context.Context, failures []Trajectory, currentSkills []Skill) ([]Skill, error) {
	uncovered := filterUncovered(failures, currentSkills)
	if len(uncovered) == 0 {
		return nil, nil
	}

	newSkills, newMistakes, err := u.distiller.Distill(ctx, uncovered)
	if err != nil {
		return nil, fmt.Errorf("distill uncovered failures: %w", err)
	}

	var added []Skill
	for _, s := range newSkills {
		if err := u.store.Add(s); err != nil {
			if err == ErrDuplicateID {
				continue // already exists, skip
			}
			return added, fmt.Errorf("store new skill %q: %w", s.ID, err)
		}
		added = append(added, s)
	}

	for _, m := range newMistakes {
		if err := u.store.AddMistake(m); err != nil && err != ErrDuplicateID {
			return added, fmt.Errorf("store new mistake %q: %w", m.ID, err)
		}
	}

	return added, nil
}

// filterUncovered returns trajectories that are NOT covered by any existing skill.
// A trajectory is "covered" if its task type matches at least one skill's task type
// or if a general skill exists (empty task type).
func filterUncovered(failures []Trajectory, skills []Skill) []Trajectory {
	// Build a set of covered task types.
	coveredTypes := make(map[string]struct{})
	hasGeneral := false
	for _, s := range skills {
		if s.TaskType == "" {
			hasGeneral = true
		} else {
			coveredTypes[s.TaskType] = struct{}{}
		}
	}

	if hasGeneral {
		// General skills cover everything.
		return nil
	}

	var uncovered []Trajectory
	for _, f := range failures {
		if _, ok := coveredTypes[f.TaskType]; !ok {
			uncovered = append(uncovered, f)
		}
	}
	return uncovered
}

// PruneStaleSkills archives skills whose success_rate is below minSuccessRate
// AND whose usage_count is at least minUsage. Archived skills are appended to
// archived_skills.jsonl in archiveDir, then deleted from the store.
// Returns the number of pruned skills.
func (u *SkillUpdater) PruneStaleSkills(ctx context.Context, minSuccessRate float64, minUsage int) (int, error) {
	skills, err := u.store.List("")
	if err != nil {
		return 0, fmt.Errorf("list skills: %w", err)
	}

	var toArchive []Skill
	for _, s := range skills {
		if s.UsageCount >= minUsage && s.SuccessRate < minSuccessRate {
			toArchive = append(toArchive, s)
		}
	}

	if len(toArchive) == 0 {
		return 0, nil
	}

	archivePath := filepath.Join(u.archiveDir, archiveFileName)
	if err := appendJSONL(archivePath, toArchive); err != nil {
		return 0, fmt.Errorf("archive skills: %w", err)
	}

	pruned := 0
	for _, s := range toArchive {
		if err := u.store.Delete(s.ID); err != nil {
			return pruned, fmt.Errorf("delete stale skill %q: %w", s.ID, err)
		}
		pruned++
	}

	return pruned, nil
}

// BoostSkillConfidence updates a skill's SuccessRate using an exponential moving average
// with alpha=0.1. A success nudges the rate up; a failure nudges it down.
func (u *SkillUpdater) BoostSkillConfidence(skillID string, succeeded bool) error {
	s, err := u.store.Get(skillID)
	if err != nil {
		return fmt.Errorf("get skill %q: %w", skillID, err)
	}

	var outcome float64
	if succeeded {
		outcome = 1.0
	}
	s.SuccessRate = emaAlpha*outcome + (1-emaAlpha)*s.SuccessRate
	s.UsageCount++
	s.UpdatedAt = time.Now()

	return u.store.Update(s)
}

// appendJSONL appends skills as JSONL lines to path, creating the file if needed.
func appendJSONL(path string, skills []Skill) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, s := range skills {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	return w.Flush()
}

// isCoveredBySkills is a helper used in testing and for transparency.
func isCoveredBySkills(taskType string, skills []Skill) bool {
	for _, s := range skills {
		if s.TaskType == "" || s.TaskType == taskType {
			return true
		}
	}
	return false
}

// SkillSummary returns a brief human-readable summary of a skill for logging.
func SkillSummary(s Skill) string {
	return fmt.Sprintf("[%s] %s (confidence=%.2f, usage=%d, successRate=%.2f, source=%s)",
		s.Title, strings.TrimSpace(s.Principle), s.Confidence, s.UsageCount, s.SuccessRate, s.Source)
}
