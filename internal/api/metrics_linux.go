//go:build linux

package api

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type hostSampler struct {
	cpuIdle, cpuTotal uint64
	rx, tx            uint64
	at                time.Time
}

func newHostSampler() *hostSampler { return &hostSampler{} }

func readCPU() (idle, total uint64) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	f := strings.Fields(line)
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return idle, total
}

func readNet() (rx, tx uint64) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n")[2:] {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx, tx = rx+r, tx+t
	}
	return rx, tx
}

func (h *hostSampler) sample(diskPath string) (Sample, bool) {
	idle, total := readCPU()
	rx, tx := readNet()
	now := time.Now()
	defer func() { h.cpuIdle, h.cpuTotal, h.rx, h.tx, h.at = idle, total, rx, tx, now }()
	if h.at.IsZero() || total <= h.cpuTotal {
		return Sample{}, false // first call only primes the counters
	}
	var s Sample
	dt := total - h.cpuTotal
	s.CPU = round1(100 * float64(dt-(idle-h.cpuIdle)) / float64(dt))
	secs := now.Sub(h.at).Seconds()
	if rx >= h.rx && tx >= h.tx && secs > 0 {
		s.NetRxKBs = round1(float64(rx-h.rx) / 1024 / secs)
		s.NetTxKBs = round1(float64(tx-h.tx) / 1024 / secs)
	}
	if l := hostLoad(); len(l) > 0 {
		s.Load = round2(l[0])
	}
	if m := hostMemory(); m["total_mb"] > 0 {
		s.MemPct = round1(100 * float64(m["used_mb"]) / float64(m["total_mb"]))
	}
	if d := hostDisk(diskPath); d["total_mb"] > 0 {
		s.DiskPct = round1(100 * float64(d["used_mb"]) / float64(d["total_mb"]))
	}
	return s, true
}

func readUint(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n, err == nil
}

// sample reads cgroup v2 counters of user-<uid>.slice.
func (c *cgroupSampler) sample(uid int, now time.Time) (cgroupUsage, bool) {
	dir := fmt.Sprintf("/sys/fs/cgroup/user.slice/user-%d.slice", uid)
	stat, err := os.ReadFile(dir + "/cpu.stat")
	if err != nil {
		return cgroupUsage{}, false
	}
	var usec uint64
	for _, line := range strings.Split(string(stat), "\n") {
		if v, ok := strings.CutPrefix(line, "usage_usec "); ok {
			usec, _ = strconv.ParseUint(v, 10, 64)
		}
	}
	mem, _ := readUint(dir + "/memory.current")
	procs, _ := readUint(dir + "/pids.current")
	u := cgroupUsage{memMB: int(mem / 1048576), procs: int(procs)}
	c.mu.Lock()
	prev, ok := c.last[uid]
	c.last[uid] = cpuMark{usec: usec, at: now}
	c.mu.Unlock()
	if ok && usec >= prev.usec {
		if el := now.Sub(prev.at).Microseconds(); el > 0 {
			u.cpuPct = round1(100 * float64(usec-prev.usec) / float64(el))
		}
	}
	return u, true
}
