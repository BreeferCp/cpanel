package auth

import "testing"

// Vectors from Drepper's SHA-crypt specification.
func TestSHA512CryptVectors(t *testing.T) {
	cases := []struct {
		pw, salt string
		rounds   int
		explicit bool
		want     string
	}{
		{"Hello world!", "saltstring", 5000, false,
			"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"Hello world!", "saltstringsaltstring", 10000, true,
			"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v."},
		{"This is just a test", "toolongsaltstring", 5000, true,
			"$6$rounds=5000$toolongsaltstrin$lQ8jolhgVRVhY4b5pZKaysCLi0QBxGoNeKQzQ3glMhwllF7oGDZxUhx1yxdYcz/e1JSbq3y6JMxxl8audkUEm0"},
	}
	for _, c := range cases {
		if got := sha512Crypt([]byte(c.pw), c.salt, c.rounds, c.explicit); got != c.want {
			t.Errorf("crypt(%q, %q) = %s, want %s", c.pw, c.salt, got, c.want)
		}
		if !VerifySHA512Crypt(c.pw, c.want) {
			t.Errorf("verify failed for %s", c.want)
		}
	}
	h, err := SHA512Crypt("correct horse")
	if err != nil || !VerifySHA512Crypt("correct horse", h) || VerifySHA512Crypt("wrong", h) {
		t.Fatalf("round trip failed: %s %v", h, err)
	}
}
