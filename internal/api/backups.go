package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/store"
)

type backupRow struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Server) backupDir(a *acct) string {
	return filepath.Join(s.cfg.DataDir, "backups", a.SysUser)
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, kind, size_bytes, status, created_at FROM backups WHERE account_id = ? ORDER BY created_at DESC`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []backupRow{}
	for rows.Next() {
		var b backupRow
		if err := rows.Scan(&b.ID, &b.Kind, &b.SizeBytes, &b.Status, &b.CreatedAt); err != nil {
			return err
		}
		out = append(out, b)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Kind string `json:"kind"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Kind == "" {
		req.Kind = "full"
	}
	if req.Kind != "full" && req.Kind != "files" && req.Kind != "databases" {
		return errBadRequest("kind must be full, files or databases")
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	b := backupRow{ID: ids.New(), Kind: req.Kind, Status: "running", CreatedAt: time.Now().Unix()}
	file := filepath.Join(s.backupDir(a), fmt.Sprintf("%s-%s-%d.tar.gz", a.SysUser, req.Kind, b.CreatedAt))
	if _, err := s.db.Exec(r.Context(), `INSERT INTO backups (id, account_id, kind, path, status, created_at) VALUES (?,?,?,?,'running',?)`,
		b.ID, a.ID, req.Kind, file, b.CreatedAt); err != nil {
		return err
	}
	// Run detached: backups outlive the HTTP request.
	go s.runBackup(*a, b.ID, req.Kind, file)
	s.audit(r, "backup.create", a.SysUser, true, req.Kind)
	writeJSON(w, 202, b)
	return nil
}

func (s *Server) runBackup(a acct, id, kind, file string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	err := s.buildBackup(ctx, a, kind, file)
	status, size := "complete", int64(0)
	if err != nil {
		s.log.Error("backup failed", "account", a.SysUser, "err", err)
		status = "failed"
	} else if st, e := os.Stat(file); e == nil {
		size = st.Size()
	}
	if _, e := s.db.Exec(ctx, `UPDATE backups SET status = ?, size_bytes = ? WHERE id = ?`, status, size, id); e != nil {
		s.log.Error("backup status update failed", "err", e)
	}
}

func (s *Server) buildBackup(ctx context.Context, a acct, kind, file string) error {
	run := s.prov.Runner()
	stage := filepath.Join(s.cfg.DataDir, "staging", a.SysUser)
	if _, err := run.Run(ctx, "rm", "-rf", "--", stage); err != nil {
		return err
	}
	defer run.Run(context.WithoutCancel(ctx), "rm", "-rf", "--", stage) //nolint:errcheck
	if _, err := run.Run(ctx, "install", "-d", "-m", "0700", stage, filepath.Dir(file)); err != nil {
		return err
	}
	if kind == "full" || kind == "databases" {
		rows, err := s.db.Query(ctx, `SELECT name FROM db_instances WHERE account_id = ?`, a.ID)
		if err != nil {
			return err
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			names = append(names, n)
		}
		rows.Close()
		for _, n := range names {
			if err := run.RunToFile(ctx, filepath.Join(stage, n+".sql"), "mariadb-dump", "--single-transaction", "--routines", "--events", "--", n); err != nil {
				return err
			}
		}
	}
	args := []string{"-czf", file, "-C", stage, "."}
	if kind == "full" || kind == "files" {
		args = []string{"-czf", file, "--exclude=./backups", "--exclude=./tmp", "-C", a.HomeDir, ".", "-C", stage, "."}
	}
	_, err := run.Run(ctx, "tar", args...)
	if err == nil {
		_, err = run.Run(ctx, "chmod", "0600", file)
	}
	return err
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var file, status string
	if err := s.db.QueryRow(r.Context(), `SELECT path, status FROM backups WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&file, &status); err != nil {
		return store.IsNoRows(err)
	}
	if status != "complete" {
		return errConflict("backup is not complete")
	}
	// Extract as the account user so a hostile archive cannot write outside
	// what the account itself could modify.
	if err := s.prov.Runner().RunWithFileInput(r.Context(), file, "runuser", "-u", a.SysUser, "--", "tar", "-xzf", "-", "-C", a.HomeDir,
		"--no-same-owner", "--no-overwrite-dir", "--exclude=*.sql"); err != nil {
		return err
	}
	s.audit(r, "backup.restore", a.SysUser, true, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var file string
	if err := s.db.QueryRow(r.Context(), `SELECT path FROM backups WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&file); err != nil {
		return store.IsNoRows(err)
	}
	if err := s.prov.Runner().Remove(file); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM backups WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "backup.delete", a.SysUser, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
