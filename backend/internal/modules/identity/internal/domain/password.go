package domain

// MinPasswordBytes and MaxPasswordBytes are the password policy. 72 is
// bcrypt's input limit: PHP silently truncated beyond it (doc 02 §10 bug 23),
// so new passwords are capped and the gap never grows.
const (
	MinPasswordBytes = 8
	MaxPasswordBytes = 72
)

// CheckPasswordPolicy validates a NEW password. Byte length, not runes:
// bcrypt counts bytes.
func CheckPasswordPolicy(pw string) error {
	switch {
	case len(pw) < MinPasswordBytes:
		return ErrPasswordTooShort
	case len(pw) > MaxPasswordBytes:
		return ErrPasswordTooLong
	}
	return nil
}

// LegacyTruncate returns the first 72 bytes of a presented password, which
// is exactly what PHP's bcrypt hashed. Applied before every compare so
// migrated `$2y$` hashes of long passwords keep verifying.
func LegacyTruncate(pw string) []byte {
	b := []byte(pw)
	if len(b) > MaxPasswordBytes {
		return b[:MaxPasswordBytes]
	}
	return b
}
