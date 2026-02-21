package tarsdapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

type gatewayAgentsWatcherOptions struct {
	WorkspaceDir string
	Debounce     time.Duration
	Logger       zerolog.Logger
	Refresh      func(reason string)
}

type gatewayAgentsWatcher struct {
	opts gatewayAgentsWatcherOptions

	mu      sync.Mutex
	cancel  context.CancelFunc
	watcher *fsnotify.Watcher
	started bool
	wg      sync.WaitGroup
}

func newGatewayAgentsWatcher(opts gatewayAgentsWatcherOptions) *gatewayAgentsWatcher {
	return &gatewayAgentsWatcher{opts: opts}
}

func (w *gatewayAgentsWatcher) Start(ctx context.Context) (bool, error) {
	if w == nil {
		return false, nil
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return true, nil
	}
	w.mu.Unlock()

	workspace := strings.TrimSpace(w.opts.WorkspaceDir)
	if workspace == "" {
		return false, nil
	}
	agentsDir := filepath.Join(workspace, "agents")
	info, err := os.Stat(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat gateway agents dir %q: %w", agentsDir, err)
	}
	if !info.IsDir() {
		return false, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return false, fmt.Errorf("create gateway agents watcher: %w", err)
	}
	if err := addGatewayAgentsWatchRecursive(watcher, agentsDir); err != nil {
		_ = watcher.Close()
		return false, err
	}

	watchCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.watcher = watcher
	w.started = true
	w.mu.Unlock()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.loop(watchCtx, watcher)
	}()
	return true, nil
}

func (w *gatewayAgentsWatcher) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	cancel := w.cancel
	watcher := w.watcher
	w.cancel = nil
	w.watcher = nil
	w.started = false
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if watcher != nil {
		_ = watcher.Close()
	}
	w.wg.Wait()
}

func (w *gatewayAgentsWatcher) loop(ctx context.Context, watcher *fsnotify.Watcher) {
	debounce := w.opts.Debounce
	if debounce <= 0 {
		debounce = 200 * time.Millisecond
	}

	var timer *time.Timer
	var timerCh <-chan time.Time
	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
			timerCh = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
		timerCh = timer.C
	}
	stopTimer := func() {
		if timer != nil {
			timer.Stop()
		}
	}
	defer stopTimer()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					if addErr := addGatewayAgentsWatchRecursive(watcher, event.Name); addErr != nil {
						w.opts.Logger.Warn().Err(addErr).Str("path", event.Name).Msg("gateway agents watcher add dir failed")
					}
				}
			}
			if !shouldRefreshGatewayAgents(event.Name, event.Op) {
				continue
			}
			resetTimer()
		case <-timerCh:
			timerCh = nil
			if w.opts.Refresh != nil {
				w.opts.Refresh("watch")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			w.opts.Logger.Warn().Err(err).Msg("gateway agents watcher error")
		}
	}
}

func shouldRefreshGatewayAgents(path string, op fsnotify.Op) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
		return false
	}
	return strings.EqualFold(filepath.Base(path), "AGENT.md")
}

func addGatewayAgentsWatchRecursive(w *fsnotify.Watcher, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat gateway watch dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := w.Add(root); err != nil {
		return fmt.Errorf("watch gateway dir %q: %w", root, err)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() || path == root {
			return nil
		}
		_ = w.Add(path)
		return nil
	})
}
