# IslandWeave Protocol (IWP) Wire Specification

**Protocol Version:** `1` (`IWP/1.0`)

## 1. Identifiers & Cryptography

- **Signature Algorithm:** Ed25519 (RFC 8032).
- **Key Exchange:** Ephemeral X25519 (RFC 7748).
- **Key Derivation:** HKDF-SHA256 (RFC 5869).
- **AEAD Cipher:** ChaCha20-Poly1305 (RFC 8439).
- **Hash:** SHA-256.

### 1.1 Node ID (`IWP-IDENTITY`)
A Node ID is derived from the node's 32-byte Ed25519 public key `pk_node`:
```text
NodeID = "iw:node:" + lowercase_hex(SHA256(pk_node)[:16])
```
(128-bit collision-resistant identifier with full 256-bit public key exchanged during handshake and discovery.)

### 1.2 Island ID (`IWP-SPACE`)
An Island is identified by a 32-byte Ed25519 root public key `pk_island`:
```text
IslandID = "iw:island:" + lowercase_hex(SHA256(pk_island)[:16])
```

## 2. IWP-LINK Handshake & Encrypted Framing

All peer-to-peer TCP connections begin with the **IWP-LINK/1** handshake.

### 2.1 Handshake Wire Format
Messages are length-prefixed: `[4-byte big-endian uint32 length][JSON payload]`. Max handshake frame size: 64 KiB.

1. **ClientHello (Initiator -> Responder):**
   - `version`: `1`
   - `ephemeral_pub`: 32-byte X25519 public key (base64)
   - `node_pub`: 32-byte Ed25519 public key (base64)
   - `nonce`: 32-byte random nonce (base64)
   - `capabilities`: `["iwp.msg/1"]`
   - `island_ids`: list of IslandIDs the initiator is requesting to communicate within

2. **ServerHello (Responder -> Initiator):**
   - `version`: `1`
   - `ephemeral_pub`: 32-byte X25519 public key (base64)
   - `node_pub`: 32-byte Ed25519 public key (base64)
   - `nonce`: 32-byte random nonce (base64)
   - `capabilities`: negotiated intersection of capabilities
   - `island_ids`: mutually authorized IslandIDs
   - `signature`: Ed25519 signature over `SHA256("IWP-LINK-1-SERVER" || ClientHelloCanonical || ServerHelloBodyCanonical)`

3. **ClientAuth (Initiator -> Responder):**
   - `signature`: Ed25519 signature over `SHA256("IWP-LINK-1-CLIENT" || ClientHelloCanonical || ServerHelloBodyCanonical)`

### 2.2 Session Key Derivation
Let `shared_secret = X25519(eph_priv, peer_eph_pub)`.
Let `salt = SHA256(client_nonce || server_nonce)`.
Using HKDF-SHA256 with `ikm = shared_secret`, `salt = salt`:
- `key_i2r = HKDF-Expand(prk, "iwp-link-1-i2r", 32)` (Initiator-to-Responder ChaCha20-Poly1305 key)
- `key_r2i = HKDF-Expand(prk, "iwp-link-1-r2i", 32)` (Responder-to-Initiator ChaCha20-Poly1305 key)

### 2.3 Encrypted Record Framing
After `ClientAuth` is verified, all subsequent traffic uses AEAD records:
```text
[4-byte big-endian uint32 ciphertext_len][ciphertext + 16-byte Poly1305 tag]
```
- **Nonce (12 bytes):** `[4 zero bytes][8-byte big-endian monotonic uint64 counter]` (starts at 0, incremented by 1 per frame).
- **Additional Authenticated Data (AAD):** `[8-byte big-endian uint64 counter]`.
- Max record plaintext size: 1 MiB.

### 2.4 Inner Application Frame
Decrypted plaintext is a JSON-encoded `Envelope`:
```json
{
  "version": 1,
  "type": "chat.msg",
  "id": "uuid-v4",
  "island_id": "iw:island:...",
  "sender_id": "iw:node:...",
  "timestamp": 1794749770,
  "payload": { ... }
}
```
Supported Phase 1 frame types:
- `chat.msg`: `{ "body": "text", "reply_to": "" }`
- `chat.ack`: `{ "msg_id": "uuid-v4" }`
- `ping` / `pong`: `{ "seq": 1 }`

## 3. IWP-DISCOVER / IWP-BEACON Wire Format

Sent over UDP multicast `239.255.73.87:47470` (configurable). Max datagram: 4096 bytes.

```json
{
  "version": 1,
  "node_id": "iw:node:...",
  "node_pub": "<base64 Ed25519 pubkey>",
  "node_name": "alice-laptop",
  "listen_port": 47472,
  "beacon_mode": "public | restricted | silent",
  "island_id": "iw:island:...",
  "island_name": "Community Net",
  "timestamp": 1794749770,
  "ttl_sec": 30,
  "capabilities": ["iwp.msg/1"],
  "signature": "<base64 Ed25519 signature over canonical announcement body>"
}
```
Receivers MUST verify `NodeID == DeriveNodeID(node_pub)`, check `timestamp + ttl_sec >= now` (within clock skew tolerance of 60s), and verify `signature` before recording the peer.

## 4. IWP-SPACE Signed Invitations

An Island administrator issues an `InviteToken`:
```json
{
  "version": 1,
  "island_id": "iw:island:...",
  "island_name": "Community Net",
  "island_pub": "<base64 Ed25519 pubkey>",
  "inviter_id": "iw:node:...",
  "bootstrap_addrs": ["192.168.1.10:47472"],
  "expires_at": 1794836170,
  "max_uses": 0,
  "nonce": "<random hex>",
  "signature": "<base64 Ed25519 signature by island_priv>"
}
```
Joining nodes verify `island_pub` matches `island_id`, verify `signature`, and store the Island membership locally.
