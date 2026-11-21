// Package ids generates opaque random identifiers.
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// New returns a 128-bit random identifier as 32 hex characters.
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("ids: system randomness unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Ordered returns an identifier that sorts by creation time (nanosecond
// prefix) followed by randomness, for rows that must keep insertion order
// even when created within the same second.
func Ordered() string {
	return fmt.Sprintf("%019d", time.Now().UnixNano()) + New()[:13]
}
