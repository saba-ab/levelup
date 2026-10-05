package domain

import (
	"crypto/rand"
	"math/big"
	"strings"
)

// codeAlphabet omits 0/O and 1/I/L so codes survive being read aloud.
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewVoucherCode returns a random XXXX-XXXX-XXXX code (31^12 ≈ 2^59
// combinations). Uniqueness per tenant is enforced by a unique index; a
// collision surfaces as ErrCodeCollision and the caller retries.
func NewVoucherCode() string {
	var b strings.Builder
	limit := big.NewInt(int64(len(codeAlphabet)))
	for i := range 12 {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String()
}
