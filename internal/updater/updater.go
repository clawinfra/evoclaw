package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

const (
	githubAPI      = "https://api.github.com/repos/clawinfra/evoclaw/releases/latest"
	updateCheckURL = "https://evoclaw.win/version.json"
	timeout        = 30 * time.Second
)

type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type Updater struct {
	cfg           *config.Config
	log           *slog.Logger
	currentVer    string
	autoUpdate    bool
	checkInterval time.Duration
}

func New(cfg *config.Config, log *slog.Logger, currentVer string) *Updater {
	autoUpdate := true
	checkInterval := 24 * time.Hour
	
	if cfg.Updates != nil {
		autoUpdate = cfg.Updates.Enabled
		if cfg.Updates.CheckInterval > 0 {
			checkInterval = time.Duration(cfg.Updates.CheckInterval) * time.Second
		}
	}
	
	return &Updater{
		cfg:           cfg,
		log:           log,
		currentVer:    currentVer,
		autoUpdate:    autoUpdate,
		checkInterval: checkInterval,
	}
}

// CheckForUpdates checks if a new version is available
func (u *Updater) CheckForUpdates(ctx context.Context) (*Release, bool, error) {
	client := &http.Client{Timeout: timeout}
	
	req, err := http.NewRequestWithContext(ctx, "GET", githubAPI, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create request: %w", err)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetch release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}
	
	// Skip drafts and prereleases unless configured
	if release.Draft || (release.Prerelease && !u.cfg.Updates.IncludePrereleases) {
		return nil, false, nil
	}
	
	// Compare versions
	latestVer := strings.TrimPrefix(release.TagName, "v")
	currentVer := strings.TrimPrefix(u.currentVer, "v")
	
	if latestVer == currentVer {
		return &release, false, nil
	}
	
	// Simple version comparison (semantic versioning would be better)
	updateAvailable := latestVer > currentVer
	
	return &release, updateAvailable, nil
}

// DownloadAndInstall downloads the latest release and installs it
func (u *Updater) DownloadAndInstall(ctx context.Context, release *Release) error {
	// Find the appropriate asset for current platform
	assetName := u.getAssetName()
	var targetAsset *Asset
	
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			targetAsset = &asset
			break
		}
	}
	
	if targetAsset == nil {
		return fmt.Errorf("no asset found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	
	u.log.Info("downloading update", "version", release.TagName, "asset", targetAsset.Name, "size", targetAsset.Size)
	
	// Download to temp file
	tmpDir, err := os.MkdirTemp("", "evoclaw-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	
	archivePath := filepath.Join(tmpDir, targetAsset.Name)
	if err := u.downloadFile(ctx, targetAsset.BrowserDownloadURL, archivePath); err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	
	// Extract binary
	binaryPath, err := u.extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}
	
	// Replace current binary
	if err := u.replaceBinary(binaryPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	
	u.log.Info("update installed successfully", "version", release.TagName)
	
	return nil
}

// RunBackgroundChecker starts a background goroutine that checks for updates periodically
func (u *Updater) RunBackgroundChecker(ctx context.Context) {
	if !u.autoUpdate {
		u.log.Info("auto-update disabled")
		return
	}
	
	ticker := time.NewTicker(u.checkInterval)
	defer ticker.Stop()
	
	// Check immediately on startup
	u.checkAndNotify(ctx)
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.checkAndNotify(ctx)
		}
	}
}

func (u *Updater) checkAndNotify(ctx context.Context) {
	release, updateAvailable, err := u.CheckForUpdates(ctx)
	if err != nil {
		u.log.Warn("update check failed", "error", err)
		return
	}
	
	if !updateAvailable {
		return
	}
	
	u.log.Info("update available", "version", release.TagName, "current", u.currentVer)
	
	// If auto-install is enabled, install it
	if u.cfg.Updates != nil && u.cfg.Updates.AutoInstall {
		u.log.Info("auto-installing update")
		if err := u.DownloadAndInstall(ctx, release); err != nil {
			u.log.Error("auto-install failed", "error", err)
			return
		}
		
		u.log.Info("update installed, restart required")
		// Could trigger a graceful restart here
	}
}

func (u *Updater) getAssetName() string {
	osName := runtime.GOOS
	archName := runtime.GOARCH
	
	// Map Go arch names to release asset names
	if archName == "amd64" {
		archName = "amd64"
	} else if archName == "arm64" {
		archName = "arm64"
	} else if strings.HasPrefix(runtime.GOARCH, "arm") {
		archName = "armv7"
	}
	
	if osName == "windows" {
		return fmt.Sprintf("evoclaw-%s-%s.zip", osName, archName)
	}
	
	return fmt.Sprintf("evoclaw-%s-%s.tar.gz", osName, archName)
}

func (u *Updater) downloadFile(ctx context.Context, url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	
	_, err = io.Copy(out, resp.Body)
	return err
}

func (u *Updater) extractBinary(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	
	// Handle .tar.gz
	if strings.HasSuffix(archivePath, ".tar.gz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer func() { _ = gzr.Close() }()
		
		tr := tar.NewReader(gzr)
		
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			
			// Find the binary (starts with evoclaw-)
			if strings.HasPrefix(header.Name, "evoclaw-") || header.Name == "evoclaw" {
				binaryPath := filepath.Join(destDir, "evoclaw")
				
				out, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
				if err != nil {
					return "", err
				}
				defer func() { _ = out.Close() }()
				
				if _, err := io.Copy(out, tr); err != nil {
					return "", err
				}
				
				return binaryPath, nil
			}
		}
	}
	
	// TODO: Handle .zip for Windows
	
	return "", fmt.Errorf("binary not found in archive")
}

func (u *Updater) replaceBinary(newBinaryPath string) error {
	// Get current executable path
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	
	// Resolve symlinks
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	
	// Backup current binary
	backupPath := currentPath + ".backup"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}
	
	// Copy new binary
	if err := u.copyFile(newBinaryPath, currentPath); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, currentPath)
		return fmt.Errorf("copy new binary: %w", err)
	}
	
	// Make executable
	if err := os.Chmod(currentPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	
	// Remove backup
	_ = os.Remove(backupPath)
	
	return nil
}

func (u *Updater) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()
	
	_, err = io.Copy(destFile, sourceFile)
	return err
}
