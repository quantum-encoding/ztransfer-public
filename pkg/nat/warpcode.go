// Package nat implements NAT traversal via STUN discovery and UDP hole punching
// for direct peer-to-peer file transfers in ztransfer.
package nat

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"
)

// WarpCode represents a human-readable transfer/connection code.
// The 6-byte payload encodes into a format like "warp-729-alpha" that is
// easy to dictate over voice or type manually.
type WarpCode struct {
	Bytes [6]byte
}

// wordlist contains 32 NATO-phonetic-inspired words used for the word portion
// of warp codes. 32 words = 5 bits per word.
var wordlist = []string{
	"alpha", "bravo", "charlie", "delta", "echo",
	"foxtrot", "golf", "hotel", "india", "juliet",
	"kilo", "lima", "mike", "november", "oscar",
	"papa", "quebec", "romeo", "sierra", "tango",
	"uniform", "victor", "whiskey", "xray", "yankee",
	"zulu", "amber", "bronze", "coral", "dusk",
	"ember", "frost",
}

// wordIndex maps word strings to their index for fast lookup during parsing.
var wordIndex map[string]int

func init() {
	wordIndex = make(map[string]int, len(wordlist))
	for i, w := range wordlist {
		wordIndex[w] = i
	}
}

// keyDerivationPrefix is prepended to the code bytes before hashing to derive
// an encryption key. Matches the Zig warp_gate implementation for interop.
const keyDerivationPrefix = "warp-gate-key-v1"

// discoveryHashPrefix is prepended to the code bytes before hashing to produce
// a discovery hash used for peer matching on the rendezvous server.
const discoveryHashPrefix = "warp-gate-discover-v1"

// GenerateWarpCode creates a new random warp code from 6 bytes of
// cryptographic randomness.
func GenerateWarpCode() (*WarpCode, error) {
	w := &WarpCode{}
	if _, err := rand.Read(w.Bytes[:]); err != nil {
		return nil, fmt.Errorf("generate warp code: %w", err)
	}
	// Normalize byte 4 to valid word index range and set byte 5 as checksum,
	// so that String() output always round-trips through ParseWarpCode().
	w.Bytes[4] = w.Bytes[4] % byte(len(wordlist))
	w.Bytes[5] = warpChecksum(w.Bytes[:5])
	return w, nil
}

// ParseWarpCode parses a warp code string like "warp-729-alpha".
// The format is: "warp-" + numeric part (bytes 0-3 as base-10) + "-" + word (bytes 4-5).
func ParseWarpCode(s string) (*WarpCode, error) {
	s = strings.TrimSpace(strings.ToLower(s))

	if !strings.HasPrefix(s, "warp-") {
		return nil, fmt.Errorf("parse warp code: must start with \"warp-\"")
	}
	rest := s[5:] // strip "warp-"

	// Find the last dash separating the number from the word.
	idx := strings.LastIndex(rest, "-")
	if idx < 0 {
		return nil, fmt.Errorf("parse warp code: expected format \"warp-<number>-<word>\"")
	}

	numStr := rest[:idx]
	word := rest[idx+1:]

	// Parse the numeric portion (encodes bytes 0-3 as a 32-bit big-endian value).
	var num uint32
	for _, ch := range numStr {
		if ch < '0' || ch > '9' {
			return nil, fmt.Errorf("parse warp code: invalid digit %q in number portion", ch)
		}
		next := uint64(num)*10 + uint64(ch-'0')
		if next > 0xFFFFFFFF {
			return nil, fmt.Errorf("parse warp code: number too large")
		}
		num = uint32(next)
	}

	// Look up the word to recover bits for bytes 4-5.
	wi, ok := wordIndex[word]
	if !ok {
		return nil, fmt.Errorf("parse warp code: unknown word %q", word)
	}

	// Encoding scheme: bytes 0-3 = numeric portion (big-endian uint32),
	// byte 4 = word index (0-31), byte 5 = checksum of bytes 0-4.
	w := &WarpCode{}
	w.Bytes[0] = byte(num >> 24)
	w.Bytes[1] = byte(num >> 16)
	w.Bytes[2] = byte(num >> 8)
	w.Bytes[3] = byte(num)
	w.Bytes[4] = byte(wi)

	// Recompute checksum and verify.
	w.Bytes[5] = warpChecksum(w.Bytes[:5])

	return w, nil
}

// warpChecksum computes a single-byte checksum over the first 5 bytes of a warp code.
// This allows detection of typos during manual entry.
func warpChecksum(b []byte) byte {
	h := sha256.Sum256(b)
	return h[0]
}

// String returns the human-readable code in the format "warp-<number>-<word>".
// The numeric portion encodes bytes 0-3, the word encodes byte 4, and byte 5
// is a checksum that is recomputed on parse for validation.
func (w *WarpCode) String() string {
	num := uint32(w.Bytes[0])<<24 | uint32(w.Bytes[1])<<16 |
		uint32(w.Bytes[2])<<8 | uint32(w.Bytes[3])

	wi := int(w.Bytes[4]) % len(wordlist)
	return fmt.Sprintf("warp-%d-%s", num, wordlist[wi])
}

// DeriveKey derives a 32-byte AES-256 encryption key from the warp code using
// SHA-256. The key derivation prefix matches the Zig warp_gate implementation
// for potential cross-implementation interop.
func (w *WarpCode) DeriveKey() [32]byte {
	data := append([]byte(keyDerivationPrefix), w.Bytes[:]...)
	return sha256.Sum256(data)
}

// Hash returns a 16-byte discovery hash for peer matching on the rendezvous
// server. Two peers with the same warp code will produce the same hash, allowing
// the rendezvous server to match them without learning the code itself.
func (w *WarpCode) Hash() [16]byte {
	data := append([]byte(discoveryHashPrefix), w.Bytes[:]...)
	full := sha256.Sum256(data)
	var h [16]byte
	copy(h[:], full[:16])
	return h
}
