//go:build !linux

package api

import "errors"

// Non-Linux builds (development only) show sample data.
func listProcesses() []procInfo {
	return []procInfo{
		{PID: 812, User: "mysql", State: "S", CPUSeconds: 5421.2, MemMB: 412.6, Command: "/usr/sbin/mariadbd"},
		{PID: 1430, User: "acme", State: "S", CPUSeconds: 812.4, MemMB: 96.1, Command: "php-fpm: pool breefer-acme"},
		{PID: 1544, User: "acme", State: "S", CPUSeconds: 1290.0, MemMB: 88.4, Command: "node /home/acme/apps/api/server.js"},
		{PID: 1102, User: "www-data", State: "S", CPUSeconds: 640.8, MemMB: 42.0, Command: "nginx: worker process"},
		{PID: 951, User: "pdns", State: "S", CPUSeconds: 210.3, MemMB: 31.7, Command: "/usr/sbin/pdns_server --guardian=no"},
		{PID: 1012, User: "dovecot", State: "S", CPUSeconds: 75.1, MemMB: 12.2, Command: "dovecot/imap-login"},
		{PID: 1660, User: "northwind", State: "S", CPUSeconds: 22.9, MemMB: 61.3, Command: "php-fpm: pool breefer-northwind"},
		{PID: 1, User: "root", State: "S", CPUSeconds: 31.0, MemMB: 11.8, Command: "/sbin/init"},
	}
}

func killProcess(int, string) error { return errors.New("process control needs Linux") }

func serverInfo() map[string]any {
	return map[string]any{"os": "Debian GNU/Linux 12 (bookworm)", "kernel": "6.1.0-28-amd64",
		"cpu": "AMD EPYC 7543P 32-Core Processor", "cores": 4, "uptime_s": int64(1_209_600), "cgroups": "v2", "demo": true}
}
