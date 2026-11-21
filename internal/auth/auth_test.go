package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse 9")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse 9", h) {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword("wrong horse 9", h) {
		t.Fatal("invalid password accepted")
	}
	if VerifyPassword("x", "$argon2id$v=19$m=999999999,t=1,p=1$AAAA$AAAA") {
		t.Fatal("absurd parameters must be refused")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short1!":              false,
		"alllettersnodigitsxx": false,
		"123456789012345":      false,
		"a-good-passphrase-7":  true,
	} {
		if (CheckPasswordPolicy(pw) == nil) != ok {
			t.Errorf("policy(%q) want ok=%v", pw, ok)
		}
	}
}

// RFC 6238 appendix B, SHA1, secret "12345678901234567890".
func TestTOTPVector(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(59, 0)); !ok {
		t.Fatal("RFC 6238 vector at t=59 rejected")
	}
	if _, ok := VerifyTOTP(secret, "000000", time.Unix(59, 0)); ok {
		t.Fatal("wrong code accepted")
	}
}

func TestRandomPasswordMeetsPolicy(t *testing.T) {
	for i := 0; i < 50; i++ {
		pw, err := RandomPassword(16)
		if err != nil || CheckPasswordPolicy(pw) != nil {
			t.Fatalf("generated password %q violates policy (%v)", pw, err)
		}
	}
}
