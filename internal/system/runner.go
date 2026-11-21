// Package system executes privileged host commands safely. Commands are
// never passed through a shell; arguments are always discrete strings.
package system

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner runs external programs. In dry-run mode it only records them.
type Runner struct {
	DryRun bool
	Log    *slog.Logger

	mu      sync.Mutex
	History []string // dry-run transcript, used by tests
}

// Result is the captured outcome of a command.
type Result struct {
	Stdout string
	Stderr string
}

// Run executes name with args, bounded by a 2 minute default timeout.
func (r *Runner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	return r.RunInput(ctx, nil, name, args...)
}

// RunInput is Run with data piped to stdin (used for secrets, so they never
// appear in the process table).
func (r *Runner) RunInput(ctx context.Context, stdin []byte, name string, args ...string) (Result, error) {
	line := name + " " + strings.Join(args, " ")
	if r.DryRun {
		r.mu.Lock()
		r.History = append(r.History, line)
		r.mu.Unlock()
		if r.Log != nil {
			r.Log.Debug("dry-run exec", "cmd", line)
		}
		return Result{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	err := cmd.Run()
	res := Result{Stdout: so.String(), Stderr: se.String()}
	if err != nil {
		return res, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(res.Stderr))
	}
	return res, nil
}

// RunToFile runs a command and streams its stdout into path (mode 0600),
// avoiding buffering large dumps in memory.
func (r *Runner) RunToFile(ctx context.Context, path string, name string, args ...string) error {
	if r.DryRun {
		r.mu.Lock()
		r.History = append(r.History, name+" "+strings.Join(args, " ")+" > "+path)
		r.mu.Unlock()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	var se bytes.Buffer
	cmd.Stdout, cmd.Stderr = f, &se
	if err := cmd.Run(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(se.String()))
	}
	return nil
}

// WriteFile atomically writes data to path (temp file + rename), creating
// parent directories. In dry-run mode the write is recorded, not performed.
func (r *Runner) WriteFile(path string, data []byte, perm os.FileMode) error {
	if r.DryRun {
		r.mu.Lock()
		r.History = append(r.History, fmt.Sprintf("write %s (%d bytes)", path, len(data)))
		r.mu.Unlock()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".breefer-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Remove deletes a file, ignoring "does not exist".
func (r *Runner) Remove(path string) error {
	if r.DryRun {
		r.mu.Lock()
		r.History = append(r.History, "rm "+path)
		r.mu.Unlock()
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RunWithFileInput runs a command with the contents of inPath streamed to its
// stdin. Restores use it so the extracting process (running as the customer)
// never needs read access to the root-owned backup archive.
func (r *Runner) RunWithFileInput(ctx context.Context, inPath string, name string, args ...string) error {
	if r.DryRun {
		r.mu.Lock()
		r.History = append(r.History, name+" "+strings.Join(args, " ")+" < "+inPath)
		r.mu.Unlock()
		return nil
	}
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	var se bytes.Buffer
	cmd.Stdin, cmd.Stderr = in, &se
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(se.String()))
	}
	return nil
}
