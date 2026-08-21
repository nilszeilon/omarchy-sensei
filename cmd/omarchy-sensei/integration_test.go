package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInstallHyprIntegrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{
		HyprlandConfig: filepath.Join(dir, "hyprland.lua"),
		SenseiLua:      filepath.Join(dir, "sensei.lua"),
	}
	original := "-- Load Omarchy defaults.\nrequire(\"default.hypr.omarchy\")\n"
	if err := os.WriteFile(paths.HyprlandConfig, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installHyprIntegration(paths); err != nil {
		t.Fatal(err)
	}
	if err := installHyprIntegration(paths); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(paths.HyprlandConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), hyprStart) != 1 {
		t.Fatalf("expected one managed block:\n%s", content)
	}
	lua, err := os.ReadFile(paths.SenseiLua)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lua), "function hl.bind") || !strings.Contains(string(lua), "omarchy-sensei complete") {
		t.Fatalf("generated integration does not wrap bindings:\n%s", lua)
	}
	if strings.Contains(string(lua), "--trigger") || strings.Contains(string(lua), "--shortcut") {
		t.Fatalf("generated integration must not pass or retain shortcut chords:\n%s", lua)
	}
	if strings.Contains(string(lua), "record_options.transparent") || !strings.Contains(string(lua), "hl.dispatch(dispatcher)") {
		t.Fatalf("generated observer must dispatch the original action in one binding:\n%s", lua)
	}
	if !strings.Contains(string(lua), "dispatcher.__omarchy_dispatcher") {
		t.Fatalf("generated observer must preserve Super+K source scanning:\n%s", lua)
	}
	if !strings.Contains(string(lua), `return "workspace-switching", "Workspace switching"`) ||
		!strings.Contains(string(lua), `return "bar-panels", "Bar panels"`) {
		t.Fatalf("generated observer must collapse equivalent workspace and positional panel shortcuts:\n%s", lua)
	}
	if luac, err := exec.LookPath("luac"); err == nil {
		if output, err := exec.Command(luac, "-p", paths.SenseiLua).CombinedOutput(); err != nil {
			t.Fatalf("generated Lua is invalid: %v\n%s", err, output)
		}
	}
}

func TestMenuIntegrationPreservesUserEntriesAndUninstalls(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{MenuExtension: filepath.Join(dir, "omarchy-menu.jsonc")}
	original := "{\n  // mine\n  \"personal.notes\": {\"label\":\"Notes\",\"action\":\"notes\"},\n}\n"
	if err := os.WriteFile(paths.MenuExtension, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	matches := []CatalogMatch{{
		Menu:       MenuItem{ID: "trigger.reminder.set", Parent: "trigger.reminder", Label: "Set one", Icon: "bell", Aliases: []string{"reminder-set", "remind"}, Action: "omarchy-reminder -i"},
		Binding:    Binding{Description: "Set reminder", Shortcuts: []string{"SUPER CTRL + R"}},
		Confidence: "token-exact",
	}}
	if err := installMenuIntegration(paths, Catalog{Matches: matches}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(paths.MenuExtension)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "personal.notes") || !strings.Contains(string(content), "trigger.reminder.set") {
		t.Fatalf("expected user and Sensei entries:\n%s", content)
	}
	if !strings.Contains(string(content), `"aliases":["reminder-set","remind"]`) {
		t.Fatalf("expected reminder route aliases to be preserved:\n%s", content)
	}
	block := strings.TrimSuffix(strings.TrimSpace(menuOverrideBlock(matches)), ",")
	var overrides map[string]map[string]any
	if err := json.Unmarshal([]byte("{"+block+"}"), &overrides); err != nil {
		t.Fatalf("generated menu overrides are invalid: %v\n%s", err, block)
	}
	if len(overrides) != len(matches) {
		t.Fatalf("expected %d menu overrides, got %d", len(matches), len(overrides))
	}
	if overrides["trigger.reminder.set"]["icon"] != "bell" || overrides["trigger.reminder.set"]["label"] != "Set one" {
		t.Fatalf("generated override lost menu metadata: %#v", overrides)
	}
	if err := removeManagedBlock(paths.MenuExtension, menuStart, menuEnd); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(paths.MenuExtension)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), menuStart) || !strings.Contains(string(content), "personal.notes") {
		t.Fatalf("uninstall did not preserve user entry:\n%s", content)
	}
}

func TestPausePreventsCoachingUpdate(t *testing.T) {
	dir := t.TempDir()
	paths := testStatePaths(dir)
	if err := setPaused(paths, true); err != nil {
		t.Fatal(err)
	}
	if err := updateCoachingState(paths, Observation{Action: "test", Title: "Test", Trigger: "shortcut"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.State); !os.IsNotExist(err) {
		t.Fatalf("paused coaching wrote state")
	}
}

func TestLegacyHistoryIsCompactedAndDeleted(t *testing.T) {
	dir := t.TempDir()
	paths := testStatePaths(dir)
	old := strings.Join([]string{
		`{"occurredAt":"2026-08-21T12:00:00Z","action":"browser","title":"Open browser","trigger":"menu","shortcut":"SUPER + B"}`,
		`{"occurredAt":"2026-08-21T12:00:01Z","action":"browser","title":"Open browser","trigger":"shortcut","shortcut":"SUPER + B"}`,
		`{"occurredAt":"2026-08-21T12:00:02Z","action":"terminal","title":"Open terminal","trigger":"mouse","shortcut":"SUPER + RETURN"}`,
		`{"occurredAt":"2026-08-21T12:00:03Z","action":"terminal","title":"Open terminal","trigger":"mouse","shortcut":"SUPER + RETURN"}`,
		`{"occurredAt":"2026-08-21T12:00:04Z","action":"focus-next","title":"Focus next","trigger":"shortcut","shortcut":"ALT + TAB"}`,
		`{"occurredAt":"2026-08-21T12:00:04.020Z","action":"reveal-active","title":"Reveal active","trigger":"shortcut","shortcut":"ALT + TAB"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(paths.LegacyEvents, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := initializeState(paths); err != nil {
		t.Fatal(err)
	}
	state, err := readState(paths, time.Date(2026, 8, 21, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if state.TotalShortcuts != 2 || len(state.Tasks) != 1 || state.Tasks[0].Action != "terminal" || state.Tasks[0].SlowUses != 2 {
		t.Fatalf("migration did not preserve compact progress: %#v", state)
	}
	if _, err := os.Stat(paths.LegacyEvents); !os.IsNotExist(err) {
		t.Fatalf("legacy history still exists after migration: %v", err)
	}
	data, err := os.ReadFile(paths.State)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"trigger"`, `"occurredAt"`, "focus-next", "SUPER + B", "ALT + TAB"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("compact state retained historical detail %q:\n%s", forbidden, data)
		}
	}
	if err := initializeState(paths); err != nil {
		t.Fatal(err)
	}
	state, err = readState(paths, time.Now())
	if err != nil || state.TotalShortcuts != 2 {
		t.Fatalf("migration was not idempotent: %#v, %v", state, err)
	}
}

func TestConcurrentCoachingUpdatesAreNotLost(t *testing.T) {
	dir := t.TempDir()
	paths := testStatePaths(dir)
	const updates = 40
	var group sync.WaitGroup
	group.Add(updates)
	for index := 0; index < updates; index++ {
		go func() {
			defer group.Done()
			err := updateCoachingState(paths, Observation{
				ObservedAt: time.Now(), Action: "terminal", Title: "Open terminal",
				Trigger: "mouse", Shortcut: "SUPER + RETURN",
			})
			if err != nil {
				t.Errorf("update coaching state: %v", err)
			}
		}()
	}
	group.Wait()
	state, err := readState(paths, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Tasks) != 1 || state.Tasks[0].SlowUses != updates {
		t.Fatalf("concurrent updates were lost: %#v", state.Tasks)
	}
}

func testStatePaths(dir string) Paths {
	return Paths{
		StateDir:     dir,
		State:        filepath.Join(dir, "state.json"),
		StateLock:    filepath.Join(dir, "state.lock"),
		LegacyEvents: filepath.Join(dir, "events.jsonl"),
		Paused:       filepath.Join(dir, "paused"),
	}
}

func TestInstallBindingCacheIncludesEveryBindingOnce(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{BindingCache: filepath.Join(dir, "bindings.json")}
	catalog := Catalog{
		Matches: []CatalogMatch{
			{Binding: Binding{Description: "Bluetooth", Shortcuts: []string{"SUPER CTRL + B"}}},
			{Binding: Binding{Description: "Bluetooth", Shortcuts: []string{"SUPER CTRL + B"}}},
		},
		UnmatchedBindings: []Binding{{Description: "Bar panel 2", Shortcuts: []string{"SUPER CTRL + 2"}}},
	}
	if err := installBindingCache(paths, catalog); err != nil {
		t.Fatal(err)
	}
	bindings, err := loadBindingCache(paths.BindingCache)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 || bindings[0].Description != "Bluetooth" || bindings[1].Description != "Bar panel 2" {
		t.Fatalf("expected deduplicated matched and unmatched bindings, got %#v", bindings)
	}
}
