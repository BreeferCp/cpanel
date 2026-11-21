package system

import "testing"

func TestValidators(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) error
		in   string
		ok   bool
	}{
		{"user ok", SysUser, "alice01", true},
		{"user reserved", SysUser, "root", false},
		{"user inject", SysUser, "a;rm -rf /", false},
		{"user too short", SysUser, "ab", false},
		{"domain ok", Domain, "example.co.uk", true},
		{"domain newline", Domain, "a.com\nserver_name evil", false},
		{"domain ip", Domain, "10.0.0.1", false},
		{"domain single label", Domain, "localhost", false},
		{"db ok", DBName, "alice_wp", true},
		{"db dash", DBName, "a-b", false},
		{"cron ok", Cron, "*/5 0-6 * * 1,3", true},
		{"cron injection", Cron, "* * * * * ; id", false},
		{"cron word", Cron, "@reboot", false},
		{"php ok", PHPVersion, "8.3", true},
		{"php bad", PHPVersion, "8.3;ls", false},
		{"port range", Port, "3000:3010", true},
		{"port bad", Port, "80;", false},
	}
	for _, c := range cases {
		if got := c.fn(c.in) == nil; got != c.ok {
			t.Errorf("%s: %q accepted=%v want %v", c.name, c.in, got, c.ok)
		}
	}
}
