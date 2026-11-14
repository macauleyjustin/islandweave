# islandd Local Control API Reference

All endpoints require `Authorization: Bearer <token>` from `<data-dir>/control.token` (`0600`). Default bind address: `127.0.0.1:47471`.

- `GET /v1/status` — Node identity, keys, endpoints, Beacon status, link counts
- `GET|POST /v1/islands` — List or create Islands
- `POST /v1/islands/invite` — Create signed `InviteToken`
- `POST /v1/islands/join` — Verify and join Island
- `POST /v1/islands/leave` — Leave Island
- `GET /v1/peers` — List known/connected peers
- `POST /v1/peers/connect` — Connect via `addr` or `onion_addr`
- `POST /v1/peers/trust` / `POST /v1/peers/revoke` — Trust or revoke peer
- `POST /v1/peers/import-key` — Import offline peer's Ed25519 + X25519 public keys
- `GET /v1/discover` — List Beacon-discovered peers
- `GET|POST /v1/beacon` — Inspect or configure Beacon Mode
- `GET /v1/messages` / `POST /v1/messages/send` — Direct encrypted messaging
- `GET /v1/services` / `POST /v1/services/host` / `POST /v1/services/fetch` — Static website hosting & fetching
- `GET /v1/transfers` / `POST /v1/transfers/send` / `POST /v1/transfers/cancel` — IWTP file transfers
- `GET /v1/courier/status` / `GET /v1/courier/queue` / `POST /v1/courier/send` / `POST /v1/courier/export` / `POST /v1/courier/import` — DTN & physical courier mode
- `GET /v1/mail` / `POST /v1/mail/send` — IWMP mail (inbox/outbox/drafts/sent/search)
- `GET /v1/bulletins` / `POST /v1/bulletins/publish` / `POST /v1/bulletins/subscribe` — IWBN bulletins
- `POST /v1/server/enable` — Toggle headless VPS relay mode
- `GET /v1/tor/status` / `POST /v1/tor/config` — Configure Tor SOCKS5 & policy
- `GET /v1/federation` / `POST /v1/federation/create` / `POST /v1/federation/import` / `POST /v1/federation/revoke` — Island federation
- `GET /v1/satellite/status` / `GET /v1/satellite/passes` — IWST satellite status & simulated passes
