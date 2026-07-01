package ws

import (
	"testing"
	"time"
)

// startHub runs a hub for the duration of a test and stops it on cleanup.
func startHub(t *testing.T) *Hub {
	t.Helper()
	h := NewHub()
	go h.Run()
	t.Cleanup(h.Stop)
	return h
}

// recv reads one payload from a client's send channel, failing if none arrives.
func recv(t *testing.T, c *Client) []byte {
	t.Helper()
	select {
	case p, ok := <-c.send:
		if !ok {
			t.Fatal("send channel closed, want a payload")
		}
		return p
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for payload")
		return nil
	}
}

func TestHubDeliversToOrderSubscribers(t *testing.T) {
	h := startHub(t)
	c := NewClient(h, nil, 1)
	h.Register(c)

	payload := []byte("confirmed")
	if !h.Notify(1, payload) {
		t.Fatal("Notify(1) = false, want true (one subscriber)")
	}
	if got := recv(t, c); string(got) != string(payload) {
		t.Errorf("delivered %q, want %q", got, payload)
	}

	// A different order has no subscriber, so delivery reports false.
	if h.Notify(2, payload) {
		t.Error("Notify(2) = true, want false (no subscriber)")
	}
}

func TestHubFansOutToMultipleClientsOfSameOrder(t *testing.T) {
	h := startHub(t)
	c1 := NewClient(h, nil, 7)
	c2 := NewClient(h, nil, 7)
	h.Register(c1)
	h.Register(c2)

	if !h.Notify(7, []byte("shipped")) {
		t.Fatal("Notify(7) = false, want true")
	}
	if got := recv(t, c1); string(got) != "shipped" {
		t.Errorf("c1 got %q, want shipped", got)
	}
	if got := recv(t, c2); string(got) != "shipped" {
		t.Errorf("c2 got %q, want shipped", got)
	}
}

func TestHubUnregisterStopsDeliveryAndClosesSend(t *testing.T) {
	h := startHub(t)
	c := NewClient(h, nil, 3)
	h.Register(c)
	h.Unregister(c)

	// Notify serializes behind the unregister in the run loop, so by the time it
	// returns the client is gone: no subscriber, and its send channel is closed.
	if h.Notify(3, []byte("x")) {
		t.Error("Notify(3) = true after unregister, want false")
	}
	if _, ok := <-c.send; ok {
		t.Error("send channel open after unregister, want closed")
	}
}
