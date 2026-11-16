# IslandWeave

**IslandWeave** is a decentralized, offline-first, transport-independent networking ecosystem built around the **IslandWeave Protocol (`IWP/1.0`)** family.

Formal protocol standards live in the companion specification repository: [macauleyjustin/islandweave-specifications](https://github.com/macauleyjustin/islandweave-specifications).

## Components (v1.0.0)

- **Core Daemon (`islandd`)**: Headless Go daemon with pure-Go SQLite (`modernc.org/sqlite`), signed `IWP-BEACON` & `IWP-DISCOVER` peer discovery, `IWP-LINK/1` encrypted transport (X25519 + Ed25519 + HKDF-SHA256 + ChaCha20-Poly1305), `IWP-ROUTE` multi-transport routing, `IWP-SYNC` anti-entropy state synchronization, `IWP-OBJECT` chunk storage, `IWTP` file transfers, `IWP-DTN` courier bundles, `IWMP` encrypted mail, `IWBN` signed bulletins, Tor SOCKS5 (`tor-only` fail-closed), bilateral `IWP-FED-1` federation, `IWST` satellite HAL/simulator, and security/bandwidth controls.
- **CLI (`island`)**: Full command-line interface controlling `islandd` and launching Desktop/Navigator.
- **Desktop Control Center (`island-desktop`)**: Local browser/app UI covering dashboard, Islands, peers, interactive SVG network map & routes, Beacon Mode, chat, mail, bulletins, transfers & shared file libraries, hosted websites, courier packs, Tor/relay/federation/satellite controls, security audit, bandwidth, sync history, and diagnostics.
- **IslandWeave Navigator (`island-navigator`)**: Dedicated `island://` URI browser with sandboxed gateway isolation, offline cached replicas, persistent bookmarks & history, and verification metadata.
- **Ecosystem Simulator (`island-simulator`)**: Multi-node runnable scenario demonstrating LAN/relay links, website hosting, `.iwcourier` transport, `IWST` satellite contact simulation, and `IWP-FED-1` federation.

## Quick Start

```bash
make build

./bin/island init --name my-node
./bin/island start
./bin/island status
./bin/island create "Community Net"
./bin/island host ./examples/harbor-news --name harbor-news
./bin/island desktop
./bin/island navigator
```

Run the multi-node scenario simulator:

```bash
./bin/island-simulator
```

Run tests with race detection:

```bash
make test
```

Build cross-platform release archives (`darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`):

```bash
make release
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Protocol Specification](docs/PROTOCOL_SPECIFICATION.md)
- [Browser Specification](docs/BROWSER_SPECIFICATION.md)
- [Control API Reference](docs/API_REFERENCE.md)
- [Security Model](docs/SECURITY_MODEL.md)
- [Platform Compatibility](docs/PLATFORM_COMPATIBILITY.md)
- [Server Deployment](docs/SERVER_DEPLOYMENT.md)
- [Satellite Transport](docs/SATELLITE_TRANSPORT.md)
- [Implementation Status](docs/IMPLEMENTATION_STATUS.md)
- [Test Results](docs/TEST_RESULTS.md)
- [Known Limitations](docs/KNOWN_LIMITATIONS.md)
- [Changelog](docs/CHANGELOG.md)
