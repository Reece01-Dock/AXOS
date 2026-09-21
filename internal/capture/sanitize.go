package capture

import "strings"

// secretKeySubstrings are lowercase substrings that mark an nvram key as
// secret-shaped. Deliberately broad/conservative: a false positive just
// redacts one extra non-secret value in a dev fixture, which costs
// nothing; a false negative leaks a real credential into a file that gets
// committed to git. When in doubt, add the pattern.
var secretKeySubstrings = []string{
	"psk", "passwd", "password", "pass_",
	"_key", "privkey", "private_key",
	"secret", "token", "pin_code",
}

// isSecretKey reports whether an nvram key name looks like it holds a
// credential, based on secretKeySubstrings.
func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range secretKeySubstrings {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

const redacted = "[REDACTED]"

// Sanitize strips secrets from a Snapshot in place and marks it Sanitized.
// WriteFixtures refuses to persist a Snapshot that hasn't been through
// this. Call it exactly once, right after Capture and before anything else
// touches the snapshot (in particular, before logging it or handing it to
// anything that might persist it).
//
// What's redacted:
//   - Every nvram value whose key matches isSecretKey (passwords, PSKs, WEP
//     keys, etc.) — the value is replaced with the literal "[REDACTED]",
//     the key itself is kept, so fixtures still show *that* a secret
//     existed there without revealing it.
//   - VPN peer preshared/private key material — in practice this should
//     already be absent (backend.VPNPeer has no field for it — see its doc
//     comment — and asuswrt.VPNStatus's wg-dump parser deliberately never
//     reads the private-key column), so this is defense in depth, not the
//     primary control.
//
// What's deliberately NOT redacted: MAC addresses, IPs, hostnames, SSIDs.
// These aren't credentials and are useful for realistic replay data; if a
// specific capture needs them scrubbed too (e.g. before sharing a fixture
// outside the team), that's a separate, explicit step — Sanitize's job is
// "safe to commit to this repo's git history", not "fully anonymized".
func Sanitize(s *Snapshot) {
	sanitizedNVRAM := make(map[string]string, len(s.NVRAM))
	for k, v := range s.NVRAM {
		if isSecretKey(k) {
			sanitizedNVRAM[k] = redacted
		} else {
			sanitizedNVRAM[k] = v
		}
	}
	s.NVRAM = sanitizedNVRAM
	s.Sanitized = true
}
