# IslandWeave Test Results

## Automated Test Suite (`go test -v -race -count=1 -timeout 60s ./...`)

Executed on `darwin/arm64` (Go 1.27.4) after final Phases 0–8 edits:

| Package | Test | Result | Duration |
|---|---|---|---|
| `apps/desktop` | `TestDesktopUIServesIndex` | PASS | 0.01s |
| `apps/navigator` | `TestResolveIslandURIAndUI` | PASS | 0.01s |
| `core/identity` | `TestIdentitySaveLoadAndSign` | PASS | 0.04s |
| `core/islands` | `TestIslandCreateInviteAndVerify` | PASS | 0.02s |
| `core/objects` | `TestObjectStoreIngestVerifyAndAssemble` | PASS | 0.05s |
| `core/transport` | `TestLinkHandshakeAndEncryptedMessaging` | PASS | 0.02s |
| `protocols/dtn` | `TestDTNBundleEncryptVerifyAndDecrypt` | PASS | 0.02s |
| `tests` | `TestPhase1TwoNodesDiscoverConnectMessageAndRevoke` | PASS | 0.36s |
| `tests` | `TestPhase2FileTransferIWTP` | PASS | 0.40s |
| `tests` | `TestPhase2StaticWebsiteHostingAndOfflineCache` | PASS | 0.12s |
| `tests` | `TestPhase2TransferResumeAfterReconnect` | PASS | 0.76s |
| `tests` | `TestPhase3CourierRelayNeverSimultaneouslyOnline` | PASS | 0.35s |
| `tests` | `TestPhase3PhysicalCourierExportImportAndQuota` | PASS | 0.13s |
| `tests` | `TestPhase4MailAndBulletinThroughCourier` | PASS | 0.47s |
| `tests` | `TestPhase5VPSRelayWithoutPlaintextAccess` | PASS | 0.30s |
| `tests` | `TestPhase5TorOnlyPolicyNeverFallsBackToDirect` | PASS | 0.10s |
| `tests` | `TestPhase5IslandFederation` | PASS | 0.05s |
| `tests` | `TestPhase7SatelliteContactSimulatorInterruptAndResume` | PASS | 0.10s |

All 18 tests across 8 test packages passed with `-race -count=1`. Cross-compilation succeeded for `linux/amd64`, `linux/arm64`, and `windows/amd64`.
