# IslandWeave System Architecture

**Release:** `v1.0.0` (`IWP/1.0`)

## 1. Overview

IslandWeave is a decentralized, offline-first, transport-independent networking ecosystem built around the **IslandWeave Protocol (`IWP/1.0`)** family. Its primary design principle is that local communication, service hosting, file sharing, mail, publications, and synchronization continue functioning when conventional internet connectivity is absent or intermittent.

All user-facing applications (`island` CLI, `island-desktop` Control Center, `island-navigator` Browser) interact with a single headless core daemon (`islandd`) over a loopback authenticated HTTP Control API (`127.0.0.1:47471` default).

## 2. Technology Stack & Dependency Decisions

| Layer | Choice | Rationale |
|---|---|---|
| Core Language | Go (`1.27.4`) | Single-binary cross-compilation (`darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`), strong concurrency primitives, mature standard-library cryptography. |
| Local Database | `modernc.org/sqlite` | Pure-Go SQLite implementation; eliminates CGO cross-compilation complexity across all target platforms. |
| Cryptography | Go stdlib (`crypto/ed25519`, `crypto/ecdh`, `crypto/sha256`) + `golang.org/x/crypto` (`chacha20poly1305`, `hkdf`) | Audited cryptographic primitives with zero custom ciphers. |
| Transport Security | `IWP-LINK/1` over TCP (plus RFC 1928 Tor SOCKS5) | Minimal, formally specified encrypted framing shared across LAN, WAN, VPS relays, and Tor hidden services. |
| Discovery | `IWP-BEACON` (UDP Multicast `239.255.73.87:47470`) + `IWP-DISCOVER` (`peer.exchange` + bootstraps) | Avoids conflicts with OS mDNS responders while supporting zero-config LAN discovery and consented peer exchange. |
| Desktop & Navigator UI | Standalone HTTP+Web UI (`apps/desktop`, `apps/navigator`) launched via `platform/launch` + isolated `apps/gateway` | Keeps `islandd` protocol independent of GUI toolkits while isolating untrusted website origins from daemon control tokens. |

## 3. Component Architecture

```text
+-------------------+   +----------------------+   +------------------------+
|   island (CLI)    |   |   island-desktop     |   |   island-navigator     |
+---------+---------+   +----------+-----------+   +-----------+------------+
          |                        |                           |
          +------------------------+---------------------------+
                                   |
           Authenticated Loopback Control API (Bearer 256-bit Token)
                                   |
+----------------------------------v-----------------------------------------+
|                         islandd (Core Daemon)                              |
|                                                                            |
|  +--------------+  +------------+  +------------+  +--------------------+  |
|  | IWP-IDENTITY |  | IWP-SPACE  |  | IWP-SERVICE|  | IWP-OBJECT         |  |
|  +--------------+  +------------+  +------------+  +--------------------+  |
|  +--------------+  +------------+  +------------+  +--------------------+  |
|  | IWP-DISCOVER |  | IWP-ROUTE  |  | IWP-SYNC   |  | security.Engine    |  |
|  +--------------+  +------------+  +------------+  +--------------------+  |
|  +----------------------------------------------------------------------+  |
|  | IWP-LINK/1 (Ephemeral X25519 + Ed25519 Auth + ChaCha20-Poly1305)     |  |
|  +----------------------------------------------------------------------+  |
|  +----------------------------------------------------------------------+  |
|  | Protocols: Messaging | IWTP | IWMP | IWBN | IWP-DTN | IWP-FED-1 | IWST| |
|  +----------------------------------------------------------------------+  |
+----------------------------------+-----------------------------------------+
                                   |
+----------------------------------v-----------------------------------------+
| Transports: LAN | Internet | Tor SOCKS5 | VPS Relay | .iwcourier | Sat HAL |
+----------------------------------------------------------------------------+
```

## 4. Implemented Protocol Modules

- **`IWP-IDENTITY` (`core/identity`):** Ed25519 signing + X25519 encryption keypairs (`iw:node:<32-hex>`), Island root keys (`iw:island:<32-hex>`).
- **`IWP-SPACE` (`core/islands`):** Island creation, signed invite tokens, membership tracking, and revocation.
- **`IWP-DISCOVER` & `IWP-BEACON` (`core/discovery`, `core/beacon`):** Consent-first signed UDP multicast discovery (`public`/`restricted`/`silent`), bootstrap seeds, and authenticated `peer.exchange`.
- **`IWP-LINK/1` (`core/transport`):** Ephemeral X25519 ECDH + Ed25519 mutual authentication + HKDF-SHA256 + ChaCha20-Poly1305 encrypted framing.
- **`IWP-ROUTE` (`core/routing`):** Multi-transport route table scoring (`lan-tcp`, `wan-tcp`, `tor-onion`, `dtn-only`) respecting strict `tor-only` exclusion.
- **`IWP-OBJECT` (`core/objects`) & `IWP-SERVICE` (`core/services`):** 256 KiB SHA-256 content-addressed chunk store, signed `ServiceRecord`s (`website` and `share` types), and `SiteManifest`s.
- **`IWP-SYNC` (`core/synchronization`):** Anti-entropy inventory diffing (`sync.inv`, `sync.req`, `sync.resp`) and object backfill on reconnect.
- **`IWTP/1`, `IWP-DTN`, `IWMP`, `IWBN`, `IWP-FED-1`, `IWST`:** Chunked file transfer, E2E encrypted store-and-forward bundles and `.iwcourier` packs, decentralized mail, signed bulletins, scoped bilateral federation, and satellite contact HAL/simulator.

## 5. Control API & Origin Isolation

- `islandd` binds its Control API exclusively on loopback (`127.0.0.1:47471` default) requiring a 256-bit random bearer token stored in `<data-dir>/control.token` (`0600`).
- Hosted websites and shared file libraries are served from a separate HTTP gateway origin (`127.0.0.1:47475` default) with strict Content Security Policy headers and sandboxed `<iframe>` execution inside Navigator.
