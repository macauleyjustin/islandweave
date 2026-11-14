# Known Limitations

1. **Phase Scope:** Currently in Phase 0 / Phase 1 implementation. File transfer (IWTP), website hosting, DTN store-and-forward, Mail (IWMP), Bulletins (IWBN), Tor transport, Federation, Desktop GUI, Navigator browser, and Satellite simulator (IWST) are designed/planned for subsequent phases.
2. **Wireless Hardware Integration:** Wi-Fi hotspot creation (SoftAP), Bluetooth Low Energy (BLE), and Wi-Fi Aware/Direct require OS-specific privileged adapters and are not yet implemented.
3. **Key Storage:** Private keys are currently stored on disk in `<data-dir>/keys/` with `0600` file permissions rather than OS-native keychains (Keychain Services / libsecret / Windows DPAPI).
4. **Transport:** Currently uses TCP with `IWP-LINK/1` encryption; QUIC transport is planned as an additional transport adapter.
5. **Cross-Platform Runtime Verification:** Only `darwin/arm64` has been runtime-tested in this environment; Linux and Windows targets are supported by the pure-Go stack and cross-compilable, but require target-OS testing.
