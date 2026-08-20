package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
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
}

type Hint struct {
	Action   string `json:"action"`
	Title    string `json:"title"`
	Shortcut string `json:"shortcut"`
	SlowUses int    `json:"slowUses"`
	FastUses int    `json:"fastUses"`
	Avoided  int    `json:"avoided"`
}

type Snapshot struct {
	Days     []Day `json:"days"`
	MaxCount int   `json:"maxCount"`
	Hint     *Hint `json:"hint"`
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: omarchy-sensei <record|snapshot>")
	}

	path, err := eventPath()
	if err != nil {
		fatal(err.Error())
	}

	switch os.Args[1] {
	case "record":
		record(path, os.Args[2:])
	case "snapshot":
		snapshot(path)
	default:
		fatal("unknown command: " + os.Args[1])
	}
}

func record(path string, args []string) {
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

	if err := appendEvent(path, event); err != nil {
		fatal(err.Error())
	}
}

func snapshot(path string) {
	events, err := loadEvents(path)
	if err != nil {
		fatal(err.Error())
	}
	result := buildSnapshot(events, time.Now())
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

func eventPath() (string, error) {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "omarchy-sensei", "events.jsonl"), nil
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
	for decoder.More() {
		var event Event
		if err := decoder.Decode(&event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func buildSnapshot(events []Event, now time.Time) Snapshot {
	localNow := now.In(time.Local)
	today := dayStart(localNow)
	start := today.AddDate(0, 0, -370)
	counts := map[string]int{}
	type score struct {
		hint Hint
	}
	scores := map[string]*score{}

	for _, event := range events {
		date := event.OccurredAt.In(time.Local).Format("2006-01-02")
		if event.Trigger == "shortcut" && !event.OccurredAt.Before(start) {
			counts[date]++
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

	result := Snapshot{Days: make([]Day, 0, 371)}
	for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		count := counts[date]
		result.Days = append(result.Days, Day{Date: date, Count: count})
		if count > result.MaxCount {
			result.MaxCount = count
		}
	}

	var candidates []Hint
	for _, entry := range scores {
		hint := entry.hint
		if hint.Shortcut == "" || hint.SlowUses < 3 || hint.FastUses >= 5 {
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
	return result
}

func dayStart(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "omarchy-sensei:", message)
	os.Exit(1)
}
