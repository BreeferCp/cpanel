package provision

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/breefercp/breefercp/internal/system"
)

// Vhost describes one nginx server block.
type Vhost struct {
	Domain     string
	Aliases    []string
	DocRoot    string
	PHP        string
	ForceHTTPS bool
	CertPath   string // empty: HTTP only
	KeyPath    string
	Suspended  bool
	// ProxyPort, when >0, proxies to a local app instead of serving files.
	ProxyPort int
	// Redirects are exact-path redirects served before anything else.
	Redirects []Redirect
	// ErrorPages lists statuses with a custom page in ErrorDir/<code>.html.
	ErrorPages []int
	ErrorDir   string
}

// Redirect sends requests for an exact path to an absolute URL.
type Redirect struct {
	Source string // "/old-page"
	Target string // "https://example.com/new"
	Code   int    // 301, 302, 307, 308
}

// ErrorCodes are the statuses a customer may customise.
var ErrorCodes = []int{400, 401, 403, 404, 500, 502, 503}

// Neither pattern admits ; $ quotes, braces or whitespace: nginx would treat
// them as syntax or variable interpolation.
var (
	reRedirectPath = regexp.MustCompile(`^/[A-Za-z0-9._~!&+,=:@%/-]{0,512}$`)
	reRedirectURL  = regexp.MustCompile(`^https?://[A-Za-z0-9.-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~!&+,=:@%/?#-]*)?$`)
)

// ValidateRedirect rejects anything that could break out of a config line.
func ValidateRedirect(r Redirect) error {
	if !reRedirectPath.MatchString(r.Source) || strings.Contains(r.Source, "..") {
		return fmt.Errorf("%w: source must be a path such as /old-page", system.ErrInvalid)
	}
	if !reRedirectURL.MatchString(r.Target) {
		return fmt.Errorf("%w: target must be a full http(s) URL", system.ErrInvalid)
	}
	switch r.Code {
	case 301, 302, 307, 308:
	default:
		return fmt.Errorf("%w: redirect code must be 301, 302, 307 or 308", system.ErrInvalid)
	}
	return nil
}

func validErrorCode(c int) bool {
	for _, x := range ErrorCodes {
		if x == c {
			return true
		}
	}
	return false
}

var vhostTmpl = template.Must(template.New("vhost").Parse(`# Managed by BreeferCP. Do not edit.
{{- if .Suspended}}
server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    return 503;
    error_page 503 /suspended.html;
    location = /suspended.html { root /usr/share/breefercp/static; internal; }
}
{{- else}}
{{- if and .CertPath .ForceHTTPS}}
server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}}{{range .Aliases}} {{.}}{{end}};
    location ^~ /.well-known/acme-challenge/ { root {{.ACMERoot}}; }
    location / { return 301 https://$host$request_uri; }
}
{{- end}}
server {
{{- if .CertPath}}
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    ssl_certificate {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;
    add_header Strict-Transport-Security "max-age=31536000" always;
{{- end}}
{{- if not (and .CertPath .ForceHTTPS)}}
    listen 80;
    listen [::]:80;
{{- end}}
    server_name {{.Domain}}{{range .Aliases}} {{.}}{{end}};
    access_log {{.LogDir}}/{{.Domain}}.access.log;
    error_log  {{.LogDir}}/{{.Domain}}.error.log warn;
    server_tokens off;
    client_max_body_size 256m;

    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    location ^~ /.well-known/acme-challenge/ { root {{.ACMERoot}}; }
    # Never serve dotfiles (.git, .env, .htpasswd) except ACME.
    location ~ /\.(?!well-known) { deny all; return 404; }
{{- range .Redirects}}
    location = {{.Source}} { return {{.Code}} {{.Target}}; }
{{- end}}
{{- if .ErrorPages}}
{{- range .ErrorPages}}
    error_page {{.}} /.breefer-errors/{{.}}.html;
{{- end}}
    location ^~ /.breefer-errors/ { alias {{.ErrorDir}}/; internal; }
{{- end}}
{{- if gt .ProxyPort 0}}
    location / {
        proxy_pass http://127.0.0.1:{{.ProxyPort}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
{{- else}}
    root {{.DocRoot}};
    index index.php index.html;
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        try_files $uri =404;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        fastcgi_param DOCUMENT_ROOT $realpath_root;
        fastcgi_pass unix:{{.Socket}};
    }
{{- end}}
}
{{- end}}
`))

type vhostView struct {
	Vhost
	ACMERoot string
	LogDir   string
	Socket   string
}

// RenderVhost produces nginx configuration text. Exposed for tests.
func RenderVhost(v Vhost, a Account, acmeRoot string) (string, error) {
	if err := system.Domain(v.Domain); err != nil {
		return "", err
	}
	for _, al := range v.Aliases {
		if err := system.Domain(al); err != nil {
			return "", err
		}
	}
	if v.PHP == "" {
		v.PHP = a.PHP
	}
	if err := system.PHPVersion(v.PHP); err != nil {
		return "", err
	}
	if v.DocRoot == "" {
		v.DocRoot = filepath.ToSlash(filepath.Join(a.Home, "public_html"))
	}
	for _, r := range v.Redirects {
		if err := ValidateRedirect(r); err != nil {
			return "", err
		}
	}
	for _, c := range v.ErrorPages {
		if !validErrorCode(c) {
			return "", fmt.Errorf("%w: error code %d", system.ErrInvalid, c)
		}
	}
	if len(v.ErrorPages) > 0 && v.ErrorDir == "" {
		v.ErrorDir = ErrorDir(a, v.Domain)
	}
	view := vhostView{
		Vhost:    v,
		ACMERoot: acmeRoot,
		LogDir:   filepath.ToSlash(filepath.Join(a.Home, "logs")),
		Socket:   fpmSocket(a.SysUser, v.PHP),
	}
	var b bytes.Buffer
	if err := vhostTmpl.Execute(&b, view); err != nil {
		return "", err
	}
	return b.String(), nil
}

// WriteVhost renders, validates with nginx -t and activates a vhost.
func (p *Provisioner) WriteVhost(ctx context.Context, v Vhost, a Account) error {
	if p.cfg.WebServer == "openlitespeed" {
		return p.writeOLSVhost(ctx, v, a)
	}
	conf, err := RenderVhost(v, a, "/var/lib/breefercp/acme-webroot")
	if err != nil {
		return err
	}
	path := filepath.Join(p.cfg.NginxConfDir, v.Domain+".conf")
	if err := p.run.WriteFile(path, []byte(conf), 0o644); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "nginx", "-t"); err != nil {
		// Roll back a bad config instead of leaving nginx unable to reload.
		_ = p.run.Remove(path)
		return fmt.Errorf("nginx rejected vhost for %s: %w", v.Domain, err)
	}
	return p.ReloadWeb(ctx)
}

// RemoveVhost deletes a vhost file.
func (p *Provisioner) RemoveVhost(ctx context.Context, domain string) error {
	if err := system.Domain(domain); err != nil {
		return err
	}
	if p.cfg.WebServer == "openlitespeed" {
		return p.removeOLSVhost(ctx, domain)
	}
	return p.run.Remove(filepath.Join(p.cfg.NginxConfDir, domain+".conf"))
}

// ReloadWeb gracefully reloads nginx.
func (p *Provisioner) ReloadWeb(ctx context.Context) error {
	if p.cfg.WebServer == "openlitespeed" {
		_, err := p.run.Run(ctx, olsCtl, "restart")
		return err
	}
	_, err := p.run.Run(ctx, "systemctl", "reload", "nginx")
	return err
}

func fpmSocket(user, php string) string {
	return fmt.Sprintf("/run/php/breefer-%s-%s.sock", user, php)
}

var poolTmpl = template.Must(template.New("pool").Parse(`; Managed by BreeferCP. Do not edit.
[{{.User}}]
user = {{.User}}
group = {{.User}}
listen = {{.Socket}}
listen.owner = {{.WebUser}}
listen.group = {{.WebUser}}
listen.mode = 0660

pm = ondemand
pm.max_children = {{.MaxChildren}}
pm.process_idle_timeout = 20s
pm.max_requests = 500

php_admin_value[open_basedir] = {{.Home}}/:/tmp/:/usr/share/php/
php_admin_value[upload_tmp_dir] = {{.Home}}/tmp
php_admin_value[sys_temp_dir] = {{.Home}}/tmp
php_admin_value[session.save_path] = {{.Home}}/tmp
php_admin_value[error_log] = {{.Home}}/logs/php-error.log
php_admin_flag[log_errors] = on
php_admin_flag[allow_url_include] = off
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,pcntl_exec,show_source,dl,symlink,link
php_admin_value[memory_limit] = {{.MemoryLimit}}M
php_admin_value[post_max_size] = 256M
php_admin_value[upload_max_filesize] = 256M
{{- range $k, $v := .Ini}}
php_value[{{$k}}] = {{$v}}
{{- end}}
`))

// ApplyPHPPool writes the per-account FPM pool and reloads FPM.
func (p *Provisioner) ApplyPHPPool(ctx context.Context, a Account) error {
	if err := system.PHPVersion(a.PHP); err != nil {
		return err
	}
	ini, err := CleanPHPIni(a.PHPIni)
	if err != nil {
		return err
	}
	mem := a.Limits.MemoryMB / 4
	if mem < 128 {
		mem = 128
	}
	children := a.Limits.MaxProcs / 10
	if children < 4 {
		children = 4
	}
	var b bytes.Buffer
	if err := poolTmpl.Execute(&b, map[string]any{
		"User": a.SysUser, "Socket": fpmSocket(a.SysUser, a.PHP), "Home": filepath.ToSlash(a.Home),
		"MaxChildren": children, "MemoryLimit": mem, "WebUser": p.cfg.WebUser, "Ini": ini,
	}); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/%s/fpm/pool.d/breefer-%s.conf", p.cfg.PHPFPMPoolDir, a.PHP, a.SysUser)
	if err := p.run.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return err
	}
	_, err = p.run.Run(ctx, "systemctl", "reload", "php"+a.PHP+"-fpm")
	return err
}

// StopPHPPool removes the pool file and reloads FPM.
func (p *Provisioner) StopPHPPool(ctx context.Context, a Account) error {
	path := fmt.Sprintf("%s/%s/fpm/pool.d/breefer-%s.conf", p.cfg.PHPFPMPoolDir, a.PHP, a.SysUser)
	if err := p.run.Remove(path); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "systemctl", "reload", "php"+a.PHP+"-fpm")
	return err
}

// ErrorDir is where a domain's custom error pages live inside the account.
func ErrorDir(a Account, domain string) string {
	return filepath.ToSlash(filepath.Join(a.Home, "error_pages", domain))
}
