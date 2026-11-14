# IslandWeave User Guide

## 1. Build Binaries
```bash
go build -o bin/islandd ./cmd/islandd
go build -o bin/island ./cmd/island
go build -o bin/island-desktop ./apps/desktop
go build -o bin/island-navigator ./apps/navigator
```

## 2. Quick Start (10 Steps)
1. Initialize your node: `./bin/island init --name my-laptop`
2. Start the daemon: `./bin/island start`
3. Create an Island: `./bin/island create "Community Net"`
4. Enable Beacon Mode: `./bin/island beacon on --mode public --island <island-id>`
5. Create an invite for another computer: `./bin/island invite <island-id>`
6. On the other computer: `./bin/island join <invite-token>`
7. Send a message: `./bin/island send-msg <peer-node-id> "Hello offline!"`
8. Send a file: `./bin/island send ./document.pdf --to <peer-node-id>`
9. Host a website: `./bin/island host ./mywebsite --name mywebsite`
10. Launch Desktop Control Center (`./bin/island-desktop`) or Navigator (`./bin/island-navigator`) and open `island://mywebsite/`.
