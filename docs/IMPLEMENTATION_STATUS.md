# IslandWeave Implementation Status

**Current Release:** v1.0.0 (All Core Software Subsystems Complete)

| Subsystem / Module | Status | Notes |
|---|---|---|
| `IWP-IDENTITY` (`core/identity`) | Implemented and tested | Ed25519 signing + X25519 encryption keypairs, `iw:node:...` / `iw:island:...`, `0600` persistence (`keys/node.json`). |
| `IWP-SPACE` (`core/islands`) | Implemented and tested | Island creation, signed invite tokens, expiration & signature verification, join/leave. |
| `IWP-LINK/1` (`core/transport`) | Implemented and tested | Ephemeral X25519 + Ed25519 mutual auth + HKDF-SHA256 + ChaCha20-Poly1305 AEAD TCP transport. |
| `IWP-DISCOVER` & `IWP-BEACON` (`core/discovery`, `core/beacon`) | Implemented and tested | Signed UDP multicast (`239.255.73.87:47470`), bootstrap peers, authenticated `peer.exchange`, TTL & skew guards. |
| `IWP-ROUTE` (`core/routing`) | Implemented and tested | Multi-transport route scoring (`lan-tcp`, `wan-tcp`, `tor-onion`, `dtn-only`) with strict `tor-only` exclusion. |
| `IWP-SYNC` (`core/synchronization`) | Implemented and tested | Anti-entropy `sync.inv` / `sync.req` / `sync.resp` reconciliation + automatic sync on connect & history. |
| `Security & Bandwidth` (`core/security`) | Implemented and tested | Per-peer rate limiter, security audit ring, node permission flags, and metered TX/RX byte accounting. |
| `IWP-OBJECT` (`core/objects`) | Implemented and tested | Content-addressed SHA-256 chunk store (256 KiB chunks), integrity verification, file assembly. |
| `IWP-SERVICE` & Hosting (`core/services`, `hosting`) | Implemented and tested | Signed `ServiceRecord` (`IWP-SERVICE-1`), `SiteManifest`, static websites (`website`) & browsable Shared Libraries (`share`). |
| `IWTP/1` (`protocols/iwtp`) | Implemented and tested | Chunked encrypted file transfer over `IWP-LINK/1`, automatic resume on reconnect, conflict-safe downloads. |
| `Local Browser Gateway` (`apps/gateway`) | Implemented and tested | Isolated HTTP gateway (`127.0.0.1:47475`) serving websites, shared directory listings, & offline cached replicas. |
| `IWP-DTN` Store-and-Forward & Courier (`protocols/dtn`) | Implemented and tested | E2E X25519+ChaCha20-Poly1305 encrypted bundles, hop limits, expiration, deduplication, quotas, physical `.iwcourier` export/import. |
| `IWMP` Mail (`protocols/iwmp`) | Implemented and tested | E2E encrypted offline mail over `IWP-DTN`, inbox/outbox/drafts/sent, attachments, search, delivery acks. |
| `IWBN` Bulletins (`protocols/iwbn`) | Implemented and tested | Signed versioned bulletins, channel subscriptions, live & `.iwcourier` sync, verified official emergency check. |
| `Transports` (`transports/lan`, `internet`, `relay`, `tor`) | Implemented and tested | LAN subnet classifier, direct TCP helper, relay metadata, and fail-closed RFC 1928 Tor SOCKS5 dialer. |
| `Island Federation` (`federation`) | Implemented and tested | Signed `IWP-FED-1` agreements with service allowlists, expiration, and revocation. |
| `Desktop Control Center` (`apps/desktop`) | Implemented and tested | All Control Center tabs including interactive SVG Network Map, Shared Libraries, Federation, Security, Bandwidth & Diagnostics. |
| `IslandWeave Navigator` (`apps/navigator`) | Implemented and tested | `island://` URI resolver, tabs, persisted history & bookmarks, native views, sandboxed website isolation. |
| `IWST` & Simulator (`protocols/iwst`, `transports/satellite`, `simulator`) | Implemented and tested | Ground-station HAL, pass logging, multi-pass interrupt/resume simulator, and runnable `island-simulator` scenario. |
| `Packaging & Release` (`Makefile`, `platform/`, `packaging/`) | Implemented and cross-compiled | Release tarballs/zips & SHA256SUMS for `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`; macOS/Linux/Windows service installers. |
| `Wi-Fi SoftAP / BLE / Real RF Hardware` | Hardware-gated | Requires OS-privileged wireless adapters / licensed RF ground-station equipment. |
