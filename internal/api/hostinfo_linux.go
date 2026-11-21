//go:build linux

package api

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func hostLoad() []float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return []float64{0, 0, 0}
	}
	f := strings.Fields(string(b))
	out := []float64{0, 0, 0}
	for i := 0; i < 3 && i < len(f); i++ {
		out[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return out
}

func hostMemory() map[string]int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return map[string]int64{"total_mb": 0, "used_mb": 0}
	}
	vals := map[string]int64{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(strings.Fields(v)[0], 10, 64)
		vals[k] = n
	}
	total, avail := vals["MemTotal"]/1024, vals["MemAvailable"]/1024
	return map[string]int64{"total_mb": total, "used_mb": total - avail}
}

func hostDisk(path string) map[string]int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return map[string]int64{"total_mb": 0, "used_mb": 0}
	}
	total := int64(st.Blocks) * int64(st.Bsize) / 1048576
	free := int64(st.Bavail) * int64(st.Bsize) / 1048576
	return map[string]int64{"total_mb": total, "used_mb": total - free}
}
