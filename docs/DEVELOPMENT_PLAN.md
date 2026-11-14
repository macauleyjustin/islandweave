# IslandWeave Development Plan

## Phase 0 — Architecture & Project Foundation
- System architecture, wire specifications, security threat model, platform compatibility matrix, repository layout, and Go module initialization.

## Phase 1 — Working Networking Foundation (Current Target)
- `islandd` headless core daemon with SQLite state store and authenticated local control API.
- `island` CLI communicating with `islandd` via the control API.
- `IWP-IDENTITY`: Ed25519 node keys and `iw:node:...` identifiers.
- `IWP-SPACE`: Island creation, signed invite tokens, membership, peer trust/revocation.
- `IWP-LINK`: Ephemeral X25519 + Ed25519 mutual authentication + ChaCha20-Poly1305 encrypted TCP transport.
- `IWP-DISCOVER` & `IWP-BEACON`: Signed UDP multicast LAN discovery and Beacon Mode controls (`public`, `restricted`, `silent`).
- Direct & Island messaging (`chat.msg`, `chat.ack`) over authenticated encrypted links.
- **Acceptance Test:** Two local `islandd` nodes discover each other on LAN/loopback without internet, authenticate via `IWP-LINK`, and exchange messages.

## Phase 2 — File Transfer (IWTP) & Static Website Hosting
- `IWP-OBJECT` content-addressed SHA-256 chunk store.
- `IWTP` resumable, chunk-verified, encrypted file transfer.
- Static website hosting (`island host`), service identities (`IWP-SERVICE`), and local browser gateway (`apps/gateway`).

## Phase 3 — Delay-Tolerant Store-and-Forward (IWP-DTN)
- Persistent outbound queues, bundle expiration, hop limits, duplicate suppression, custody acknowledgments, and physical courier export/import.

## Phase 4 — Mail (IWMP) & Bulletins (IWBN)
- Decentralized offline mail (inbox, outbox, drafts, post-office nodes) and signed community publications/bulletins.

## Phase 5 — Remote Infrastructure, Relay, Tor & Federation
- Headless VPS mode, encrypted relay services, Tor SOCKS5/Onion transport with strict Tor-only policy, and signed Island federation agreements.

## Phase 6 — Desktop Control Center & IslandWeave Navigator
- Cross-platform GUI Control Center and dedicated Navigator browser with `island://` resolution and origin isolation.

## Phase 7 — Satellite Transport (IWST) & Contact Simulator
- Contact-window scheduler, link-capacity estimator, and multi-pass interrupt/resume simulator.

## Phase 8 — Cross-Platform Packaging & Hardening
- OS installers, systemd/launchd/Windows Service definitions, OS keychain integration, security audit, and release CI.
