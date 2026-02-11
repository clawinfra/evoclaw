package skills

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Executor runs skill tools as subprocesses.
type Executor struct {
	logger *slog.Logger
}

// NewExecutor creates a new tool executor.
func NewExecutor(logger *slog.Logger) *Executor {
	return &Executor{logger: logger}
}

// Execute runs a tool with the given arguments and returns the result.
func (e *Executor) Execute(ctx context.Context, tool *ToolDef, skill *Skill, args map[string]string) *ToolResult {
	timeout := tool.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command string with arg substitution
	cmdStr := tool.Command
	cmdArgs := make([]string, len(tool.Args))
	for i, arg := range tool.Args {
		if strings.HasPrefix(arg, "$") {
			key := arg[1:]
			if val, ok := args[key]; ok {
				cmdArgs[i] = val
			} else {
				cmdArgs[i] = ""
			}
		} else {
			cmdArgs[i] = arg
		}
	}

	e.logger.Debug("executing tool",
		"command", cmdStr,
		"args", cmdArgs,
		"skill", skill.Manifest.Name,
	)

	cmd := exec.CommandContext(ctx, cmdStr, cmdArgs...)
	cmd.Dir = skill.Dir

	// Build environment
	cmd.Env = os.Environ()
	// Inject tool-level env vars
	for _, envDef := range tool.Env {
		expanded := os.ExpandEnv(envDef)
		cmd.Env = append(cmd.Env, expanded)
	}
	// Inject args as env vars too
	for k, v := range args {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SKILL_ARG_%s=%s", strings.ToUpper(k), v))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &ToolResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		result.Err = err
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}

	return result
}
