# IslandWeave Platform Compatibility Matrix

## 1. Software Components (`islandd`, `island`, `island-desktop`, `island-navigator`)

| Target | Architecture | Build & Archive (`v1.0.0`) | Runtime Tested | Notes |
|---|---|---|---|---|
| macOS | ARM64 (Apple Silicon) | Native build & `islandweave-v1.0.0-darwin-arm64.tar.gz` | Tested (`go1.27.4`) | Primary development & verification platform; includes `platform/macos/com.islandweave.islandd.plist`. |
| macOS | x86_64 | Cross-compile (`GOOS=darwin GOARCH=amd64`) | Not tested on hardware | Pure-Go stack requires no CGO. |
| Linux | x86_64 | Cross-compiled `islandweave-v1.0.0-linux-amd64.tar.gz` | Cross-compile verified | Target for VPS, desktop, and Docker; includes `platform/linux/install.sh` & `.desktop` entries. |
| Linux | ARM64 | Cross-compiled `islandweave-v1.0.0-linux-arm64.tar.gz` | Cross-compile verified | Target for Raspberry Pi and ARM64 servers. |
| Windows | x86_64 | Cross-compiled `islandweave-v1.0.0-windows-amd64.zip` | Cross-compile verified | Pure-Go `modernc.org/sqlite` needs no CGO; includes `platform/windows/install-service.ps1`. |
| Windows | ARM64 | Cross-compile (`GOOS=windows GOARCH=arm64`) | Not tested on hardware | Experimental until verified on Windows ARM64 hardware. |

## 2. Transport & Platform Hardware Capabilities

| Capability | macOS | Linux | Windows | Status |
|---|---|---|---|---|
| UDP Multicast LAN Discovery (`IWP-BEACON`) | Yes | Yes | Yes | Implemented and runtime-tested on `darwin/arm64`. |
| TCP Encrypted Transport (`IWP-LINK/1`) | Yes | Yes | Yes | Implemented and runtime-tested on `darwin/arm64`. |
| Tor SOCKS5 (`tor-only` fail-closed) | Yes | Yes | Yes | Pure-Go RFC 1928 SOCKS5 dialer implemented and tested. |
| Standalone App Window Launch (`platform/launch`) | Chrome/Edge/Brave `--app` or `open` | Chrome/Chromium/Edge `--app` or `xdg-open` | Edge/Chrome `--app` or `rundll32` | Implemented across macOS, Linux, and Windows. |
| Wi-Fi Hotspot / SoftAP Creation | Restricted by macOS APIs | `hostapd` / NetworkManager | `netsh wlan` / Wi-Fi Direct | Hardware-gated; not implemented. |
| Bluetooth LE Discovery | CoreBluetooth | BlueZ D-Bus | WinRT BLE | Hardware-gated; not implemented. |
| Wi-Fi Aware / Wi-Fi Direct | Limited / iOS-centric | `wpa_supplicant` P2P | WinRT Wi-Fi Direct | Hardware-gated; not implemented. |
| Private Key Storage | POSIX `0600` (`keys/node.json`) | POSIX `0600` (`keys/node.json`) | File ACL / user directory | Local key files with strict permissions; OS keychain backends reserved for future release. |
