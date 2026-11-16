# IslandWeave Satellite Transport (`IWST`)

## 1. Overview
`IWST` integrates directly with `IWP-DTN` so queued mail, bulletins, and messages can be transmitted during scheduled, bandwidth-limited satellite contact windows without building a separate messaging stack.

## 2. Hardware Abstraction & Compliance
- **HAL (`protocols/iwst/iwst.go`):** Defines `GroundStationAdapter` (`Name`, `LinkCapacityBps`, `IsPassActive`, `MaxBytesPerPass`), `ContactWindow`, and `PassLog`.
- **Important:** Normal consumer Wi-Fi hardware cannot communicate directly with satellites. Real RF transmission requires authorized ground-station equipment, licensing, and satellite operator authorization.

## 3. Contact Simulator & Control Endpoints
- **Multi-Pass Simulator (`transports/satellite/simulator.go`):** `RunContactSimulation` transfers queued `IWP-DTN` bundles between two disconnected `islandd` networks across multiple bandwidth-limited `ContactWindow`s, resuming partial bundle bytes cleanly across subsequent passes.
- **Control Endpoints:** `/v1/satellite/status`, `/v1/satellite/passes` (scheduled windows + persisted SQLite `PassLog`s), and `POST /v1/satellite/simulate`.
