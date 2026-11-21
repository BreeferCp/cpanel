package provision

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// FTPUser is one Pure-FTPd virtual user. It logs in as Login, runs with the
// account's uid and is chrooted to Home/Dir.
type FTPUser struct {
	Login   string // name@domain
	Hash    string // $6$ crypt
	UID     int
	Home    string // account home
	Dir     string // relative to Home, "" = whole home
	QuotaMB int
}

const (
	pureDBText = "/etc/pure-ftpd/pureftpd.passwd"
	pureDB     = "/etc/pure-ftpd/pureftpd.pdb"
)

var (
	reFTPLogin = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}@[a-z0-9.-]{3,253}$`)
	reRelDir   = regexp.MustCompile(`^[A-Za-z0-9._ -]+(/[A-Za-z0-9._ -]+)*$`)
)

// FTPLogin validates a login such as "upload@example.com".
func FTPLogin(s string) error {
	if !reFTPLogin.MatchString(s) {
		return fmt.Errorf("%w: FTP user names use lowercase letters, digits, . _ - (max 32)", system.ErrInvalid)
	}
	_, dom, _ := strings.Cut(s, "@")
	return system.Domain(dom)
}

// CleanRelDir validates a directory relative to a home and normalises it.
func CleanRelDir(s string) (string, error) {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" {
		return "", nil
	}
	c := path.Clean(s)
	if !reRelDir.MatchString(c) || strings.HasPrefix(c, "..") || strings.Contains(c, "/..") || strings.HasPrefix(c, ".") {
		return "", fmt.Errorf("%w: choose a folder inside your home directory, such as public_html/uploads", system.ErrInvalid)
	}
	return c, nil
}

// RenderPureDB produces the Pure-FTPd passwd text for every FTP user on the
// server. Fields: login:hash:uid:gid:gecos:home/./:ul_bw:dl_bw:ul_ratio:
// dl_ratio:max_conn:files_quota:size_quota:...; "/./" marks the chroot.
func RenderPureDB(users []FTPUser) (string, error) {
	sort.Slice(users, func(i, j int) bool { return users[i].Login < users[j].Login })
	var b strings.Builder
	for _, u := range users {
		if err := FTPLogin(u.Login); err != nil {
			return "", err
		}
		if !strings.HasPrefix(u.Hash, "$6$") || strings.ContainsAny(u.Hash, ":\n") {
			return "", fmt.Errorf("%w: FTP password hash", system.ErrInvalid)
		}
		dir, err := CleanRelDir(u.Dir)
		if err != nil {
			return "", err
		}
		home := filepath.ToSlash(filepath.Join(u.Home, dir))
		if strings.ContainsAny(home, ":\n") || u.UID < 1000 {
			return "", fmt.Errorf("%w: FTP home", system.ErrInvalid)
		}
		quota := ""
		if u.QuotaMB > 0 {
			quota = fmt.Sprint(int64(u.QuotaMB) << 20)
		}
		fmt.Fprintf(&b, "%s:%s:%d:%d::%s/./::::::%s::::::\n", u.Login, u.Hash, u.UID, u.UID, home, quota)
	}
	return b.String(), nil
}

// SyncFTP rewrites the Pure-FTPd user database from the full user list.
func (p *Provisioner) SyncFTP(ctx context.Context, users []FTPUser) error {
	text, err := RenderPureDB(users)
	if err != nil {
		return err
	}
	if err := p.run.WriteFile(pureDBText, []byte(text), 0o600); err != nil {
		return err
	}
	_, err = p.run.Run(ctx, "pure-pw", "mkdb", pureDB, "-f", pureDBText)
	return err
}

// ---- Git ------------------------------------------------------------------

var reCloneURL = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:[0-9]{1,5})?/[A-Za-z0-9._~/-]+$`)

// GitCloneURL accepts public https URLs only (no credentials, no ssh or
// file transports, which could read other files on the server).
func GitCloneURL(s string) error {
	if !reCloneURL.MatchString(s) || strings.Contains(s, "..") {
		return fmt.Errorf("%w: use an https:// clone URL without credentials", system.ErrInvalid)
	}
	return nil
}

// GitRepo is a repository inside an account home.
type GitRepo struct {
	Dir      string // relative to home
	CloneURL string
}

// GitCreate initialises or clones a repository as the account user.
func (p *Provisioner) GitCreate(ctx context.Context, a Account, r GitRepo) error {
	dir, err := CleanRelDir(r.Dir)
	if err != nil || dir == "" {
		return fmt.Errorf("%w: repository folder", system.ErrInvalid)
	}
	full := filepath.ToSlash(filepath.Join(a.Home, dir))
	if r.CloneURL != "" {
		if err := GitCloneURL(r.CloneURL); err != nil {
			return err
		}
		_, err = p.asUser(ctx, a, nil, "git", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always",
			"clone", "--", r.CloneURL, full)
		return err
	}
	_, err = p.asUser(ctx, a, nil, "git", "init", "-b", "main", "--", full)
	return err
}

// GitPull fast-forwards a cloned repository.
func (p *Provisioner) GitPull(ctx context.Context, a Account, dir string) (string, error) {
	dir, err := CleanRelDir(dir)
	if err != nil || dir == "" {
		return "", fmt.Errorf("%w: repository folder", system.ErrInvalid)
	}
	res, err := p.asUser(ctx, a, nil, "git", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always",
		"-C", filepath.ToSlash(filepath.Join(a.Home, dir)), "pull", "--ff-only")
	return string(res.Stdout), err
}

// GitInfo is the state of a repository.
type GitInfo struct {
	Branch string `json:"branch"`
	Head   string `json:"head"`
	Commit string `json:"commit"`
	When   string `json:"when"`
}

// GitStatus reads branch and last commit.
func (p *Provisioner) GitStatus(ctx context.Context, a Account, dir string) GitInfo {
	dir, err := CleanRelDir(dir)
	if err != nil || dir == "" {
		return GitInfo{}
	}
	full := filepath.ToSlash(filepath.Join(a.Home, dir))
	var info GitInfo
	if res, err := p.asUser(ctx, a, nil, "git", "-C", full, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.Branch = strings.TrimSpace(string(res.Stdout))
	}
	if res, err := p.asUser(ctx, a, nil, "git", "-C", full, "log", "-1", "--format=%h%x09%s%x09%cr"); err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(res.Stdout)), "\t", 3)
		if len(parts) == 3 {
			info.Head, info.Commit, info.When = parts[0], parts[1], parts[2]
		}
	}
	return info
}
