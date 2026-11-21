package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/system"
)

// PowerDNS is a minimal client for the PowerDNS authoritative HTTP API.
type PowerDNS struct {
	base   string
	key    string
	dryRun bool
	hc     *http.Client
}

// NewPowerDNS creates a client. base looks like http://127.0.0.1:8081.
func NewPowerDNS(base, key string, dryRun bool) *PowerDNS {
	return &PowerDNS{base: strings.TrimRight(base, "/"), key: key, dryRun: dryRun,
		hc: &http.Client{Timeout: 15 * time.Second}}
}

// Record is one DNS record as the panel presents it.
type Record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      int    `json:"ttl"`
	Content  string `json:"content"`
	Priority int    `json:"priority,omitempty"` // MX / SRV
}

var allowedTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "MX": true, "TXT": true, "NS": true,
	"SRV": true, "CAA": true, "PTR": true,
}

func fqdn(s string) string { return strings.TrimSuffix(strings.ToLower(s), ".") + "." }

type rrset struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	TTL        int        `json:"ttl"`
	ChangeType string     `json:"changetype"`
	Records    []rrRecord `json:"records"`
}
type rrRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

func (d *PowerDNS) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if d.dryRun {
		return []byte("{}"), nil
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+"/api/v1/servers/localhost"+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", d.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("powerdns %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// CreateZone creates a master zone with the default record set.
func (d *PowerDNS) CreateZone(ctx context.Context, zone, serverIP string, nameservers []string) error {
	if err := system.Domain(zone); err != nil {
		return err
	}
	if len(nameservers) == 0 {
		nameservers = []string{"ns1." + zone, "ns2." + zone}
	}
	ns := make([]string, len(nameservers))
	for i, n := range nameservers {
		ns[i] = fqdn(n)
	}
	body := map[string]any{"name": fqdn(zone), "kind": "Native", "nameservers": ns}
	if _, err := d.do(ctx, http.MethodPost, "/zones", body); err != nil {
		return err
	}
	defaults := []Record{
		{Name: zone, Type: "A", TTL: 3600, Content: serverIP},
		{Name: "www." + zone, Type: "CNAME", TTL: 3600, Content: fqdn(zone)},
		{Name: "mail." + zone, Type: "A", TTL: 3600, Content: serverIP},
		{Name: zone, Type: "MX", TTL: 3600, Content: "10 " + fqdn("mail."+zone)},
		{Name: zone, Type: "TXT", TTL: 3600, Content: `"v=spf1 mx a ~all"`},
	}
	for _, r := range defaults {
		if err := d.UpsertRecord(ctx, zone, r); err != nil {
			return err
		}
	}
	return nil
}

// DeleteZone removes a zone.
func (d *PowerDNS) DeleteZone(ctx context.Context, zone string) error {
	if err := system.Domain(zone); err != nil {
		return err
	}
	_, err := d.do(ctx, http.MethodDelete, "/zones/"+url.PathEscape(fqdn(zone)), nil)
	return err
}

// UpsertRecord replaces the RRset for name/type with a single record.
func (d *PowerDNS) UpsertRecord(ctx context.Context, zone string, r Record) error {
	return d.patch(ctx, zone, r, "REPLACE")
}

// DeleteRecord removes the RRset for name/type.
func (d *PowerDNS) DeleteRecord(ctx context.Context, zone string, r Record) error {
	return d.patch(ctx, zone, r, "DELETE")
}

func (d *PowerDNS) patch(ctx context.Context, zone string, r Record, change string) error {
	if err := system.Domain(zone); err != nil {
		return err
	}
	r.Type = strings.ToUpper(r.Type)
	if !allowedTypes[r.Type] {
		return fmt.Errorf("%w: record type %s", system.ErrInvalid, r.Type)
	}
	name := strings.ToLower(r.Name)
	if name == "@" || name == "" {
		name = zone
	}
	if !strings.HasSuffix(name, zone) {
		name = name + "." + zone
	}
	if r.TTL < 60 || r.TTL > 86400 {
		r.TTL = 3600
	}
	if strings.ContainsAny(r.Content, "\r\n") {
		return fmt.Errorf("%w: record content", system.ErrInvalid)
	}
	content := r.Content
	if r.Type == "MX" && r.Priority > 0 {
		content = fmt.Sprintf("%d %s", r.Priority, fqdn(content))
	}
	if r.Type == "CNAME" || r.Type == "NS" || r.Type == "PTR" {
		content = fqdn(content)
	}
	set := rrset{Name: fqdn(name), Type: r.Type, TTL: r.TTL, ChangeType: change}
	if change == "REPLACE" {
		set.Records = []rrRecord{{Content: content}}
	}
	_, err := d.do(ctx, http.MethodPatch, "/zones/"+url.PathEscape(fqdn(zone)), map[string]any{"rrsets": []rrset{set}})
	return err
}

// ListRecords returns every record in a zone.
func (d *PowerDNS) ListRecords(ctx context.Context, zone string) ([]Record, error) {
	if err := system.Domain(zone); err != nil {
		return nil, err
	}
	raw, err := d.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(fqdn(zone)), nil)
	if err != nil {
		return nil, err
	}
	var z struct {
		RRsets []struct {
			Name    string     `json:"name"`
			Type    string     `json:"type"`
			TTL     int        `json:"ttl"`
			Records []rrRecord `json:"records"`
		} `json:"rrsets"`
	}
	if err := json.Unmarshal(raw, &z); err != nil {
		return nil, err
	}
	var out []Record
	for _, s := range z.RRsets {
		for _, rec := range s.Records {
			out = append(out, Record{Name: strings.TrimSuffix(s.Name, "."), Type: s.Type, TTL: s.TTL, Content: rec.Content})
		}
	}
	return out, nil
}
