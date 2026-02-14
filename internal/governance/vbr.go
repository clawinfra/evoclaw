package governance

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// VBRCheckType represents the type of verification check.
type VBRCheckType string

const (
	CheckFileExists  VBRCheckType = "file_exists"
	CheckFileChanged VBRCheckType = "file_changed"
	CheckCommand     VBRCheckType = "command"
	CheckGitPushed   VBRCheckType = "git_pushed"
)

// VBRResult represents the result of a verification check.
type VBRResult struct {
	TaskID    string       `json:"task_id"`
	CheckType VBRCheckType `json:"check_type"`
	Target    string       `json:"target"`
	Passed    bool         `json:"passed"`
	Notes     string       `json:"notes"`
	Timestamp time.Time    `json:"timestamp"`
}

// VBRStats represents verification statistics for an agent.
type VBRStats struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	PassRate float64 `json:"pass_rate"`
}

// VBR implements Verify Before Reporting protocol.
type VBR struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
	cache   map[string]time.Time // file modification time cache
}

// NewVBR creates a new VBR instance.
func NewVBR(baseDir string, logger *slog.Logger) (*VBR, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create VBR directory: %w", err)
	}
	return &VBR{
		baseDir: baseDir,
		logger:  logger.With("component", "vbr"),
		cache:   make(map[string]time.Time),
	}, nil
}

func (v *VBR) logPath(agentID string) string {
	return filepath.Join(v.baseDir, agentID+"_vbr.jsonl")
}

// Check performs a verification check and returns the result.
func (v *VBR) Check(taskID string, checkType VBRCheckType, target string) (bool, error) {
	var passed bool
	var notes string

	switch checkType {
	case CheckFileExists:
		_, err := os.Stat(target)
		passed = err == nil
		if !passed {
			notes = fmt.Sprintf("file not found: %s", target)
		}

	case CheckFileChanged:
		info, err := os.Stat(target)
		if err != nil {
			passed = false
			notes = fmt.Sprintf("file not found: %s", target)
		} else {
			v.mu.Lock()
			oldTime, exists := v.cache[target]
			v.cache[target] = info.ModTime()
			v.mu.Unlock()

			if !exists {
				passed = true
				notes = "first check, file exists"
			} else {
				passed = info.ModTime().After(oldTime)
				if !passed {
					notes = "file not modified since last check"
				}
			}
		}

	case CheckCommand:
		cmd := exec.Command("sh", "-c", target)
		output, err := cmd.CombinedOutput()
		passed = err == nil
		if !passed {
			notes = fmt.Sprintf("command failed: %s", string(output))
		}

	case CheckGitPushed:
		// Check if local HEAD matches remote HEAD
		cmd := exec.Command("sh", "-c", fmt.Sprintf("cd %s && git rev-parse HEAD", target))
		localHead, err := cmd.Output()
		if err != nil {
			passed = false
			notes = "failed to get local HEAD"
			break
		}

		cmd = exec.Command("sh", "-c", fmt.Sprintf("cd %s && git rev-parse @{u}", target))
		remoteHead, err := cmd.Output()
		if err != nil {
			passed = false
			notes = "failed to get remote HEAD (no upstream?)"
			break
		}

		passed = string(localHead) == string(remoteHead)
		if !passed {
			notes = "local HEAD differs from remote"
		}

	default:
		return false, fmt.Errorf("unknown check type: %s", checkType)
	}

	v.logger.Debug("VBR check", "task", taskID, "type", checkType, "target", target, "passed", passed)
	return passed, nil
}

// Log records a verification result.
func (v *VBR) Log(agentID, taskID string, passed bool, notes string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	result := VBRResult{
		TaskID:    taskID,
		Passed:    passed,
		Notes:     notes,
		Timestamp: time.Now(),
	}

	f, err := os.OpenFile(v.logPath(agentID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open VBR log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write result: %w", err)
	}

	v.logger.Debug("VBR logged", "agent", agentID, "task", taskID, "passed", passed)
	return nil
}

// Stats returns verification statistics for an agent.
func (v *VBR) Stats(agentID string) (*VBRStats, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	f, err := os.Open(v.logPath(agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return &VBRStats{}, nil
		}
		return nil, fmt.Errorf("open VBR log: %w", err)
	}
	defer f.Close()

	stats := &VBRStats{}
	decoder := json.NewDecoder(f)
	for {
		var result VBRResult
		if err := decoder.Decode(&result); err != nil {
			break
		}
		stats.Total++
		if result.Passed {
			stats.Passed++
		} else {
			stats.Failed++
		}
	}

	if stats.Total > 0 {
		stats.PassRate = float64(stats.Passed) / float64(stats.Total)
	}

	return stats, nil
}
