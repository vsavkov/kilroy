package server

import (
	"testing"
	"time"
)

func TestBroadcaster_SendAndSubscribe(t *testing.T) {
	b := NewBroadcaster()

	// Subscribe before any events.
	ch, _, unsub := b.Subscribe()
	defer unsub()

	// Send an event.
	b.Send(map[string]any{"event": "test", "n": 1})

	select {
	case ev := <-ch:
		if ev["event"] != "test" || ev["n"] != 1 {
			t.Fatalf("unexpected event: %v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroadcaster_HistoryReplay(t *testing.T) {
	b := NewBroadcaster()

	// Send events before subscribing.
	b.Send(map[string]any{"event": "first"})
	b.Send(map[string]any{"event": "second"})

	// Subscribe — should replay history.
	ch, _, unsub := b.Subscribe()
	defer unsub()

	var events []string
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			events = append(events, ev["event"].(string))
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for replayed event")
		}
	}
	if events[0] != "first" || events[1] != "second" {
		t.Fatalf("unexpected replay order: %v", events)
	}
}

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()

	ch1, _, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, _, unsub2 := b.Subscribe()
	defer unsub2()

	b.Send(map[string]any{"event": "broadcast"})

	for _, ch := range []<-chan map[string]any{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev["event"] != "broadcast" {
				t.Fatalf("unexpected event: %v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event on subscriber")
		}
	}
}

func TestBroadcaster_Close(t *testing.T) {
	b := NewBroadcaster()

	ch, _, unsub := b.Subscribe()
	defer unsub()

	b.Close()

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestBroadcaster_SubscribeAfterClose(t *testing.T) {
	b := NewBroadcaster()
	b.Send(map[string]any{"event": "before_close"})
	b.Close()

	// Subscribe after close — should get history replay then immediate close.
	ch, _, _ := b.Subscribe()

	var events []map[string]any
	for ev := range ch {
		events = append(events, ev)
	}
	if len(events) != 1 || events[0]["event"] != "before_close" {
		t.Fatalf("expected history replay on post-close subscribe, got: %v", events)
	}
}

func TestBroadcaster_History(t *testing.T) {
	b := NewBroadcaster()
	b.Send(map[string]any{"n": 1})
	b.Send(map[string]any{"n": 2})

	h := b.History()
	if len(h) != 2 {
		t.Fatalf("expected 2 events in history, got %d", len(h))
	}
}

func TestBroadcaster_SendAfterClose(t *testing.T) {
	b := NewBroadcaster()
	b.Close()
	// Should not panic.
	b.Send(map[string]any{"event": "after_close"})
	h := b.History()
	if len(h) != 0 {
		t.Fatalf("expected no events after close, got %d", len(h))
	}
}

func TestBroadcaster_HistoryReplayOver256(t *testing.T) {
	b := NewBroadcaster()

	// Send 300 events (exceeds the old hardcoded 256 buffer).
	for i := 0; i < 300; i++ {
		b.Send(map[string]any{"n": i})
	}

	// Subscribe must not deadlock — channel is sized to fit all history.
	done := make(chan struct{})
	go func() {
		ch, _, unsub := b.Subscribe()
		defer unsub()
		count := 0
		for range ch {
			count++
			if count == 300 {
				break
			}
		}
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe() deadlocked with >256 history events")
	}
}

func TestBroadcaster_DoneCh_RealClose(t *testing.T) {
	b := NewBroadcaster()
	_, doneCh, unsub := b.Subscribe()
	defer unsub()

	// doneCh should not be closed yet.
	select {
	case <-doneCh:
		t.Fatal("doneCh closed before broadcaster.Close()")
	default:
	}

	b.Close()

	// doneCh should now be closed.
	select {
	case <-doneCh:
		// expected
	case <-time.After(time.Second):
		t.Fatal("doneCh not closed after broadcaster.Close()")
	}
}

func TestBroadcaster_SlowClientDropDoesNotCloseDoneCh(t *testing.T) {
	b := NewBroadcaster()

	// Subscribe with a buffer that will fill up.
	ch, doneCh, _ := b.Subscribe()

	// Fill the channel buffer (history=0, so buffer=256).
	for i := 0; i < 256; i++ {
		b.Send(map[string]any{"n": i})
	}

	// This send should drop the slow client (channel full, not reading).
	b.Send(map[string]any{"n": 256})

	// Drain ch to see it's closed (dropped).
	drained := 0
	for range ch {
		drained++
	}

	// But doneCh should NOT be closed — broadcaster is still alive.
	select {
	case <-doneCh:
		t.Fatal("doneCh closed on slow-client drop (should only close on broadcaster.Close)")
	default:
		// correct
	}

	b.Close()
}
