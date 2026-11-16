# IslandWeave User Guide (`v1.0.0`)

## 1. Install or Build

Download a prebuilt archive from [GitHub Releases (`v1.0.0`)](https://github.com/macauleyjustin/islandweave/releases/tag/v1.0.0) or build from source:

```bash
make build
# Optionally install binaries into ~/.local/bin (or set PREFIX):
make install
```

On Linux, you can also run `./platform/linux/install.sh` to install binaries, `.desktop` menu shortcuts, and a systemd user unit.

## 2. Quick Start Workflow

1. **Initialize Node Identity:**
   ```bash
   ./bin/island init --name my-laptop
   ```
2. **Start Core Daemon (`islandd`):**
   ```bash
   ./bin/island start
   ./bin/island status
   ```
3. **Create & Join an Island:**
   ```bash
   ./bin/island create "Community Net"
   ./bin/island invite <island-id>
   # On another node:
   ./bin/island join <invite-token>
   ```
4. **Discover & Connect Locally:**
   ```bash
   ./bin/island beacon on --mode public --island <island-id>
   ./bin/island discover
   ./bin/island peers
   ./bin/island routes
   ```
5. **Host Websites & Shared Libraries:**
   ```bash
   ./bin/island host ./examples/harbor-news --name harbor-news
   ./bin/island share ./examples/community-library --name community-library
   ./bin/island gateway list
   ```
6. **Send Encrypted Chat, Files, Mail & Bulletins:**
   ```bash
   ./bin/island send-msg <peer-node-id> "Hello offline!"
   ./bin/island send ./document.pdf --to <peer-node-id>
   ./bin/island mail send --to <peer-node-id> --subject "Update" --body "Status report"
   ./bin/island publish --channel community-news --title "Alert" --body "Mesh active"
   ./bin/island sync
   ```
7. **Physical Courier Mode (`.iwcourier`):**
   ```bash
   ./bin/island courier export ./shuttle.iwcourier
   # On destination offline node:
   ./bin/island courier import ./shuttle.iwcourier
   ```
8. **Launch Graphical Apps:**
   ```bash
   ./bin/island desktop
   ./bin/island navigator
   ```
   Inside Navigator, open `island://harbor-news/`, `island://community-library/`, `island://mail`, `island://news`, or `island://home`.

## 3. Run the Scenario Simulator

Run a full end-to-end 3-node local demonstration (`harbor-hub`, `hilltop-outpost`, `vps-relay`) without modifying your personal `~/.islandweave` state:

```bash
./bin/island-simulator
```
