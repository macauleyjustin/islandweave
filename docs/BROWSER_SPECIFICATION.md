# IslandWeave Navigator & Address Specification

## 1. `island://` Address Grammar

```abnf
island-uri   = "island://" authority [ "/" path ] [ "?" query ]
authority    = native-view / service-ref
native-view  = "home" / "mail" / "news" / "files" / "peers" / "services" / "downloads"
service-ref  = service-id / service-alias
service-id   = "iw:service:" 32HEXDIG
service-alias = 1*( ALPHA / DIGIT / "-" / "_" / "." )
```

### Examples
- `island://home` — Native IslandWeave start page and network status
- `island://mail` — Integrated IWMP Mail client
- `island://news` — Integrated IWBN Bulletin reader
- `island://files` — IWTP transfers and downloads
- `island://peers` — Connected peers and local discovery
- `island://services` — Hosted and cached service directory
- `island://harbor-news/` — Hosted website resolved to verified `ServiceRecord`
- `island://iw:service:a1b2.../archive` — Canonical service identity resolution

## 2. Origin Isolation & Security
1. **No Privileged Daemon Access:** Hosted websites are served from the isolated `apps/gateway` HTTP origin (`127.0.0.1:47475`) inside a sandboxed `<iframe>` (`sandbox="allow-scripts allow-forms"`), and never receive the `islandd` Control API bearer token.
2. **Verification & Availability States:** Navigator displays whether a resource is `Live Local Host`, `Cached Offline Replica`, or `Unavailable`, along with its `ServiceID` and `PublisherID`.
3. **Standard HTTPS:** Conventional `https://` URLs are opened in a separate external browser tab/context so `island://` and `https://` origins are never confused.
