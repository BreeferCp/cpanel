package provision

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/breefercp/breefercp/internal/system"
)

// OpenLiteSpeed support. Each domain gets its own virtual host directory with
// a vhconf.conf; a generated include file registers every virtual host and
// its listener mappings with the server. Apache-style .htaccess files are
// honoured (rewrite rules are auto-loaded), which is the main reason people
// choose LiteSpeed, and PHP runs through per-account LSAPI processes as the
// account's own user.
//
// Status: experimental. The renderer is unit-tested but has not yet been
// validated against a live OpenLiteSpeed server.

const (
	olsRoot     = "/usr/local/lsws"
	olsCtl      = "/usr/local/lsws/bin/lswsctrl"
	olsIncludes = "/usr/local/lsws/conf/breefercp-vhosts.conf"
)

var olsVhostTmpl = template.Must(template.New("ols").Parse(`# Managed by BreeferCP. Do not edit.
docRoot                   {{.DocRoot}}
vhDomain                  {{.Domain}}
vhAliases                 {{range $i, $a := .Aliases}}{{if $i}}, {{end}}{{$a}}{{end}}
enableGzip                1
enableBr                  1

errorlog {{.LogDir}}/{{.Domain}}.error.log {
  useServer               0
  logLevel                WARN
  rollingSize             10M
}

accesslog {{.LogDir}}/{{.Domain}}.access.log {
  useServer               0
  rollingSize             10M
  keepDays                30
}
{{- range .ErrorPages}}

errorpage {{.}} {
  url                     /.breefer-errors/{{.}}.html
}
{{- end}}

{{- if .Suspended}}

context / {
  location                /usr/share/breefercp/static/
  allowBrowse             1
  rewrite {
    enable                1
    rules                 <<<END_rules
RewriteRule ^.*$ /suspended.html [R=503,L]
    END_rules
  }
}
{{- else}}

extprocessor lsphp-{{.User}} {
  type                    lsapi
  address                 uds://tmp/lshttpd/breefer-{{.User}}.sock
  maxConns                {{.MaxConns}}
  env                     PHP_LSAPI_CHILDREN={{.MaxConns}}
  initTimeout             60
  retryTimeout            0
  respBuffer              0
  autoStart               2
  path                    {{.LSPHP}}
  backlog                 100
  instances               1
  extUser                 {{.User}}
  extGroup                {{.User}}
  runOnStartUp            0
  memSoftLimit            {{.MemSoft}}M
  memHardLimit            {{.MemHard}}M
  procSoftLimit           {{.Procs}}
  procHardLimit           {{.Procs}}
}

scripthandler {
  add                     lsapi:lsphp-{{.User}} php
}

phpIniOverride {
  php_admin_value open_basedir "{{.Home}}/:/tmp/"
  php_admin_value upload_tmp_dir "{{.Home}}/tmp"
  php_admin_value session.save_path "{{.Home}}/tmp"
  php_admin_value disable_functions "exec,passthru,shell_exec,system,proc_open,popen,pcntl_exec,show_source,dl,symlink,link"
{{- range $k, $v := .Ini}}
  php_value {{$k}} "{{$v}}"
{{- end}}
}

rewrite {
  enable                  1
  autoLoadHtaccess        1
{{- if .Redirects}}
  rules                   <<<END_rules
{{- range .Redirects}}
RewriteRule ^{{.Pattern}}$ {{.Target}} [R={{.Code}},L]
{{- end}}
  END_rules
{{- end}}
}

context /.well-known/acme-challenge/ {
  location                {{.ACMERoot}}/.well-known/acme-challenge/
  allowBrowse             1
}
{{- if .ErrorPages}}

context /.breefer-errors/ {
  location                {{.ErrorDir}}/
  allowBrowse             1
}
{{- end}}
{{- if gt .ProxyPort 0}}

extprocessor app-{{.User}}-{{.ProxyPort}} {
  type                    proxy
  address                 127.0.0.1:{{.ProxyPort}}
  maxConns                100
  initTimeout             60
  retryTimeout            0
  respBuffer              0
}

context / {
  type                    proxy
  handler                 app-{{.User}}-{{.ProxyPort}}
  addDefaultCharset       off
}
{{- end}}
{{- end}}
{{- if .CertPath}}

vhssl {
  keyFile                 {{.KeyPath}}
  certFile                {{.CertPath}}
  certChain               1
  sslProtocol             24
  enableECDHE             1
  renegProtection         1
  sslSessionCache         1
  enableStapling          1
}
{{- end}}
`))

type olsView struct {
	Vhost
	User, Home, LogDir, LSPHP, ACMERoot string
	MaxConns, MemSoft, MemHard, Procs   int
	Ini                                 map[string]string
	Redirects                           []olsRedirect
}

type olsRedirect struct {
	Pattern, Target string
	Code            int
}

// RenderOLSVhost produces vhconf.conf for one domain. Exposed for tests.
func RenderOLSVhost(v Vhost, a Account, acmeRoot string) (string, error) {
	// Reuse the nginx renderer's validation of every field.
	if _, err := RenderVhost(v, a, acmeRoot); err != nil {
		return "", err
	}
	if v.PHP == "" {
		v.PHP = a.PHP
	}
	if v.DocRoot == "" {
		v.DocRoot = filepath.ToSlash(filepath.Join(a.Home, "public_html"))
	}
	if len(v.ErrorPages) > 0 && v.ErrorDir == "" {
		v.ErrorDir = ErrorDir(a, v.Domain)
	}
	ini, err := CleanPHPIni(a.PHPIni)
	if err != nil {
		return "", err
	}
	procs := a.Limits.MaxProcs / 10
	if procs < 4 {
		procs = 4
	}
	view := olsView{
		Vhost: v, User: a.SysUser, Home: filepath.ToSlash(a.Home),
		LogDir:   LogDir(a),
		LSPHP:    fmt.Sprintf("%s/lsphp%s/bin/lsphp", olsRoot, strings.ReplaceAll(v.PHP, ".", "")),
		ACMERoot: acmeRoot, MaxConns: procs, Procs: procs * 50,
		MemSoft: a.Limits.MemoryMB * 3 / 4, MemHard: a.Limits.MemoryMB, Ini: ini,
	}
	for _, r := range v.Redirects {
		// Paths were validated to a strict character set; escape regex metacharacters.
		view.Redirects = append(view.Redirects, olsRedirect{
			Pattern: regexpQuote(strings.TrimPrefix(r.Source, "/")), Target: r.Target, Code: r.Code,
		})
	}
	var b bytes.Buffer
	if err := olsVhostTmpl.Execute(&b, view); err != nil {
		return "", err
	}
	return b.String(), nil
}

func regexpQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`.+*?()|[]{}^$\`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func olsVhostDir(domain string) string { return filepath.Join(olsRoot, "conf", "vhosts", domain) }

func (p *Provisioner) writeOLSVhost(ctx context.Context, v Vhost, a Account) error {
	conf, err := RenderOLSVhost(v, a, "/var/lib/breefercp/acme-webroot")
	if err != nil {
		return err
	}
	path := filepath.Join(olsVhostDir(v.Domain), "vhconf.conf")
	if err := p.run.WriteFile(path, []byte(conf), 0o640); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "chown", "-R", "lsadm:nogroup", olsVhostDir(v.Domain)); err != nil {
		return err
	}
	if err := p.syncOLSIncludes(ctx); err != nil {
		return err
	}
	return p.ReloadWeb(ctx)
}

func (p *Provisioner) removeOLSVhost(ctx context.Context, domain string) error {
	if _, err := p.run.Run(ctx, "rm", "-rf", "--", olsVhostDir(domain)); err != nil {
		return err
	}
	return p.syncOLSIncludes(ctx)
}

// syncOLSIncludes regenerates the include file that registers every vhost
// directory present on disk and maps it on the HTTP and HTTPS listeners.
func (p *Provisioner) syncOLSIncludes(_ context.Context) error {
	var domains []string
	if !p.run.DryRun {
		entries, _ := readDirNames(filepath.Join(olsRoot, "conf", "vhosts"))
		for _, d := range entries {
			if system.Domain(d) == nil {
				domains = append(domains, d)
			}
		}
	}
	return p.run.WriteFile(olsIncludes, []byte(RenderOLSIncludes(domains)), 0o640)
}

// RenderOLSIncludes builds the server-level include for the given domains.
func RenderOLSIncludes(domains []string) string {
	sort.Strings(domains)
	var b strings.Builder
	b.WriteString("# Managed by BreeferCP. Do not edit.\n")
	for _, d := range domains {
		fmt.Fprintf(&b, "virtualhost %s {\n  vhRoot %s/\n  configFile %s/vhconf.conf\n  allowSymbolLink 1\n  enableScript 1\n  restrained 1\n}\n\n",
			d, olsVhostDir(d), olsVhostDir(d))
	}
	for _, l := range []string{"HTTP", "HTTPS"} {
		fmt.Fprintf(&b, "listener %s {\n", l)
		for _, d := range domains {
			fmt.Fprintf(&b, "  map %s %s, www.%s\n", d, d, d)
		}
		b.WriteString("}\n")
	}
	return b.String()
}
