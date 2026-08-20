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
}

type Task struct {
	Action   string    `json:"action"`
	Title    string    `json:"title"`
	Shortcut string    `json:"shortcut"`
	OpenedAt time.Time `json:"openedAt"`
	SlowUses int       `json:"slowUses"`
}

type Snapshot struct {
	Tasks  []Task `json:"tasks"`
	Paused bool   `json:"paused"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: omarchy-sensei <setup|uninstall|record|run|snapshot|pause|resume|clear|status>")
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
	case "uninstall":
		if err := uninstallIntegration(paths); err != nil {
			fatal(err.Error())
		}
		fmt.Println("Omarchy Sensei observation was removed. Your activity data was kept.")
	case "record":
		record(paths, os.Args[2:])
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
	if event.Trigger != "shortcut" {
		event.Shortcut = resolveCurrentShortcut(event.Title, event.Shortcut)
	}
	if event.Action == "" || event.Title == "" || !validTrigger(event.Trigger) {
		fatal("record requires --action, --title, and a valid --trigger")
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
	currentShortcut := resolveCurrentShortcut(strings.TrimSpace(*title), strings.TrimSpace(*shortcut))
	if err := recordEvent(paths, Event{
		OccurredAt: time.Now(),
		Action:     strings.TrimSpace(*action),
		Title:      strings.TrimSpace(*title),
		Trigger:    "menu",
		Shortcut:   currentShortcut,
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
	Paused         string
	HyprlandConfig string
	SenseiLua      string
	MenuExtension  string
	LocalBinary    string
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
		Paused:         filepath.Join(stateDir, "paused"),
		HyprlandConfig: filepath.Join(home, ".config", "hypr", "hyprland.lua"),
		SenseiLua:      filepath.Join(home, ".config", "hypr", "sensei.lua"),
		MenuExtension:  filepath.Join(home, ".config", "omarchy", "extensions", "omarchy-menu.jsonc"),
		LocalBinary:    filepath.Join(home, ".local", "bin", "omarchy-sensei"),
	}, nil
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
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})

	open := map[string]*Task{}
	knownShortcuts := map[string]string{}
	for _, event := range ordered {
		if event.Trigger == "shortcut" && event.Shortcut != "" {
			knownShortcuts[event.Action] = event.Shortcut
		}
	}
	for _, event := range ordered {
		if event.Action == "" {
			continue
		}
		switch event.Trigger {
		case "menu", "mouse":
			if event.Shortcut == "" {
				continue
			}
			task := open[event.Action]
			if task == nil {
				task = &Task{Action: event.Action, OpenedAt: event.OccurredAt}
				open[event.Action] = task
			}
			task.Title = event.Title
			task.Shortcut = event.Shortcut
			if current := knownShortcuts[event.Action]; current != "" {
				task.Shortcut = current
			}
			task.SlowUses++
		case "shortcut":
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

type hyprBinding struct {
	Modmask     int    `json:"modmask"`
	Key         string `json:"key"`
	Keycode     int    `json:"keycode"`
	Description string `json:"description"`
}

func resolveCurrentShortcut(title, fallback string) string {
	output, err := exec.Command("hyprctl", "binds", "-j").Output()
	if err != nil {
		return fallback
	}
	return shortcutFromBindings(output, title, fallback)
}

func shortcutFromBindings(data []byte, title, fallback string) string {
	var bindings []hyprBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return fallback
	}
	shortcut := fallback
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.Description), strings.TrimSpace(title)) {
			continue
		}
		key := strings.TrimSpace(binding.Key)
		if key == "" && binding.Keycode > 0 {
			key = fmt.Sprintf("code:%d", binding.Keycode)
		}
		if key == "" {
			continue
		}
		var parts []string
		if binding.Modmask&64 != 0 {
			parts = append(parts, "SUPER")
		}
		if binding.Modmask&1 != 0 {
			parts = append(parts, "SHIFT")
		}
		if binding.Modmask&4 != 0 {
			parts = append(parts, "CTRL")
		}
		if binding.Modmask&8 != 0 {
			parts = append(parts, "ALT")
		}
		parts = append(parts, strings.ToUpper(key))
		shortcut = strings.Join(parts, " + ")
	}
	return shortcut
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "omarchy-sensei:", message)
	os.Exit(1)
}
