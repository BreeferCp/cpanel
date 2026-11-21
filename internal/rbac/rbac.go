// Package rbac defines panel roles and the permissions each one holds.
package rbac

// Role is one of the three panel personas.
type Role string

// The three panel roles. Root is the WHM administrator, Reseller manages its
// own customers, User is a cPanel-style account holder.
const (
	Root     Role = "root"
	Reseller Role = "reseller"
	User     Role = "user"
)

// Permission is "<resource>:<verb>".
type Permission string

// Permissions used by the API.
const (
	// Server administration (WHM, root only).
	ServerRead    Permission = "server:read"
	ServerWrite   Permission = "server:write"
	FirewallRead  Permission = "firewall:read"
	FirewallWrite Permission = "firewall:write"
	AuditRead     Permission = "audit:read"
	SettingsWrite Permission = "settings:write"

	// Customer management (root + reseller).
	AccountsRead   Permission = "accounts:read"
	AccountsWrite  Permission = "accounts:write"
	PackagesRead   Permission = "packages:read"
	PackagesWrite  Permission = "packages:write"
	ResellersWrite Permission = "resellers:write"
	BillingWrite   Permission = "billing:write"

	// Hosting features (account holder, or an admin acting on an account).
	HostingRead  Permission = "hosting:read"
	HostingWrite Permission = "hosting:write"
	FilesRead    Permission = "files:read"
	FilesWrite   Permission = "files:write"

	// Self service, every role.
	ProfileWrite Permission = "profile:write"
	TokensWrite  Permission = "tokens:write"
)

var matrix = map[Role]map[Permission]bool{
	Root: all(
		ServerRead, ServerWrite, FirewallRead, FirewallWrite, AuditRead, SettingsWrite,
		AccountsRead, AccountsWrite, PackagesRead, PackagesWrite, ResellersWrite, BillingWrite,
		HostingRead, HostingWrite, FilesRead, FilesWrite, ProfileWrite, TokensWrite,
	),
	Reseller: all(
		AccountsRead, AccountsWrite, PackagesRead, PackagesWrite, BillingWrite, ResellersWrite,
		HostingRead, HostingWrite, FilesRead, FilesWrite, ProfileWrite, TokensWrite,
	),
	User: all(HostingRead, HostingWrite, FilesRead, FilesWrite, ProfileWrite, TokensWrite),
}

func all(p ...Permission) map[Permission]bool {
	m := make(map[Permission]bool, len(p))
	for _, x := range p {
		m[x] = true
	}
	return m
}

// Can reports whether role holds permission p.
func Can(r Role, p Permission) bool { return matrix[r][p] }

// Valid reports whether r is a known role.
func Valid(r Role) bool { _, ok := matrix[r]; return ok }
