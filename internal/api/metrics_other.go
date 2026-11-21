//go:build !linux

package api

import "time"

type hostSampler struct{}

func newHostSampler() *hostSampler { return &hostSampler{} }

func (h *hostSampler) sample(string) (Sample, bool) { return Sample{}, false }

func (c *cgroupSampler) sample(int, time.Time) (cgroupUsage, bool) { return cgroupUsage{}, false }
