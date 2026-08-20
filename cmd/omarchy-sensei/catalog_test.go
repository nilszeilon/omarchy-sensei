package main

import (
	"strings"
	"testing"
)

func TestParseMenuJSONCPreservesMetadata(t *testing.T) {
	items, err := parseMenuJSONC([]byte(`{
  // comment
  "trigger.reminder.set": {"icon":"bell","label":"Set one","aliases":["reminder-set","remind"],"when":"ready","action":"omarchy-reminder -i"},
}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Parent != "trigger.reminder" || items[0].Aliases[1] != "remind" || items[0].When != "ready" {
		t.Fatalf("unexpected parsed menu: %#v", items)
	}
}

func TestCatalogMatchesExactAndSemanticActions(t *testing.T) {
	menu := []MenuItem{
		{ID: "trigger.capture.screenshot", Label: "Screenshot", Action: "capture"},
		{ID: "trigger.reminder.set", Label: "Set one", Aliases: []string{"reminder-set"}, Action: "remind"},
		{ID: "trigger.capture.color", Label: "Color", Action: "pick"},
		{ID: "install.docker", Label: "Docker", Action: "install"},
	}
	bindings := []Binding{
		{Description: "Screenshot", Shortcuts: []string{"PRINT"}, Dispatcher: "exec", Argument: "capture"},
		{Description: "Set reminder", Shortcuts: []string{"SUPER CTRL + R"}},
		{Description: "Color picker", Shortcuts: []string{"SUPER PRINT"}, Dispatcher: "exec", Argument: "pick"},
	}
	catalog := matchCatalog(menu, bindings)
	if len(catalog.Matches) != 3 || len(catalog.UnmatchedMenu) != 1 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if catalog.Matches[1].Binding.Description != "Set reminder" || catalog.Matches[2].Binding.Description != "Color picker" {
		t.Fatalf("semantic matching chose wrong bindings: %#v", catalog.Matches)
	}
}

func TestAmbiguousTokenMatchIsRejected(t *testing.T) {
	menu := []MenuItem{{ID: "trigger.browser", Label: "Open", Action: "browser"}}
	bindings := []Binding{{Description: "Open browser"}, {Description: "Open editor"}}
	catalog := matchCatalog(menu, bindings)
	if len(catalog.Matches) != 0 || len(catalog.UnmatchedMenu) != 1 {
		t.Fatalf("ambiguous match should remain unmatched: %#v", catalog)
	}
}

func TestLifecycleLabelsDoNotMatchApplicationLaunches(t *testing.T) {
	menu := []MenuItem{{ID: "install.service.signal", Label: "Signal", Action: "omarchy-install-signal"}}
	bindings := []Binding{{Description: "Signal", Shortcuts: []string{"SUPER SHIFT + G"}, Dispatcher: "exec", Argument: "omarchy-launch-signal"}}
	catalog := matchCatalog(menu, bindings)
	if len(catalog.Matches) != 0 {
		t.Fatalf("installing and launching Signal are different actions: %#v", catalog.Matches)
	}
}

func TestExactCommandMatchCanCoachAnyNamespace(t *testing.T) {
	menu := []MenuItem{{ID: "learn.keybindings", Label: "Keybindings", Action: "omarchy-menu-keybindings"}}
	bindings := []Binding{{Description: "Keybindings", Shortcuts: []string{"SUPER + K"}, Dispatcher: "exec", Argument: "omarchy-menu-keybindings"}}
	catalog := matchCatalog(menu, bindings)
	if len(catalog.Matches) != 1 || catalog.Matches[0].Confidence != "command-exact" {
		t.Fatalf("same resolved command should be coached: %#v", catalog)
	}
}

func TestShareClipboardDoesNotMatchClipboardManager(t *testing.T) {
	menu := []MenuItem{{ID: "trigger.share.clipboard", Label: "Clipboard", Action: "omarchy-menu-share clipboard"}}
	bindings := []Binding{{Description: "Clipboard manager", Shortcuts: []string{"SUPER CTRL + V"}, Dispatcher: "exec", Argument: "omarchy-menu-clipboard"}}
	if catalog := matchCatalog(menu, bindings); len(catalog.Matches) != 0 {
		t.Fatalf("sharing clipboard and opening its manager are different: %#v", catalog.Matches)
	}
}

func TestBindingsFromKeybindingsCollectsEveryShortcut(t *testing.T) {
	bindings := bindingsFromKeybindings([]byte("PRINT → Screenshot\nSUPER + P → Screenshot\n"))
	if len(bindings) != 1 || strings.Join(bindings[0].Shortcuts, ",") != "PRINT,SUPER + P" {
		t.Fatalf("unexpected bindings: %#v", bindings)
	}
}
