# IslandWeave Protocol (`IWP/1.0`) Specification Summary

**Full Normative Specifications:** [macauleyjustin/islandweave-specifications](https://github.com/macauleyjustin/islandweave-specifications) (`IWP-0001` through `IWP-0011` + JSON Schemas & conformance derivation vectors).

---

## 1. Cryptographic Primitives & Identifiers (`IWP-0001`)

- **Signatures:** Ed25519 (RFC 8032)
- **Key Exchange:** Ephemeral X25519 (RFC 7748)
- **KDF:** HKDF-SHA256 (RFC 5869)
- **AEAD:** ChaCha20-Poly1305 (RFC 8439)
- **Hashes:** SHA-256 (FIPS 180-4)

Identifiers:
```text
NodeID    = "iw:node:"    + lowercase_hex(SHA256(pk_node)[0:16])
IslandID  = "iw:island:"  + lowercase_hex(SHA256(pk_island)[0:16])
ServiceID = "iw:service:" + lowercase_hex(SHA256(PublisherID || 0x00 || name)[0:16])
```

Domain Separators:
`IWP-LINK-1-SERVER`, `IWP-LINK-1-CLIENT`, `iwp-link-1-i2r`, `iwp-link-1-r2i`, `IWP-SERVICE-1`, `IWP-DTN-BUNDLE-1`, `IWP-IWBN-1`, `IWP-FED-1`.

## 2. Implemented Wire Modules

| Spec ID | Module | Transport / Wire Format |
|---|---|---|
| `IWP-0002` | `IWP-LINK/1` | 3-step mutual auth handshake (`ClientHello`, `ServerHello`, `ClientAuth`), 64 KiB max handshake frame, ChaCha20-Poly1305 encrypted `Envelope` records (1 MiB max plaintext, monotonic 64-bit counter nonce/AAD). |
| `IWP-0003` | `IWP-DISCOVER` / `IWP-BEACON` | Signed UDP multicast beacons (`239.255.73.87:47470`, `public`/`restricted`/`silent`), 60s clock-skew & TTL checks, plus authenticated `peer.exchange` gossip over `IWP-LINK/1`. |
| `IWP-0004` | `IWP-SPACE` / `IWP-BRIDGE` | Signed `InviteToken` capabilities and bilateral `IWP-FED-1` `Agreement`s gating `allow_bulletin_sync`, `allow_dtn_forward`, `shared_services`, and `allowed_identities`. |
| `IWP-0005` | `IWP-OBJECT` / `IWP-SERVICE` / `IWTP/1` | 256 KiB SHA-256 chunks, signed `ServiceRecord` (`website` and `share` types), `SiteManifest`, `iwtp.*` chunked transfer messages, `obj.get`/`obj.data`, `svc.list`/`svc.list_resp`. |
| `IWP-0006` | `IWP-DTN` / `IWMP` / `IWBN` | X25519+ChaCha20-Poly1305 E2E encrypted store-and-forward `Bundle`s (8 max hops, 7d TTL, 64 MiB default courier quota), `.iwcourier` `CourierPack`s, `MailPayload` (`inbox`/`outbox`/`drafts`/`sent`), signed `Bulletin`s (`news`/`article`/`notice`/`event`/`alert`). |
| `IWP-0007` | Tor / Relay / `IWST` | RFC 1928 SOCKS5 (`direct-first`, `tor-preferred`, fail-closed `tor-only`), `--relay` encrypted DTN forwarding, `GroundStationAdapter` HAL + resumable `ContactWindow` pass simulator. |
| `IWP-0008` | Navigator & Gateway | `island://` ABNF URI grammar resolving native views (`home`, `mail`, `news`, `files`, `downloads`, `peers`, `services`), hosted websites, and shared libraries in isolated sandboxed contexts. |
| `IWP-0010` | `IWP-ROUTE` / `IWP-SYNC` | Multi-transport route scoring (`lan-tcp`, `wan-tcp`, `tor-onion`, `dtn-only`) and anti-entropy `sync.inv` / `sync.req` / `sync.resp` reconciliation. |
