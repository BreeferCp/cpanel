//go:build !linux

package api

// Development stubs so the daemon builds and runs (in dry-run mode) on
// workstations that are not Linux.

func hostLoad() []float64 { return []float64{0, 0, 0} }

func hostMemory() map[string]int64 { return map[string]int64{"total_mb": 0, "used_mb": 0} }

func hostDisk(string) map[string]int64 { return map[string]int64{"total_mb": 0, "used_mb": 0} }
