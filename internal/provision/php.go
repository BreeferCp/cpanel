package provision

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/breefercp/breefercp/internal/system"
)

// PHPIniKeys are the php.ini directives a customer may override, with the
// pattern each value must match. Everything else (disable_functions,
// open_basedir, allow_url_include, ...) stays under administrator control.
var PHPIniKeys = map[string]*regexp.Regexp{
	"memory_limit":            regexp.MustCompile(`^([1-9][0-9]{1,4}M|[1-8]G|-1)$`),
	"max_execution_time":      regexp.MustCompile(`^[0-9]{1,4}$`),
	"max_input_time":          regexp.MustCompile(`^-?[0-9]{1,4}$`),
	"max_input_vars":          regexp.MustCompile(`^[0-9]{3,6}$`),
	"post_max_size":           regexp.MustCompile(`^[1-9][0-9]{0,3}[KMG]$`),
	"upload_max_filesize":     regexp.MustCompile(`^[1-9][0-9]{0,3}[KMG]$`),
	"display_errors":          regexp.MustCompile(`^(On|Off)$`),
	"error_reporting":         regexp.MustCompile(`^(E_ALL|E_ALL & ~E_DEPRECATED & ~E_STRICT|E_ALL & ~E_NOTICE|0)$`),
	"date.timezone":           regexp.MustCompile(`^[A-Za-z]+(/[A-Za-z_+-]+){0,2}$`),
	"session.gc_maxlifetime":  regexp.MustCompile(`^[0-9]{2,7}$`),
	"output_buffering":        regexp.MustCompile(`^(On|Off|[0-9]{1,6})$`),
	"short_open_tag":          regexp.MustCompile(`^(On|Off)$`),
	"zlib.output_compression": regexp.MustCompile(`^(On|Off)$`),
}

// PHPIniDefaults are shown in the editor when the customer has not set a value.
var PHPIniDefaults = map[string]string{
	"memory_limit": "256M", "max_execution_time": "30", "max_input_time": "60", "max_input_vars": "1000",
	"post_max_size": "64M", "upload_max_filesize": "64M", "display_errors": "Off", "error_reporting": "E_ALL & ~E_DEPRECATED & ~E_STRICT",
	"date.timezone": "UTC", "session.gc_maxlifetime": "1440", "output_buffering": "4096", "short_open_tag": "Off",
	"zlib.output_compression": "Off",
}

// CleanPHPIni validates overrides and returns them unchanged when every key is
// allow-listed and every value matches its pattern.
func CleanPHPIni(in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for k, v := range in {
		re, ok := PHPIniKeys[k]
		if !ok {
			return nil, fmt.Errorf("%w: %s cannot be changed", system.ErrInvalid, k)
		}
		if !re.MatchString(v) {
			return nil, fmt.Errorf("%w: invalid value for %s", system.ErrInvalid, k)
		}
		out[k] = v
	}
	return out, nil
}

// InstalledPHPVersions lists PHP-FPM versions present on the host
// (/etc/php/<v>/fpm). In dry-run mode it reports a typical set.
func (p *Provisioner) InstalledPHPVersions() []string {
	if p.run.DryRun {
		return []string{"8.4", "8.3", "8.2", "8.1"}
	}
	entries, err := os.ReadDir(p.cfg.PHPFPMPoolDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if system.PHPVersion(e.Name()) != nil {
			continue
		}
		if st, err := os.Stat(filepath.Join(p.cfg.PHPFPMPoolDir, e.Name(), "fpm")); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}
