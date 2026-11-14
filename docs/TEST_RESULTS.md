# IslandWeave Test Results

## Automated Test Suite (`go test -v -race -count=1 -timeout 30s ./...`)

Executed on `darwin/arm64` (Go 1.27.4) after final Phase 2 edits:

| Package | Test | Result | Duration |
|---|---|---|---|
| `core/identity` | `TestIdentitySaveLoadAndSign` | PASS | 0.01s |
| `core/islands` | `TestIslandCreateInviteAndVerify` | PASS | 0.01s |
| `core/objects` | `TestObjectStoreIngestVerifyAndAssemble` | PASS | 0.01s |
| `core/transport` | `TestLinkHandshakeAndEncryptedMessaging` | PASS | 0.01s |
| `tests` | `TestPhase1TwoNodesDiscoverConnectMessageAndRevoke` | PASS | 0.28s |
| `tests` | `TestPhase2FileTransferIWTP` | PASS | 0.37s |
| `tests` | `TestPhase2StaticWebsiteHostingAndOfflineCache` | PASS | 0.06s |
| `tests` | `TestPhase2TransferResumeAfterReconnect` | PASS | 0.38s |

### Verified Scenarios
1. **Unauthenticated Control API rejection:** `GET /v1/status` without bearer token returns `401 Unauthorized`.
2. **Offline LAN discovery & encrypted messaging:** Two `islandd` nodes discover via UDP multicast Beacon, authenticate via `IWP-LINK/1`, exchange encrypted messages, and receive `chat.ack`.
3. **Peer revocation:** Revoking a peer closes the active link and rejects subsequent handshake attempts.
4. **IWTP chunked file transfer & verification:** 650 KiB multi-chunk file transfers over `IWP-LINK/1`, verifies every SHA-256 chunk, and reassembles byte-for-byte.
5. **IWTP reconnect & resume:** Connection drop/reconnect mid-transfer triggers `resumePendingTransfers` and completes with exact SHA-256 match.
6. **Static website hosting, gateway isolation, and offline replica:** Node A hosts a multi-file static site; Node B fetches and serves it via `apps/gateway`; gateway returns `404` on `/v1/status` (no privileged daemon access); after Node A stops, Node B's gateway continues serving the cached site offline.
7. **Cross-compilation:** `go build ./cmd/islandd ./cmd/island` succeeds for `darwin/arm64`, `linux/amd64`, `linux/arm64`, and `windows/amd64`.
