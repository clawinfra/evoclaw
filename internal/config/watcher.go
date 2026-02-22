package config

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

// Watcher monitors a config file for changes using polling.
// It checks the file's modification time at a configurable interval.
type Watcher struct {
	path     string
	interval time.Duration
	logger   *slog.Logger
	onChange func()
	stop     chan struct{}
	once     sync.Once
	lastMod  time.Time
}

// NewWatcher creates a config file watcher that polls for changes.
func NewWatcher(path string, interval time.Duration, logger *slog.Logger, onChange func()) *Watcher {
	return &Watcher{
		path:     path,
		interval: interval,
		logger:   logger,
		onChange: onChange,
		stop:     make(chan struct{}),
	}
}

// Start begins polling for file changes in a goroutine.
func (w *Watcher) Start() {
	// Record initial mod time
	if info, err := os.Stat(w.path); err == nil {
		w.lastMod = info.ModTime()
	}

	go w.poll()
	w.logger.Info("config watcher started", "path", w.path, "interval", w.interval)
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	w.once.Do(func() {
		close(w.stop)
		w.logger.Info("config watcher stopped")
	})
}

func (w *Watcher) poll() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *Watcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		w.logger.Warn("config watcher: cannot stat file", "path", w.path, "error", err)
		return
	}

	modTime := info.ModTime()
	if modTime.After(w.lastMod) {
		w.logger.Info("config file changed", "path", w.path, "modTime", modTime)
		w.lastMod = modTime
		if w.onChange != nil {
			w.onChange()
		}
	}
}
