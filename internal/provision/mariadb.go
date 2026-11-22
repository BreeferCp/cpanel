package provision

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// MariaDB manages customer databases and users through an admin connection.
type MariaDB struct {
	db     *sql.DB
	dryRun bool
	log    func(string, ...any)
}

// NewMariaDB wraps an admin *sql.DB (nil is allowed in dry-run mode).
func NewMariaDB(db *sql.DB, dryRun bool) *MariaDB { return &MariaDB{db: db, dryRun: dryRun} }

func (m *MariaDB) exec(ctx context.Context, q string, args ...any) error {
	if m.dryRun || m.db == nil {
		return nil
	}
	_, err := m.db.ExecContext(ctx, q, args...)
	return err
}

// quoteIdent backtick-quotes an identifier that already passed validation.
func quoteIdent(s string) (string, error) {
	if err := system.DBName(s); err != nil {
		return "", err
	}
	return "`" + s + "`", nil
}

// escapePattern makes a database name literal where MariaDB expects a pattern.
// In GRANT and REVOKE "_" matches any character, so `abc_corp_wp` would also
// cover another customer's `abcecorp_wp`.
func escapePattern(name string) string {
	return strings.ReplaceAll(name, "_", `\_`)
}

// grantTarget is quoteIdent for GRANT and REVOKE, where the name is a pattern.
func grantTarget(name string) (string, error) {
	if err := system.DBName(name); err != nil {
		return "", err
	}
	return "`" + escapePattern(name) + "`", nil
}

// EscapeExistingGrants narrows grants written by versions that did not escape.
// The rows are edited in place so that custom privilege lists are preserved.
func (m *MariaDB) EscapeExistingGrants(ctx context.Context, names []string) error {
	for _, name := range names {
		if err := system.DBName(name); err != nil {
			return err
		}
		if !strings.Contains(name, "_") {
			continue
		}
		// IGNORE leaves rows that already have an escaped twin; those go next.
		if err := m.exec(ctx, "UPDATE IGNORE mysql.db SET Db = ? WHERE Db = ?", escapePattern(name), name); err != nil {
			return err
		}
		if err := m.exec(ctx, "DELETE FROM mysql.db WHERE Db = ?", name); err != nil {
			return err
		}
	}
	return m.exec(ctx, "FLUSH PRIVILEGES")
}

// CreateDatabase creates a utf8mb4 database.
func (m *MariaDB) CreateDatabase(ctx context.Context, name string) error {
	id, err := quoteIdent(name)
	if err != nil {
		return err
	}
	return m.exec(ctx, "CREATE DATABASE "+id+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
}

// DropDatabase removes a database.
func (m *MariaDB) DropDatabase(ctx context.Context, name string) error {
	id, err := quoteIdent(name)
	if err != nil {
		return err
	}
	if err := m.exec(ctx, "DROP DATABASE IF EXISTS "+id); err != nil {
		return err
	}
	return m.exec(ctx, "DROP USER IF EXISTS ?@'localhost'", importUser(name))
}

// CreateUser creates a localhost-only user. The password is sent as a bound
// parameter (the driver is configured with interpolateParams=true because
// CREATE USER cannot be prepared server-side).
func (m *MariaDB) CreateUser(ctx context.Context, name, password string) error {
	if err := system.DBName(name); err != nil {
		return err
	}
	return m.exec(ctx, "CREATE USER ?@'localhost' IDENTIFIED BY ?", name, password)
}

// SetUserPassword changes a database user's password.
func (m *MariaDB) SetUserPassword(ctx context.Context, name, password string) error {
	if err := system.DBName(name); err != nil {
		return err
	}
	return m.exec(ctx, "ALTER USER ?@'localhost' IDENTIFIED BY ?", name, password)
}

// DropUser removes a database user.
func (m *MariaDB) DropUser(ctx context.Context, name string) error {
	if err := system.DBName(name); err != nil {
		return err
	}
	return m.exec(ctx, "DROP USER IF EXISTS ?@'localhost'", name)
}

var allowedPrivs = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "CREATE": true, "DROP": true,
	"ALTER": true, "INDEX": true, "REFERENCES": true, "CREATE TEMPORARY TABLES": true,
	"LOCK TABLES": true, "EXECUTE": true, "CREATE VIEW": true, "SHOW VIEW": true,
	"CREATE ROUTINE": true, "ALTER ROUTINE": true, "EVENT": true, "TRIGGER": true,
}

// Grant gives user the listed privileges (or ALL) on one database.
func (m *MariaDB) Grant(ctx context.Context, db, user string, privs []string) error {
	dbID, err := grantTarget(db)
	if err != nil {
		return err
	}
	if err := system.DBName(user); err != nil {
		return err
	}
	list := "ALL PRIVILEGES"
	if len(privs) > 0 && !(len(privs) == 1 && strings.EqualFold(privs[0], "ALL")) {
		clean := make([]string, 0, len(privs))
		for _, p := range privs {
			p = strings.ToUpper(strings.TrimSpace(p))
			if !allowedPrivs[p] {
				return fmt.Errorf("%w: privilege %q", system.ErrInvalid, p)
			}
			clean = append(clean, p)
		}
		list = strings.Join(clean, ", ")
	}
	if err := m.exec(ctx, "GRANT "+list+" ON "+dbID+".* TO ?@'localhost'", user); err != nil {
		return err
	}
	return m.exec(ctx, "FLUSH PRIVILEGES")
}

// Revoke removes all privileges of user on one database.
func (m *MariaDB) Revoke(ctx context.Context, db, user string) error {
	dbID, err := grantTarget(db)
	if err != nil {
		return err
	}
	if err := system.DBName(user); err != nil {
		return err
	}
	return m.exec(ctx, "REVOKE ALL PRIVILEGES ON "+dbID+".* FROM ?@'localhost'", user)
}

// SizeMB reports the on-disk size of a database.
func (m *MariaDB) SizeMB(ctx context.Context, name string) (int64, error) {
	if err := system.DBName(name); err != nil {
		return 0, err
	}
	if m.dryRun || m.db == nil {
		return 0, nil
	}
	var size sql.NullFloat64
	err := m.db.QueryRowContext(ctx,
		"SELECT SUM(data_length+index_length)/1048576 FROM information_schema.tables WHERE table_schema = ?", name).Scan(&size)
	return int64(size.Float64), err
}
