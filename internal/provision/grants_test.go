package provision

import "testing"

func TestGrantTargetIsNotAPattern(t *testing.T) {
	got, err := grantTarget("abc_corp_wp")
	if err != nil {
		t.Fatal(err)
	}
	if want := "`abc\\_corp\\_wp`"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if _, err := grantTarget("a%"); err == nil {
		t.Fatal("accepted a % wildcard")
	}
}
