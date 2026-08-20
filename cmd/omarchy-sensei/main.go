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

type Day struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
	Today bool   `json:"today,omitempty"`
}

type Hint struct {
	Action   string `json:"action"`
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	SlowUses int    `json:"slowUses"`
	FastUses int    `json:"fastUses"`
	Avoided  int    `json:"avoided"`
}

type Skill struct {
	Action   string `json:"action"`
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	State    string `json:"state"`
	Uses     int    `json:"uses"`
}

type Branch struct {
	Name     string  `json:"name"`
	Glyph    string  `json:"glyph"`
	Skills   []Skill `json:"skills"`
	Mastered int     `json:"mastered"`
}

type Trial struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Defeated    bool   `json:"defeated"`
}

type Snapshot struct {
	MouseDays    []Day    `json:"mouseDays"`
	ShortcutDays []Day    `json:"shortcutDays"`
	MaxCount     int      `json:"maxCount"`
	Hint         *Hint    `json:"hint"`
	Branches     []Branch `json:"branches"`
	Trial        *Trial   `json:"trial"`
	XP           int      `json:"xp"`
	Level        int      `json:"level"`
	Paused       bool     `json:"paused"`
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
	if err := recordEvent(paths, Event{
		OccurredAt: time.Now(),
		Action:     strings.TrimSpace(*action),
		Title:      strings.TrimSpace(*title),
		Trigger:    "menu",
		Shortcut:   strings.TrimSpace(*shortcut),
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
	localNow := now.In(time.Local)
	today := dayStart(localNow)
	start := today.AddDate(0, 0, -6)
	shortcutCounts := map[string]int{}
	mouseCounts := map[string]int{}
	type score struct {
		hint Hint
	}
	scores := map[string]*score{}

	for _, event := range events {
		date := event.OccurredAt.In(time.Local).Format("2006-01-02")
		if !event.OccurredAt.Before(start) {
			if event.Trigger == "shortcut" {
				shortcutCounts[date]++
			} else if event.Trigger == "menu" || event.Trigger == "mouse" {
				mouseCounts[date]++
			}
		}
		entry := scores[event.Action]
		if entry == nil {
			entry = &score{hint: Hint{Action: event.Action, Title: event.Title, Shortcut: event.Shortcut}}
			scores[event.Action] = entry
		}
		if event.Shortcut != "" {
			entry.hint.Shortcut = event.Shortcut
		}
		if event.Trigger == "shortcut" {
			entry.hint.FastUses++
		} else if event.Trigger == "menu" || event.Trigger == "mouse" {
			entry.hint.SlowUses++
		}
	}

	result := Snapshot{
		MouseDays:    make([]Day, 0, 7),
		ShortcutDays: make([]Day, 0, 7),
	}
	for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		mouseCount := mouseCounts[date]
		shortcutCount := shortcutCounts[date]
		result.MouseDays = append(result.MouseDays, Day{Date: date, Count: mouseCount, Today: day.Equal(today)})
		result.ShortcutDays = append(result.ShortcutDays, Day{Date: date, Count: shortcutCount, Today: day.Equal(today)})
		if mouseCount > result.MaxCount {
			result.MaxCount = mouseCount
		}
		if shortcutCount > result.MaxCount {
			result.MaxCount = shortcutCount
		}
	}

	var candidates []Hint
	for _, entry := range scores {
		hint := entry.hint
		if hint.Shortcut == "" || hint.SlowUses < 1 || hint.FastUses > 0 {
			continue
		}
		hint.Avoided = hint.SlowUses - hint.FastUses
		candidates = append(candidates, hint)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Avoided == candidates[j].Avoided {
			return candidates[i].SlowUses > candidates[j].SlowUses
		}
		return candidates[i].Avoided > candidates[j].Avoided
	})
	if len(candidates) > 0 {
		result.Hint = &candidates[0]
	}

	branchSkills := map[string][]Skill{}
	for _, entry := range scores {
		hint := entry.hint
		if hint.Title == "" || hint.Shortcut == "" || (hint.FastUses == 0 && hint.SlowUses == 0) {
			continue
		}
		state := "discovered"
		if hint.FastUses >= 5 {
			state = "mastered"
		} else if hint.FastUses > 0 {
			state = "learned"
		}
		result.XP += hint.FastUses
		name, _ := skillBranch(hint.Action)
		branchSkills[name] = append(branchSkills[name], Skill{
			Action: hint.Action, Title: hint.Title, Shortcut: hint.Shortcut,
			State: state, Uses: hint.FastUses,
		})
	}
	result.Level = 1 + result.XP/25
	branchOrder := []string{"Window Arts", "Workspace Way", "Swift Launching", "Capture Craft", "System Lore"}
	for _, name := range branchOrder {
		skills := branchSkills[name]
		if len(skills) == 0 {
			continue
		}
		sort.Slice(skills, func(i, j int) bool {
			if skills[i].Uses == skills[j].Uses {
				return skills[i].Title < skills[j].Title
			}
			return skills[i].Uses > skills[j].Uses
		})
		if len(skills) > 4 {
			skills = skills[:4]
		}
		_, glyph := skillBranch(skills[0].Action)
		branch := Branch{Name: name, Glyph: glyph, Skills: skills}
		for _, skill := range skills {
			if skill.State == "mastered" {
				branch.Mastered++
			}
		}
		result.Branches = append(result.Branches, branch)
		if len(result.Branches) == 3 {
			break
		}
	}
	if len(result.Branches) > 0 {
		branch := result.Branches[0]
		goal := len(branch.Skills)
		if goal > 3 {
			goal = 3
		}
		defeated := branch.Mastered >= goal
		description := fmt.Sprintf("Master %d %s skills", goal, branch.Name)
		if defeated {
			description = fmt.Sprintf("%d skills mastered", branch.Mastered)
		}
		result.Trial = &Trial{Title: branchTrial(branch.Name), Description: description, Defeated: defeated}
	}
	return result
}

func skillBranch(action string) (string, string) {
	switch {
	case strings.Contains(action, "workspace"):
		return "Workspace Way", "◇"
	case strings.Contains(action, "window") || strings.Contains(action, "focus") || strings.Contains(action, "swap"):
		return "Window Arts", "◆"
	case strings.Contains(action, "screenshot") || strings.Contains(action, "capture") || strings.Contains(action, "record") || strings.Contains(action, "color"):
		return "Capture Craft", "◈"
	case strings.Contains(action, "terminal") || strings.Contains(action, "browser") || strings.Contains(action, "launch"):
		return "Swift Launching", "▲"
	default:
		return "System Lore", "✦"
	}
}

func branchTrial(branch string) string {
	switch branch {
	case "Window Arts":
		return "The Window Tamer"
	case "Workspace Way":
		return "The Workspace Wanderer"
	case "Swift Launching":
		return "The Zero-Mouse Start"
	case "Capture Craft":
		return "The Capture Master"
	default:
		return "The Omarchy Initiate"
	}
}

func dayStart(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "omarchy-sensei:", message)
	os.Exit(1)
}
