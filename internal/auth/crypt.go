package auth

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"strconv"
	"strings"
)

// SHA512Crypt hashes password in the glibc "$6$" format (Drepper's
// SHA-crypt), which nginx auth_basic, Pure-FTPd and PAM all accept. It is
// used only where those external programs must verify the hash themselves;
// panel logins use Argon2id.
func SHA512Crypt(password string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	salt := cryptB64(raw)[:16]
	// The default 5000 rounds: nginx re-verifies basic auth on every request,
	// so a much higher cost would make protected pages slow.
	return sha512Crypt([]byte(password), salt, 5000, false), nil
}

// VerifySHA512Crypt checks a password against a "$6$" hash.
func VerifySHA512Crypt(password, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) < 4 || parts[1] != "6" {
		return false
	}
	rounds, explicit, salt := 5000, false, parts[2]
	if r, ok := strings.CutPrefix(parts[2], "rounds="); ok && len(parts) == 5 {
		n, err := strconv.Atoi(r)
		if err != nil || n < 1000 || n > 999999999 {
			return false
		}
		rounds, explicit, salt = n, true, parts[3]
	}
	got := sha512Crypt([]byte(password), salt, rounds, explicit)
	return subtle.ConstantTimeCompare([]byte(got), []byte(hash)) == 1
}

func sha512Crypt(pw []byte, salt string, rounds int, explicitRounds bool) string {
	if len(salt) > 16 {
		salt = salt[:16]
	}
	s := []byte(salt)

	b := sha512.New()
	b.Write(pw)
	b.Write(s)
	b.Write(pw)
	digB := b.Sum(nil)

	a := sha512.New()
	a.Write(pw)
	a.Write(s)
	for i := len(pw); i > 0; i -= 64 {
		a.Write(digB[:min(i, 64)])
	}
	for i := len(pw); i > 0; i >>= 1 {
		if i&1 != 0 {
			a.Write(digB)
		} else {
			a.Write(pw)
		}
	}
	digA := a.Sum(nil)

	dp := sha512.New()
	for range len(pw) {
		dp.Write(pw)
	}
	p := repeat(dp.Sum(nil), len(pw))

	ds := sha512.New()
	for range 16 + int(digA[0]) {
		ds.Write(s)
	}
	sb := repeat(ds.Sum(nil), len(s))

	c := digA
	for i := range rounds {
		h := sha512.New()
		if i&1 != 0 {
			h.Write(p)
		} else {
			h.Write(c)
		}
		if i%3 != 0 {
			h.Write(sb)
		}
		if i%7 != 0 {
			h.Write(p)
		}
		if i&1 != 0 {
			h.Write(c)
		} else {
			h.Write(p)
		}
		c = h.Sum(nil)
	}

	var out strings.Builder
	out.WriteString("$6$")
	if explicitRounds {
		out.WriteString("rounds=" + strconv.Itoa(rounds) + "$")
	}
	out.WriteString(salt)
	out.WriteByte('$')
	order := [][3]int{
		{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4}, {47, 5, 26}, {6, 27, 48},
		{28, 49, 7}, {50, 8, 29}, {9, 30, 51}, {31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13},
		{56, 14, 35}, {15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19}, {62, 20, 41},
	}
	for _, o := range order {
		enc24(&out, c[o[0]], c[o[1]], c[o[2]], 4)
	}
	enc24(&out, 0, 0, c[63], 2)
	return out.String()
}

func repeat(d []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, d[:min(n-len(out), len(d))]...)
	}
	return out
}

const cryptAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func enc24(out *strings.Builder, b2, b1, b0 byte, n int) {
	w := uint(b2)<<16 | uint(b1)<<8 | uint(b0)
	for range n {
		out.WriteByte(cryptAlphabet[w&0x3f])
		w >>= 6
	}
}

func cryptB64(b []byte) string {
	var out strings.Builder
	for i := 0; i+2 < len(b); i += 3 {
		enc24(&out, b[i], b[i+1], b[i+2], 4)
	}
	return out.String()
}
