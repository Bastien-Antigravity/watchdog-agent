package shipper

// =============================================================================
// ESSENTIAL PROCESS:
// Real-time incident event dispatcher and alert rate-limiter for watchdog-agent.
// Emits structured incident notifications to the Squad Agent over the ecosystem
// event bus (NATS) to trigger real-time AI triage and tiered auto-repair.
//
// DATA FLOW:
// 1. Receives ParsedErrorLog events from the log shipper loop.
// 2. Evaluates signature in IncidentThrottler to prevent alert storms / stampedes.
// 3. Serializes qualifying incidents into brain-partitioned IncidentEvent payloads.
// 4. Dispatches non-blocking to NATS topic 'bastien-antigravity[.<brain>].fleet.incident'.
//
// KEY PARAMETERS:
// - Throttler: Mutex-protected cooldown store tracking unique error signatures.
// - Cooldown: Window duration (default 60s) between repeated alerts for same error.
// =============================================================================

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------

// IncidentEvent represents a real-time actionable error payload sent to the Squad Agent.
type IncidentEvent struct {
	IncidentID      string    `json:"incident_id"`
	BrainID         string    `json:"brain_id"`
	EventHash       string    `json:"event_hash"`
	Timestamp       time.Time `json:"timestamp"`
	ServiceName     string    `json:"service_name"`
	Hostname        string    `json:"hostname"`
	Level           int       `json:"level"`
	LevelStr        string    `json:"level_str"`
	Filename        string    `json:"filename"`
	FunctionName    string    `json:"function_name"`
	LineNumber      string    `json:"line_number"`
	Message         string    `json:"message"`
	StackTrace      string    `json:"stack_trace,omitempty"`
	OccurrenceCount int       `json:"occurrence_count"`
	SuggestedAction string    `json:"suggested_action,omitempty"`
}

// -----------------------------------------------------------------------------

// IncidentPublisher interface defines the contract for dispatching incidents.
type IncidentPublisher interface {
	PublishIncident(ctx context.Context, inc *IncidentEvent) error
}

// -----------------------------------------------------------------------------

// throttleRecord tracks recurrence state for a single error signature.
type throttleRecord struct {
	lastDispatched time.Time
	count          int
}

// IncidentThrottler debounces high-frequency log lines to prevent AI prompt flooding.
type IncidentThrottler struct {
	mu       sync.Mutex
	records  map[string]*throttleRecord
	cooldown time.Duration
}

// NewIncidentThrottler creates a throttler with a specified suppression window.
func NewIncidentThrottler(cooldown time.Duration) *IncidentThrottler {
	if cooldown <= 0 {
		cooldown = 60 * time.Second
	}
	return &IncidentThrottler{
		records:  make(map[string]*throttleRecord),
		cooldown: cooldown,
	}
}

// ShouldDispatch checks whether an error should trigger an alert.
// Returns (shouldDispatch, accumulatedCount).
func (t *IncidentThrottler) ShouldDispatch(entry *ParsedErrorLog) (bool, int) {
	if entry == nil {
		return false, 0
	}

	sig := fmt.Sprintf("%s:%s:%s:%s:%s", entry.BrainID, entry.ServiceName, entry.Filename, entry.LineNumber, entry.LevelStr)
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.records) > 1000 {
		cutoff := now.Add(-10 * t.cooldown)
		for k, r := range t.records {
			if r.lastDispatched.Before(cutoff) {
				delete(t.records, k)
			}
		}
	}

	rec, exists := t.records[sig]
	if !exists {
		t.records[sig] = &throttleRecord{
			lastDispatched: now,
			count:          1,
		}
		return true, 1
	}

	rec.count++
	if now.Sub(rec.lastDispatched) >= t.cooldown {
		dispatchedCount := rec.count
		rec.lastDispatched = now
		rec.count = 1
		return true, dispatchedCount
	}

	return false, 0
}

// BuildIncidentEvent creates a structured IncidentEvent from a ParsedErrorLog.
func BuildIncidentEvent(entry *ParsedErrorLog, occurrenceCount int) *IncidentEvent {
	brainID := entry.BrainID
	if brainID == "" {
		brainID = "obsidian-brain"
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%s-%s-%s-%d",
		brainID, entry.ServiceName, entry.Filename, entry.LineNumber, entry.Message, entry.Timestamp.UnixNano())))
	incidentID := fmt.Sprintf("inc-%s-%s", entry.ServiceName, hex.EncodeToString(hash[:])[:8])

	return &IncidentEvent{
		IncidentID:      incidentID,
		BrainID:         brainID,
		EventHash:       entry.EventHash,
		Timestamp:       entry.Timestamp,
		ServiceName:     entry.ServiceName,
		Hostname:        entry.Hostname,
		Level:           entry.Level,
		LevelStr:        entry.LevelStr,
		Filename:        entry.Filename,
		FunctionName:    entry.FunctionName,
		LineNumber:      entry.LineNumber,
		Message:         entry.Message,
		StackTrace:      entry.StackTrace,
		OccurrenceCount: occurrenceCount,
		SuggestedAction: "AutoRepair triage: classify Tier 1-4; isolate patches in /tmp git worktree with test gate.",
	}
}

// -----------------------------------------------------------------------------

// LocalIncidentNotifier is an in-memory/callback notifier for unit testing or direct routing.
type LocalIncidentNotifier struct {
	mu        sync.Mutex
	incidents []*IncidentEvent
}

// NewLocalIncidentNotifier creates a testable local incident recorder.
func NewLocalIncidentNotifier() *LocalIncidentNotifier {
	return &LocalIncidentNotifier{
		incidents: make([]*IncidentEvent, 0),
	}
}

// PublishIncident records the incident locally.
func (l *LocalIncidentNotifier) PublishIncident(ctx context.Context, inc *IncidentEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.incidents = append(l.incidents, inc)
	return nil
}

// GetIncidents returns a copy of captured incidents.
func (l *LocalIncidentNotifier) GetIncidents() []*IncidentEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	res := make([]*IncidentEvent, len(l.incidents))
	copy(res, l.incidents)
	return res
}

// -----------------------------------------------------------------------------

// ResolveIncidentSubject builds the canonical NATS topic partitioned per brain_id.
func ResolveIncidentSubject(subjectPrefix, brainID string) string {
	if subjectPrefix == "" {
		subjectPrefix = "bastien-antigravity"
	}
	if brainID == "" || brainID == "obsidian-brain" || brainID == "root" || brainID == "main" {
		return fmt.Sprintf("%s.fleet.incident", subjectPrefix)
	}
	return fmt.Sprintf("%s.%s.fleet.incident", subjectPrefix, brainID)
}

// NatsIncidentPublisher publishes events to NATS subject 'bastien-antigravity[.<brain>].fleet.incident'.
type NatsIncidentPublisher struct {
	publishFunc func(subject string, data []byte) error
	subject     string
}

// NewNatsIncidentPublisher constructs a brain-aware NATS-backed incident publisher.
func NewNatsIncidentPublisher(publishFunc func(subject string, data []byte) error, subjectPrefix, brainID string) *NatsIncidentPublisher {
	return &NatsIncidentPublisher{
		publishFunc: publishFunc,
		subject:     ResolveIncidentSubject(subjectPrefix, brainID),
	}
}

// PublishIncident serializes and publishes the incident to NATS.
func (p *NatsIncidentPublisher) PublishIncident(ctx context.Context, inc *IncidentEvent) error {
	if p.publishFunc == nil || inc == nil {
		return nil
	}
	payload, err := json.Marshal(inc)
	if err != nil {
		return err
	}
	return p.publishFunc(p.subject, payload)
}
