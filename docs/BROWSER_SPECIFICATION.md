# IslandWeave Navigator & Address Specification

## 1. `island://` Address Grammar

```abnf
island-uri    = "island://" authority [ "/" path ] [ "?" query ]
authority     = native-view / service-ref
native-view   = "home" / "mail" / "news" / "files" / "peers" / "services" / "downloads"
service-ref   = service-id / service-alias
service-id    = "iw:service:" 32HEXDIG
service-alias = 1*( ALPHA / DIGIT / "-" / "_" / "." )
```

### Supported Views & Resources
- `island://home` — Network status, hosted/cached services catalog, and browsing history
- `island://mail` — Integrated `IWMP` encrypted mailbox reader
- `island://news` — Integrated `IWBN` signed community bulletin reader
- `island://files` & `island://downloads` — `IWTP` file transfer progress & completed downloads
- `island://peers` — Connected and known peers with online/offline status
- `island://services` — Service and shared library directory
- `island://harbor-news/` — Static website resolved to signed `ServiceRecord`
- `island://community-library/` — Browsable Shared File Library (`type: "share"`) rendered via isolated gateway directory listing
- `island://iw:service:<32-hex>/` — Canonical publisher-bound service identity resolution

## 2. Origin Isolation, Persistence & Security
1. **No Privileged Daemon Access:** Hosted websites and shared libraries are served from the isolated `apps/gateway` HTTP origin (`127.0.0.1:47475` default) inside a sandboxed `<iframe>` (`sandbox="allow-scripts allow-forms"`) and never receive the `islandd` Control API bearer token.
2. **Verification & Availability Badges:** Navigator displays whether a resource is `Live Local Host`, `Cached Offline Replica`, or `Unavailable`, alongside its verified `ServiceID` and `PublisherID`, plus live daemon connection state (`Connected (N peers)`, `Offline / Local`, or `Daemon Offline`).
3. **Persisted Bookmarks & History:** Bookmarks (`★`) and navigation history persist across restarts in `islandd`'s local SQLite store (`/v1/navigator/bookmarks` and `/v1/navigator/history`).
4. **Standard HTTPS:** Conventional `https://` / `http://` URLs open cleanly in an external browser context with `noopener,noreferrer` so `island://` and web origins never mix.
