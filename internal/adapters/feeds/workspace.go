package feeds

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Workspace owns temporary feed stages for one state directory. The process lock
// prevents overlapping imports and makes abandoned stages safe to reclaim.
type Workspace struct {
	root string
	lock *os.File
}

func OpenWorkspace(root string) (*Workspace, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("feed workspace must be a private owned directory")
	}
	fd, err := syscall.Open(filepath.Join(root, ".lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), "feed-workspace-lock")
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("feed import already running")
	}
	workspace := &Workspace{root: root, lock: lock}
	entries, err := os.ReadDir(root)
	if err == nil {
		for _, entry := range entries {
			// Never follow symlinks or remove unrelated files.
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "infra-feed-") {
				if err = os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
					break
				}
			}
		}
	}
	if err != nil {
		workspace.Close()
		return nil, err
	}
	return workspace, nil
}
func (w *Workspace) NewStage() (*Stage, error) { return NewStage(w.root) }
func (w *Workspace) Close() error              { return w.lock.Close() }
