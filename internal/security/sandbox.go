package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validatePath checks that a path is within allowed boundaries.
func validatePath(path, workspacePath string, forbiddenPaths, allowedRoots []string, workspaceOnly bool) error {
	// Block null byte injection
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("path contains null byte: blocked")
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}

	// Try to resolve symlinks; if the path doesn't exist yet, resolve the parent
	resolved, err := resolveSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	// Check forbidden paths (always enforced)
	for _, forbidden := range forbiddenPaths {
		expandedForbidden := expandHome(forbidden)
		absForbidden, err2 := filepath.Abs(expandedForbidden)
		if err2 != nil {
			continue
		}
		if isSubpath(resolved, absForbidden) {
			return fmt.Errorf("path %q is within forbidden path %q", path, forbidden)
		}
	}

	// If workspace-only mode, check workspace containment
	if workspaceOnly {
		absWorkspace, err2 := filepath.Abs(workspacePath)
		if err2 != nil {
			return fmt.Errorf("cannot resolve workspace path: %w", err2)
		}
		wsResolved, err2 := resolveSymlinks(absWorkspace)
		if err2 != nil {
			wsResolved = absWorkspace
		}

		if isSubpath(resolved, wsResolved) {
			return nil
		}

		// Check allowed roots
		for _, root := range allowedRoots {
			absRoot, err3 := filepath.Abs(root)
			if err3 != nil {
				continue
			}
			rootResolved, err3 := resolveSymlinks(absRoot)
			if err3 != nil {
				rootResolved = absRoot
			}
			if isSubpath(resolved, rootResolved) {
				return nil
			}
		}

		return fmt.Errorf("path %q is outside workspace %q", path, workspacePath)
	}

	return nil
}

// resolveSymlinks resolves symlinks, falling back to resolving the parent for non-existent paths.
func resolveSymlinks(absPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Resolve parent directory
			parent := filepath.Dir(absPath)
			resolvedParent, err2 := filepath.EvalSymlinks(parent)
			if err2 != nil {
				return absPath, nil // best effort
			}
			return filepath.Join(resolvedParent, filepath.Base(absPath)), nil
		}
		return absPath, nil
	}
	return resolved, nil
}

// isSubpath checks if child is equal to or a subdirectory of parent.
func isSubpath(child, parent string) bool {
	if child == parent {
		return true
	}
	prefix := parent + string(filepath.Separator)
	return strings.HasPrefix(child, prefix)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
