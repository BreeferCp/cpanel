package provision

import (
	"bytes"
	"context"
	"fmt"

	"github.com/breefercp/breefercp/internal/system"
)

// FirewallRule is a single panel-managed rule.
type FirewallRule struct {
	Action   string // allow | deny
	Protocol string // tcp | udp | any
	Port     string
	Source   string
}

// baseOpenPorts are always reachable so the operator cannot lock themselves
// out of the services the panel itself depends on.
var baseOpenPorts = []struct{ proto, port string }{
	{"tcp", "22"}, {"tcp", "80"}, {"tcp", "443"}, {"tcp", "2087"}, {"tcp", "8443"},
	{"tcp", "25"}, {"tcp", "465"}, {"tcp", "587"}, {"tcp", "993"}, {"tcp", "143"}, {"tcp", "995"},
	{"tcp", "53"}, {"udp", "53"},
}

// RenderNFT builds the complete nftables ruleset. Default policy is drop.
func RenderNFT(rules []FirewallRule, blocked []string) (string, error) {
	var b bytes.Buffer
	b.WriteString("# Managed by BreeferCP. Do not edit.\nflush ruleset\n\ntable inet breefer {\n")
	b.WriteString("  set blocklist { type ipv4_addr; flags interval; }\n")
	b.WriteString("  chain input {\n    type filter hook input priority 0; policy drop;\n")
	b.WriteString("    ct state established,related accept\n    ct state invalid drop\n    iif lo accept\n")
	b.WriteString("    ip protocol icmp icmp type { echo-request, destination-unreachable, time-exceeded } limit rate 10/second accept\n")
	b.WriteString("    ip6 nexthdr icmpv6 accept\n")
	for _, ip := range blocked {
		if err := system.CIDR(ip); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "    ip saddr %s drop\n", ip)
	}
	for _, r := range rules {
		line, err := ruleLine(r)
		if err != nil {
			return "", err
		}
		b.WriteString("    " + line + "\n")
	}
	for _, p := range baseOpenPorts {
		fmt.Fprintf(&b, "    %s dport %s accept\n", p.proto, p.port)
	}
	b.WriteString("  }\n  chain forward { type filter hook forward priority 0; policy accept; }\n")
	b.WriteString("  chain output { type filter hook output priority 0; policy accept; }\n}\n")
	return b.String(), nil
}

func ruleLine(r FirewallRule) (string, error) {
	if r.Action != "allow" && r.Action != "deny" {
		return "", fmt.Errorf("%w: action", system.ErrInvalid)
	}
	if err := system.Port(r.Port); err != nil {
		return "", err
	}
	if err := system.CIDR(r.Source); err != nil {
		return "", err
	}
	verdict := map[string]string{"allow": "accept", "deny": "drop"}[r.Action]
	line := ""
	if r.Source != "" {
		line += "ip saddr " + r.Source + " "
	}
	switch r.Protocol {
	case "tcp", "udp":
		if r.Port == "" {
			line += "meta l4proto " + r.Protocol + " "
		} else {
			port := r.Port
			for i := range port {
				if port[i] == ':' {
					port = port[:i] + "-" + port[i+1:]
				}
			}
			line += r.Protocol + " dport " + port + " "
		}
	case "any":
	default:
		return "", fmt.Errorf("%w: protocol", system.ErrInvalid)
	}
	return line + verdict, nil
}

// ApplyFirewall validates the ruleset with nft -c before loading it, so a bad
// rule can never replace a working firewall.
func (p *Provisioner) ApplyFirewall(ctx context.Context, rules []FirewallRule, blocked []string) error {
	conf, err := RenderNFT(rules, blocked)
	if err != nil {
		return err
	}
	const path = "/etc/nftables.d/breefercp.nft"
	if err := p.run.WriteFile(path, []byte(conf), 0o600); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "nft", "-c", "-f", path); err != nil {
		return fmt.Errorf("firewall ruleset rejected: %w", err)
	}
	_, err = p.run.Run(ctx, "nft", "-f", path)
	return err
}
