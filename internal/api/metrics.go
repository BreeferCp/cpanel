package api

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/breefercp/breefercp/internal/rbac"
)

// Sample is one host measurement.
type Sample struct {
	At       int64   `json:"t"`
	CPU      float64 `json:"cpu"`  // percent of all cores
	MemPct   float64 `json:"mem"`  // percent of RAM used
	Load     float64 `json:"load"` // 1-minute load average
	DiskPct  float64 `json:"disk"`
	NetRxKBs float64 `json:"rx"` // KB/s received
	NetTxKBs float64 `json:"tx"` // KB/s sent
}

const (
	sampleEvery    = 30 * time.Second
	ringSize       = 24 * 60 * 2 // 24h at 30s
	accountSampleE = 5 * time.Minute
	accountKeep    = 7 * 24 * time.Hour
)

type metricsRing struct {
	mu      sync.RWMutex
	samples []Sample
}

func (m *metricsRing) add(s Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, s)
	if len(m.samples) > ringSize {
		m.samples = m.samples[len(m.samples)-ringSize:]
	}
}

func (m *metricsRing) since(t int64, maxPoints int) []Sample {
	m.mu.RLock()
	defer m.mu.RUnlock()
	i := sort.Search(len(m.samples), func(i int) bool { return m.samples[i].At >= t })
	src := m.samples[i:]
	return downsample(src, maxPoints)
}

// downsample averages consecutive samples so charts stay light.
func downsample(src []Sample, max int) []Sample {
	if len(src) <= max || max <= 0 {
		out := make([]Sample, len(src))
		copy(out, src)
		return out
	}
	step := float64(len(src)) / float64(max)
	out := make([]Sample, 0, max)
	for i := 0; i < max; i++ {
		lo, hi := int(float64(i)*step), int(float64(i+1)*step)
		if hi > len(src) {
			hi = len(src)
		}
		var a Sample
		for _, s := range src[lo:hi] {
			a.CPU += s.CPU
			a.MemPct += s.MemPct
			a.Load += s.Load
			a.DiskPct += s.DiskPct
			a.NetRxKBs += s.NetRxKBs
			a.NetTxKBs += s.NetTxKBs
		}
		n := float64(hi - lo)
		a.At = src[hi-1].At
		a.CPU, a.MemPct, a.Load, a.DiskPct = round1(a.CPU/n), round1(a.MemPct/n), round2(a.Load/n), round1(a.DiskPct/n)
		a.NetRxKBs, a.NetTxKBs = round1(a.NetRxKBs/n), round1(a.NetTxKBs/n)
		out = append(out, a)
	}
	return out
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
func round2(f float64) float64 { return math.Round(f*100) / 100 }

// collectMetrics runs until ctx ends.
func (s *Server) collectMetrics(ctx context.Context) {
	if s.cfg.DryRun {
		s.seedDemoMetrics()
	}
	host := newHostSampler()
	tick := time.NewTicker(sampleEvery)
	acctTick := time.NewTicker(accountSampleE)
	defer tick.Stop()
	defer acctTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			if s.cfg.DryRun {
				s.metrics.add(demoSample(now))
				continue
			}
			if smp, ok := host.sample(s.cfg.HomeBase); ok {
				smp.At = now.Unix()
				s.metrics.add(smp)
			}
		case now := <-acctTick.C:
			s.sampleAccounts(ctx, now)
		}
	}
}

func (s *Server) sampleAccounts(ctx context.Context, now time.Time) {
	rows, err := s.db.Query(ctx, `SELECT id, sys_uid FROM accounts WHERE status = 'active'`)
	if err != nil {
		return
	}
	type item struct {
		id  string
		uid int
	}
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.uid) == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	for i, it := range items {
		var u cgroupUsage
		var ok bool
		if s.cfg.DryRun {
			u, ok = demoAccountUsage(now, i), true
		} else {
			u, ok = s.cgroups.sample(it.uid, now)
		}
		if !ok {
			continue
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO account_metrics (account_id, at, cpu_pct, mem_mb, procs) VALUES (?,?,?,?,?)`,
			it.id, now.Unix(), u.cpuPct, u.memMB, u.procs)
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM account_metrics WHERE at < ?`, now.Add(-accountKeep).Unix())
}

// ---- HTTP ---------------------------------------------------------------------

func (s *Server) metricsRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/server/metrics", rbac.ServerRead, s.serverMetrics)
	s.route(mux, "GET /api/v1/server/top-accounts", rbac.AccountsRead, s.topAccounts)
	s.route(mux, "GET /api/v1/accounts/{aid}/metrics", rbac.HostingRead, s.accountMetrics)
}

func rangeOf(r *http.Request) time.Duration {
	switch r.URL.Query().Get("range") {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (s *Server) serverMetrics(w http.ResponseWriter, r *http.Request) error {
	d := rangeOf(r)
	if d > 24*time.Hour {
		d = 24 * time.Hour
	}
	writeJSON(w, 200, map[string]any{"samples": s.metrics.since(time.Now().Add(-d).Unix(), 240), "demo": s.cfg.DryRun})
	return nil
}

type acctPoint struct {
	At    int64   `json:"t"`
	CPU   float64 `json:"cpu"`
	MemMB int     `json:"mem_mb"`
	Procs int     `json:"procs"`
}

func (s *Server) accountMetrics(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	since := time.Now().Add(-rangeOf(r)).Unix()
	rows, err := s.db.Query(r.Context(), `SELECT at, cpu_pct, mem_mb, procs FROM account_metrics WHERE account_id = ? AND at >= ? ORDER BY at`, a.ID, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []acctPoint{}
	for rows.Next() {
		var p acctPoint
		if err := rows.Scan(&p.At, &p.CPU, &p.MemMB, &p.Procs); err != nil {
			return err
		}
		p.CPU = round1(p.CPU)
		out = append(out, p)
	}
	writeJSON(w, 200, map[string]any{"samples": out, "limits": map[string]int{"cpu_pct": a.pkg.CPUPercent, "mem_mb": a.pkg.MemoryMB, "procs": a.pkg.MaxProcs}, "demo": s.cfg.DryRun})
	return rows.Err()
}

// topAccounts ranks the caller's visible accounts by recent CPU and memory.
func (s *Server) topAccounts(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	since := time.Now().Add(-time.Hour).Unix()
	q := `SELECT a.id, a.primary_domain, a.sys_user, AVG(m.cpu_pct), MAX(m.mem_mb), a.disk_used_mb, k.disk_mb
		FROM account_metrics m JOIN accounts a ON a.id = m.account_id JOIN users u ON u.id = a.user_id
		LEFT JOIN users o ON o.id = u.owner_id JOIN packages k ON k.id = a.package_id WHERE m.at >= ?`
	args := []any{since}
	if p.Role == rbac.Reseller {
		q += ` AND (u.owner_id = ? OR o.owner_id = ?)`
		args = append(args, p.UserID, p.UserID)
	}
	rows, err := s.db.Query(r.Context(), q+` GROUP BY a.id, a.primary_domain, a.sys_user, a.disk_used_mb, k.disk_mb ORDER BY AVG(m.cpu_pct) DESC LIMIT 10`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type top struct {
		ID          string  `json:"id"`
		Domain      string  `json:"domain"`
		User        string  `json:"user"`
		CPU         float64 `json:"cpu"`
		MemMB       int     `json:"mem_mb"`
		DiskUsedMB  int     `json:"disk_used_mb"`
		DiskLimitMB int     `json:"disk_limit_mb"`
	}
	out := []top{}
	for rows.Next() {
		var t top
		var mem float64
		if err := rows.Scan(&t.ID, &t.Domain, &t.User, &t.CPU, &mem, &t.DiskUsedMB, &t.DiskLimitMB); err != nil {
			return err
		}
		t.CPU, t.MemMB = round1(t.CPU), int(mem)
		out = append(out, t)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// ---- demo data (dry-run only) ---------------------------------------------------
//
// Dry-run installs have no real host to measure, so they show a smooth
// synthetic series. The API marks it with "demo": true and the UI labels it.

func demoSample(t time.Time) Sample {
	x := float64(t.Unix()) / 3600
	wave := (math.Sin(x*2*math.Pi/24) + 1) / 2       // daily cycle
	jit := 0.6*math.Sin(x*3.1) + 0.4*math.Sin(x*7.3) // gentle, not noise
	return Sample{
		At: t.Unix(), CPU: round1(12 + 30*wave + 6*jit), MemPct: round1(38 + 10*wave + 2*jit),
		Load: round2(0.6 + 1.8*wave + 0.3*jit), DiskPct: round1(29 + 0.0001*x),
		NetRxKBs: round1(220 + 900*wave + 120*jit), NetTxKBs: round1(480 + 1600*wave + 200*jit),
	}
}

func (s *Server) seedDemoMetrics() {
	now := time.Now()
	for t := now.Add(-24 * time.Hour); t.Before(now); t = t.Add(sampleEvery) {
		s.metrics.add(demoSample(t))
	}
	ctx := context.Background()
	var n int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM account_metrics`).Scan(&n)
	if n > 0 {
		return
	}
	for t := now.Add(-24 * time.Hour); t.Before(now); t = t.Add(accountSampleE) {
		s.sampleAccounts(ctx, t)
	}
}

func demoAccountUsage(t time.Time, i int) cgroupUsage {
	x := float64(t.Unix())/3600 + float64(i)*3
	wave := (math.Sin(x*2*math.Pi/24) + 1) / 2
	return cgroupUsage{cpuPct: round1(18 + 60*wave/float64(i+1) + 4*math.Sin(x*2.7)), memMB: int(380 + 900*wave/float64(i+1)), procs: 6 + int(10*wave)}
}

// cgroupUsage is one account's resource use from its systemd slice.
type cgroupUsage struct {
	cpuPct float64
	memMB  int
	procs  int
}

// cgroupSampler keeps the previous CPU counter per uid to compute rates.
type cgroupSampler struct {
	mu   sync.Mutex
	last map[int]cpuMark
}

type cpuMark struct {
	usec uint64
	at   time.Time
}

func newCgroupSampler() *cgroupSampler { return &cgroupSampler{last: map[int]cpuMark{}} }

var _ = strconv.Itoa
