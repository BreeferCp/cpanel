//go:build linux

package api

import (
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// listProcesses reads /proc. CPU is cumulative seconds; memory is RSS.
func listProcesses() []procInfo {
	entries, _ := os.ReadDir("/proc")
	page := int64(os.Getpagesize())
	hz := 100.0 // USER_HZ on every mainstream Linux build
	names := map[string]string{}
	out := []procInfo{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		// comm may contain spaces and parentheses: split on the last ')'.
		s := string(stat)
		close := strings.LastIndexByte(s, ')')
		if close < 0 {
			continue
		}
		f := strings.Fields(s[close+1:])
		if len(f) < 22 {
			continue
		}
		utime, _ := strconv.ParseFloat(f[11], 64)
		stime, _ := strconv.ParseFloat(f[12], 64)
		rss, _ := strconv.ParseInt(f[21], 10, 64)
		var st syscall.Stat_t
		uid := ""
		if syscall.Stat(dir, &st) == nil {
			uid = strconv.Itoa(int(st.Uid))
		}
		name, ok := names[uid]
		if !ok {
			name = uid
			if u, err := user.LookupId(uid); err == nil {
				name = u.Username
			}
			names[uid] = name
		}
		cmd, _ := os.ReadFile(filepath.Join(dir, "cmdline"))
		c := strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " "))
		if c == "" {
			c = "[" + s[strings.IndexByte(s, '(')+1:close] + "]"
		}
		if len(c) > 300 {
			c = c[:300]
		}
		out = append(out, procInfo{PID: pid, User: name, State: f[0], CPUSeconds: (utime + stime) / hz,
			MemMB: float64(rss*page) / 1048576, Command: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MemMB > out[j].MemMB })
	return out
}

func killProcess(pid int, sig string) error {
	s := syscall.SIGTERM
	if sig == "KILL" {
		s = syscall.SIGKILL
	}
	return syscall.Kill(pid, s)
}

func serverInfo() map[string]any {
	info := map[string]any{}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
				info["os"] = strings.Trim(v, `"`)
			}
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info["kernel"] = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		cores := 0
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "processor") {
				cores++
			}
			if v, ok := strings.CutPrefix(l, "model name"); ok && info["cpu"] == nil {
				info["cpu"] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), ":"))
			}
		}
		info["cores"] = cores
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			up, _ := strconv.ParseFloat(f[0], 64)
			info["uptime_s"] = int64(up)
		}
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		info["cgroups"] = "v2"
	} else {
		info["cgroups"] = "v1 (unsupported)"
	}
	return info
}
