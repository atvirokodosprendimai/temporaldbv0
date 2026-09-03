// Package temporal implements bitemporal visibility logic for TemporalDB.
// It provides the single point where all interval comparisons happen, preventing
// inclusive-end bugs (the documented gotcha from the ADR's Existing Primitives Audit).
package temporal

import (
	"fmt"
	"sort"
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

	// Filter events by asOf to get the set of events to consider.
	var candidates []event.Event
	for _, ev := range events {
		// Filter by transaction time: only consider events up to asOf.
		// If asOf is zero, consider all events.
		if asOf != (time.Time{}) && ev.TxTime.After(asOf) {
			// This event was written after the asOf cutoff, skip it.
			continue
		}
		candidates = append(candidates, ev)
	}

	if len(candidates) == 0 {
		return event.Event{}, false
	}

	// Build a map of ValidFrom -> latest event (highest Seq) for each distinct ValidFrom.
	// This deduplicates: if two events have the same ValidFrom, we only consider the latest one.
	type versionInfo struct {
		Event event.Event
	}
	versionMap := make(map[int64]*versionInfo) // Key is ValidFrom.UnixNano()

	for i, ev := range candidates {
		key := ev.ValidFrom.UnixNano()
		if existing, found := versionMap[key]; !found || ev.Seq > existing.Event.Seq {
			versionMap[key] = &versionInfo{Event: candidates[i]}
		}
	}

	// Collect distinct ValidFrom values (as nanoseconds since epoch) and sort them.
	var validFroms []int64
	for key := range versionMap {
		validFroms = append(validFroms, key)
	}
	sort.Slice(validFroms, func(i, j int) bool {
		return validFroms[i] < validFroms[j]
	})

	// Find the event whose interval [ValidFrom, NextValidFrom) contains validAt.
	// Iterate through sorted distinct ValidFroms and check the interval [ValidFrom_i, ValidFrom_{i+1}).
	for i, validFromNano := range validFroms {
		validFrom := time.Unix(0, validFromNano)
		ev := versionMap[validFromNano].Event

		// Check lower bound: validAt >= ValidFrom (inclusive).
		if validAt.Before(validFrom) {
			// validAt is before this event's ValidFrom, so it's not in this interval yet.
			continue
		}

		// Check upper bound: validAt < NextValidFrom (exclusive).
		// If there's a next version, check that validAt is strictly before it.
		if i+1 < len(validFroms) {
			nextValidFrom := time.Unix(0, validFroms[i+1])
			if !validAt.Before(nextValidFrom) {
				// validAt is at or after the next event's ValidFrom, so it's not in this interval.
				// This check is the critical boundary enforcement: [valid_from, next_valid_from).
				continue
			}
		}
		// If i+1 >= len(validFroms), there is no next version, so validAt is always < infinity.

		// Found the interval that contains validAt.
		// If the event is a delete, the key is not visible even though the interval contains validAt.
		if ev.Op == event.OpDelete {
			return event.Event{}, false
		}

		return ev, true
	}

	// validAt does not fall within any event's interval.
	return event.Event{}, false
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
