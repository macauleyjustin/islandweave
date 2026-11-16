# islandd Local Control API Reference

All endpoints require `Authorization: Bearer <token>` from `<data-dir>/control.token` (`0600`). Default bind address: `127.0.0.1:47471`.

## Core Node, Islands & Discovery
- `GET /v1/status` — Node identity, keys, endpoints, Beacon status, relay flag, traffic counters
- `GET|POST /v1/islands` — List or create Islands
- `POST /v1/islands/invite` — Create signed `InviteToken`
- `POST /v1/islands/join` — Verify and join Island
- `POST /v1/islands/leave` — Leave Island
- `GET /v1/peers` — List known/connected peers
- `POST /v1/peers/connect` — Connect via `addr` or `onion_addr`
- `POST /v1/peers/trust` / `POST /v1/peers/revoke` — Trust or revoke peer
- `POST /v1/peers/import-key` — Import offline peer's Ed25519 + X25519 public keys
- `GET /v1/discover` / `GET /v1/discover/merged` — Beacon-only or merged (Beacon + Peer-Exchange) discovery
- `GET|POST /v1/beacon` — Inspect or configure Beacon Mode

## Routing, Synchronization & Content
- `GET /v1/routes` — Multi-transport `IWP-ROUTE` routing table
- `POST /v1/sync` / `GET /v1/sync/history` — Trigger `IWP-SYNC` reconciliation and inspect sync history
- `GET /v1/messages` / `POST /v1/messages/send` — Direct encrypted messaging
- `GET /v1/services` / `POST /v1/services/host` / `POST /v1/services/fetch` — Static website hosting & fetching
- `POST /v1/shares` — Publish a browsable Shared File Library (`type: "share"`)
- `GET /v1/gateway` — Gateway endpoints and relay metadata
- `GET /v1/transfers` / `POST /v1/transfers/send` / `POST /v1/transfers/cancel` — IWTP file transfers

## DTN, Mail, Bulletins & Infrastructure
- `GET /v1/courier/status` / `GET /v1/courier/queue` / `POST /v1/courier/send` / `POST /v1/courier/export` / `POST /v1/courier/import` — DTN & physical `.iwcourier` packs
- `GET /v1/mail` / `POST /v1/mail/send` — IWMP mail (inbox/outbox/drafts/sent/search)
- `GET /v1/bulletins` / `POST /v1/bulletins/publish` / `POST /v1/bulletins/subscribe` — IWBN bulletins
- `POST /v1/server/enable` — Toggle headless VPS relay mode
- `GET /v1/tor/status` / `POST /v1/tor/config` — Configure Tor SOCKS5 & policy (`direct-first`, `tor-preferred`, `tor-only`)
- `GET /v1/federation` / `POST /v1/federation/create` / `POST /v1/federation/import` / `POST /v1/federation/revoke` — Island federation (`IWP-FED-1`)
- `GET /v1/satellite/status` / `GET /v1/satellite/passes` / `POST /v1/satellite/simulate` — IWST satellite status, pass logs & simulation

## Security, Navigator & Diagnostics
- `GET /v1/security/status` / `GET /v1/security/audit` / `GET|POST /v1/security/permissions` — Keystore posture, audit ring, node permissions
- `GET|POST /v1/bandwidth` — Live P2P TX/RX counters & optional hourly cap
- `GET /v1/activity` — Persisted node activity history
- `GET /v1/diagnostics` — Runtime OS/arch, memory, uptime, SQLite DB size, active links
- `GET|POST|DELETE /v1/navigator/bookmarks` — Persisted Navigator bookmarks
- `GET|POST /v1/navigator/history` — Persisted Navigator history
