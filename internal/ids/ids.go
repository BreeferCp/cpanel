// Package ids generates opaque random identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a 128-bit random identifier as 32 hex characters.
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("ids: system randomness unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
