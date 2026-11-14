package tests

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/daemon"
	"github.com/islandweave/islandweave/core/storage"
	"github.com/islandweave/islandweave/federation"
	"github.com/islandweave/islandweave/transports/tor"
)

func TestPhase5VPSRelayWithoutPlaintextAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// VPS Relay node: RelayEnabled=true, NOT a member of any Island
	dRelay, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "vps-relay",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
		RelayEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dRelay.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dRelay.Stop()

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "client-a",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dA.Stop()

	dC, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "client-c",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dC.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dC.Stop()

	// A and C share an Island, Relay does NOT
	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{"name": "PrivateNet"}, http.StatusCreated)
	var isl struct{ ID string `json:"id"` }
	_ = json.Unmarshal(islRaw, &isl)
	invRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands/invite", map[string]any{"island_id": isl.ID}, http.StatusOK)
	var inv struct{ Token string `json:"token"` }
	_ = json.Unmarshal(invRaw, &inv)
	apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/islands/join", map[string]string{"token": inv.Token}, http.StatusOK)

	// Exchange public keys between A and C
	stCRaw := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/status", nil, http.StatusOK)
	var stC struct {
		NodeID       string `json:"node_id"`
		PublicKey    string `json:"public_key"`
		EncPublicKey string `json:"enc_public_key"`
	}
	_ = json.Unmarshal(stCRaw, &stC)
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/import-key", map[string]string{
		"name":           "client-c",
		"public_key":     stC.PublicKey,
		"enc_public_key": stC.EncPublicKey,
	}, http.StatusOK)

	// Both A and C connect to the VPS Relay
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dRelay.P2PAddr()}, http.StatusOK)
	apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{"addr": dRelay.P2PAddr()}, http.StatusOK)

	// A sends an encrypted bundle to C through the VPS Relay
	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/courier/send", map[string]any{
		"to":        stC.NodeID,
		"island_id": isl.ID,
		"body":      "End-to-end encrypted through VPS relay",
	}, http.StatusOK)

	var msgsC []storage.MessageRecord
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rawC := apiCall(t, dC.ControlAddr(), dC.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
		_ = json.Unmarshal(rawC, &msgsC)
		if len(msgsC) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(msgsC) == 0 || msgsC[0].Body != "End-to-end encrypted through VPS relay" {
		t.Fatalf("Client C did not receive relayed message: %+v", msgsC)
	}

	// Relay must have 0 decrypted messages and 0 Island memberships
	relayMsgsRaw := apiCall(t, dRelay.ControlAddr(), dRelay.Token(), http.MethodGet, "/v1/messages", nil, http.StatusOK)
	var relayMsgs []storage.MessageRecord
	_ = json.Unmarshal(relayMsgsRaw, &relayMsgs)
	if len(relayMsgs) != 0 {
		t.Fatalf("VPS relay should have 0 plaintext messages, got %d", len(relayMsgs))
	}
	relayIslRaw := apiCall(t, dRelay.ControlAddr(), dRelay.Token(), http.MethodGet, "/v1/islands", nil, http.StatusOK)
	var relayIslands []any
	_ = json.Unmarshal(relayIslRaw, &relayIslands)
	if len(relayIslands) != 0 {
		t.Fatalf("VPS relay should not hold Island memberships, got %d", len(relayIslands))
	}
}

func TestPhase5TorOnlyPolicyNeverFallsBackToDirect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reachable target node
	dTarget, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "tor-target",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
		RelayEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dTarget.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dTarget.Stop()

	// Client configured with TorOnly and an unreachable SOCKS5 port
	dClient, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "tor-client",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
		Tor: tor.Config{
			Policy:    tor.PolicyTorOnly,
			SocksAddr: "127.0.0.1:1", // unreachable
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dClient.Stop()

	// Must FAIL CLOSED (502) and NOT connect directly to dTarget.P2PAddr()!
	apiCall(t, dClient.ControlAddr(), dClient.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dTarget.P2PAddr(),
	}, http.StatusBadGateway)

	// Now start a local SOCKS5 test proxy and verify TorOnly connects THROUGH it
	var proxiedCount atomic.Int32
	socksAddr, closeSocks := startTestSOCKS5Proxy(t, &proxiedCount)
	defer closeSocks()

	apiCall(t, dClient.ControlAddr(), dClient.Token(), http.MethodPost, "/v1/tor/config", tor.Config{
		Policy:    tor.PolicyTorOnly,
		SocksAddr: socksAddr,
	}, http.StatusOK)

	apiCall(t, dClient.ControlAddr(), dClient.Token(), http.MethodPost, "/v1/peers/connect", map[string]string{
		"addr": dTarget.P2PAddr(),
	}, http.StatusOK)

	if proxiedCount.Load() != 1 {
		t.Fatalf("expected connection to traverse SOCKS5 proxy, got count=%d", proxiedCount.Load())
	}
}

func TestPhase5IslandFederation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dA, err := daemon.New(daemon.Config{
		DataDir:       t.TempDir(),
		NodeName:      "fed-a",
		ListenAddr:    "127.0.0.1:0",
		ControlAddr:   "127.0.0.1:0",
		GatewayAddr:   "127.0.0.1:0",
		DisableBeacon: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := dA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer dA.Stop()

	islRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/islands", map[string]string{"name": "IslandA"}, http.StatusCreated)
	var isl struct{ ID string `json:"id"` }
	_ = json.Unmarshal(islRaw, &isl)

	agRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/federation/create", map[string]any{
		"local_island_id":     isl.ID,
		"remote_island_id":    "iw:island:remote9999",
		"shared_services":     []string{"harbor-news"},
		"allow_bulletin_sync": true,
		"allow_dtn_forward":   false,
	}, http.StatusCreated)
	var ag federation.Agreement
	_ = json.Unmarshal(agRaw, &ag)

	if err := federation.VerifyAgreement(&ag, time.Now()); err != nil {
		t.Fatalf("VerifyAgreement: %v", err)
	}
	if !ag.AllowsService("harbor-news", time.Now()) || ag.AllowsService("private-vault", time.Now()) {
		t.Fatalf("unexpected service allowance")
	}

	apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodPost, "/v1/federation/revoke", map[string]string{
		"agreement_id": ag.AgreementID,
	}, http.StatusOK)

	listRaw := apiCall(t, dA.ControlAddr(), dA.Token(), http.MethodGet, "/v1/federation", nil, http.StatusOK)
	var list []federation.Agreement
	_ = json.Unmarshal(listRaw, &list)
	if len(list) != 1 || !list[0].Revoked {
		t.Fatalf("expected revoked agreement, got %+v", list)
	}
}

func startTestSOCKS5Proxy(t *testing.T, counter *atomic.Int32) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleSOCKS5(c, counter)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func handleSOCKS5(c net.Conn, counter *atomic.Int32) {
	defer c.Close()
	var greet [3]byte
	if _, err := io.ReadFull(c, greet[:]); err != nil {
		return
	}
	_, _ = c.Write([]byte{0x05, 0x00})

	var hdr [4]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return
	}
	var host string
	switch hdr[3] {
	case 0x01:
		var ip [4]byte
		_, _ = io.ReadFull(c, ip[:])
		host = net.IP(ip[:]).String()
	case 0x03:
		var l [1]byte
		_, _ = io.ReadFull(c, l[:])
		b := make([]byte, l[0])
		_, _ = io.ReadFull(c, b)
		host = string(b)
	default:
		return
	}
	var portB [2]byte
	_, _ = io.ReadFull(c, portB[:])
	port := binary.BigEndian.Uint16(portB[:])

	target, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		_, _ = c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer target.Close()
	counter.Add(1)
	_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	go io.Copy(target, c)
	_, _ = io.Copy(c, target)
}
