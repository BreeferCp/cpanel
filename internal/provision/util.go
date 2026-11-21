package provision

import (
	"crypto/rand"
	"math/big"
)

func bigSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return big.NewInt(1)
	}
	return n
}
