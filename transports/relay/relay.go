package relay

import "strings"

// Info summarizes relay operational capabilities for headless VPS and community relay nodes.
type Info struct {
	Enabled        bool   `json:"enabled"`
	Role           string `json:"role"` // "relay" or "edge-node"
	EncryptionNote string `json:"encryption_note"`
}

// BuildInfo constructs relay metadata for control/status reporting.
func BuildInfo(enabled bool) Info {
	role := "edge-node"
	if enabled {
		role = "relay"
	}
	return Info{
		Enabled:        enabled,
		Role:           role,
		EncryptionNote: "IWP-DTN payloads are end-to-end encrypted for destination X25519 keys; relays observe routing headers only.",
	}
}

// IsOnionAddr reports whether endpoint uses a Tor hidden service hostname.
func IsOnionAddr(addr string) bool {
	return strings.Contains(strings.ToLower(addr), ".onion")
}
