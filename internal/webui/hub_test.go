package webui

import (
	"testing"
	"time"
)

func TestHubPublishSubscribe(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.subscribe()
	defer unsubscribe()

	hub.publish(LiveEvent{Msg: "hello"})

	select {
	case e := <-ch:
		if e.Msg != "hello" {
			t.Fatalf("expected 'hello', got %q", e.Msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestHubDropsSlowSubscriberInsteadOfBlocking(t *testing.T) {
	hub := NewHub()
	_, unsubscribe := hub.subscribe() // never drained
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			hub.publish(LiveEvent{Msg: "spam"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a slow subscriber instead of dropping")
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.subscribe()
	unsubscribe()

	hub.publish(LiveEvent{Msg: "after unsubscribe"})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed, got a value instead")
		}
	case <-time.After(time.Second):
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}

func TestTwitchAuthStateTransitions(t *testing.T) {
	var s twitchAuthState

	if got := s.snapshot(); got.State != "idle" {
		t.Fatalf("expected idle initially, got %+v", got)
	}

	s.setPending("https://example/verify", "ABCD-1234")
	got := s.snapshot()
	if got.State != "pending" || got.VerificationURI == "" || got.UserCode == "" {
		t.Fatalf("expected pending with code/uri, got %+v", got)
	}

	s.setConnected()
	if got := s.snapshot(); got.State != "connected" || got.Error != "" {
		t.Fatalf("expected connected with no error, got %+v", got)
	}
}
