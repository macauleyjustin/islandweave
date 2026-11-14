# Changelog

## v0.1.0 (Phases 0–8)
- **Phase 0 & 1:** Architecture, wire specifications, `IWP-IDENTITY`, `IWP-SPACE`, `IWP-LINK/1` encrypted TCP transport, `IWP-BEACON` UDP multicast discovery, `islandd` daemon, `island` CLI.
- **Phase 2:** `IWP-OBJECT` chunk store, `IWP-SERVICE`, `IWTP/1` resumable file transfers, static website hosting, isolated local HTTP gateway.
- **Phase 3:** `IWP-DTN` end-to-end encrypted store-and-forward bundles, quotas, deduplication, custody/delivery acks, physical `.iwcourier` export/import.
- **Phase 4:** `IWMP` decentralized mail and `IWBN` signed community bulletins.
- **Phase 5:** Headless VPS encrypted relay, Tor SOCKS5 fail-closed `tor-only` transport, Island federation, systemd/Docker deployment.
- **Phase 6:** IslandWeave Desktop Control Center (`apps/desktop`) and IslandWeave Navigator (`apps/navigator`) with `island://` URI resolution and sandboxed website isolation.
- **Phase 7:** `IWST` satellite transport HAL and multi-pass interrupt/resume Contact Simulator.
- **Phase 8:** Cross-platform service definitions (macOS launchd, Linux systemd, Windows Service), GitHub Actions CI, and full documentation.
