# IslandWeave Satellite Transport (`IWST`)

## 1. Overview
`IWST` integrates with `IWP-DTN` so queued mail, bulletins, and messages can be transmitted during scheduled, bandwidth-limited satellite contact windows without building a separate messaging stack.

## 2. Hardware Abstraction & Compliance
- **HAL (`protocols/iwst/iwst.go`):** Defines `GroundStationAdapter` (`Name`, `LinkCapacityBps`, `IsPassActive`, `MaxBytesPerPass`) and `ContactWindow`.
- **Important:** Normal Wi-Fi hardware cannot communicate directly with satellites. Real RF transmission requires authorized ground-station equipment, licensing, and satellite operator authorization.

## 3. Contact Simulator (`transports/satellite/simulator.go`)
`RunContactSimulation` transfers queued `IWP-DTN` bundles between two disconnected `islandd` networks across multiple simulated `ContactWindow`s. When a pass's `MaxBytes` budget is exhausted mid-bundle, partial byte progress is preserved and resumes cleanly in subsequent passes.
