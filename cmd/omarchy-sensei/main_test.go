package main

import (
	"testing"
	"time"
)

func TestSlowActionOpensTask(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	result := buildSnapshot([]Event{{
		OccurredAt: now,
		Action:     "focus-window",
		Title:      "Focus window",
		Trigger:    "mouse",
		Shortcut:   "SUPER + LEFT/RIGHT",
	}}, now)

	if len(result.Tasks) != 1 {
		t.Fatalf("expected one open task, got %#v", result.Tasks)
	}
	if result.Tasks[0].Shortcut != "SUPER + LEFT/RIGHT" {
		t.Fatalf("expected keyboard hint, got %#v", result.Tasks[0])
	}
}

func TestMatchingShortcutClosesTask(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "menu", Shortcut: "SUPER + B"},
		{OccurredAt: now.Add(time.Second), Action: "browser", Title: "Open browser", Trigger: "shortcut", Shortcut: "SUPER + B"},
	}

	if tasks := buildSnapshot(events, now).Tasks; len(tasks) != 0 {
		t.Fatalf("expected shortcut to close task, got %#v", tasks)
	}
}

func TestSlowActionAfterShortcutReopensTask(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "shortcut", Shortcut: "SUPER + B"},
		{OccurredAt: now.Add(time.Second), Action: "browser", Title: "Open browser", Trigger: "menu", Shortcut: "SUPER + B"},
	}

	if tasks := buildSnapshot(events, now).Tasks; len(tasks) != 1 {
		t.Fatalf("expected later slow use to reopen task, got %#v", tasks)
	}
}

func TestRepeatedSlowUsesStayOneTask(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "screenshot", Title: "Screenshot", Trigger: "menu", Shortcut: "PRINT"},
		{OccurredAt: now.Add(time.Second), Action: "screenshot", Title: "Screenshot", Trigger: "mouse", Shortcut: "PRINT"},
	}

	tasks := buildSnapshot(events, now).Tasks
	if len(tasks) != 1 || tasks[0].SlowUses != 2 {
		t.Fatalf("expected one task with two slow uses, got %#v", tasks)
	}
}

func TestTasksAreOrderedByWorstOffender(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "browser", Title: "Open browser", Trigger: "menu", Shortcut: "SUPER + B"},
		{OccurredAt: now.Add(time.Second), Action: "terminal", Title: "Open terminal", Trigger: "menu", Shortcut: "SUPER + RETURN"},
		{OccurredAt: now.Add(2 * time.Second), Action: "terminal", Title: "Open terminal", Trigger: "mouse", Shortcut: "SUPER + RETURN"},
		{OccurredAt: now.Add(3 * time.Second), Action: "terminal", Title: "Open terminal", Trigger: "menu", Shortcut: "SUPER + RETURN"},
	}

	tasks := buildSnapshot(events, now).Tasks
	if len(tasks) != 2 || tasks[0].Action != "terminal" || tasks[0].SlowUses != 3 {
		t.Fatalf("expected terminal to be the worst offender, got %#v", tasks)
	}
}
