package config

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a configuration file and notifies typed handlers
// when the file changes. Config is loaded fresh on each change to ensure
// handlers never receive stale data.
type Watcher[T any] struct {
	path     string
	debounce time.Duration
	loader   func(path string) (T, error)
	handlers []func(T)
	onError  func(error)
	mu       sync.RWMutex
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
}

// WatcherOption configures a Watcher.
type WatcherOption[T any] func(*Watcher[T])

// WithDebounce sets the debounce duration for config changes.
// Default is 1500ms.
func WithDebounce[T any](d time.Duration) WatcherOption[T] {
	return func(w *Watcher[T]) {
		w.debounce = d
	}
}

// WithErrorHandler sets a callback for config load errors.
// If not set, errors are only logged.
func WithErrorHandler[T any](handler func(error)) WatcherOption[T] {
	return func(w *Watcher[T]) {
		w.onError = handler
	}
}

// NewConfigWatcher creates a new typed configuration file watcher.
// The loader function is called fresh on every file change to ensure
// handlers always receive up-to-date config data.
func NewConfigWatcher[T any](
	path string,
	loader func(path string) (T, error),
	logger *slog.Logger,
	opts ...WatcherOption[T],
) *Watcher[T] {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher[T]{
		path:     path,
		debounce: 1500 * time.Millisecond,
		loader:   loader,
		handlers: make([]func(T), 0),
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// OnReload registers a handler to be called when config changes.
// Returns an unsubscribe function to remove the handler.
func (w *Watcher[T]) OnReload(handler func(T)) func() {
	w.mu.Lock()
	w.handlers = append(w.handlers, handler)
	idx := len(w.handlers) - 1
	w.mu.Unlock()

	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if idx < len(w.handlers) {
			w.handlers[idx] = nil
		}
	}
}

// Start begins watching the configuration file for changes.
func (w *Watcher[T]) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	if addErr := watcher.Add(w.path); addErr != nil {
		watcher.Close()
		return addErr
	}

	w.logger.Info("Config watcher started", "path", w.path, "debounce", w.debounce)
	go w.watch()
	return nil
}

// Stop stops watching and cleans up resources.
func (w *Watcher[T]) Stop() error {
	w.cancel()
	if w.watcher != nil {
		return w.watcher.Close()
	}
	return nil
}

// watch is the main loop that listens for file changes.
func (w *Watcher[T]) watch() {
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case <-w.ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			w.logger.Debug("Config watcher stopped")
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Handle write events (most common for config changes)
			// Also handle create events (some editors replace the file)
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.logger.Debug("Config file change detected", "op", event.Op.String())

				// Reset debounce timer
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(w.debounce)
				timerC = timer.C
			}

		case <-timerC:
			w.logger.Info("Config file changed, loading and notifying handlers")
			w.loadAndNotify()
			timerC = nil

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn("Config watcher error", "error", err)
		}
	}
}

// loadAndNotify loads config fresh and notifies all handlers.
func (w *Watcher[T]) loadAndNotify() {
	// Load config FRESH - no caching
	config, err := w.loader(w.path)
	if err != nil {
		w.logger.Warn("Failed to load config", "error", err)
		if w.onError != nil {
			w.onError(err)
		}
		return
	}

	// All handlers receive the SAME fresh config snapshot
	w.mu.RLock()
	handlers := make([]func(T), 0, len(w.handlers))
	for _, h := range w.handlers {
		if h != nil {
			handlers = append(handlers, h)
		}
	}
	w.mu.RUnlock()

	for _, handler := range handlers {
		handler(config)
	}
}
