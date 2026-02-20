package orchestrator

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
)

// FileOps defines pluggable file system operations for tool backends.
// Implement this to target local fs, SSH remotes, HTTP endpoints, etc.
type FileOps interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
	ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error)
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	Remove(ctx context.Context, path string) error
}

// ExecOps defines pluggable command execution operations.
type ExecOps interface {
	// Run executes a command and returns stdout, stderr, exit code.
	Run(ctx context.Context, cmd string, args []string, env []string, workdir string) (stdout string, stderr string, exitCode int, err error)
}

// ---------------------------------------------------------------------------
// LocalFileOps — local filesystem implementation
// ---------------------------------------------------------------------------

// LocalFileOps implements FileOps for the local filesystem.
type LocalFileOps struct{}

func (l *LocalFileOps) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (l *LocalFileOps) WriteFile(_ context.Context, path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (l *LocalFileOps) Stat(_ context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (l *LocalFileOps) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (l *LocalFileOps) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (l *LocalFileOps) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

// ---------------------------------------------------------------------------
// LocalExecOps — local subprocess execution
// ---------------------------------------------------------------------------

// LocalExecOps implements ExecOps for local subprocess execution.
type LocalExecOps struct{}

func (l *LocalExecOps) Run(ctx context.Context, cmd string, args []string, env []string, workdir string) (stdout string, stderr string, exitCode int, err error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if workdir != "" {
		c.Dir = workdir
	}
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	c.Stdout = &stdoutBuf
	c.Stderr = &stderrBuf

	runErr := c.Run()
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			err = runErr
			return
		}
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

// ---------------------------------------------------------------------------
// SSHFileOps — stub for future SSH implementation
// ---------------------------------------------------------------------------

// SSHFileOps implements FileOps over SSH (stub for future use).
// For now, it embeds LocalFileOps and falls back to local operations.
// TODO: Replace with real SSH client when go.mod has golang.org/x/crypto.
// The SSH implementation should use sftp for file ops and ssh.Session for exec.
type SSHFileOps struct {
	Host    string
	User    string
	KeyPath string
	// session *ssh.Session  -- future: holds the SSH session
	LocalFileOps // Temporary: falls back to local until SSH client is added
}

// ---------------------------------------------------------------------------
// ToolBackend — bundles FileOps + ExecOps for a target environment
// ---------------------------------------------------------------------------

// ToolBackend bundles FileOps + ExecOps for a target environment.
type ToolBackend struct {
	File FileOps
	Exec ExecOps
	// Name for logging/debugging (e.g., "local", "gpu-server", "pi-edge")
	Name string
}

// LocalBackend returns a ToolBackend targeting the local machine.
func LocalBackend() *ToolBackend {
	return &ToolBackend{
		File: &LocalFileOps{},
		Exec: &LocalExecOps{},
		Name: "local",
	}
}

// RemoteBackend returns a ToolBackend stub for a named remote.
// SSH implementation to be completed when go.mod has golang.org/x/crypto.
func RemoteBackend(name, host, user, keyPath string) *ToolBackend {
	return &ToolBackend{
		File: &SSHFileOps{Host: host, User: user, KeyPath: keyPath},
		Exec: &LocalExecOps{}, // TODO: replace with SSHExecOps
		Name: name,
	}
}
