# IslandWeave Test Results

## Automated Test Suite (`go test -v -race -timeout 30s ./...`)

Executed on `darwin/arm64` (Go 1.27.4) after final Phase 1 edits:

| Package | Test | Result | Duration |
|---|---|---|---|
| `core/identity` | `TestIdentitySaveLoadAndSign` | PASS | 0.01s |
| `core/islands` | `TestIslandCreateInviteAndVerify` | PASS | 0.01s |
| `core/transport` | `TestLinkHandshakeAndEncryptedMessaging` | PASS | 0.01s |
| `tests` | `TestPhase1TwoNodesDiscoverConnectMessageAndRevoke` | PASS | 0.33s |

### Scenario Coverage
1. **Unauthenticated Control API rejection:** Verified `GET /v1/status` without bearer token returns `401 Unauthorized`.
2. **Offline LAN discovery:** Verified Node B discovers Node A via signed UDP multicast Beacon Mode announcements without internet access.
3. **Authenticated encrypted link (`IWP-LINK/1`):** Verified mutual Ed25519 + X25519 handshake, capability & Island intersection, and ChaCha20-Poly1305 message + `chat.ack` exchange.
4. **Peer revocation:** Verified revoking a peer closes the active link and rejects subsequent handshake attempts.
5. **CLI + Daemon end-to-end:** Verified compiled `island` and `islandd` binaries initialize identities, start, report status, create an Island, and stop cleanly via `SIGTERM`.
6. **Cross-compilation:** Verified `go build` succeeds for `linux/amd64`, `linux/arm64`, and `windows/amd64`.
