# Known Limitations

1. **Current Release Scope (v0.1.0):** Core `IWP/1.0` daemon (`islandd`), CLI (`island`), Gateway (`apps/gateway`), Desktop Control Center (`island-desktop`), Navigator (`island-navigator`), DTN/Courier, IWMP Mail, IWBN Bulletins, Tor SOCKS5, Federation, and IWST Contact Simulator are implemented. `IWP-ROUTE` multi-hop live routing and `IWP-SYNC` anti-entropy sync remain planned.
2. **Wireless Hardware Integration:** Wi-Fi hotspot creation (SoftAP), Bluetooth Low Energy (BLE), and Wi-Fi Aware/Direct require OS-specific privileged adapters and are not yet implemented.
3. **Key Storage:** Private keys are currently stored on disk in `<data-dir>/keys/` with `0600` file permissions rather than OS-native keychains (Keychain Services / libsecret / Windows DPAPI).
4. **Transport:** Currently uses TCP with `IWP-LINK/1` encryption; QUIC transport is planned as an additional transport adapter.
5. **Cross-Platform Runtime Verification:** Only `darwin/arm64` has been runtime-tested in this environment; Linux and Windows targets are supported by the pure-Go stack and cross-compilable, but require target-OS testing.
