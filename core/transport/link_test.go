package transport

import (
	"crypto/ed25519"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/islandweave/islandweave/core/identity"
)

func TestLinkHandshakeAndEncryptedMessaging(t *testing.T) {
	idA, err := identity.GenerateNodeIdentity("alice")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.GenerateNodeIdentity("bob")
	if err != nil {
		t.Fatal(err)
	}

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	errCh := make(chan error, 2)
	var linkA, linkB *Link

	go func() {
		var e error
		linkA, e = HandshakeInitiator(c1, HandshakeConfig{
			Identity:     idA,
			Capabilities: []string{DefaultCapabilityMsg, "iwtp/1"},
			IslandIDs:    []string{"iw:island:1111", "iw:island:2222"},
			AuthorizePeer: func(_ string, _ ed25519.PublicKey, shared []string) bool {
				return len(shared) > 0
			},
		})
		errCh <- e
	}()

	go func() {
		var e error
		linkB, e = HandshakeResponder(c2, HandshakeConfig{
			Identity:     idB,
			Capabilities: []string{DefaultCapabilityMsg},
			IslandIDs:    []string{"iw:island:2222", "iw:island:3333"},
			AuthorizePeer: func(_ string, _ ed25519.PublicKey, shared []string) bool {
				return len(shared) > 0
			},
		})
		errCh <- e
	}()

	for i := 0; i < 2; i++ {
		if e := <-errCh; e != nil {
			t.Fatalf("handshake failed: %v", e)
		}
	}

	if linkA.PeerID() != idB.ID || linkB.PeerID() != idA.ID {
		t.Fatalf("peer ID mismatch")
	}
	if len(linkA.SharedIslands()) != 1 || linkA.SharedIslands()[0] != "iw:island:2222" {
		t.Fatalf("unexpected shared islands: %v", linkA.SharedIslands())
	}
	if len(linkA.Capabilities()) != 1 || linkA.Capabilities()[0] != DefaultCapabilityMsg {
		t.Fatalf("unexpected negotiated capabilities: %v", linkA.Capabilities())
	}

	payload, _ := json.Marshal(ChatPayload{Body: "offline-first hello"})
	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- linkA.SendEnvelope(&Envelope{
			Version:   ProtocolVersion,
			Type:      "chat.msg",
			ID:        "msg-1",
			IslandID:  "iw:island:2222",
			SenderID:  idA.ID,
			Timestamp: time.Now().Unix(),
			Payload:   payload,
		})
	}()

	env, err := linkB.RecvEnvelope()
	if err != nil {
		t.Fatalf("RecvEnvelope: %v", err)
	}
	if err := <-sendErrCh; err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	var got ChatPayload
	if err := json.Unmarshal(env.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Body != "offline-first hello" || env.SenderID != idA.ID {
		t.Fatalf("unexpected envelope: %+v / %+v", env, got)
	}
}
