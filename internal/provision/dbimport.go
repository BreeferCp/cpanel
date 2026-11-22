package provision

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"

	"github.com/breefercp/breefercp/internal/system"
)

// importDir holds one short-lived credentials file per running import.
const importDir = "/run/breefercp-import"

var reDefiner = regexp.MustCompile("DEFINER=`[^`]*`@`[^`]*`")

// ImportDatabase loads a SQL dump into one database.
//
// A dump is customer-controlled: backups can contain planted files, and the
// mariadb client runs shell commands it finds in its input (\!). So the client
// runs as the customer, signed in as a database user that has rights on this
// one database and nothing else. That user is locked between imports and not
// dropped: it becomes the DEFINER of restored views, routines and triggers,
// and those stop working when their definer is gone.
func (p *Provisioner) ImportDatabase(ctx context.Context, a Account, db, dump string) (err error) {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	target, err := grantTarget(db)
	if err != nil {
		return err
	}
	var raw [28]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	user, pass := importUser(db), hex.EncodeToString(raw[4:])
	cleanup := context.WithoutCancel(ctx)

	// Both values are hex generated above and target is validated and quoted.
	create := "CREATE USER IF NOT EXISTS '" + user + "'@'localhost' IDENTIFIED BY '" + pass + "';\n" +
		"ALTER USER '" + user + "'@'localhost' IDENTIFIED BY '" + pass + "' ACCOUNT UNLOCK;\n" +
		"GRANT ALL PRIVILEGES ON " + target + ".* TO '" + user + "'@'localhost';\n"
	if _, err := p.run.RunInput(ctx, []byte(create), "mariadb"); err != nil {
		return err
	}
	defer func() {
		_, e := p.run.RunInput(cleanup, []byte("ALTER USER '"+user+"'@'localhost' ACCOUNT LOCK;\n"), "mariadb")
		err = errors.Join(err, e)
	}()

	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0711", importDir); err != nil {
		return err
	}
	cnf := importDir + "/" + user + "-" + hex.EncodeToString(raw[:4]) + ".cnf"
	if err := p.run.WriteFile(cnf, []byte("[client]\nuser="+user+"\npassword="+pass+"\nhost=localhost\n"), 0o600); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, p.run.Remove(cnf)) }()
	if _, err := p.run.Run(ctx, "chgrp", a.SysUser, cnf); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "chmod", "0640", cnf); err != nil {
		return err
	}

	input := dump
	if !p.run.DryRun {
		input = dump + ".import"
		if err := withoutDefiners(dump, input); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, p.run.Remove(input)) }()
	}
	return p.run.RunWithFileInput(ctx, input, "runuser", "-u", a.SysUser, "--",
		"mariadb", "--defaults-file="+cnf, "--", db)
}

// importUser is fixed per database. "bfimp" is a reserved account name, so no
// customer can create a database user with this name.
func importUser(db string) string {
	sum := sha256.Sum256([]byte(db))
	return "bfimp_" + hex.EncodeToString(sum[:10])
}

// withoutDefiners copies a dump, dropping DEFINER clauses: only a superuser
// may create objects owned by someone else. Row data is copied untouched.
func withoutDefiners(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := stripDefiners(out, in); err != nil {
		return err
	}
	return out.Close()
}

func stripDefiners(out io.Writer, in io.Reader) error {
	r := bufio.NewReaderSize(in, 64<<10)
	w := bufio.NewWriter(out)
	start := true // at the beginning of a line
	for {
		chunk, err := r.ReadSlice('\n')
		if start && (bytes.HasPrefix(chunk, []byte("CREATE")) || bytes.HasPrefix(chunk, []byte("/*!"))) {
			chunk = reDefiner.ReplaceAll(chunk, nil)
		}
		if _, werr := w.Write(chunk); werr != nil {
			return werr
		}
		// Long INSERT lines arrive in several pieces.
		start = !errors.Is(err, bufio.ErrBufferFull)
		if err == io.EOF {
			return w.Flush()
		}
		if err != nil && start {
			return err
		}
	}
}
