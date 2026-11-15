BIN_DIR := bin
PREFIX  ?= $(HOME)/.local/bin

.PHONY: all build install test cross-build clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/islandd ./cmd/islandd
	go build -o $(BIN_DIR)/island ./cmd/island
	go build -o $(BIN_DIR)/island-desktop ./apps/desktop
	go build -o $(BIN_DIR)/island-navigator ./apps/navigator

install: build
	@mkdir -p $(PREFIX)
	cp $(BIN_DIR)/islandd $(BIN_DIR)/island $(BIN_DIR)/island-desktop $(BIN_DIR)/island-navigator $(PREFIX)/

test:
	go test -v -race -count=1 -timeout 60s ./...

cross-build:
	@mkdir -p $(BIN_DIR)/dist
	GOOS=linux  GOARCH=amd64 go build -o $(BIN_DIR)/dist/islandd-linux-amd64 ./cmd/islandd
	GOOS=linux  GOARCH=arm64 go build -o $(BIN_DIR)/dist/islandd-linux-arm64 ./cmd/islandd
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/islandd-windows-amd64.exe ./cmd/islandd
	GOOS=linux  GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-linux-amd64 ./cmd/island
	GOOS=linux  GOARCH=arm64 go build -o $(BIN_DIR)/dist/island-linux-arm64 ./cmd/island
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/dist/island-windows-amd64.exe ./cmd/island

clean:
	rm -rf $(BIN_DIR)
