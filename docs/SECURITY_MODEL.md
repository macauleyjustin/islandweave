# IslandWeave Security Model & Threat Analysis

**Release:** `v1.0.0` (`IWP/1.0`)

## 1. Security Objectives

1. **Cryptographic Identity:** Every node, Island, and service is anchored to Ed25519 public keys (`iw:node:<32-hex>`, `iw:island:<32-hex>`, `iw:service:<32-hex>`); no IP address or DNS name is trusted as identity.
2. **Confidentiality & Forward Secrecy:** All peer-to-peer links negotiate ephemeral X25519 session keys with Ed25519 mutual transcript signatures and ChaCha20-Poly1305 AEAD framing.
3. **End-to-End Delay-Tolerant Privacy:** `IWP-DTN` bundles and `IWMP` mail encrypt payloads end-to-end for the recipient's X25519 key; relays, couriers, and satellite links cannot decrypt contents.
4. **Consent-First Discovery:** Nodes default to `silent` Beacon Mode until explicitly changed by the user and never perform unconsented LAN/port scanning.
5. **Local Control & Origin Isolation:** `islandd` binds Control API on `127.0.0.1` with a 256-bit `0600` bearer token (`control.token`), separate from `apps/gateway`, and CORS disabled.
6. **Least Privilege & Rate Enforcement:** Receiving files, bulletins, or sites never executes payloads; path-traversal checks sanitize downloads; `core/security` enforces per-peer fixed-window rate limiting and records security audit events.

## 2. Threat Model

| Threat | Attack Vector | Implemented Mitigation |
|---|---|---|
| **Impersonation / MITM** | Attacker spoofs a `NodeID` or intercepts TCP | Mutual Ed25519 signatures over `ClientHello` + `ServerHello` transcript with domain separators (`IWP-LINK-1-SERVER`, `IWP-LINK-1-CLIENT`); `NodeID` verified against `SHA256(pk_node)[0:16]`. |
| **Forged Beacons / Hints** | Spoofed multicast packets or peer hints | Every beacon (`IWP-BEACON`) checks Ed25519 signature + `timestamp` skew (60s) + `ttl_sec`; `peer.exchange` hints verify `DeriveNodeID(pub) == node_id`. |
| **Replay Attacks** | Re-sending captured frames or DTN bundles | Directional AEAD keys (`i2r`, `r2i`) with strictly monotonic 64-bit counter nonces/AAD; DTN `dtn_seen` table suppresses duplicate `bundle_id`s. |
| **Unauthorized Local Control** | Malicious webpage calls `islandd` API | Control API enforces `Authorization: Bearer <token>` (`0600` file), loopback bind, and no CORS. Gateway serves untrusted sites on a separate origin. |
| **Unauthorized Peer Access** | Unknown peer initiates session | Responder checks shared `island_ids` intersection, local peer trust, and `revoked` flag before completing handshake. |
| **Resource Exhaustion / DoS** | Oversized frames, storage exhaustion, flood | 64 KiB handshake cap, 1 MiB record/bundle cap, configurable DTN courier quota (64 MiB default), bundle TTL expiration, and `core/security.RateLimiter`. |
| **Path Traversal** | Malicious `iwtp.offer` filename (`../../...`) | `iwtp.ResolveDownloadPath` normalizes to `filepath.Base` and enforces non-executable `rename`/`overwrite`/`skip` policies. |
| **Tor Clearnet Leak** | Fallback to direct TCP when Tor fails | Strict fail-closed `tor-only` policy in both `tor.DialWithPolicy` and `routing.ComputeRoutes`; SOCKS5 uses `ATYP=0x03` hostname forwarding. |

## 3. Trust, Permissions & Key Storage

- **Node Permissions (`core/security`):** Runtime toggles govern `allow_incoming_transfers`, `allow_dtn_forwarding`, `allow_auto_sync`, and `allow_peer_exchange` (inspectable/updatable via CLI `island security` or `/v1/security/permissions`).
- **Key Storage:** Node signing and X25519 encryption keys are persisted in `<data-dir>/keys/node.json` with POSIX `0600` file permissions.
