package provision

import (
	"context"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// MailboxUsage is the stored size of one mailbox.
type MailboxUsage struct {
	Bytes    int64
	Messages int
}

// MailUsage measures every mailbox of an account, keyed by address. find
// runs as the account, so a symlink cannot make it measure anything else.
func (p *Provisioner) MailUsage(ctx context.Context, a Account) (map[string]MailboxUsage, error) {
	dir := path.Join(filepath.ToSlash(a.Home), "mail")
	res, err := p.asUser(ctx, a, nil, "find", dir, "-type", "f", "-printf", `%s %P\n`)
	if err != nil {
		return nil, err
	}
	return parseMailUsage(res.Stdout), nil
}

func parseMailUsage(out string) map[string]MailboxUsage {
	m := map[string]MailboxUsage{}
	for _, line := range strings.Split(out, "\n") {
		size, rel, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			continue
		}
		// <domain>/<local part>/[.Folder/](cur|new|tmp)/<file>
		parts := strings.Split(rel, "/")
		if len(parts) < 3 {
			continue
		}
		addr := parts[1] + "@" + parts[0]
		u := m[addr]
		u.Bytes += n
		if d := parts[len(parts)-2]; len(parts) >= 4 && (d == "cur" || d == "new") {
			u.Messages++
		}
		m[addr] = u
	}
	return m
}

// TableStatus is one table's result from mariadb-check.
type TableStatus struct {
	Table  string `json:"table"`
	Status string `json:"status"`
	OK     bool   `json:"ok"`
}

// CheckDatabase checks, or checks and repairs, every table of one database.
func (p *Provisioner) CheckDatabase(ctx context.Context, name string, repair bool) ([]TableStatus, error) {
	if err := system.DBName(name); err != nil {
		return nil, err
	}
	args := []string{"--check"}
	if repair {
		args = append(args, "--auto-repair")
	}
	res, err := p.run.Run(ctx, "mariadb-check", append(args, "--databases", name)...)
	if err != nil {
		return nil, err
	}
	return parseTableStatus(res.Stdout, name), nil
}

var reTableLine = regexp.MustCompile(`^(\S+)\s+(.+?)\s*$`)

// parseTableStatus reads mariadb-check output. A clean table is "db.t   OK".
// Otherwise the table is on a line of its own and the messages follow it.
// After a repair the table appears again, and the last result wins.
func parseTableStatus(out, db string) []TableStatus {
	list := []TableStatus{}
	at := map[string]int{}
	cur := -1
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, db+"."); ok {
			table, status := rest, ""
			if m := reTableLine.FindStringSubmatch(rest); m != nil {
				table, status = m[1], m[2]
			}
			i, seen := at[table]
			if !seen {
				i = len(list)
				at[table] = i
				list = append(list, TableStatus{Table: table})
			}
			list[i].Status, list[i].OK = status, status == "OK"
			cur = i
			continue
		}
		if cur < 0 || line == "Repairing tables" {
			continue
		}
		// "note" and "status: OK" are fine, anything else needs attention.
		t := &list[cur]
		if t.Status == "" {
			t.Status = line
		} else {
			t.Status += "; " + line
		}
		kind, msg, _ := strings.Cut(line, ":")
		if strings.TrimSpace(kind) == "status" {
			t.OK = strings.TrimSpace(msg) == "OK"
		}
	}
	return list
}

var reMailTo = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}@[a-z0-9.-]{3,253}$`)

// CronMailTo is stricter than ValidAddress: the value becomes a line in a
// root-owned crontab, and cron passes it to sendmail.
func CronMailTo(s string) bool {
	_, dom, _ := strings.Cut(s, "@")
	return reMailTo.MatchString(s) && system.Domain(dom) == nil
}
