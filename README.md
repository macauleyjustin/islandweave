# IslandWeave

**IslandWeave** is a decentralized, offline-first, transport-independent networking ecosystem built around the **IslandWeave Protocol (IWP)** family.

## Current Milestone: Phase 0 & Phase 1

- **Core Daemon (`islandd`)**: Headless Go daemon with SQLite state (`modernc.org/sqlite`), signed UDP multicast discovery & Beacon Mode (`IWP-BEACON`), and authenticated local HTTP Control API.
- **CLI (`island`)**: Command-line client communicating with `islandd`.
- **Authenticated Encrypted Transport (`IWP-LINK/1`)**: Ephemeral X25519 + Ed25519 mutual authentication + HKDF-SHA256 + ChaCha20-Poly1305 AEAD framing over TCP.
- **Islands (`IWP-SPACE`)**: Island creation, signed invite tokens, membership, and peer trust/revocation.

## Quick Start

```bash
go build -o bin/islandd ./cmd/islandd
go build -o bin/island ./cmd/island

./bin/island init --name my-node
./bin/island start
./bin/island status
./bin/island create "Community Net"
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Protocol Specification](docs/PROTOCOL_SPECIFICATION.md)
- [Security Model](docs/SECURITY_MODEL.md)
- [Platform Compatibility](docs/PLATFORM_COMPATIBILITY.md)
- [Development Plan](docs/DEVELOPMENT_PLAN.md)
- [Implementation Status](docs/IMPLEMENTATION_STATUS.md)
- [Test Results](docs/TEST_RESULTS.md)
- [Known Limitations](docs/KNOWN_LIMITATIONS.md)
