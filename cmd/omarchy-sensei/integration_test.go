package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(string(lua), "function hl.bind") || !strings.Contains(string(lua), "shortcut") {
		t.Fatalf("generated integration does not wrap bindings:\n%s", lua)
	}
	if strings.Contains(string(lua), "record_options.transparent") || !strings.Contains(string(lua), "hl.dispatch(dispatcher)") {
		t.Fatalf("generated observer must dispatch the original action in one binding:\n%s", lua)
	}
	if !strings.Contains(string(lua), "dispatcher.__omarchy_dispatcher") {
		t.Fatalf("generated observer must preserve Super+K source scanning:\n%s", lua)
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

func TestPausePreventsRecording(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{StateDir: dir, Events: filepath.Join(dir, "events.jsonl"), Paused: filepath.Join(dir, "paused")}
	if err := setPaused(paths, true); err != nil {
		t.Fatal(err)
	}
	if err := recordEvent(paths, Event{Action: "test", Title: "Test", Trigger: "shortcut"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Events); !os.IsNotExist(err) {
		t.Fatalf("paused recorder wrote an event")
	}
}
