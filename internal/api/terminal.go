package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
)

// Browser terminal: a WebSocket bridged to a PTY running a login shell as the
// account user, inside the account's cgroup slice. It requires the account's
// shell mode to be "full".
//
// Protocol: binary frames carry terminal bytes in both directions; the client
// sends text frames {"type":"resize","cols":N,"rows":N}.

const (
	terminalMaxSession = 4 * time.Hour
	terminalIdle       = 30 * time.Minute
)

// ptySession abstracts the platform PTY (Linux only; see terminal_linux.go).
type ptySession interface {
	io.ReadWriteCloser
	Resize(cols, rows uint16) error
	Wait() error
}

func (s *Server) terminalRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/accounts/{aid}/terminal", rbac.HostingWrite, s.terminal)
}

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if a.Status != "active" {
		return errConflict("account is suspended")
	}
	if a.Shell != provision.ShellFull {
		return &apiError{403, "shell_disabled", "shell access is not enabled for this account"}
	}
	if s.cfg.DryRun {
		return &apiError{501, "dry_run", "the terminal is unavailable in dry-run mode"}
	}
	// websocket.Accept rejects cross-origin requests by default, which (with
	// the SameSite=Strict session cookie) prevents cross-site WebSocket hijacking.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil // Accept already wrote the HTTP error
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)

	sess, err := startPTY(a.SysUser, a.SysUID, a.HomeDir)
	if err != nil {
		s.log.Error("terminal start", "user", a.SysUser, "err", err)
		conn.Close(websocket.StatusInternalError, "could not start shell")
		return nil
	}
	defer sess.Close()
	s.audit(r, "terminal.open", a.SysUser, true, "")

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), terminalMaxSession)
	defer cancel()

	var mu sync.Mutex
	last := time.Now()
	touch := func() { mu.Lock(); last = time.Now(); mu.Unlock() }

	// PTY -> browser.
	go func() {
		defer cancel()
		buf := make([]byte, 16<<10)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// Idle watchdog.
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				mu.Lock()
				idle := time.Since(last)
				mu.Unlock()
				if idle > terminalIdle {
					cancel()
					return
				}
			}
		}
	}()
	// Browser -> PTY.
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		touch()
		if typ == websocket.MessageText {
			var msg struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 && msg.Cols < 1000 && msg.Rows < 500 {
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
			continue
		}
		if _, err := sess.Write(data); err != nil {
			break
		}
	}
	_ = sess.Close()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		conn.Close(websocket.StatusNormalClosure, "session time limit reached")
	} else {
		conn.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}
