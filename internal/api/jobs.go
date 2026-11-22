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
	go s.upgradeVhosts(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-renew.C:
			s.scheduledAutoSSL(ctx)
		case <-usage.C:
			s.collectUsage(ctx)
		case <-housekeeping.C:
			s.purgeExpired(ctx)
			s.purgePMAUsers(ctx)
			s.runScheduledBackups(ctx)
			s.autoUpdateTick(ctx)
			s.wpAutoUpdateTick(ctx)
			s.quotaNotices(ctx)
		}
	}
}

func (s *Server) scheduledAutoSSL(ctx context.Context) {
	if s.cfg.DryRun {
		s.renewCerts(ctx)
		return
	}
	jobs, err := s.domainsWithoutCert(ctx)
	if err != nil {
		s.log.Error("autossl: query", "err", err)
	}
	s.startAutoSSL(jobs) // renews even when the list is empty
}

// vhostLayout is bumped when a fix only takes effect once vhosts are rewritten.
const vhostLayout = "2"

// upgradeVhosts rewrites every active account's vhosts once per layout.
func (s *Server) upgradeVhosts(ctx context.Context) {
	if s.cfg.DryRun || s.setting(ctx, "vhost_layout") == vhostLayout {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM accounts WHERE status = 'active'`)
	if err != nil {
		s.log.Error("vhost upgrade", "err", err)
		return
	}
	var list []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			list = append(list, id)
		}
	}
	rows.Close()
	ok := rows.Err() == nil
	for _, id := range list {
		a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, id))
		if err == nil {
			err = s.rewriteVhosts(ctx, a)
		}
		if err == nil {
			if e := s.prov.PrepareDocRoot(ctx, a.prov(), a.HomeDir+"/public_html"); e != nil {
				s.log.Warn("document root permissions", "account", id, "err", e)
			}
		}
		if err != nil {
			ok = false
			s.log.Error("vhost upgrade", "account", id, "err", err)
		}
	}
	if ok {
		if err := s.putSetting(ctx, "vhost_layout", vhostLayout); err != nil {
			s.log.Error("vhost upgrade", "err", err)
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
