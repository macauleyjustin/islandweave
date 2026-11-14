# IslandWeave Implementation Status

**Current Milestone:** Phases 0–8 Complete (v0.1.0)

| Subsystem / Module | Status | Notes |
|---|---|---|
| `IWP-IDENTITY` (`core/identity`) | Implemented and tested | Ed25519 signing + X25519 encryption keypairs, `iw:node:...` / `iw:island:...`, `0600` persistence. |
| `IWP-SPACE` (`core/islands`) | Implemented and tested | Island creation, signed invite tokens, expiration & signature verification, join/leave. |
| `IWP-LINK/1` (`core/transport`) | Implemented and tested | Ephemeral X25519 + Ed25519 mutual auth + HKDF-SHA256 + ChaCha20-Poly1305 AEAD TCP transport. |
| `IWP-DISCOVER` & `IWP-BEACON` (`core/beacon`) | Implemented and tested | Signed UDP multicast (`239.255.73.87:47470`) discovery, `public`/`restricted`/`silent` modes, TTL expiration. |
| `IWP-OBJECT` (`core/objects`) | Implemented and tested | Content-addressed SHA-256 chunk store (256 KiB chunks), integrity verification, file assembly. |
| `IWP-SERVICE` (`core/services`) | Implemented and tested | Signed `ServiceRecord` (`IWP-SERVICE-1`), deterministic `iw:service:...` IDs, `SiteManifest`. |
| `IWTP/1` (`protocols/iwtp`) | Implemented and tested | Chunked encrypted file transfer over `IWP-LINK/1`, automatic resume on reconnect, conflict-safe downloads. |
| Static Website Hosting (`hosting`) | Implemented and tested | Directory ingestion, versioned `SiteManifest`, peer service discovery & object fetch. |
| Local Browser Gateway (`apps/gateway`) | Implemented and tested | Isolated HTTP gateway (`127.0.0.1:47475`) serving local & cached-replica sites with CSP & no Control API access. |
| `IWP-DTN` Store-and-Forward & Courier (`protocols/dtn`) | Implemented and tested | E2E X25519+ChaCha20-Poly1305 encrypted bundles, hop limits, expiration, deduplication, quotas, physical `.iwcourier` export/import. |
| `IWMP` Mail (`protocols/iwmp`) | Implemented and tested | E2E encrypted offline mail over `IWP-DTN`, inbox/outbox/drafts/sent, attachments, search, delivery acks. |
| `IWBN` Bulletins (`protocols/iwbn`) | Implemented and tested | Signed versioned bulletins, channel subscriptions, live & `.iwcourier` sync, verified official emergency check. |
| VPS Relay (`--relay`) & Deployment (`packaging/`) | Implemented and tested | Headless relay forwards encrypted DTN bundles without plaintext access or Island admin keys; systemd & Docker configs. |
| Tor Transport (`transports/tor`) | Implemented and tested | Pure-Go SOCKS5 dialer with `direct-first`, `tor-preferred`, and strict fail-closed `tor-only` policy. |
| Island Federation (`federation`) | Implemented and tested | Signed `IWP-FED-1` agreements with service allowlists, expiration, and revocation. |
| Desktop Control Center (`apps/desktop`) | Implemented and tested | Embedded offline-capable Control Center UI proxying authenticated requests to `islandd`. |
| IslandWeave Navigator (`apps/navigator`) | Implemented and tested | `island://` URI resolver, tabs, history, bookmarks, native views, sandboxed website isolation. |
| `IWST` Satellite Simulator (`protocols/iwst`, `transports/satellite`) | Implemented and tested | Ground-station HAL and multi-pass bandwidth-limited interrupt/resume Contact Simulator. |
| Cross-Platform Packaging & CI (`.github/workflows/ci.yml`, `platform/`) | Implemented but not fully tested | Builds cleanly for `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`; runtime tested on `darwin/arm64` only. |
| Wi-Fi SoftAP / BLE / Wi-Fi Direct & Real RF Satellite Hardware | Blocked by hardware or platform limitations | Requires OS-specific privileged wireless adapters / licensed RF ground-station hardware. |
