package identity

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"unicode"

	"github.com/charmbracelet/ssh"
)

const maxSlotLen = 32

// SessionIdentity is the resolved account for one SSH connection.
type SessionIdentity struct {
	Fingerprint string
	Slot        string
}

// FromSession derives fingerprint and save slot from an authenticated session.
func FromSession(s ssh.Session, defaultSlot string) SessionIdentity {
	fp := Fingerprint(s.PublicKey())
	slot := ResolveSlot(s.User(), defaultSlot)
	return SessionIdentity{Fingerprint: fp, Slot: slot}
}

// Fingerprint returns a stable SHA-256 fingerprint for the public key.
func Fingerprint(key ssh.PublicKey) string {
	if key == nil {
		return ""
	}
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// ResolveSlot maps the SSH username to a sanitized save slot.
func ResolveSlot(username, defaultSlot string) string {
	slot := SanitizeSlot(username)
	if slot == "" {
		return SanitizeSlot(defaultSlot)
	}
	return slot
}

// SanitizeSlot normalizes a save name: lowercase, [a-z0-9_-], length 1–32.
// Returns empty if nothing valid remains.
func SanitizeSlot(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			continue
		default:
			continue
		}
		if b.Len() >= maxSlotLen {
			break
		}
	}
	return b.String()
}
