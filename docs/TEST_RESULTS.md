# IslandWeave Test Results

## Automated Test Suite (`go test -v -race -count=1 -timeout 60s ./...`)

Executed on `darwin/arm64` (`go1.27.4`) for release `v1.0.0`:

| Package | Test | Result | Duration |
|---|---|---|---|
| `apps/desktop` | `TestDesktopUIServesIndex` | PASS | 0.03s |
| `apps/navigator` | `TestResolveIslandURIAndUI` | PASS | 0.03s |
| `core/discovery` | `TestCoordinatorVerifiesPeerHints` | PASS | 0.01s |
| `core/identity` | `TestIdentitySaveLoadAndSign` | PASS | 0.03s |
| `core/islands` | `TestIslandCreateInviteAndVerify` | PASS | 0.01s |
| `core/objects` | `TestObjectStoreIngestVerifyAndAssemble` | PASS | 0.02s |
| `core/routing` | `TestComputeRoutesAndTorOnlyExclusion` | PASS | 0.00s |
| `core/security` | `TestRateLimiterAndAuditEngine` | PASS | 0.00s |
| `core/synchronization` | `TestInventoryDiffAndApplyResponse` | PASS | 0.10s |
| `core/transport` | `TestLinkHandshakeAndEncryptedMessaging` | PASS | 0.01s |
| `protocols/dtn` | `TestDTNBundleEncryptVerifyAndDecrypt` | PASS | 0.02s |
| `tests` | `TestPhase1TwoNodesDiscoverConnectMessageAndRevoke` | PASS | 0.41s |
| `tests` | `TestPhase2FileTransferIWTP` | PASS | 0.44s |
| `tests` | `TestPhase2StaticWebsiteHostingAndOfflineCache` | PASS | 0.14s |
| `tests` | `TestPhase2TransferResumeAfterReconnect` | PASS | 0.68s |
| `tests` | `TestPhase3CourierRelayNeverSimultaneouslyOnline` | PASS | 0.42s |
| `tests` | `TestPhase3PhysicalCourierExportImportAndQuota` | PASS | 0.24s |
| `tests` | `TestPhase4MailAndBulletinThroughCourier` | PASS | 0.62s |
| `tests` | `TestPhase5VPSRelayWithoutPlaintextAccess` | PASS | 0.39s |
| `tests` | `TestPhase5TorOnlyPolicyNeverFallsBackToDirect` | PASS | 0.14s |
| `tests` | `TestPhase5IslandFederation` | PASS | 0.08s |
| `tests` | `TestPhase7SatelliteContactSimulatorInterruptAndResume` | PASS | 0.17s |
| `tests` | `TestPhase8CompleteEcosystemFeatures` | PASS | 0.28s |

All 23 tests across 12 test packages passed with `-race -count=1`.

## Live Binary, Simulator & Release Verification
- `./bin/island-simulator` completed multi-node scenario cleanly (`exit 0`).
- Live CLI + `islandd` + `island-desktop` + `island-navigator` + Gateway verification passed (`host`, `share`, `gateway list`, `routes`, `security`, `diagnostics`, `/nav/resolve`).
- `make release` built cross-platform archives (`darwin-arm64.tar.gz`, `linux-amd64.tar.gz`, `linux-arm64.tar.gz`, `windows-amd64.zip`, `SHA256SUMS.txt`).
