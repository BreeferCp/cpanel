// Command breeferd is the BreeferCP daemon: control plane, API and web UI.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/breefercp/breefercp/internal/api"
	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/config"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
	"github.com/breefercp/breefercp/internal/update"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "breeferd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	confPath := fs.String("config", "/etc/breefercp/breefercp.conf", "path to the configuration file")
	username := fs.String("username", "admin", "admin username (create-admin)")
	email := fs.String("email", "", "admin email (create-admin)")
	password := fs.String("password", "", "admin password (create-admin); generated if empty")
	checkOnly := fs.Bool("check", false, "only report whether an update is available (update)")
	rollback := fs.Bool("rollback", false, "restore the version replaced by the last update (update)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*confPath)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch cmd {
	case "serve":
		return serve(cfg, log)
	case "create-admin":
		return createAdmin(cfg, *username, *email, *password)
	case "version":
		fmt.Println(version)
		return nil
	case "update":
		return runUpdate(cfg, *checkOnly, *rollback)
	default:
		return fmt.Errorf("unknown command %q (use serve, create-admin, update or version)", cmd)
	}
}

func serve(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureSecret(&cfg); err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	var mdb *sql.DB
	if cfg.MariaDBDSN != "" {
		if mdb, err = sql.Open("mysql", cfg.MariaDBDSN); err != nil {
			return fmt.Errorf("mariadb: %w", err)
		}
		defer mdb.Close()
	} else if !cfg.DryRun {
		log.Warn("mariadb_dsn is not set: database management is disabled")
	}

	runner := &system.Runner{DryRun: cfg.DryRun, Log: log}
	prov := provision.New(cfg, runner, log)
	srv := api.New(cfg, db, prov, mdb, log, version)

	// WHM and cPanel are separate listeners (2087 and 2083 by default).
	// With cpanel_listen empty, one listener serves both.
	type listener struct {
		addr  string
		panel api.Panel
	}
	listeners := []listener{{cfg.Listen, api.PanelWHM}, {cfg.CpanelListen, api.PanelCPanel}}
	if cfg.CpanelListen == "" || cfg.CpanelListen == cfg.Listen {
		listeners = []listener{{cfg.Listen, api.PanelBoth}}
	}
	go srv.RunJobs(ctx)

	errc := make(chan error, len(listeners))
	servers := make([]*http.Server, 0, len(listeners))
	for _, l := range listeners {
		hs := &http.Server{
			Addr:              l.addr,
			Handler:           srv.Handler(l.panel),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
			MaxHeaderBytes:    1 << 16,
		}
		servers = append(servers, hs)
		go func() {
			log.Info("breeferd listening", "panel", l.panel, "addr", l.addr, "tls", cfg.TLSCert != "", "db", cfg.DBDriver, "dry_run", cfg.DryRun, "version", version)
			if cfg.TLSCert != "" {
				errc <- hs.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
			} else {
				errc <- hs.ListenAndServe()
			}
		}()
	}
	var runErr error
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, hs := range servers {
		_ = hs.Shutdown(shutCtx)
	}
	return runErr
}

// ensureSecret loads the signing key, creating a random one on first start.
func ensureSecret(cfg *config.Config) error {
	if cfg.SecretKey != "" {
		return nil
	}
	p := filepath.Join(cfg.DataDir, "secret.key")
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		cfg.SecretKey = strings.TrimSpace(string(b))
		return nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	key := hex.EncodeToString(raw)
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(key), 0o600); err != nil {
		return err
	}
	cfg.SecretKey = key
	return nil
}

// createAdmin provisions the first root user, printing a generated password
// once when none is supplied.
func createAdmin(cfg config.Config, username, email, password string) error {
	ctx := context.Background()
	if email == "" {
		return errors.New("-email is required")
	}
	if err := system.SysUser(username); err != nil && username != "admin" {
		return err
	}
	db, err := store.Open(ctx, cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	generated := false
	if password == "" {
		if password, err = auth.RandomPassword(24); err != nil {
			return err
		}
		generated = true
	}
	if err := auth.CheckPasswordPolicy(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := db.Exec(ctx, `INSERT INTO users (id, username, email, password_hash, role, created_at, updated_at) VALUES (?,?,?,?,'root',?,?)`,
		ids.New(), strings.ToLower(username), email, hash, now, now); err != nil {
		if store.IsUnique(err) {
			return fmt.Errorf("user %q already exists", username)
		}
		return err
	}
	fmt.Printf("Created root administrator %q\n", username)
	if generated {
		fmt.Printf("Password: %s\n(shown once; change it after first sign-in)\n", password)
	}
	return nil
}

// runUpdate is the command-line form of WHM > Updates.
func runUpdate(cfg config.Config, checkOnly, rollback bool) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	paths := update.Paths{Binary: bin, WebDir: cfg.WebDir, WorkDir: filepath.Join(cfg.DataDir, "updates")}
	if rollback {
		if err := update.Rollback(paths); err != nil {
			return err
		}
		fmt.Println("Rolled back. Restart the panel: systemctl restart breeferd")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	c := update.New("")
	rel, err := c.Latest(ctx)
	if err != nil {
		return err
	}
	if !update.Newer(rel.Version, version) {
		fmt.Printf("BreeferCP %s is up to date (latest release: %s)\n", version, rel.Version)
		return nil
	}
	fmt.Printf("Update available: %s -> %s\n", version, rel.Version)
	if checkOnly {
		return nil
	}
	if err := c.Apply(ctx, rel, paths); err != nil {
		return err
	}
	fmt.Println("Installed. Restart the panel: systemctl restart breeferd")
	return nil
}
