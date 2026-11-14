# IslandWeave Platform Compatibility Matrix

## 1. Core Daemon (`islandd`) & CLI (`island`)

| Target | Architecture | Build Status | Runtime Tested | Notes |
|---|---|---|---|---|
| macOS | ARM64 (Apple Silicon) | Supported (native) | Tested | Primary development environment (`darwin/arm64`). |
| macOS | x86_64 | Cross-compile (`GOOS=darwin GOARCH=amd64`) | Not yet tested on hardware | Pure-Go stack requires no CGO. |
| Linux | x86_64 | Cross-compile (`GOOS=linux GOARCH=amd64`) | Not yet tested on hardware | Target for VPS, desktop, and containers. |
| Linux | ARM64 | Cross-compile (`GOOS=linux GOARCH=arm64`) | Not yet tested on hardware | Target for Raspberry Pi and ARM servers. |
| Windows | x86_64 | Cross-compile (`GOOS=windows GOARCH=amd64`) | Not yet tested on hardware | Pure-Go SQLite (`modernc.org/sqlite`) supports Windows without CGO. |
| Windows | ARM64 | Cross-compile (`GOOS=windows GOARCH=arm64`) | Not yet tested on hardware | Experimental until tested on Windows ARM64 hardware. |

## 2. Platform-Specific Hardware Capabilities

| Capability | macOS | Linux | Windows | Status |
|---|---|---|---|---|
| UDP Multicast LAN Discovery | Yes | Yes | Yes | Implemented and tested on macOS loopback/LAN. |
| TCP Encrypted Transport (`IWP-LINK`) | Yes | Yes | Yes | Implemented and tested on macOS. |
| Wi-Fi Hotspot / SoftAP Creation | Restricted by macOS APIs | `hostapd` / NetworkManager (requires privileges) | `netsh wlan` / Wi-Fi Direct | Designed; not implemented. Never auto-enabled without user approval. |
| Bluetooth LE Discovery | CoreBluetooth | BlueZ D-Bus | WinRT BLE | Designed; not implemented. |
| Wi-Fi Aware / Wi-Fi Direct | Limited / iOS-centric | `wpa_supplicant` P2P | WinRT Wi-Fi Direct | Designed; not implemented. |
| OS Keychain Integration | Keychain Services | Secret Service (`libsecret`) | Windows DPAPI / Credential Manager | Planned for Phase 8; currently uses `0600` key files. |
