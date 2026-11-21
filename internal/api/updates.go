package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/update"
)

// updateState is shared by the check, apply and auto-update paths.
type updateState struct {
	mu       sync.Mutex
	latest   *update.Release
	checked  time.Time
	checkErr string
	running  bool
	lastErr  string
}

var updates updateState

func (s *Server) updateRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/server/updates", rbac.ServerRead, s.getUpdates)
	s.route(mux, "POST /api/v1/server/updates/check", rbac.ServerWrite, s.checkUpdates)
	s.route(mux, "POST /api/v1/server/updates/apply", rbac.ServerWrite, s.applyUpdate)
	s.route(mux, "POST /api/v1/server/updates/rollback", rbac.ServerWrite, s.rollbackUpdate)
	s.route(mux, "PUT /api/v1/server/updates/config", rbac.SettingsWrite, s.putUpdateConfig)
	s.route(mux, "GET /api/v1/server/system-updates", rbac.ServerRead, s.getSystemUpdates)
	s.route(mux, "POST /api/v1/server/system-updates", rbac.ServerWrite, s.runSystemUpdates)
}

func (s *Server) updatePaths() update.Paths {
	bin, err := os.Executable()
	if err != nil {
		bin = "/usr/local/bin/breeferd"
	}
	return update.Paths{Binary: bin, WebDir: s.cfg.WebDir, WorkDir: filepath.Join(s.cfg.DataDir, "updates")}
}

func (s *Server) updateClient() *update.Client {
	return update.New(s.setting(context.Background(), "update_repo"))
}

// doCheck refreshes the cached latest release.
func (s *Server) doCheck(ctx context.Context) {
	rel, err := s.updateClient().Latest(ctx)
	updates.mu.Lock()
	defer updates.mu.Unlock()
	updates.checked = time.Now()
	if err != nil {
		updates.checkErr = err.Error()
		return
	}
	updates.latest, updates.checkErr = rel, ""
}

func (s *Server) updateView(ctx context.Context) map[string]any {
	updates.mu.Lock()
	defer updates.mu.Unlock()
	tier := s.setting(ctx, "update_tier")
	if tier == "" {
		tier = "manual"
	}
	out := map[string]any{
		"current": s.version, "tier": tier, "running": updates.running, "error": updates.lastErr,
		"check_error": updates.checkErr, "available": false,
		"can_rollback": fileExists(s.updatePaths().Binary + ".prev"),
	}
	if !updates.checked.IsZero() {
		out["checked_at"] = updates.checked.Unix()
	}
	if r := updates.latest; r != nil {
		out["latest"] = r
		out["available"] = update.Newer(r.Version, s.version)
	}
	return out
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.updateView(r.Context()))
	return nil
}

func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	s.doCheck(ctx)
	writeJSON(w, 200, s.updateView(r.Context()))
	return nil
}

// install applies the cached release and schedules a restart. It runs in the
// background; the UI polls getUpdates.
func (s *Server) install(reason string) error {
	updates.mu.Lock()
	rel := updates.latest
	if updates.running {
		updates.mu.Unlock()
		return errConflict("an update is already running")
	}
	if rel == nil || !update.Newer(rel.Version, s.version) {
		updates.mu.Unlock()
		return errConflict("no newer version is available; check for updates first")
	}
	updates.running, updates.lastErr = true, ""
	updates.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		var err error
		if s.cfg.DryRun {
			s.log.Info("dry run: would install update", "version", rel.Version)
		} else {
			err = s.updateClient().Apply(ctx, rel, s.updatePaths())
		}
		updates.mu.Lock()
		updates.running = false
		if err != nil {
			updates.lastErr = err.Error()
		}
		updates.mu.Unlock()
		s.auditAs(ctx, "", "system", "", "panel.update", rel.Version, err == nil, reason)
		if err == nil && !s.cfg.DryRun {
			s.restartSelf(ctx)
		}
	}()
	return nil
}

// restartSelf asks systemd to restart the panel a few seconds from now, from
// outside this process, so the current request can finish.
func (s *Server) restartSelf(ctx context.Context) {
	_, err := s.prov.Runner().Run(ctx, "systemd-run", "--on-active=3", "--unit=breefercp-restart", "--collect",
		"systemctl", "restart", "breeferd")
	if err != nil {
		s.log.Error("restart after update", "err", err)
	}
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) error {
	if err := s.install("manual"); err != nil {
		return err
	}
	s.audit(r, "panel.update.start", "", true, "")
	writeJSON(w, 202, s.updateView(r.Context()))
	return nil
}

func (s *Server) rollbackUpdate(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.DryRun {
		return errConflict("rollback is not available in dry-run mode")
	}
	if err := update.Rollback(s.updatePaths()); err != nil {
		return errConflict(err.Error())
	}
	s.audit(r, "panel.rollback", "", true, "")
	s.restartSelf(context.WithoutCancel(r.Context()))
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (s *Server) putUpdateConfig(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Tier string `json:"tier"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Tier != "manual" && req.Tier != "automatic" {
		return errBadRequest("tier must be manual or automatic")
	}
	if err := s.putSetting(r.Context(), "update_tier", req.Tier); err != nil {
		return err
	}
	s.audit(r, "panel.update.config", req.Tier, true, "")
	writeJSON(w, 200, s.updateView(r.Context()))
	return nil
}

// autoUpdateTick checks for releases every 6 hours and, with the automatic
// tier, installs new ones between 03:00 and 05:00 server time.
func (s *Server) autoUpdateTick(ctx context.Context) {
	updates.mu.Lock()
	stale := time.Since(updates.checked) > 6*time.Hour
	updates.mu.Unlock()
	if stale {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		s.doCheck(cctx)
		cancel()
	}
	if s.setting(ctx, "update_tier") != "automatic" {
		return
	}
	if h := time.Now().Hour(); h < 3 || h >= 5 {
		return
	}
	_ = s.install("automatic")
}

// ---- operating system packages (WHM "System Update") ----------------------------------

const aptUnit = "breefercp-apt"

func (s *Server) getSystemUpdates(w http.ResponseWriter, r *http.Request) error {
	out := map[string]any{"running": false, "packages": []map[string]string{}, "log": ""}
	if s.cfg.DryRun {
		out["packages"] = []map[string]string{
			{"name": "openssl", "version": "3.0.15-1~deb12u1"}, {"name": "nginx", "version": "1.22.1-9+deb12u2"},
			{"name": "linux-image-amd64", "version": "6.1.119-1"},
		}
		writeJSON(w, 200, out)
		return nil
	}
	if res, err := s.prov.Runner().Run(r.Context(), "systemctl", "is-active", aptUnit); err == nil {
		out["running"] = strings.TrimSpace(res.Stdout) == "active"
	}
	pkgs := []map[string]string{}
	if res, err := s.prov.Runner().Run(r.Context(), "apt", "list", "--upgradable"); err == nil {
		for _, l := range strings.Split(res.Stdout, "\n") {
			name, rest, ok := strings.Cut(l, "/")
			if !ok || strings.HasPrefix(l, "Listing") {
				continue
			}
			f := strings.Fields(rest)
			v := ""
			if len(f) > 1 {
				v = f[1]
			}
			pkgs = append(pkgs, map[string]string{"name": name, "version": v})
		}
	}
	out["packages"] = pkgs
	if b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "system-update.log")); err == nil {
		if len(b) > 16<<10 {
			b = b[len(b)-16<<10:]
		}
		out["log"] = string(b)
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) runSystemUpdates(w http.ResponseWriter, r *http.Request) error {
	logFile := filepath.Join(s.cfg.DataDir, "system-update.log")
	// A transient systemd unit: it survives a panel restart and its output
	// goes to a log the UI tails. The command line is constant.
	script := `export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get -y -o Dpkg::Options::=--force-confold upgrade`
	if _, err := s.prov.Runner().Run(r.Context(), "systemd-run", "--unit="+aptUnit, "--collect",
		"-p", "StandardOutput=file:"+logFile, "-p", "StandardError=file:"+logFile, "sh", "-c", script); err != nil {
		return errConflict("could not start the update (is one already running?)")
	}
	s.audit(r, "system.update", "", true, "")
	b, _ := json.Marshal(map[string]bool{"started": true})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_, _ = w.Write(b)
	return nil
}
