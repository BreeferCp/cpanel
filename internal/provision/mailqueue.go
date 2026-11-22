package provision

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// QueuedMessage is one message in the Postfix queue (postqueue -j).
type QueuedMessage struct {
	ID         string   `json:"id"`
	Queue      string   `json:"queue"` // active, deferred, hold, incoming
	Arrival    int64    `json:"arrival"`
	Size       int64    `json:"size"`
	Sender     string   `json:"sender"`
	Recipients []string `json:"recipients"`
	Reason     string   `json:"reason"`
}

var reQueueID = regexp.MustCompile(`^[0-9A-Za-z]{6,24}$`)

// MailQueue lists the Postfix queue.
func (p *Provisioner) MailQueue(ctx context.Context) ([]QueuedMessage, error) {
	if p.cfg.DryRun {
		return demoQueue(), nil
	}
	res, err := p.run.Run(ctx, "postqueue", "-j")
	if err != nil {
		return nil, err
	}
	return parsePostqueue(res.Stdout)
}

func parsePostqueue(out string) ([]QueuedMessage, error) {
	msgs := []QueuedMessage{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var raw struct {
			QueueName   string `json:"queue_name"`
			QueueID     string `json:"queue_id"`
			ArrivalTime int64  `json:"arrival_time"`
			MessageSize int64  `json:"message_size"`
			Sender      string `json:"sender"`
			Recipients  []struct {
				Address     string `json:"address"`
				DelayReason string `json:"delay_reason"`
			} `json:"recipients"`
		}
		if json.Unmarshal(sc.Bytes(), &raw) != nil {
			continue
		}
		m := QueuedMessage{ID: raw.QueueID, Queue: raw.QueueName, Arrival: raw.ArrivalTime, Size: raw.MessageSize, Sender: raw.Sender}
		for _, r := range raw.Recipients {
			m.Recipients = append(m.Recipients, r.Address)
			if m.Reason == "" {
				m.Reason = r.DelayReason
			}
		}
		msgs = append(msgs, m)
	}
	return msgs, sc.Err()
}

// MailQueueAction flushes the queue or deletes, holds or releases messages.
// id "ALL" applies to every message (delete only).
func (p *Provisioner) MailQueueAction(ctx context.Context, action, id string) error {
	if action == "flush" {
		_, err := p.run.Run(ctx, "postqueue", "-f")
		return err
	}
	if id != "ALL" && !reQueueID.MatchString(id) {
		return fmt.Errorf("%w: queue id", system.ErrInvalid)
	}
	flags := map[string]string{"delete": "-d", "hold": "-h", "release": "-H", "requeue": "-r"}
	flag, ok := flags[action]
	if !ok {
		return fmt.Errorf("%w: unknown queue action", system.ErrInvalid)
	}
	if id == "ALL" && action != "delete" && action != "requeue" {
		return fmt.Errorf("%w: only delete and requeue apply to the whole queue", system.ErrInvalid)
	}
	_, err := p.run.Run(ctx, "postsuper", flag, id)
	return err
}

func demoQueue() []QueuedMessage {
	return []QueuedMessage{
		{ID: "4Xk2Lm0QzRd", Queue: "deferred", Arrival: 1763700000, Size: 18233, Sender: "wordpress@acme-shop.com",
			Recipients: []string{"orders@gmial.com"}, Reason: "Host or domain name not found. Name service error for name=gmial.com type=MX"},
		{ID: "4Xk2Pq7TzWb", Queue: "deferred", Arrival: 1763703600, Size: 4521, Sender: "info@northwind.dev",
			Recipients: []string{"team@example.org"}, Reason: "connect to mx.example.org[192.0.2.25]:25: Connection timed out"},
		{ID: "4Xk2Vb1HzNc", Queue: "active", Arrival: 1763707200, Size: 2210, Sender: "noreply@globex.io",
			Recipients: []string{"customer@example.net"}},
	}
}

// ---- remote MySQL ----------------------------------------------------------

var reMySQLHost = regexp.MustCompile(`^([0-9]{1,3}\.){0,3}%$|^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$|^[0-9a-fA-F:]{2,39}$`)

// RemoteHost validates a MariaDB host pattern: an IP, a hostname, a
// trailing-% IPv4 wildcard such as 203.0.113.% or just %.
func RemoteHost(s string) error {
	if s == "localhost" || s == "127.0.0.1" || s == "::1" || !reMySQLHost.MatchString(s) {
		return fmt.Errorf("%w: enter an IP address, a host name or a pattern such as 203.0.113.%%", system.ErrInvalid)
	}
	return nil
}

// SyncRemoteUser makes the database user name@host exist for exactly the
// given hosts (besides localhost), with the localhost password and ALL
// privileges on dbs.
func (m *MariaDB) SyncRemoteUser(ctx context.Context, name string, hosts, dbs []string) error {
	if err := system.DBName(name); err != nil {
		return err
	}
	for _, h := range hosts {
		if err := RemoteHost(h); err != nil {
			return err
		}
	}
	if m.dryRun || m.db == nil {
		return nil
	}
	var hash string
	if err := m.db.QueryRowContext(ctx, "SELECT authentication_string FROM mysql.user WHERE User = ? AND Host = 'localhost'", name).Scan(&hash); err != nil {
		return err
	}
	rows, err := m.db.QueryContext(ctx, "SELECT Host FROM mysql.user WHERE User = ? AND Host <> 'localhost'", name)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return err
		}
		have[h] = true
	}
	rows.Close()
	want := map[string]bool{}
	for _, h := range hosts {
		want[h] = true
	}
	for h := range have {
		if !want[h] {
			if err := m.exec(ctx, "DROP USER IF EXISTS ?@?", name, h); err != nil {
				return err
			}
		}
	}
	for h := range want {
		if !have[h] {
			if err := m.exec(ctx, "CREATE USER ?@? IDENTIFIED BY PASSWORD ?", name, h, hash); err != nil {
				return err
			}
		}
		for _, d := range dbs {
			id, err := grantTarget(d)
			if err != nil {
				return err
			}
			if err := m.exec(ctx, "GRANT ALL PRIVILEGES ON "+id+".* TO ?@?", name, h); err != nil {
				return err
			}
		}
	}
	return m.exec(ctx, "FLUSH PRIVILEGES")
}
