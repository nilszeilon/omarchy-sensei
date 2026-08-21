package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Event struct {
	OccurredAt time.Time `json:"occurredAt"`
	Action     string    `json:"action"`
	Title      string    `json:"title"`
	Trigger    string    `json:"trigger"`
	Shortcut   string    `json:"shortcut,omitempty"`
	Shortcuts  []string  `json:"shortcuts,omitempty"`
}

type Task struct {
	Action    string    `json:"action"`
	Title     string    `json:"title"`
	Shortcut  string    `json:"shortcut"`
	Shortcuts []string  `json:"shortcuts"`
	OpenedAt  time.Time `json:"openedAt"`
	SlowUses  int       `json:"slowUses"`
}

type Snapshot struct {
	Tasks  []Task `json:"tasks"`
	Paused bool   `json:"paused"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: omarchy-sensei <setup|refresh|catalog|doctor|uninstall|record|coach-click|run|snapshot|pause|resume|clear|status>")
	}

	paths, err := senseiPaths()
	if err != nil {
		fatal(err.Error())
	}

	switch os.Args[1] {
	case "setup":
		if err := setupIntegration(paths); err != nil {
			fatal(err.Error())
		}
		fmt.Println("Omarchy Sensei observation is installed. Run `hyprctl reload` to activate it.")
	case "refresh":
		catalog, err := refreshIntegration(paths)
		if err != nil {
			fatal(err.Error())
		}
		fmt.Printf("Sensei catalog refreshed: %d coached menu actions, %d unmatched.\n", len(catalog.Matches), len(catalog.UnmatchedMenu))
	case "catalog":
		printCatalog(paths, os.Args[2:])
	case "doctor":
		doctor(paths)
	case "uninstall":
		if err := uninstallIntegration(paths); err != nil {
			fatal(err.Error())
		}
		fmt.Println("Omarchy Sensei observation was removed. Your activity data was kept.")
	case "record":
		record(paths, os.Args[2:])
	case "coach-click":
		coachClick(paths, os.Args[2:])
	case "run":
		run(paths, os.Args[2:])
	case "snapshot":
		snapshot(paths)
	case "pause":
		if err := setPaused(paths, true); err != nil {
			fatal(err.Error())
		}
	case "resume":
		if err := setPaused(paths, false); err != nil {
			fatal(err.Error())
		}
	case "clear":
		if err := clearEvents(paths); err != nil {
			fatal(err.Error())
		}
	case "status":
		printStatus(paths)
	default:
		fatal("unknown command: " + os.Args[1])
	}
}

func coachClick(paths Paths, args []string) {
	flags := flag.NewFlagSet("coach-click", flag.ExitOnError)
	module := flags.String("module", "", "Omarchy bar module that received the click")
	workspace := flags.Int("workspace", 0, "semantic workspace number, when clicked")
	region := flags.String("region", "", "bar region containing the module")
	panelIndex := flags.Int("panel-index", 0, "one-based keyboard panel index")
	_ = flags.Parse(args)

	moduleID := strings.TrimSpace(*module)
	if moduleID == "" {
		fatal("coach-click requires --module")
	}

	bindings, err := loadBindingCache(paths.BindingCache)
	if err != nil {
		// A click must never be delayed or broken by stale diagnostics. The
		// setup/refresh watcher rebuilds this tiny semantic cache after remaps.
		return
	}
	binding, ok := resolveClickBinding(bindings, ClickContext{
		Module:     moduleID,
		Workspace:  *workspace,
		Region:     strings.TrimSpace(*region),
		PanelIndex: *panelIndex,
	})
	if !ok || len(binding.Shortcuts) == 0 {
		return
	}
	binding = groupedClickBinding(bindings, *workspace, binding)

	if err := recordEvent(paths, Event{
		OccurredAt: time.Now(),
		Action:     actionID(binding.Description),
		Title:      binding.Description,
		Trigger:    "mouse",
		Shortcut:   binding.Shortcuts[0],
		Shortcuts:  binding.Shortcuts,
	}); err != nil {
		fatal(err.Error())
	}
}

func groupedClickBinding(bindings []Binding, workspace int, binding Binding) Binding {
	if workspace > 0 {
		return Binding{Description: "Workspace switching", Shortcuts: []string{"SUPER + TAB"}}
	}
	if !isPositionalPanelDescription(binding.Description) {
		return binding
	}
	hint, ok := bindingWithDescription(bindings, "Bar panel 1")
	if !ok || len(hint.Shortcuts) == 0 {
		hint = binding
	}
	hint.Description = "Bar panels"
	return hint
}

func loadBindingCache(path string) ([]Binding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bindings []Binding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

type ClickContext struct {
	Module     string
	Workspace  int
	Region     string
	PanelIndex int
}

func resolveClickBinding(bindings []Binding, click ClickContext) (Binding, bool) {
	if click.Workspace > 0 && strings.Contains(strings.ToLower(click.Module), "workspace") {
		return bindingWithDescription(bindings, fmt.Sprintf("Switch to workspace %d", click.Workspace))
	}

	for _, binding := range bindings {
		if bindingTargetsModule(binding, click.Module) {
			return binding, true
		}
	}

	if strings.EqualFold(click.Region, "right") && click.PanelIndex > 0 {
		return bindingWithDescription(bindings, fmt.Sprintf("Bar panel %d", click.PanelIndex))
	}
	return Binding{}, false
}

func bindingWithDescription(bindings []Binding, description string) (Binding, bool) {
	for _, binding := range bindings {
		if strings.EqualFold(strings.TrimSpace(binding.Description), description) {
			return binding, true
		}
	}
	return Binding{}, false
}

func bindingTargetsModule(binding Binding, module string) bool {
	if !strings.EqualFold(binding.Dispatcher, "exec") || module == "" {
		return false
	}
	fields := strings.Fields(binding.Argument)
	foundShell, foundAction, foundModule := false, false, false
	for _, field := range fields {
		field = strings.Trim(field, "'\"")
		switch {
		case strings.HasSuffix(field, "omarchy-shell"):
			foundShell = true
		case field == "toggle" || field == "summon" || field == "open" || field == "show":
			foundAction = true
		case field == module:
			foundModule = true
		}
	}
	return foundShell && foundAction && foundModule
}

func doctor(paths Paths) {
	catalog, err := loadCatalog(paths)
	if err != nil {
		fatal("catalog: " + err.Error())
	}
	if len(catalog.Matches) == 0 {
		fatal("catalog has no coached actions")
	}
	if !integrationInstalled(paths) {
		fatal("Hyprland integration is not installed")
	}
	menu, err := os.ReadFile(paths.MenuExtension)
	if err != nil || !strings.Contains(string(menu), menuStart) {
		fatal("generated menu integration is not installed")
	}
	observer, err := os.ReadFile(paths.SenseiLua)
	if err != nil || !strings.Contains(string(observer), "hl.dispatch(dispatcher)") {
		fatal("generic shortcut observer is not installed")
	}
	bindings, err := loadBindingCache(paths.BindingCache)
	if err != nil || len(bindings) == 0 {
		fatal("semantic click binding cache is not installed")
	}
	if output, err := exec.Command("systemctl", "--user", "is-active", "omarchy-sensei-refresh.path").CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "active" {
		fatal("catalog refresh watcher is not active")
	}
	if output, err := exec.Command("hyprctl", "configerrors").Output(); err != nil || strings.TrimSpace(string(output)) != "" {
		fatal("Hyprland reports configuration errors")
	}
	fmt.Printf("Sensei is healthy: %d coached menu actions, %d unmatched menu actions, refresh watcher active.\n", len(catalog.Matches), len(catalog.UnmatchedMenu))
}

func record(paths Paths, args []string) {
	flags := flag.NewFlagSet("record", flag.ExitOnError)
	action := flags.String("action", "", "stable semantic action id")
	title := flags.String("title", "", "human-readable action name")
	trigger := flags.String("trigger", "", "shortcut, menu, mouse, command, or agent")
	shortcut := flags.String("shortcut", "", "known shortcut for the action")
	_ = flags.Parse(args)

	event := Event{
		OccurredAt: time.Now(),
		Action:     strings.TrimSpace(*action),
		Title:      strings.TrimSpace(*title),
		Trigger:    strings.TrimSpace(*trigger),
		Shortcut:   strings.TrimSpace(*shortcut),
	}
	if event.Action == "" || event.Title == "" || !validTrigger(event.Trigger) {
		fatal("record requires --action, --title, and a valid --trigger")
	}
	if event.Trigger != "shortcut" {
		event.Shortcuts = resolveCurrentShortcuts(event.Title, event.Shortcut)
		if len(event.Shortcuts) > 0 {
			event.Shortcut = event.Shortcuts[0]
		}
	}
	if event.Trigger != "shortcut" && event.Shortcut == "" {
		fatal("non-shortcut actions require --shortcut so Sensei can teach them")
	}

	if err := recordEvent(paths, event); err != nil {
		fatal(err.Error())
	}
}

func run(paths Paths, args []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	action := flags.String("action", "", "stable semantic action id")
	title := flags.String("title", "", "human-readable action name")
	shortcut := flags.String("shortcut", "", "known shortcut for the action")
	_ = flags.Parse(args)
	command := flags.Args()
	if *action == "" || *title == "" || *shortcut == "" || len(command) != 1 {
		fatal("run requires --action, --title, --shortcut, and one command after --")
	}
	currentShortcuts := resolveCurrentShortcuts(strings.TrimSpace(*title), strings.TrimSpace(*shortcut))
	if err := recordEvent(paths, Event{
		OccurredAt: time.Now(),
		Action:     strings.TrimSpace(*action),
		Title:      strings.TrimSpace(*title),
		Trigger:    "menu",
		Shortcut:   currentShortcuts[0],
		Shortcuts:  currentShortcuts,
	}); err != nil {
		fatal(err.Error())
	}

	cmd := exec.Command("bash", "-lc", command[0])
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fatal(err.Error())
	}
}

func snapshot(paths Paths) {
	events, err := loadEvents(paths.Events)
	if err != nil {
		fatal(err.Error())
	}
	result := buildSnapshot(events, time.Now())
	result.Paused = isPaused(paths)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fatal(err.Error())
	}
}

func validTrigger(trigger string) bool {
	switch trigger {
	case "shortcut", "menu", "mouse", "command", "agent":
		return true
	default:
		return false
	}
}

type Paths struct {
	Home           string
	StateDir       string
	Events         string
	BindingCache   string
	Paused         string
	HyprlandConfig string
	SenseiLua      string
	MenuExtension  string
	DefaultMenu    string
	LocalBinary    string
	RefreshService string
	RefreshPath    string
	PostUpdateHook string
}

func senseiPaths() (Paths, error) {
	state := os.Getenv("XDG_STATE_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	stateDir := filepath.Join(state, "omarchy-sensei")
	return Paths{
		Home:           home,
		StateDir:       stateDir,
		Events:         filepath.Join(stateDir, "events.jsonl"),
		BindingCache:   filepath.Join(stateDir, "bindings.json"),
		Paused:         filepath.Join(stateDir, "paused"),
		HyprlandConfig: filepath.Join(home, ".config", "hypr", "hyprland.lua"),
		SenseiLua:      filepath.Join(home, ".config", "hypr", "sensei.lua"),
		MenuExtension:  filepath.Join(home, ".config", "omarchy", "extensions", "omarchy-menu.jsonc"),
		DefaultMenu:    filepath.Join("/usr/share/omarchy", "default", "omarchy", "omarchy-menu.jsonc"),
		LocalBinary:    filepath.Join(home, ".local", "bin", "omarchy-sensei"),
		RefreshService: filepath.Join(home, ".config", "systemd", "user", "omarchy-sensei-refresh.service"),
		RefreshPath:    filepath.Join(home, ".config", "systemd", "user", "omarchy-sensei-refresh.path"),
		PostUpdateHook: filepath.Join(home, ".config", "omarchy", "hooks", "post-update.d", "omarchy-sensei"),
	}, nil
}

func printCatalog(paths Paths, args []string) {
	flags := flag.NewFlagSet("catalog", flag.ExitOnError)
	unmatched := flags.Bool("unmatched", false, "show only unmatched menu actions and bindings")
	jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
	_ = flags.Parse(args)
	catalog, err := loadCatalog(paths)
	if err != nil {
		fatal(err.Error())
	}
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(catalog); err != nil {
			fatal(err.Error())
		}
		return
	}
	if !*unmatched {
		for _, match := range catalog.Matches {
			fmt.Printf("✓ %-38s → %-32s %s\n", match.Menu.ID, match.Binding.Description, strings.Join(match.Binding.Shortcuts, " / "))
		}
	}
	for _, item := range catalog.UnmatchedMenu {
		fmt.Printf("· %-38s (no shortcut match for %q)\n", item.ID, item.Label)
	}
	if *unmatched {
		for _, binding := range catalog.UnmatchedBindings {
			fmt.Printf("⌨ %-38s (no matching menu action; %s)\n", binding.Description, strings.Join(binding.Shortcuts, " / "))
		}
	}
	if !*unmatched {
		fmt.Printf("\n%d coached menu actions; %d unmatched menu actions; %d shortcut-only actions.\n",
			len(catalog.Matches), len(catalog.UnmatchedMenu), len(catalog.UnmatchedBindings))
	}
}

func recordEvent(paths Paths, event Event) error {
	if isPaused(paths) {
		return nil
	}
	return appendEvent(paths.Events, event)
}

func appendEvent(path string, event Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(event)
}

func loadEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []Event
	decoder := json.NewDecoder(file)
	for {
		var event Event
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func setPaused(paths Paths, paused bool) error {
	if paused {
		if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
			return err
		}
		return os.WriteFile(paths.Paused, []byte("paused\n"), 0o600)
	}
	err := os.Remove(paths.Paused)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func isPaused(paths Paths) bool {
	_, err := os.Stat(paths.Paused)
	return err == nil
}

func clearEvents(paths Paths) error {
	err := os.Remove(paths.Events)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func printStatus(paths Paths) {
	events, err := loadEvents(paths.Events)
	if err != nil {
		fatal(err.Error())
	}
	status := struct {
		Paused    bool `json:"paused"`
		Events    int  `json:"events"`
		Installed bool `json:"installed"`
	}{isPaused(paths), len(events), integrationInstalled(paths)}
	if err := json.NewEncoder(os.Stdout).Encode(status); err != nil {
		fatal(err.Error())
	}
}

func buildSnapshot(events []Event, now time.Time) Snapshot {
	_ = now
	ordered := append([]Event(nil), events...)
	for index := range ordered {
		ordered[index] = normalizeLearningEvent(ordered[index])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})

	open := map[string]*Task{}
	knownShortcuts := map[string][]string{}
	lastShortcutAt := map[string]time.Time{}
	for _, event := range ordered {
		if event.Trigger == "shortcut" && event.Shortcut != "" && !isGroupedAction(event.Action) {
			knownShortcuts[event.Action] = mergeShortcuts(knownShortcuts[event.Action], event.Shortcut)
		}
	}
	for _, event := range ordered {
		if event.Action == "" {
			continue
		}
		switch event.Trigger {
		case "menu", "mouse":
			// Some shortcuts intentionally route through an instrumented Omarchy
			// menu action. That menu event is the shortcut's consequence, not a
			// second slow use, so do not reopen the task it just completed.
			if event.Trigger == "menu" && !lastShortcutAt[event.Action].IsZero() &&
				event.OccurredAt.Sub(lastShortcutAt[event.Action]) >= 0 &&
				event.OccurredAt.Sub(lastShortcutAt[event.Action]) < time.Second {
				continue
			}
			shortcuts := eventShortcuts(event)
			if len(shortcuts) == 0 {
				continue
			}
			task := open[event.Action]
			if task == nil {
				task = &Task{Action: event.Action, OpenedAt: event.OccurredAt}
				open[event.Action] = task
			}
			task.Title = event.Title
			task.Shortcuts = mergeShortcuts(shortcuts, knownShortcuts[event.Action]...)
			task.Shortcut = task.Shortcuts[0]
			task.SlowUses++
		case "shortcut":
			lastShortcutAt[event.Action] = event.OccurredAt
			delete(open, event.Action)
		}
	}

	result := Snapshot{Tasks: make([]Task, 0, len(open))}
	for _, task := range open {
		result.Tasks = append(result.Tasks, *task)
	}
	sort.Slice(result.Tasks, func(i, j int) bool {
		if result.Tasks[i].SlowUses != result.Tasks[j].SlowUses {
			return result.Tasks[i].SlowUses > result.Tasks[j].SlowUses
		}
		return result.Tasks[i].OpenedAt.Before(result.Tasks[j].OpenedAt)
	})
	return result
}

func normalizeLearningEvent(event Event) Event {
	if isWorkspaceSwitchDescription(event.Title) || strings.HasPrefix(event.Action, "switch-to-workspace-") ||
		event.Action == "next-workspace" || event.Action == "previous-workspace" || event.Action == "former-workspace" {
		event.Action = "workspace-switching"
		event.Title = "Workspace switching"
		if event.Trigger != "shortcut" {
			event.Shortcut = "SUPER + TAB"
			event.Shortcuts = []string{"SUPER + TAB"}
		}
		return event
	}
	if isPositionalPanelDescription(event.Title) || strings.HasPrefix(event.Action, "bar-panel-") {
		event.Action = "bar-panels"
		event.Title = "Bar panels"
	}
	return event
}

func isWorkspaceSwitchDescription(description string) bool {
	normalized := strings.ToLower(strings.TrimSpace(description))
	if normalized == "workspace switching" || normalized == "next workspace" ||
		normalized == "previous workspace" || normalized == "former workspace" {
		return true
	}
	return strings.HasPrefix(normalized, "switch to workspace ")
}

func isPositionalPanelDescription(description string) bool {
	normalized := strings.ToLower(strings.TrimSpace(description))
	if normalized == "bar panels" {
		return true
	}
	value := strings.TrimPrefix(normalized, "bar panel ")
	if value == normalized || value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isGroupedAction(action string) bool {
	return action == "workspace-switching" || action == "bar-panels"
}

func resolveCurrentShortcuts(title, fallback string) []string {
	output, err := exec.Command("omarchy-menu-keybindings", "--print").Output()
	if err != nil {
		return mergeShortcuts(nil, fallback)
	}
	return shortcutsFromKeybindings(output, title, fallback)
}

func shortcutsFromKeybindings(data []byte, title, fallback string) []string {
	var shortcuts []string
	for _, line := range strings.Split(string(data), "\n") {
		key, action, found := strings.Cut(line, "→")
		if !found || !strings.EqualFold(strings.TrimSpace(action), strings.TrimSpace(title)) {
			continue
		}
		shortcuts = mergeShortcuts(shortcuts, strings.TrimSpace(key))
	}
	if len(shortcuts) == 0 {
		shortcuts = mergeShortcuts(shortcuts, fallback)
	}
	return shortcuts
}

func eventShortcuts(event Event) []string {
	return mergeShortcuts(event.Shortcuts, event.Shortcut)
}

func mergeShortcuts(existing []string, additions ...string) []string {
	result := append([]string(nil), existing...)
	seen := make(map[string]bool, len(result))
	for _, shortcut := range result {
		seen[canonicalShortcut(shortcut)] = true
	}
	for _, shortcut := range additions {
		shortcut = strings.TrimSpace(shortcut)
		key := canonicalShortcut(shortcut)
		if shortcut != "" && !seen[key] {
			result = append(result, shortcut)
			seen[key] = true
		}
	}
	return result
}

func canonicalShortcut(shortcut string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ToUpper(shortcut), "+", " ")), " ")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "omarchy-sensei:", message)
	os.Exit(1)
}
