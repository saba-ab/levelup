package app

import (
	"crypto/rand"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/shared/errs"
)

// passwords hashes with the configured cost and verifies legacy Laravel
// `$2y$` hashes (x/crypto accepts the 2y minor version unchanged).
type passwords struct {
	cost int

	dummyOnce sync.Once
	dummy     []byte
}

func newPasswords(cost int) *passwords {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = 12
	}
	return &passwords{cost: cost}
}

// Hash enforces the policy (8..72 bytes) and hashes.
func (p *passwords) Hash(pw string) (string, error) {
	if err := domain.CheckPasswordPolicy(pw); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), p.cost)
	if err != nil {
		return "", errs.Wrap(errs.Internal, "hash password", err)
	}
	return string(h), nil
}

// Matches truncates the presented password to 72 bytes first: that is what
// PHP hashed, so migrated hashes of longer passwords keep verifying.
func (p *passwords) Matches(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), domain.LegacyTruncate(pw)) == nil
}

// Burn spends the same bcrypt work as a real compare so an unknown email
// is not distinguishable by response time (doc 02 §10 bug 3).
func (p *passwords) Burn(pw string) {
	p.dummyOnce.Do(func() {
		seed := make([]byte, 32)
		_, _ = rand.Read(seed)
		h, err := bcrypt.GenerateFromPassword(seed, p.cost)
		if err == nil {
			p.dummy = h
		}
	})
	if p.dummy != nil {
		_ = bcrypt.CompareHashAndPassword(p.dummy, domain.LegacyTruncate(pw))
	}
}

// NeedsRehash reports a hash weaker than the configured cost.
func (p *passwords) NeedsRehash(hash string) bool {
	c, err := bcrypt.Cost([]byte(hash))
	return err == nil && c < p.cost
}

const slugAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomSuffix is the 6-character slug suffix (parity with Str::random(6),
// lower-cased so slugs stay URL-canonical).
func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}
