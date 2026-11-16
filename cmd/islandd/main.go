package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/islandweave/islandweave/core/beacon"
	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/transports/tor"
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".islandweave"
	}
	return filepath.Join(home, ".islandweave")
}

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "Directory for IslandWeave state, keys, and database")
	name := flag.String("name", "", "Node display name (used when initializing new identity)")
	listenAddr := flag.String("listen", "0.0.0.0:47472", "P2P TCP listen address")
	controlAddr := flag.String("control", "127.0.0.1:47471", "Local authenticated control API address")
	gatewayAddr := flag.String("gateway", "127.0.0.1:47475", "Local HTTP browser gateway address")
	multicastAddr := flag.String("multicast", beacon.DefaultMulticastAddr, "UDP multicast discovery address")
	bootstrap := flag.String("bootstrap", "", "Comma-separated bootstrap host:port addresses")
	relayMode := flag.Bool("relay", false, "Enable headless VPS encrypted relay mode")
	torPolicy := flag.String("tor-policy", "direct-first", "Tor policy: direct-first, tor-preferred, tor-only")
	torSocks := flag.String("tor-socks", "", "Tor SOCKS5 proxy address (e.g. 127.0.0.1:9050)")
	onionSvc := flag.String("onion-service", "", "Configured .onion:port service address")
	flag.Parse()

	var bootstrapList []string
	for _, b := range strings.Split(*bootstrap, ",") {
		if b = strings.TrimSpace(b); b != "" {
			bootstrapList = append(bootstrapList, b)
		}
	}

	d, err := daemon.New(daemon.Config{
		DataDir:        *dataDir,
		NodeName:       *name,
		ListenAddr:     *listenAddr,
		ControlAddr:    *controlAddr,
		GatewayAddr:    *gatewayAddr,
		MulticastAddr:  *multicastAddr,
		BootstrapPeers: bootstrapList,
		RelayEnabled:   *relayMode,
		Tor: tor.Config{
			Policy:       tor.Policy(*torPolicy),
			SocksAddr:    *torSocks,
			OnionService: *onionSvc,
		},
	})
	if err != nil {
		log.Fatalf("failed to initialize islandd: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := d.Start(ctx); err != nil {
		log.Fatalf("failed to start islandd: %v", err)
	}

	fmt.Printf("islandd started\n")
	fmt.Printf("  Node ID:      %s (%s)\n", d.NodeID(), d.NodeName())
	fmt.Printf("  P2P Listen:   %s\n", d.P2PAddr())
	fmt.Printf("  Control API:  %s\n", d.ControlAddr())
	fmt.Printf("  Gateway HTTP: http://%s\n", d.GatewayAddr())
	fmt.Printf("  Data Dir:     %s\n", *dataDir)

	<-ctx.Done()
	fmt.Println("\nshutting down islandd...")
	if err := d.Stop(); err != nil {
		log.Printf("error stopping islandd: %v", err)
	}
}
