package shipper

import (
	"context"
	"testing"
	"time"
)

func TestIncidentThrottler_Debounce(t *testing.T) {
	throttler := NewIncidentThrottler(500 * time.Millisecond)

	entry1 := &ParsedErrorLog{
		ServiceName: "orderbook-aggregator",
		Filename:    "engine.go",
		LineNumber:  "42",
		LevelStr:    "ERROR",
		Message:     "crash 1",
	}

	// 1. First event should be allowed
	should, count := throttler.ShouldDispatch(entry1)
	if !should || count != 1 {
		t.Fatalf("expected first event to be dispatched with count 1, got %v, %d", should, count)
	}

	// 2. Immediate duplicate event should be throttled
	should, _ = throttler.ShouldDispatch(entry1)
	if should {
		t.Fatalf("expected duplicate event to be throttled")
	}

	// 3. Different signature should be dispatched immediately
	entry2 := &ParsedErrorLog{
		ServiceName: "market-observer",
		Filename:    "feed.go",
		LineNumber:  "99",
		LevelStr:    "CRITICAL",
		Message:     "feed offline",
	}
	should, count = throttler.ShouldDispatch(entry2)
	if !should || count != 1 {
		t.Fatalf("expected different signature to be dispatched")
	}

	// 4. After cooldown, should be allowed again with accumulated count
	time.Sleep(550 * time.Millisecond)
	should, count = throttler.ShouldDispatch(entry1)
	if !should || count < 2 {
		t.Fatalf("expected event after cooldown to dispatch with accumulated count, got %v, %d", should, count)
	}
}

func TestBuildIncidentEvent(t *testing.T) {
	entry := &ParsedErrorLog{
		Timestamp:    time.Now().UTC(),
		ServiceName:  "test-svc",
		Hostname:     "node-1",
		Level:        10,
		LevelStr:     "ERROR",
		Filename:     "main.go",
		FunctionName: "Start",
		LineNumber:   "10",
		Message:      "failed to bind port",
		StackTrace:   "stack trace details",
	}

	inc := BuildIncidentEvent(entry, 3)
	if inc.ServiceName != "test-svc" {
		t.Errorf("expected test-svc, got %s", inc.ServiceName)
	}
	if inc.OccurrenceCount != 3 {
		t.Errorf("expected count 3, got %d", inc.OccurrenceCount)
	}
	if inc.IncidentID == "" {
		t.Error("expected non-empty incident ID")
	}
}

func TestLocalIncidentNotifier(t *testing.T) {
	notifier := NewLocalIncidentNotifier()
	inc := &IncidentEvent{
		IncidentID:  "inc-001",
		ServiceName: "test-svc",
		Message:     "test",
	}

	err := notifier.PublishIncident(context.Background(), inc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	captured := notifier.GetIncidents()
	if len(captured) != 1 || captured[0].IncidentID != "inc-001" {
		t.Fatalf("expected 1 incident captured, got %v", captured)
	}
}

func TestResolveIncidentSubject_MultiBrain(t *testing.T) {
	if got := ResolveIncidentSubject("", "obsidian-brain"); got != "bastien-antigravity.fleet.incident" {
		t.Errorf("root brain subject mismatch: %s", got)
	}
	if got := ResolveIncidentSubject("custom", ""); got != "custom.fleet.incident" {
		t.Errorf("default brain subject mismatch: %s", got)
	}
	if got := ResolveIncidentSubject("bastien-antigravity", "trading-brain"); got != "bastien-antigravity.trading-brain.fleet.incident" {
		t.Errorf("satellite brain subject mismatch: %s", got)
	}
}

func TestNatsIncidentPublisher_PublishesPartitionedPayload(t *testing.T) {
	var publishedSubject string
	var publishedData []byte

	pub := NewNatsIncidentPublisher(func(subject string, data []byte) error {
		publishedSubject = subject
		publishedData = data
		return nil
	}, "bastien-antigravity", "trading-brain")

	inc := BuildIncidentEvent(&ParsedErrorLog{
		Timestamp:    time.Now().UTC(),
		BrainID:      "trading-brain",
		ServiceName:  "orderbook",
		Hostname:     "node-2",
		Level:        10,
		LevelStr:     "ERROR",
		Filename:     "book.go",
		FunctionName: "Update",
		LineNumber:   "77",
		Message:      "desync detected",
		EventHash:    "abc123",
	}, 2)

	if err := pub.PublishIncident(context.Background(), inc); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
	if publishedSubject != "bastien-antigravity.trading-brain.fleet.incident" {
		t.Errorf("unexpected subject: %s", publishedSubject)
	}
	if len(publishedData) == 0 {
		t.Fatal("expected non-empty JSON payload")
	}
}

