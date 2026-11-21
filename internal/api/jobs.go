package api

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// RunJobs starts the periodic maintenance loops and blocks until ctx ends.
func (s *Server) RunJobs(ctx context.Context) {
	renew := time.NewTicker(12 * time.Hour)
	usage := time.NewTicker(time.Hour)
	housekeeping := time.NewTicker(15 * time.Minute)
	defer renew.Stop()
	defer usage.Stop()
	defer housekeeping.Stop()
	// Run once shortly after start so a fresh install is not idle for hours.
	time.AfterFunc(30*time.Second, func() { s.collectUsage(ctx) })
	go s.collectMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-renew.C:
			s.renewCerts(ctx)
		case <-usage.C:
			s.collectUsage(ctx)
		case <-housekeeping.C:
			s.purgeExpired(ctx)
			s.purgePMAUsers(ctx)
		}
	}
}

// purgeExpired removes dead sessions so the table cannot grow without bound.
func (s *Server) purgeExpired(ctx context.Context) {
	if _, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix()); err != nil {
		s.log.Error("purge sessions", "err", err)
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM audit_log WHERE at < ?`, time.Now().AddDate(-1, 0, 0).Unix()); err != nil {
		s.log.Error("purge audit", "err", err)
	}
}

// renewCerts re-issues Let's Encrypt certificates expiring within 30 days.
func (s *Server) renewCerts(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT d.id, d.account_id FROM ssl_certs c JOIN domains d ON d.id = c.domain_id
		WHERE c.issuer = 'letsencrypt' AND c.auto_renew = 1 AND c.not_after < ?`, time.Now().AddDate(0, 0, 30).Unix())
	if err != nil {
		s.log.Error("renew: query", "err", err)
		return
	}
	type job struct{ domainID, accountID string }
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.domainID, &j.accountID); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, j.accountID))
		if err != nil || a.Status != "active" {
			continue
		}
		d, err := s.domainOf(ctx, a.ID, j.domainID)
		if err != nil {
			continue
		}
		names := []string{d.Name}
		if d.Kind == "primary" || d.Kind == "addon" {
			names = append(names, "www."+d.Name)
		}
		cert, err := s.prov.IssueLetsEncrypt(ctx, a.Email, names, false)
		if err != nil {
			s.log.Error("renew: issue", "domain", d.Name, "err", err)
			continue
		}
		if err := s.saveCert(ctx, a, d, "letsencrypt", names, cert); err != nil {
			s.log.Error("renew: save", "domain", d.Name, "err", err)
			continue
		}
		s.log.Info("certificate renewed", "domain", d.Name)
	}
}

// collectUsage measures disk usage per account and enforces nothing by
// itself; the numbers feed the dashboard and billing usage reports.
func (s *Server) collectUsage(ctx context.Context) {
	if s.cfg.DryRun {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id, home_dir FROM accounts WHERE status = 'active'`)
	if err != nil {
		return
	}
	type item struct{ id, home string }
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.home) == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	for _, it := range items {
		res, err := s.prov.Runner().Run(ctx, "du", "-sm", "--", it.home)
		if err != nil {
			continue
		}
		mb, err := strconv.Atoi(strings.Fields(res.Stdout + " ")[0])
		if err != nil {
			continue
		}
		_, _ = s.db.Exec(ctx, `UPDATE accounts SET disk_used_mb = ? WHERE id = ?`, mb, it.id)
	}
}
