# IslandWeave Development Plan & Roadmap

## Completed Milestones (Phases 0–8 & Release `v1.0.0`)

- **Phase 0 — Architecture & Foundation:** Cryptographic identity model, protocol family specs (`IWP-0001`–`IWP-0011`), threat model, and pure-Go project structure. **Completed.**
- **Phase 1 — Core Networking Engine (`islandd`, `island` CLI):** SQLite store, authenticated Control API, `IWP-IDENTITY`, `IWP-SPACE` signed invite tokens, `IWP-LINK/1` mutual auth + ChaCha20-Poly1305 encrypted framing, `IWP-BEACON` multicast discovery, encrypted chat. **Completed.**
- **Phase 2 — `IWP-OBJECT`, `IWTP/1` & Website Hosting:** 256 KiB SHA-256 chunk store, resumable conflict-safe file transfers, static websites (`island host`) and browsable Shared File Libraries (`island share`), and isolated local HTTP gateway (`apps/gateway`). **Completed.**
- **Phase 3 — Delay-Tolerant Store-and-Forward (`IWP-DTN`):** End-to-end X25519+ChaCha20-Poly1305 bundles, hop limits, TTL expiration, deduplication, storage quotas, custody/delivery acks, and physical `.iwcourier` export/import. **Completed.**
- **Phase 4 — Mail (`IWMP`) & Community Bulletins (`IWBN`):** Encrypted offline mailbox (`inbox`, `outbox`, `drafts`, `sent`) and signed, versioned, syncable bulletins. **Completed.**
- **Phase 5 — Relay, Tor & Federation:** Headless VPS `--relay` mode, fail-closed `tor-only` SOCKS5 integration, bilateral `IWP-FED-1` federation agreements, systemd & Docker deployment. **Completed.**
- **Phase 6 — Desktop Control Center & IslandWeave Navigator:** Standalone browser/app-window UIs (`island-desktop`, `island-navigator`) with interactive SVG topology map, persisted bookmarks/history, and sandboxed origin isolation. **Completed.**
- **Phase 7 — Satellite Transport (`IWST`) & Scenario Simulator:** Ground-station HAL, pass log persistence, multi-pass interrupt/resume simulator, and runnable `island-simulator` binary. **Completed.**
- **Phase 8 & `v1.0.0` Completion:** Implemented `IWP-ROUTE` (`core/routing`), `IWP-SYNC` (`core/synchronization`), `IWP-DISCOVER` peer-exchange coordinator (`core/discovery`), `core/security` (rate limits, permissions, audit log, metered TX/RX counters), `transports/{lan,internet,relay}`, Linux desktop/systemd installer, sample sites (`examples/`), and cross-platform release packaging (`make release`). **Completed.**

## Future Hardware & Optional Extensions

- **Direct RF / OS Wireless Adapters:** Privileged OS adapters for BLE discovery, Wi-Fi Aware / Direct, SoftAP hotspot provisioning, and licensed satellite RF ground-station drivers.
- **Keychain & Stream Adapters:** Native OS keychain backends (macOS Keychain Services / Linux `libsecret` / Windows DPAPI) and secondary QUIC stream transport alongside `IWP-LINK/1` over TCP.
- **Target-OS Runtime CI:** Automated integration runs on physical Linux (`amd64`/`arm64`) and Windows (`amd64`) hosts.
