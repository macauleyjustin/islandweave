# IslandWeave System Architecture

## 1. Overview

IslandWeave is a decentralized, offline-first, transport-independent networking ecosystem built around the **IslandWeave Protocol (IWP)** family. Its primary design principle is that local communication, service hosting, file sharing, mail, and publications must continue functioning when conventional internet connectivity is absent or intermittent.

All user-facing applications (`island` CLI, IslandWeave Desktop, IslandWeave Navigator) interact with a single headless core daemon (`islandd`) over an authenticated local control interface.

## 2. Technology Stack & Dependency Decisions

| Layer | Choice | Rationale |
|---|---|---|
| Core Language | Go (1.27+) | Single-binary cross-compilation (macOS/Linux/Windows, x86_64/ARM64), strong concurrency primitives, mature standard-library cryptography. |
| Local Database | `modernc.org/sqlite` | Pure-Go SQLite implementation; eliminates CGO cross-compilation complexity across target platforms. |
| Cryptography | Go stdlib (`crypto/ed25519`, `crypto/ecdh`, `crypto/sha256`) + `golang.org/x/crypto` (`chacha20poly1305`, `hkdf`) | Audited primitives with zero custom cryptographic algorithms. |
| Transport Layer | Direct IWP-LINK over TCP (with QUIC planned) | Evaluated `go-libp2p` and opted for a minimal, formally specified IWP-LINK framing over TCP so that Tor SOCKS5/Onion services, LAN TCP, relays, and DTN/satellite streams share one simple, auditable wire protocol with minimal dependencies. |
| Local Discovery | UDP Multicast (`239.255.73.87:47470`) + Signed Beacons | Avoids conflicts with OS mDNS responders (`mDNSResponder`/`avahi`) while supporting multi-node local testing and LAN discovery without external dependencies. |
| Desktop & Browser (Planned) | Native WebView / Wails + Local HTTP Gateway (`apps/gateway`) | Keeps the daemon protocol independent of UI frameworks and avoids writing a custom HTML/CSS/JS engine. |

## 3. Component Architecture

```text
+-------------------+   +----------------------+   +------------------------+
|   island (CLI)    |   |  IslandWeave Desktop |   | IslandWeave Navigator  |
+---------+---------+   +----------+-----------+   +-----------+------------+
          |                        |                           |
          +------------------------+---------------------------+
                                   |
                 Authenticated Local Control API (HTTP + Bearer Token)
                                   |
          +------------------------v---------------------------+
          |                  islandd (Core Daemon)             |
          |                                                    |
          |  +--------------+  +------------+  +------------+  |
          |  | IWP-IDENTITY |  | IWP-SPACE  |  | IWP-SERVICE|  |
          |  +--------------+  +------------+  +------------+  |
          |  +--------------+  +------------+  +------------+  |
          |  | IWP-DISCOVER |  | IWP-BEACON |  | IWP-OBJECT |  |
          |  +--------------+  +------------+  +------------+  |
          |  +----------------------------------------------+  |
          |  | IWP-LINK (Authenticated Encrypted Transport) |  |
          |  +----------------------------------------------+  |
          |  +----------------------------------------------+  |
          |  | Protocols: Messaging | IWTP | IWMP | IWBN | DTN |
          |  +----------------------------------------------+  |
          +------------------------+---------------------------+
                                   |
          +------------------------v---------------------------+
          | Transports: LAN | Internet | Tor | Relay | Satellite|
          +----------------------------------------------------+
```

## 4. Core Protocol Modules

- **IWP-IDENTITY**: Ed25519 node identities (`iw:node:<base32-hash>`), Island root keys, and key fingerprints.
- **IWP-SPACE**: Island creation, invite tokens (signed capabilities), membership lists, and peer authorization.
- **IWP-DISCOVER & IWP-BEACON**: Consent-based local discovery via signed UDP multicast announcements with configurable intervals, public/restricted modes, and expiration.
- **IWP-LINK**: Ephemeral X25519 ECDH + Ed25519 mutual authentication + HKDF-SHA256 + ChaCha20-Poly1305 encrypted framing with capability negotiation.
- **IWP-OBJECT**: Content-addressed SHA-256 chunk storage (planned Phase 2).
- **IWTP / IWMP / IWBN / IWP-DTN / IWST**: Application and delay-tolerant protocols layered on top of IWP-LINK and IWP-OBJECT (planned Phases 2–7).

## 5. Daemon Control Interface

`islandd` binds a local loopback HTTP control server (default `127.0.0.1:47471`, configurable) protected by a 256-bit random bearer token stored in `<data-dir>/control.token` with `0600` file permissions. Unauthenticated requests are rejected with `401 Unauthorized`.
