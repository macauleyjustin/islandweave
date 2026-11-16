# IslandWeave Headless Server & VPS Deployment

## 1. Security Separation
Running `islandd --relay` on a VPS enables encrypted bundle forwarding (`IWP-DTN`), `IWBN` bulletin relay, and peer rendezvous **without** granting the VPS administrative control over any Island. The relay holds no Island private keys and cannot decrypt end-to-end DTN/IWMP payloads.

## 2. Linux systemd Deployment
1. Download `islandweave-v1.0.0-linux-amd64.tar.gz` (or `linux-arm64`) from [GitHub Releases](https://github.com/macauleyjustin/islandweave/releases/tag/v1.0.0), or build with `make cross-build`.
2. Install `islandd` and `island` into `/usr/local/bin/` and create service user `islandweave` with `/var/lib/islandweave`.
3. Optionally pass bootstrap addresses on startup with `--bootstrap host1:47472,host2:47472`.
4. Install [packaging/systemd/islandd.service](../packaging/systemd/islandd.service) to `/etc/systemd/system/islandd.service`:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now islandd
   ```

## 3. Container Deployment
```bash
docker build -f packaging/docker/Dockerfile -t islandweave:latest .
docker run -d --name islandd -p 47472:47472 -v islandweave-data:/var/lib/islandweave islandweave:latest --relay
```

## 4. Tor Onion Service Integration
Configure Tor's `torrc` to forward an onion service virtual port to `127.0.0.1:47472`, then run `islandd` with:
```bash
islandd --relay --tor-policy tor-only --tor-socks 127.0.0.1:9050 --onion-service <your>.onion:47472
```
Under `--tor-policy tor-only`, `islandd` enforces fail-closed Tor dialing and `IWP-ROUTE` excludes clearnet LAN/WAN dial targets.
