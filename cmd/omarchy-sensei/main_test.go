package main

import (
	"testing"
	"time"
)

func TestBuildSnapshotChoosesWorstUnlearnedAction(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	var events []Event
	for i := 0; i < 7; i++ {
		events = append(events, Event{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "menu", Shortcut: "SUPER+B"})
	}
	for i := 0; i < 3; i++ {
		events = append(events, Event{OccurredAt: now, Action: "terminal", Title: "Open terminal", Trigger: "mouse", Shortcut: "SUPER+RETURN"})
	}
	events = append(events, Event{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "shortcut", Shortcut: "SUPER+B"})

	result := buildSnapshot(events, now)
	if result.Hint == nil || result.Hint.Action != "browser" {
		t.Fatalf("expected browser hint, got %#v", result.Hint)
	}
	if result.Hint.Avoided != 6 {
		t.Fatalf("expected score 6, got %d", result.Hint.Avoided)
	}
	if got := countForDay(result, "2026-08-20"); got != 1 {
		t.Fatalf("expected one shortcut today, got %d", got)
	}
}

func countForDay(snapshot Snapshot, date string) int {
	for _, day := range snapshot.Days {
		if day.Date == date {
			return day.Count
		}
	}
	return -1
}

func TestBuildSnapshotShowsHintAfterFirstSlowUse(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	events := []Event{{
		OccurredAt: now,
		Action:     "screenshot",
		Title:      "Take screenshot",
		Trigger:    "menu",
		Shortcut:   "SUPER+SHIFT+S",
	}}

	result := buildSnapshot(events, now)
	if result.Hint == nil || result.Hint.Action != "screenshot" {
		t.Fatalf("expected immediate screenshot hint, got %#v", result.Hint)
	}
	if result.Hint.SlowUses != 1 {
		t.Fatalf("expected one slow use, got %d", result.Hint.SlowUses)
	}
}

func TestBuildSnapshotMarksToday(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	result := buildSnapshot(nil, now)

	for _, day := range result.Days {
		if day.Date == "2026-08-20" {
			if !day.Today {
				t.Fatal("expected current day to be marked as today")
			}
			return
		}
	}
	t.Fatal("expected current day in snapshot")
}

func TestBuildSnapshotHidesLearnedAction(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	var events []Event
	for i := 0; i < 8; i++ {
		events = append(events, Event{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "menu", Shortcut: "SUPER+B"})
	}
	for i := 0; i < 5; i++ {
		events = append(events, Event{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "shortcut", Shortcut: "SUPER+B"})
	}

	if hint := buildSnapshot(events, now).Hint; hint != nil {
		t.Fatalf("expected learned action to be hidden, got %#v", hint)
	}
}
