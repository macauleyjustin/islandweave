BIN_DIR  := bin
DIST_DIR := dist
PREFIX   ?= $(HOME)/.local/bin
VERSION  ?= v1.0.0

.PHONY: all build install test cross-build release clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/islandd ./cmd/islandd
	go build -o $(BIN_DIR)/island ./cmd/island
	go build -o $(BIN_DIR)/island-desktop ./apps/desktop
	go build -o $(BIN_DIR)/island-navigator ./apps/navigator
	go build -o $(BIN_DIR)/island-simulator ./simulator

install: build
	@mkdir -p $(PREFIX)
	cp $(BIN_DIR)/islandd $(BIN_DIR)/island $(BIN_DIR)/island-desktop $(BIN_DIR)/island-navigator $(PREFIX)/

test:
	go test -v -race -count=1 -timeout 60s ./...

cross-build:
	@mkdir -p $(BIN_DIR)/dist
	GOOS=darwin  GOARCH=arm64 go build -o $(BIN_DIR)/dist/islandd-darwin-arm64 ./cmd/islandd
	GOOS=darwin  GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-darwin-arm64 ./cmd/island
	GOOS=darwin  GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-desktop-darwin-arm64 ./apps/desktop
	GOOS=darwin  GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-navigator-darwin-arm64 ./apps/navigator
	GOOS=linux   GOARCH=amd64 go build -o $(BIN_DIR)/dist/islandd-linux-amd64 ./cmd/islandd
	GOOS=linux   GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-linux-amd64 ./cmd/island
	GOOS=linux   GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-desktop-linux-amd64 ./apps/desktop
	GOOS=linux   GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-navigator-linux-amd64 ./apps/navigator
	GOOS=linux   GOARCH=arm64 go build -o $(BIN_DIR)/dist/islandd-linux-arm64 ./cmd/islandd
	GOOS=linux   GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-linux-arm64 ./cmd/island
	GOOS=linux   GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-desktop-linux-arm64 ./apps/desktop
	GOOS=linux   GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-navigator-linux-arm64 ./apps/navigator
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/islandd-windows-amd64.exe ./cmd/islandd
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-windows-amd64.exe ./cmd/island
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-desktop-windows-amd64.exe ./apps/desktop
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-navigator-windows-amd64.exe ./apps/navigator

release: cross-build
	@rm -rf $(DIST_DIR) && mkdir -p $(DIST_DIR)
	@for plat in darwin-arm64 linux-amd64 linux-arm64; do \
		stage="$(DIST_DIR)/stage-$$plat/islandweave-$(VERSION)-$$plat"; \
		mkdir -p "$$stage"; \
		cp $(BIN_DIR)/dist/islandd-$$plat "$$stage/islandd"; \
		cp $(BIN_DIR)/dist/island-$$plat "$$stage/island"; \
		cp $(BIN_DIR)/dist/island-desktop-$$plat "$$stage/island-desktop"; \
		cp $(BIN_DIR)/dist/island-navigator-$$plat "$$stage/island-navigator"; \
		cp README.md "$$stage/"; \
		cp -r examples packaging platform/linux "$$stage/"; \
		tar -C "$(DIST_DIR)/stage-$$plat" -czf "$(DIST_DIR)/islandweave-$(VERSION)-$$plat.tar.gz" "islandweave-$(VERSION)-$$plat"; \
		rm -rf "$(DIST_DIR)/stage-$$plat"; \
	done
	@stage="$(DIST_DIR)/stage-win/islandweave-$(VERSION)-windows-amd64"; \
		mkdir -p "$$stage"; \
		cp $(BIN_DIR)/dist/islandd-windows-amd64.exe "$$stage/islandd.exe"; \
		cp $(BIN_DIR)/dist/island-windows-amd64.exe "$$stage/island.exe"; \
		cp $(BIN_DIR)/dist/island-desktop-windows-amd64.exe "$$stage/island-desktop.exe"; \
		cp $(BIN_DIR)/dist/island-navigator-windows-amd64.exe "$$stage/island-navigator.exe"; \
		cp README.md platform/windows/install-service.ps1 "$$stage/"; \
		cp -r examples "$$stage/"; \
		(cd "$(DIST_DIR)/stage-win" && zip -qr "../islandweave-$(VERSION)-windows-amd64.zip" "islandweave-$(VERSION)-windows-amd64"); \
		rm -rf "$(DIST_DIR)/stage-win"
	@(cd $(DIST_DIR) && shasum -a 256 *.tar.gz *.zip > SHA256SUMS.txt)

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
