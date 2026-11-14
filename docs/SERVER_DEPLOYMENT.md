# IslandWeave Headless Server & VPS Deployment

## 1. Security Separation
Running `islandd --relay` on a VPS enables encrypted bundle forwarding and peer rendezvous **without** granting the VPS administrative control over any Island. The VPS never holds Island private keys and cannot decrypt end-to-end DTN/IWMP payloads.

## 2. Linux systemd Deployment
1. Build binaries for your server architecture (`linux/amd64` or `linux/arm64`):
   ```bash
   GOOS=linux GOARCH=amd64 go build -o islandd ./cmd/islandd
   GOOS=linux GOARCH=amd64 go build -o island ./cmd/island
   ```
2. Install binaries to `/usr/local/bin/` and create user `islandweave` with `/var/lib/islandweave`.
3. Install `packaging/systemd/islandd.service` to `/etc/systemd/system/islandd.service`:
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
Configure Tor's `torrc` to forward a hidden service port to `127.0.0.1:47472`, then start `islandd` with `--tor-policy tor-only --tor-socks 127.0.0.1:9050 --onion-service <your>.onion:47472`.
