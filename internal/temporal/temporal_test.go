package temporal

import (
	"testing"
	"time"

	"oneshottest/internal/event"
)

// TestVisibleExcludesValidToBoundary verifies the half-open [valid_from, valid_to) interval semantics:
// at a version transition, the NEW version should be visible at its valid_from (inclusive lower bound),
// and the OLD version should NOT be visible at the upper bound (exclusive).
// The documented inclusive-end bug class from the ADR's Existing Primitives Audit is prevented by
// ensuring the upper-bound comparison (validAt < valid_from_of_next_version) is enforced.
func TestVisibleExcludesValidToBoundary(t *testing.T) {
	validFrom1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validFrom2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) // This is the exclusive upper bound for event1
	validFrom3 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) // This is the exclusive upper bound for event2
	txTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Three events: event1 valid from 2026-01-01 to 2026-01-02 (exclusive),
	// event2 valid from 2026-01-02 to 2026-01-03 (exclusive),
	// event3 valid from 2026-01-03 onward.
	ev1 := event.Event{
		Seq:        1,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"x":1}`),
		ValidFrom:  validFrom1,
		TxTime:     txTime,
		Meta:       []byte(`{}`),
	}

	ev2 := event.Event{
		Seq:        2,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"x":2}`),
		ValidFrom:  validFrom2,
		TxTime:     txTime,
		Meta:       []byte(`{}`),
	}

	ev3 := event.Event{
		Seq:        3,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"x":3}`),
		ValidFrom:  validFrom3,
		TxTime:     txTime,
		Meta:       []byte(`{}`),
	}

	events := []event.Event{ev1, ev2, ev3}

	// Query at exactly validFrom2 should return event2 (inclusive lower bound of event2, exclusive upper bound of event1).
	result, visible := Visible(events, time.Time{}, validFrom2)
	if !visible {
		t.Errorf("Visible at valid_from of event2: got false, want true (new event should be visible at its start)")
	}
	if result.Seq != 2 {
		t.Errorf("Visible at validFrom2: got event seq %d, want 2", result.Seq)
	}

	// Query just before validFrom2 should return event1 (event1's exclusive upper bound).
	justBefore2 := validFrom2.Add(-1 * time.Nanosecond)
	result, visible = Visible(events, time.Time{}, justBefore2)
	if !visible {
		t.Errorf("Visible just before validFrom2: got false, want true")
	}
	if result.Seq != 1 {
		t.Errorf("Visible just before validFrom2: got event seq %d, want 1", result.Seq)
	}

	// Query at exactly validFrom3 should return event3 (tests the upper bound of event2).
	result, visible = Visible(events, time.Time{}, validFrom3)
	if !visible {
		t.Errorf("Visible at valid_from of event3: got false, want true")
	}
	if result.Seq != 3 {
		t.Errorf("Visible at validFrom3: got event seq %d, want 3", result.Seq)
	}

	// Query just before validFrom3 should return event2 (tests event2's exclusive upper bound).
	justBefore3 := validFrom3.Add(-1 * time.Nanosecond)
	result, visible = Visible(events, time.Time{}, justBefore3)
	if !visible {
		t.Errorf("Visible just before validFrom3: got false, want true")
	}
	if result.Seq != 2 {
		t.Errorf("Visible just before validFrom3: got event seq %d, want 2", result.Seq)
	}
}

// TestVisibleIncludesValidFromBoundary verifies that an instant exactly equal to valid_from
// IS visible — the inclusive lower bound of half-open intervals.
func TestVisibleIncludesValidFromBoundary(t *testing.T) {
	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	txTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	ev := event.Event{
		Seq:        1,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"x":1}`),
		ValidFrom:  validFrom,
		TxTime:     txTime,
		Meta:       []byte(`{}`),
	}

	events := []event.Event{ev}

	// Query at exactly valid_from should return true (inclusive lower bound).
	result, visible := Visible(events, time.Time{}, validFrom)
	if !visible {
		t.Errorf("Visible at valid_from boundary: got false, want true")
	}
	if result.Key != "k1" {
		t.Errorf("Visible at valid_from boundary: got key %q, want %q", result.Key, "k1")
	}

	// Query just before valid_from should return false.
	justBefore := validFrom.Add(-1 * time.Nanosecond)
	_, visible = Visible(events, time.Time{}, justBefore)
	if visible {
		t.Errorf("Visible just before valid_from: got true, want false")
	}
}

// TestVisibleIgnoresEventsWrittenAfterAsOf verifies transaction-time travel:
// a later write (higher tx_time) is invisible to an earlier asOf cutoff.
func TestVisibleIgnoresEventsWrittenAfterAsOf(t *testing.T) {
	validTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	txTime1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC) // earlier write
	txTime2 := time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC) // later write

	ev1 := event.Event{
		Seq:        1,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"version":1}`),
		ValidFrom:  validTime,
		TxTime:     txTime1,
		Meta:       []byte(`{}`),
	}

	ev2 := event.Event{
		Seq:        2,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"version":2}`),
		ValidFrom:  validTime,
		TxTime:     txTime2,
		Meta:       []byte(`{}`),
	}

	events := []event.Event{ev1, ev2}

	// Query with asOf at txTime1 (the first write) should see ev1, not ev2.
	result, visible := Visible(events, txTime1, validTime)
	if !visible {
		t.Errorf("Visible at txTime1: got false, want true")
	}
	if string(result.Value) != `{"version":1}` {
		t.Errorf("Visible at txTime1: got value %q, want %q", string(result.Value), `{"version":1}`)
	}

	// Query with asOf between the two writes should still see only ev1.
	midTime := txTime1.Add(2 * time.Hour)
	result, visible = Visible(events, midTime, validTime)
	if !visible {
		t.Errorf("Visible between txTime1 and txTime2: got false, want true")
	}
	if string(result.Value) != `{"version":1}` {
		t.Errorf("Visible between writes: got value %q, want %q", string(result.Value), `{"version":1}`)
	}

	// Query with asOf at or after txTime2 should see ev2 (the latest).
	result, visible = Visible(events, txTime2, validTime)
	if !visible {
		t.Errorf("Visible at txTime2: got false, want true")
	}
	if string(result.Value) != `{"version":2}` {
		t.Errorf("Visible at txTime2: got value %q, want %q", string(result.Value), `{"version":2}`)
	}
}

// TestVisibleReturnsFalseForDeletedKey verifies that a delete event makes the key
// not visible rather than returning a stale version.
func TestVisibleReturnsFalseForDeletedKey(t *testing.T) {
	validTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	txTime1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	txTime2 := time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC)

	ev1 := event.Event{
		Seq:        1,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpPut,
		Value:      []byte(`{"x":1}`),
		ValidFrom:  validTime,
		TxTime:     txTime1,
		Meta:       []byte(`{}`),
	}

	ev2 := event.Event{
		Seq:        2,
		Collection: "test",
		Key:        "k1",
		Op:         event.OpDelete,
		Value:      nil,
		ValidFrom:  validTime,
		TxTime:     txTime2,
		Meta:       []byte(`{}`),
	}

	events := []event.Event{ev1, ev2}

	// Query with asOf before the delete should see the value.
	result, visible := Visible(events, txTime1, validTime)
	if !visible {
		t.Errorf("Visible before delete: got false, want true")
	}
	if result.Op != event.OpPut {
		t.Errorf("Visible before delete: got op %q, want %q", result.Op, event.OpPut)
	}

	// Query with asOf after the delete should return false.
	_, visible = Visible(events, txTime2, validTime)
	if visible {
		t.Errorf("Visible after delete: got true, want false (key is deleted)")
	}
}

// TestParseIsUTCAndRejectsAmbiguousInput verifies that Parse provides one parse path
// in UTC, with no local-zone drift or ambiguous parsing.
func TestParseIsUTCAndRejectsAmbiguousInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantUTC bool // if parsed, verify it's in UTC
	}{
		{
			name:    "RFC3339Nano UTC",
			input:   "2026-01-01T12:34:56.789123456Z",
			wantErr: false,
			wantUTC: true,
		},
		{
			name:    "RFC3339 UTC",
			input:   "2026-01-01T12:34:56Z",
			wantErr: false,
			wantUTC: true,
		},
		{
			name:    "bare date (ambiguous, should reject)",
			input:   "2026-01-01",
			wantErr: true,
		},
		{
			name:    "local time with offset (should reject, not accept)",
			input:   "2026-01-01T12:34:56+05:00",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "not a timestamp",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Parse(%q) error: got %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if tc.wantUTC {
				if got.Location() != time.UTC {
					t.Errorf("Parse(%q) location: got %v, want UTC", tc.input, got.Location())
				}
				// Verify it's actually in UTC by checking the zone offset.
				zone, offset := got.Zone()
				if zone != "UTC" || offset != 0 {
					t.Errorf("Parse(%q) zone info: got %s (offset %d), want UTC (offset 0)", tc.input, zone, offset)
				}
			}
		})
	}
}
