# IslandWeave Security Model & Threat Analysis

## 1. Security Objectives

1. **Cryptographic Identity:** Every node and Island is anchored to an Ed25519 keypair; no IP address or DNS name is trusted as an identity.
2. **Confidentiality & Integrity:** All peer-to-peer links use ephemeral X25519 key exchange (forward secrecy) + Ed25519 mutual authentication + ChaCha20-Poly1305 AEAD.
3. **Consent-First Discovery & Beacon:** Nodes do not broadcast Beacon advertisements unless Beacon Mode is explicitly enabled (`public` or `restricted`), and never scan unrelated ports/services.
4. **Local Control Authentication:** `islandd` binds only to `127.0.0.1` by default and requires a 256-bit random bearer token (`<data-dir>/control.token`, mode `0600`) for all control API calls.
5. **Least Privilege & Isolation:** Receiving files or messages never triggers execution; untrusted web content is isolated from daemon control APIs.

## 2. Threat Model

| Threat | Attack Vector | Mitigation |
|---|---|---|
| **Impersonation / MITM** | Attacker claims another peer's NodeID on LAN | Mutual Ed25519 signature over handshake transcript (`ClientHello` + `ServerHello` + ephemeral keys + nonces). `NodeID` is derived from `SHA256(pk_node)`. |
| **Forged Beacon Advertisements** | Spoofed multicast packets advertising fake Islands/peers | Every beacon is signed by `node_priv`, includes `timestamp` and `ttl_sec`; receivers verify signature and expiration before accepting. |
| **Replay Attacks** | Re-sending captured encrypted frames or old beacons | Encrypted frames use directional keys (`i2r`, `r2i`) and strictly monotonic 64-bit counters; beacons check `timestamp + ttl_sec`. |
| **Unauthorized Local Control** | Local malicious process or webpage calling `islandd` API | Control API requires `Authorization: Bearer <token>` from `0600` token file; CORS is disabled on control API. |
| **Unauthorized Peer Access** | Unknown peer connecting and sending messages | Handshake checks `island_ids` intersection and peer authorization/trust status; unknown peers outside shared Islands are rejected unless explicitly invited/trusted. |
| **Resource Exhaustion / DoS** | Oversized handshake frames, unbounded record sizes, or spam | Strict max frame sizes (64 KiB handshake, 1 MiB record), read/write deadlines, bounded message queues, and rate/quota controls (expanded in Phase 3/5). |
| **Relay / Courier Snooping** | Malicious relay or courier reads message contents | End-to-end encryption at application/bundle layer (Phase 3/4) ensures relays and couriers only see routing metadata. |
| **Malicious Website / Browser** | Island website attempts to invoke `islandd` control API | Browser gateway isolates site origin from `islandd` control port and never injects the control bearer token into web content. |

## 3. Trust & Membership Model

- **Public Island:** Discoverable via `public` Beacon Mode; peers can join via invite or public Island policy.
- **Restricted / Private Island:** Uses `restricted` or `silent` Beacon Mode; requires a signed `InviteToken` from an Island administrator.
- **Key Storage:** Node and Island private keys are stored locally in `<data-dir>/keys/` with `0600` permissions (OS keychain integration planned in Phase 8).
