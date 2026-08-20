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
	if result.Tasks[0].Shortcut != "SUPER + LEFT/RIGHT" || len(result.Tasks[0].Shortcuts) != 1 {
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

func TestMenuRoutedByShortcutDoesNotReopenTask(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "set-reminder", Title: "Set reminder", Trigger: "menu", Shortcut: "SUPER CTRL + R"},
		{OccurredAt: now.Add(time.Second), Action: "set-reminder", Title: "Set reminder", Trigger: "shortcut", Shortcut: "SUPER + CTRL + R"},
		{OccurredAt: now.Add(1200 * time.Millisecond), Action: "set-reminder", Title: "Set reminder", Trigger: "menu", Shortcut: "SUPER CTRL + R"},
	}

	if tasks := buildSnapshot(events, now).Tasks; len(tasks) != 0 {
		t.Fatalf("expected routed menu consequence to stay closed, got %#v", tasks)
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

func TestTaskIncludesAllResolvedShortcuts(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.Local)
	events := []Event{
		{OccurredAt: now, Action: "screenshot", Title: "Screenshot", Trigger: "shortcut", Shortcut: "SUPER + P"},
		{OccurredAt: now.Add(time.Second), Action: "screenshot", Title: "Screenshot", Trigger: "menu", Shortcut: "PRINT", Shortcuts: []string{"PRINT", "SUPER + P"}},
	}

	tasks := buildSnapshot(events, now).Tasks
	if len(tasks) != 1 || len(tasks[0].Shortcuts) != 2 || tasks[0].Shortcuts[0] != "PRINT" || tasks[0].Shortcuts[1] != "SUPER + P" {
		t.Fatalf("expected all active shortcuts without duplicates, got %#v", tasks)
	}
}

func TestShortcutsFromOmarchyKeybindings(t *testing.T) {
	data := []byte("PRINT                               → Screenshot\nSUPER + P                           → Screenshot\nSUPER ALT + P                       → Color picker\n")

	got := shortcutsFromKeybindings(data, "Screenshot", "FALLBACK")
	if len(got) != 2 || got[0] != "PRINT" || got[1] != "SUPER + P" {
		t.Fatalf("expected every current binding, got %#v", got)
	}
	if got := shortcutsFromKeybindings(data, "Unknown", "SUPER + U"); len(got) != 1 || got[0] != "SUPER + U" {
		t.Fatalf("expected fallback shortcut, got %#v", got)
	}
}

func TestMergeShortcutsNormalizesModifierFormatting(t *testing.T) {
	got := mergeShortcuts([]string{"SUPER CTRL + R"}, "SUPER + CTRL + R")
	if len(got) != 1 || got[0] != "SUPER CTRL + R" {
		t.Fatalf("expected equivalent chords to be deduplicated, got %#v", got)
	}
}

func TestResolveClickBindingWorkspace(t *testing.T) {
	bindings := []Binding{
		{Description: "Switch to workspace 4", Shortcuts: []string{"SUPER + 4"}, Dispatcher: "lua"},
		{Description: "Bar panel 2", Shortcuts: []string{"SUPER CTRL + 2"}, Dispatcher: "exec"},
	}

	got, ok := resolveClickBinding(bindings, ClickContext{Module: "io.example.workspaces", Workspace: 4, Region: "left"})
	if !ok || got.Description != "Switch to workspace 4" {
		t.Fatalf("expected workspace binding, got %#v, %v", got, ok)
	}
}

func TestResolveClickBindingDirectPanel(t *testing.T) {
	bindings := []Binding{
		{Description: "Bar panel 4", Shortcuts: []string{"SUPER CTRL + 4"}, Dispatcher: "exec", Argument: "omarchy-shell -q shell togglePanelAt right 4"},
		{Description: "Bluetooth", Shortcuts: []string{"SUPER CTRL + B"}, Dispatcher: "exec", Argument: "omarchy-shell shell toggle omarchy.bluetooth"},
	}

	got, ok := resolveClickBinding(bindings, ClickContext{Module: "omarchy.bluetooth", Region: "right", PanelIndex: 4})
	if !ok || got.Description != "Bluetooth" {
		t.Fatalf("expected direct semantic panel binding to win, got %#v, %v", got, ok)
	}
}

func TestResolveClickBindingPositionalPanel(t *testing.T) {
	bindings := []Binding{
		{Description: "Bar panel 3", Shortcuts: []string{"SUPER CTRL + 3"}, Dispatcher: "exec", Argument: "omarchy-shell -q shell togglePanelAt right 3"},
	}

	got, ok := resolveClickBinding(bindings, ClickContext{Module: "io.example.custom-panel", Region: "right", PanelIndex: 3})
	if !ok || got.Description != "Bar panel 3" {
		t.Fatalf("expected positional panel binding, got %#v, %v", got, ok)
	}
}

func TestResolveClickBindingRejectsUnteachableClick(t *testing.T) {
	bindings := []Binding{{Description: "Toggle weather", Shortcuts: []string{"SUPER + W"}, Dispatcher: "exec", Argument: "omarchy-notification-weather"}}
	if got, ok := resolveClickBinding(bindings, ClickContext{Module: "omarchy.weather", Region: "center"}); ok {
		t.Fatalf("expected unteachable click to be ignored, got %#v", got)
	}
}

func TestBindingTargetsModuleRequiresExactSemanticCommand(t *testing.T) {
	binding := Binding{Dispatcher: "exec", Argument: "omarchy-shell shell toggle omarchy.network"}
	if !bindingTargetsModule(binding, "omarchy.network") {
		t.Fatal("expected exact module command to match")
	}
	if bindingTargetsModule(binding, "omarchy.net") || bindingTargetsModule(binding, "omarchy.network.extra") {
		t.Fatal("partial module ids must not match")
	}
	if bindingTargetsModule(Binding{Dispatcher: "exec", Argument: "notify-send omarchy.network"}, "omarchy.network") {
		t.Fatal("non-shell commands must not match")
	}
}
