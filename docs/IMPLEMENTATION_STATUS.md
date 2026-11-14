# IslandWeave Implementation Status

**Current Milestone:** Phase 5 — Remote Infrastructure (VPS Relay, Tor Transport & Island Federation)

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
| `IWP-DTN` Store-and-Forward & Courier (`protocols/dtn`) | Implemented and tested | E2E X25519+ChaCha20-Poly1305 encrypted bundles, hop limits, expiration, duplicate suppression, storage quotas, custody/delivery acks, physical `.iwcourier` export/import. |
| `IWMP` Mail (`protocols/iwmp`) | Implemented and tested | E2E encrypted offline mail over `IWP-DTN`, inbox/outbox/drafts/sent, attachments, search, Digital Post Office forwarding, delivery acks. |
| `IWBN` Bulletins (`protocols/iwbn`) | Implemented and tested | Signed versioned bulletins, channel subscriptions, live & `.iwcourier` sync, verified official emergency publisher check. |
| VPS Relay (`--relay`) & Deployment (`packaging/`) | Implemented and tested | Headless relay forwards encrypted DTN bundles without plaintext access or Island admin keys; systemd & Docker configs added. |
| Tor Transport (`transports/tor`) | Implemented and tested | Pure-Go SOCKS5 dialer with `direct-first`, `tor-preferred`, and strict fail-closed `tor-only` policy. |
| Island Federation (`federation`) | Implemented and tested | Signed `IWP-FED-1` agreements with service allowlists, expiration, and revocation without merging memberships. |
| IslandWeave Desktop & Navigator Browser | Designed but not implemented | Scheduled for Phase 6. |
| `IWST` Satellite Simulator | Designed but not implemented | Scheduled for Phase 7. |
| Wi-Fi SoftAP / BLE / Wi-Fi Direct | Blocked by hardware or platform limitations | Requires OS-specific privileged wireless adapters and hardware validation. |
