# IslandWeave Implementation Status

**Current Milestone:** Phase 2 — File Transfer (`IWTP`) & Static Website Hosting

| Subsystem / Module | Status | Notes |
|---|---|---|
| `IWP-IDENTITY` (`core/identity`) | Implemented and tested | Ed25519 keygen, `iw:node:...` / `iw:island:...` derivation, `0600` persistence, domain-separated signing. |
| `IWP-SPACE` (`core/islands`) | Implemented and tested | Island creation, signed invite tokens, expiration & signature verification, join/leave. |
| `IWP-LINK/1` (`core/transport`) | Implemented and tested | Ephemeral X25519 + Ed25519 mutual auth + HKDF-SHA256 + ChaCha20-Poly1305 AEAD TCP transport. |
| `IWP-DISCOVER` & `IWP-BEACON` (`core/beacon`) | Implemented and tested | Signed UDP multicast (`239.255.73.87:47470`) discovery, `public`/`restricted`/`silent` modes, TTL expiration. |
| `IWP-OBJECT` (`core/objects`) | Implemented and tested | Content-addressed SHA-256 chunk store (256 KiB chunks), integrity verification on read/write, file assembly. |
| `IWP-SERVICE` (`core/services`) | Implemented and tested | Signed `ServiceRecord` (`IWP-SERVICE-1`), deterministic `iw:service:...` IDs, `SiteManifest`. |
| `IWTP/1` (`protocols/iwtp`) | Implemented and tested | Chunked encrypted file transfer over `IWP-LINK/1`, automatic resume on reconnect, conflict-safe downloads. |
| Static Website Hosting (`hosting`) | Implemented and tested | Directory ingestion, versioned `SiteManifest`, peer service discovery (`svc.list`) & object fetch (`obj.get`). |
| Local Browser Gateway (`apps/gateway`) | Implemented and tested | Isolated HTTP gateway (`127.0.0.1:47475`) serving local & cached-replica sites with CSP & no Control API access. |
| `islandd` Core Daemon (`core/daemon`, `cmd/islandd`) | Implemented and tested | SQLite state store (`modernc.org/sqlite`), P2P TCP listener, authenticated Control API, Gateway server. |
| `island` CLI (`cmd/island`) | Implemented and tested | Full Phase 1 & Phase 2 commands (`host`, `services`, `fetch`, `send`, `transfers`, `cancel-transfer`, etc.). |
| Cross-Platform Compilation | Implemented but not fully tested | Builds cleanly for `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`; runtime tested on `darwin/arm64` only. |
| `IWP-DTN` Store-and-Forward & Courier | Designed but not implemented | Scheduled for Phase 3. |
| `IWMP` Mail & `IWBN` Bulletins | Designed but not implemented | Scheduled for Phase 4. |
| VPS Relay, Tor Transport & Federation | Designed but not implemented | Scheduled for Phase 5. |
| IslandWeave Desktop & Navigator Browser | Designed but not implemented | Scheduled for Phase 6. |
| `IWST` Satellite Simulator | Designed but not implemented | Scheduled for Phase 7. |
| Wi-Fi SoftAP / BLE / Wi-Fi Direct | Blocked by hardware or platform limitations | Requires OS-specific privileged wireless adapters and hardware validation. |
