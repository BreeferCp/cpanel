package provision

import "testing"

func TestParseMailUsage(t *testing.T) {
	got := parseMailUsage("100 a.com/anna/cur/1\n200 a.com/anna/new/2\n50 a.com/anna/.Sent/cur/3\n7 a.com/anna/dovecot.index\n9 a.com/ben/tmp/4\njunk\n")
	if a := got["anna@a.com"]; a.Bytes != 357 || a.Messages != 3 {
		t.Errorf("anna: %+v", a)
	}
	if b := got["ben@a.com"]; b.Bytes != 9 || b.Messages != 0 {
		t.Errorf("ben: %+v", b)
	}
}

func TestParseTableStatus(t *testing.T) {
	out := `u_shop.good                                        OK
u_shop.memory
note     : The storage engine for the table doesn't support check
u_shop.broken
warning  : Table is marked as crashed
error    : Size of datafile is: 0
error    : Corrupt

Repairing tables
u_shop.broken                                      OK
u_shop.dead
Error    : Table 'u_shop.dead' doesn't exist
status   : Operation failed
`
	got := map[string]TableStatus{}
	for _, s := range parseTableStatus(out, "u_shop") {
		got[s.Table] = s
	}
	for table, ok := range map[string]bool{"good": true, "broken": true, "dead": false} {
		if s, found := got[table]; !found || s.OK != ok {
			t.Errorf("%s: %+v, want ok=%v", table, s, ok)
		}
	}
}

func TestCronMailTo(t *testing.T) {
	if !CronMailTo("ops+cron@example.com") {
		t.Error("refused a normal address")
	}
	for _, bad := range []string{"-oQ/tmp@example.com", "a@example.com\n* * * * * root id", "a b@example.com", "a@example.com,b@example.com", `"x"@example.com`} {
		if CronMailTo(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
}
