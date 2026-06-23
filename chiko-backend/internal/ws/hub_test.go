package ws_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/chiko/backend/internal/ws"
)

func TestHub_BroadcastToRoom(t *testing.T) {
	hub := ws.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(5 * time.Millisecond) // let hub goroutine start

	chatID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()

	sendA := make(chan []byte, 4)
	sendB := make(chan []byte, 4)

	// Register two fake clients using exported test helper.
	clientA := ws.NewTestClient(hub, chatID, userA, sendA)
	clientB := ws.NewTestClient(hub, chatID, userB, sendB)
	hub.RegisterTest(clientA)
	hub.RegisterTest(clientB)
	time.Sleep(5 * time.Millisecond)

	// Broadcast from A, B should receive, A should not.
	msg := []byte(`{"type":"order.item_updated","payload":{}}`)
	hub.Broadcast(ws.BroadcastMsg{ChatID: chatID, Data: msg, ExcludeID: userA})
	time.Sleep(5 * time.Millisecond)

	// B received
	select {
	case got := <-sendB:
		if string(got) != string(msg) {
			t.Errorf("B got %s, want %s", got, msg)
		}
	default:
		t.Error("B did not receive broadcast")
	}

	// A did not receive
	select {
	case unexpected := <-sendA:
		t.Errorf("A should not receive own broadcast, got %s", unexpected)
	default:
		// correct
	}

	_ = clientB // suppress unused warning
}

func TestHub_UnregisterReleasesRoom(t *testing.T) {
	hub := ws.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	time.Sleep(5 * time.Millisecond)

	chatID := uuid.New()
	userA := uuid.New()
	sendA := make(chan []byte, 4)

	c := ws.NewTestClient(hub, chatID, userA, sendA)
	hub.RegisterTest(c)
	time.Sleep(5 * time.Millisecond)

	if hub.OnlineCount(chatID) != 1 {
		t.Fatalf("expected 1 client, got %d", hub.OnlineCount(chatID))
	}

	hub.UnregisterTest(c)
	time.Sleep(5 * time.Millisecond)

	if hub.OnlineCount(chatID) != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.OnlineCount(chatID))
	}
}
