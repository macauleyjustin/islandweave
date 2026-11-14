# IslandWeave Implementation Status

**Current Milestone:** Phase 0 (Foundation) & Phase 1 (Working Networking Foundation)

| Subsystem / Module | Status | Notes |
|---|---|---|
| `IWP-IDENTITY` (`core/identity`) | Implemented and tested | Ed25519 keygen, `iw:node:...` / `iw:island:...` derivation, `0600` persistence, domain-separated signing. |
| `IWP-SPACE` (`core/islands`) | Implemented and tested | Island creation, signed invite tokens, expiration & signature verification, join/leave. |
| `IWP-LINK/1` (`core/transport`) | Implemented and tested | Ephemeral X25519 + Ed25519 mutual auth + HKDF-SHA256 + ChaCha20-Poly1305 AEAD TCP transport. |
| `IWP-DISCOVER` & `IWP-BEACON` (`core/beacon`) | Implemented and tested | Signed UDP multicast (`239.255.73.87:47470`) discovery, `public`/`restricted`/`silent` modes, TTL expiration. |
| `islandd` Core Daemon (`core/daemon`, `cmd/islandd`) | Implemented and tested | SQLite state store (`modernc.org/sqlite`), P2P TCP listener, authenticated local HTTP Control API. |
| `island` CLI (`cmd/island`) | Implemented and tested | Interacts with `islandd` via authenticated Control API (`init`, `start`, `stop`, `status`, `create`, `invite`, `join`, `leave`, `discover`, `peers`, `connect`, `trust`, `revoke`, `beacon`, `send-msg`, `messages`). |
| Peer Trust & Revocation | Implemented and tested | Revoked peers have active links closed and subsequent handshakes rejected. |
| Cross-Platform Compilation | Implemented but not fully tested | Builds cleanly for `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`; runtime tested on `darwin/arm64` only. |
| `IWTP` File Transfer & `IWP-OBJECT` | Designed but not implemented | Scheduled for Phase 2. |
| Static Website Hosting & Local Gateway | Designed but not implemented | Scheduled for Phase 2. |
| `IWP-DTN` Store-and-Forward & Courier | Designed but not implemented | Scheduled for Phase 3. |
| `IWMP` Mail & `IWBN` Bulletins | Designed but not implemented | Scheduled for Phase 4. |
| VPS Relay, Tor Transport & Federation | Designed but not implemented | Scheduled for Phase 5. |
| IslandWeave Desktop & Navigator Browser | Designed but not implemented | Scheduled for Phase 6. |
| `IWST` Satellite Simulator | Designed but not implemented | Scheduled for Phase 7. |
| Wi-Fi SoftAP / BLE / Wi-Fi Direct | Blocked by hardware or platform limitations | Requires OS-specific privileged wireless adapters and hardware validation. |
