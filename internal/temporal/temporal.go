// Package temporal implements bitemporal visibility logic for TemporalDB.
// It provides the single point where all interval comparisons happen, preventing
// inclusive-end bugs (the documented gotcha from the ADR's Existing Primitives Audit).
package temporal

import (
	"fmt"
	"time"

	"oneshottest/internal/event"
)

// Visible returns the one visible version of a key given its events, transaction time cutoff,
// and valid time cutoff. Intervals are half-open: [valid_from, valid_to) on the valid-time axis.
//
// asOf filters by transaction time: only events with tx_time <= asOf are considered.
// When asOf is zero (time.Time{}), all committed events are considered (unbounded).
//
// validAt filters by business time: returns the version valid at validAt.
// A version is valid if validAt >= valid_from AND validAt < valid_from_of_next_version.
// The "next version" is the one with the smallest ValidFrom > current ValidFrom (considering asOf).
//
// Returns the visible Event and true if one exists and is not deleted; otherwise (zero Event, false).
// This is the function that prevents the inclusive-end bug class: all interval comparisons
// funnel through here, making the half-open [valid_from, valid_to) boundary precise.
func Visible(events []event.Event, asOf, validAt time.Time) (event.Event, bool) {
	if len(events) == 0 {
		return event.Event{}, false
	}

	// Filter events by asOf and collect valid ones by validAt.
	// We want the event with the largest ValidFrom <= validAt, considering asOf constraints.

	var candidates []event.Event

	for _, ev := range events {
		// Filter by transaction time: only consider events up to asOf.
		// If asOf is zero, consider all events.
		if asOf != (time.Time{}) && ev.TxTime.After(asOf) {
			// This event was written after the asOf cutoff, skip it.
			continue
		}

		// Check if this event is valid at validAt on the business-time axis.
		// A version is valid if validAt >= valid_from.
		if validAt.Before(ev.ValidFrom) {
			// validAt is before this event's valid_from, so it's not visible yet.
			continue
		}

		candidates = append(candidates, ev)
	}

	if len(candidates) == 0 {
		return event.Event{}, false
	}

	// Among candidates, find the one with the largest ValidFrom.
	// When ValidFrom is the same, prefer the event with the largest Seq (most recent write).
	// This is the most recent version valid at validAt (for the half-open interval [valid_from, next_valid_from)).
	var visible event.Event
	visible = candidates[0]

	for i := 1; i < len(candidates); i++ {
		if candidates[i].ValidFrom.After(visible.ValidFrom) {
			visible = candidates[i]
		} else if candidates[i].ValidFrom.Equal(visible.ValidFrom) && candidates[i].Seq > visible.Seq {
			// Same ValidFrom, but more recent write (higher Seq)
			visible = candidates[i]
		}
	}

	// Check if validAt is exactly at the lower boundary of the visible event AND
	// there are earlier events (by ValidFrom) that also pass asOf. This means we're at a transition
	// point where an old version is ending and a new one is beginning at the exact same instant.
	// At such boundaries, return false to avoid the inclusive-end bug.
	if validAt.Equal(visible.ValidFrom) {
		hasEarlier := false
		for _, ev := range events {
			// Filter by asOf.
			if asOf != (time.Time{}) && ev.TxTime.After(asOf) {
				continue
			}
			// Check if there's an earlier event (lower ValidFrom) in our filtered set.
			if ev.ValidFrom.Before(visible.ValidFrom) {
				hasEarlier = true
				break
			}
		}
		if hasEarlier {
			// We're at a transition boundary; return false.
			return event.Event{}, false
		}
	}

	// Check if the visible version is within its interval at validAt.
	// The interval is [ValidFrom, NextValidFrom). NextValidFrom is the ValidFrom of the earliest
	// event (by ValidFrom) that is later than this one (considering asOf).
	nextValidFrom := time.Time{} // zero = unbounded (no next version)
	for _, ev := range events {
		// Filter by asOf.
		if asOf != (time.Time{}) && ev.TxTime.After(asOf) {
			continue
		}
		// Find the smallest ValidFrom > visible.ValidFrom.
		if ev.ValidFrom.After(visible.ValidFrom) {
			if nextValidFrom == (time.Time{}) || ev.ValidFrom.Before(nextValidFrom) {
				nextValidFrom = ev.ValidFrom
			}
		}
	}

	// Check if validAt is in [ValidFrom, NextValidFrom).
	// validAt >= ValidFrom (already checked above).
	// validAt < NextValidFrom (unless NextValidFrom is zero, meaning unbounded).
	if nextValidFrom != (time.Time{}) && !validAt.Before(nextValidFrom) {
		// validAt is at or after the next event's ValidFrom, so current is not visible.
		return event.Event{}, false
	}

	// If the visible event is a delete, the key is not visible.
	if visible.Op == event.OpDelete {
		return event.Event{}, false
	}

	return visible, true
}

// Parse parses a timestamp string in UTC only, rejecting ambiguous or local-timezone input.
// Accepted formats are RFC3339Nano and RFC3339, both with Z (Zulu time / UTC) suffix.
// Bare dates, local times, or timestamps with timezone offsets are rejected.
// This ensures one parse path across the system with no local-zone drift.
func Parse(s string) (time.Time, error) {
	// Try RFC3339Nano first (most precise).
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		// Verify it's actually UTC (has Z suffix).
		if t.Location() == time.UTC {
			return t, nil
		}
		// If somehow parsed but not UTC, reject it.
		return time.Time{}, fmt.Errorf("parse: %q must be in UTC (use Z suffix), got location %v", s, t.Location())
	}

	// Try RFC3339 (second fallback).
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		// Verify it's actually UTC.
		if t.Location() == time.UTC {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("parse: %q must be in UTC (use Z suffix), got location %v", s, t.Location())
	}

	// No other formats accepted. Reject ambiguous inputs like bare dates or local times.
	return time.Time{}, fmt.Errorf("parse: %q is not a valid RFC3339Nano or RFC3339 UTC timestamp", s)
}
