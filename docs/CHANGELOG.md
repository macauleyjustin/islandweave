# Changelog

## v1.0.0
- Completed `IWP-ROUTE` (`core/routing`), `IWP-SYNC` anti-entropy sync (`core/synchronization`), `IWP-DISCOVER` coordinator & peer-exchange (`core/discovery`), and `core/security` (rate limiting, permissions, security audit log, metered `TrafficStats`).
- Completed transport packages (`transports/lan`, `transports/internet`, `transports/relay`) and standalone window launcher (`platform/launch`).
- Added Shared File Libraries (`island share` / `/v1/shares` / Gateway directory listings), `island routes`, `island sync`, `island gateway list`, `island desktop`, `island navigator`, `island security`, `island activity`, and `island diagnostics`.
- Upgraded Desktop Control Center (all 14 organized views covering all 20 required areas including interactive SVG Network Map) and IslandWeave Navigator (persistent bookmarks & history).
- Added ready-to-host examples (`examples/harbor-news`, `examples/community-library`), runnable multi-node `island-simulator`, Linux installer (`platform/linux/install.sh`), and `make release` cross-platform archives (`darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`).

## v0.1.0 (Phases 0–8)
- **Phase 0 & 1:** Architecture, wire specifications, `IWP-IDENTITY`, `IWP-SPACE`, `IWP-LINK/1` encrypted TCP transport, `IWP-BEACON` UDP multicast discovery, `islandd` daemon, `island` CLI.
- **Phase 2:** `IWP-OBJECT` chunk store, `IWP-SERVICE`, `IWTP/1` resumable file transfers, static website hosting, isolated local HTTP gateway.
- **Phase 3:** `IWP-DTN` end-to-end encrypted store-and-forward bundles, quotas, deduplication, custody/delivery acks, physical `.iwcourier` export/import.
- **Phase 4:** `IWMP` decentralized mail and `IWBN` signed community bulletins.
- **Phase 5:** Headless VPS encrypted relay, Tor SOCKS5 fail-closed `tor-only` transport, Island federation, systemd/Docker deployment.
- **Phase 6:** IslandWeave Desktop Control Center (`apps/desktop`) and IslandWeave Navigator (`apps/navigator`) with `island://` URI resolution and sandboxed website isolation.
- **Phase 7:** `IWST` satellite transport HAL and multi-pass interrupt/resume Contact Simulator.
- **Phase 8:** Cross-platform service definitions (macOS launchd, Linux systemd, Windows Service), GitHub Actions CI, and full documentation.
