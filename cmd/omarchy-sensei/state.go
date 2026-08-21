package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"
	"time"
)

const (
	stateVersion        = 1
	duplicateWindow     = 100 * time.Millisecond
	menuConsequenceTime = time.Second
)

// SenseiState is intentionally small. It keeps progress and unfinished coaching
// tasks, not a history of actions or key presses.
type SenseiState struct {
	Version          int              `json:"version"`
	TotalShortcuts   int              `json:"totalShortcuts"`
	Tasks            []Task           `json:"tasks"`
	LastShortcutAt   int64            `json:"lastShortcutAt,omitempty"`
	RecentShortcutAt map[string]int64 `json:"recentShortcutAt,omitempty"`
}

func initializeState(paths Paths) error {
	_, err := withLockedState(paths, time.Now(), true, nil)
	return err
}

func updateCoachingState(paths Paths, observation Observation) error {
	if isPaused(paths) {
		return nil
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = time.Now()
	}
	_, err := withLockedState(paths, observation.ObservedAt, true, func(state *SenseiState) bool {
		applyObservation(state, observation)
		return true
	})
	return err
}

func readState(paths Paths, now time.Time) (SenseiState, error) {
	return withLockedState(paths, now, false, nil)
}

func withLockedState(paths Paths, now time.Time, ensureFile bool, update func(*SenseiState) bool) (SenseiState, error) {
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return SenseiState{}, err
	}
	lock, err := os.OpenFile(paths.StateLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return SenseiState{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return SenseiState{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck

	state, exists, err := loadStateFile(paths.State)
	if err != nil {
		return SenseiState{}, err
	}
	legacyExists := fileExists(paths.LegacyEvents)
	migrated := false
	if !exists && legacyExists {
		state, err = migrateLegacyFile(paths.LegacyEvents)
		if err != nil {
			return SenseiState{}, fmt.Errorf("migrate old Sensei data: %w", err)
		}
		migrated = true
	}

	changed := false
	if state.Tasks == nil {
		state.Tasks = []Task{}
		changed = exists
	}
	if pruneTransientState(&state, now) {
		changed = true
	}
	if update != nil && update(&state) {
		changed = true
	}
	if ensureFile && !exists {
		changed = true
	}
	if migrated || changed {
		if err := saveStateFile(paths.State, state); err != nil {
			return SenseiState{}, err
		}
	}
	if legacyExists && (exists || migrated) {
		if err := os.Remove(paths.LegacyEvents); err != nil && !errors.Is(err, os.ErrNotExist) {
			return SenseiState{}, fmt.Errorf("remove migrated event history: %w", err)
		}
	}
	return state, nil
}

func loadStateFile(path string) (SenseiState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SenseiState{Version: stateVersion, Tasks: []Task{}}, false, nil
	}
	if err != nil {
		return SenseiState{}, false, err
	}
	var state SenseiState
	if err := json.Unmarshal(data, &state); err != nil {
		return SenseiState{}, false, fmt.Errorf("read state: %w", err)
	}
	if state.Version != stateVersion {
		return SenseiState{}, false, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.TotalShortcuts < 0 {
		return SenseiState{}, false, errors.New("shortcut total cannot be negative")
	}
	return state, true, nil
}

func saveStateFile(path string, state SenseiState) error {
	state.Version = stateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'), 0o600)
}

func applyObservation(state *SenseiState, observation Observation) {
	observation = normalizeObservation(observation)
	if observation.Action == "" {
		return
	}
	when := observation.ObservedAt
	if when.IsZero() {
		when = time.Now()
	}
	nowMillis := when.UnixMilli()
	pruneTransientState(state, when)

	if observation.Trigger == "shortcut" {
		elapsed := time.Duration(nowMillis-state.LastShortcutAt) * time.Millisecond
		if state.LastShortcutAt == 0 || elapsed < 0 || elapsed >= duplicateWindow {
			state.TotalShortcuts++
		}
		state.LastShortcutAt = nowMillis
		if state.RecentShortcutAt == nil {
			state.RecentShortcutAt = make(map[string]int64)
		}
		state.RecentShortcutAt[observation.Action] = nowMillis
		state.Tasks = removeTask(state.Tasks, observation.Action)
		return
	}

	if observation.Trigger == "menu" {
		if recent := state.RecentShortcutAt[observation.Action]; recent != 0 {
			elapsed := time.Duration(nowMillis-recent) * time.Millisecond
			if elapsed >= 0 && elapsed < menuConsequenceTime {
				return
			}
		}
	}
	shortcuts := observationShortcuts(observation)
	if len(shortcuts) == 0 {
		return
	}
	for index := range state.Tasks {
		if state.Tasks[index].Action != observation.Action {
			continue
		}
		state.Tasks[index].Title = observation.Title
		state.Tasks[index].Shortcuts = mergeShortcuts(shortcuts)
		state.Tasks[index].SlowUses++
		return
	}
	state.Tasks = append(state.Tasks, Task{
		Action:    observation.Action,
		Title:     observation.Title,
		Shortcuts: shortcuts,
		OpenedAt:  when,
		SlowUses:  1,
	})
}

func removeTask(tasks []Task, action string) []Task {
	for index := range tasks {
		if tasks[index].Action == action {
			return append(tasks[:index], tasks[index+1:]...)
		}
	}
	return tasks
}

func pruneTransientState(state *SenseiState, now time.Time) bool {
	nowMillis := now.UnixMilli()
	changed := false
	if state.LastShortcutAt != 0 {
		elapsed := time.Duration(nowMillis-state.LastShortcutAt) * time.Millisecond
		if elapsed < 0 || elapsed >= duplicateWindow {
			state.LastShortcutAt = 0
			changed = true
		}
	}
	for action, recent := range state.RecentShortcutAt {
		elapsed := time.Duration(nowMillis-recent) * time.Millisecond
		if elapsed < 0 || elapsed >= menuConsequenceTime {
			delete(state.RecentShortcutAt, action)
			changed = true
		}
	}
	if len(state.RecentShortcutAt) == 0 && state.RecentShortcutAt != nil {
		state.RecentShortcutAt = nil
		changed = true
	}
	return changed
}

func snapshotFromState(state SenseiState, paused bool) Snapshot {
	tasks := make([]Task, len(state.Tasks))
	copy(tasks, state.Tasks)
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].SlowUses != tasks[j].SlowUses {
			return tasks[i].SlowUses > tasks[j].SlowUses
		}
		return tasks[i].OpenedAt.Before(tasks[j].OpenedAt)
	})
	return Snapshot{Tasks: tasks, Level: levelProgress(state.TotalShortcuts), Paused: paused}
}

func clearState(paths Paths) error {
	if err := os.MkdirAll(paths.StateDir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(paths.StateLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	for _, path := range []string{paths.State, paths.LegacyEvents} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// legacyEvent exists only to compact pre-2.0 installations once. The source
// file is deleted immediately after the aggregate state is safely written.
type legacyEvent struct {
	OccurredAt time.Time `json:"occurredAt"`
	Action     string    `json:"action"`
	Title      string    `json:"title"`
	Trigger    string    `json:"trigger"`
	Shortcut   string    `json:"shortcut,omitempty"`
	Shortcuts  []string  `json:"shortcuts,omitempty"`
}

func migrateLegacyFile(path string) (SenseiState, error) {
	file, err := os.Open(path)
	if err != nil {
		return SenseiState{}, err
	}
	defer file.Close()
	var events []legacyEvent
	decoder := json.NewDecoder(file)
	for {
		var event legacyEvent
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return SenseiState{}, err
		}
		events = append(events, event)
	}
	return compactLegacyEvents(events), nil
}

func compactLegacyEvents(events []legacyEvent) SenseiState {
	sort.SliceStable(events, func(i, j int) bool { return events[i].OccurredAt.Before(events[j].OccurredAt) })
	observations := make([]Observation, len(events))
	knownShortcuts := map[string][]string{}
	for index, event := range events {
		observation := normalizeObservation(Observation{
			ObservedAt: event.OccurredAt,
			Action:     event.Action, Title: event.Title, Trigger: event.Trigger,
			Shortcut: event.Shortcut, Shortcuts: event.Shortcuts,
		})
		observations[index] = observation
		if observation.Trigger == "shortcut" && observation.Shortcut != "" && !isGroupedAction(observation.Action) {
			knownShortcuts[observation.Action] = mergeShortcuts(knownShortcuts[observation.Action], observation.Shortcut)
		}
	}

	state := SenseiState{Version: stateVersion, Tasks: []Task{}}
	open := map[string]*Task{}
	recent := map[string]time.Time{}
	lastChord := map[string]time.Time{}
	for _, observation := range observations {
		if observation.Action == "" {
			continue
		}
		if observation.Trigger == "shortcut" {
			key := canonicalShortcut(observation.Shortcut)
			if key == "" {
				key = observation.Action
			}
			elapsed := observation.ObservedAt.Sub(lastChord[key])
			if lastChord[key].IsZero() || elapsed < 0 || elapsed >= duplicateWindow {
				state.TotalShortcuts++
			}
			lastChord[key] = observation.ObservedAt
			recent[observation.Action] = observation.ObservedAt
			delete(open, observation.Action)
			continue
		}
		if observation.Trigger != "menu" && observation.Trigger != "mouse" {
			continue
		}
		if observation.Trigger == "menu" {
			elapsed := observation.ObservedAt.Sub(recent[observation.Action])
			if !recent[observation.Action].IsZero() && elapsed >= 0 && elapsed < menuConsequenceTime {
				continue
			}
		}
		shortcuts := observationShortcuts(observation)
		if len(shortcuts) == 0 {
			continue
		}
		task := open[observation.Action]
		if task == nil {
			task = &Task{Action: observation.Action, OpenedAt: observation.ObservedAt}
			open[observation.Action] = task
		}
		task.Title = observation.Title
		task.Shortcuts = mergeShortcuts(shortcuts, knownShortcuts[observation.Action]...)
		task.SlowUses++
	}
	for _, task := range open {
		state.Tasks = append(state.Tasks, *task)
	}
	sort.Slice(state.Tasks, func(i, j int) bool {
		if state.Tasks[i].SlowUses != state.Tasks[j].SlowUses {
			return state.Tasks[i].SlowUses > state.Tasks[j].SlowUses
		}
		return state.Tasks[i].OpenedAt.Before(state.Tasks[j].OpenedAt)
	})
	return state
}
