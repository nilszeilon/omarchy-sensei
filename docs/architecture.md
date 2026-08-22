# Architecture

Sensei is a small state machine. A recognized mouse or menu action opens one task, further slow uses increment it, and the matching shortcut removes it. A later slow use opens it again. Each update is applied under a file lock and atomically writes `state.json`, so simultaneous shortcut and click processes cannot overwrite one another.

The durable state is only a lifetime shortcut total and unresolved tasks. It does not contain a stream of past actions. A bounded, sub-second guard prevents one physical shortcut from counting twice and prevents a shortcut-routed menu command from immediately reopening its task. The next coaching or diagnostic state update prunes expired guards.

Equivalent habits share one identity. Numbered, next, previous, and former workspace actions become `workspace-switching`. Positional `Bar panel N` actions become `bar-panels`. Directly named panels retain their own identity, so a positional panel shortcut cannot accidentally complete a named Bluetooth, Audio, or Network task.

The Quickshell panel renders the lifetime level plus open tasks and watches the compact state directory with native `FileView` notifications, so it does not poll or keep Python resident. Tasks are ordered by descending slow-use count, with the oldest task first when counts tie. The bounded list follows its keyboard cursor and supports arrows plus `h/j/k/l`. The required behavior itself is the completion action.

The level is derived directly from the aggregate shortcut total. Level 1 requires 10 uses; each next per-level requirement is `ceil(previous × 1.5)`. Shortcut callbacks arriving within 100ms are treated as one physical use without retaining the chord. The level card is read-only and does not alter coaching tasks.

The integration wraps every described Hyprland shortcut with one pass-through dispatcher. It sends only the semantic action ID and title after dispatching the original action. It dynamically merges Omarchy's default and user menu definitions, resolves the same bindings as Super+K, and instruments every menu leaf with a unique high-confidence command or semantic match. Ambiguous actions remain untouched and are visible through catalog diagnostics. There is no hardcoded action registry.

The Sensei bar widget subscribes to Omarchy Shell's existing clickable-widget registry. It resolves the clicked button to its owning bar module while the semantic object is still available. Workspace widgets expose their workspace number directly. Panel widgets first match exact `omarchy-shell ... <module-id>` binding metadata, then fall back to the live right-side `Bar panel N` position. Only left clicks on the bar surface are eligible; clicks inside an open panel, clicks on Sensei itself, and widgets without a keyboard equivalent are ignored. One Sensei instance handles coaching across multi-monitor copies to prevent duplicate tasks.

Generated menu wrappers preserve the complete merged item metadata and execute the original command byte-for-byte. A user-level path unit refreshes the catalog after menu or personal binding edits, and an Omarchy post-update hook refreshes it after upgrades.

Broader mouse coaching requires a reliable semantic mapping to a keyboard action. Raw mouse and keyboard input are outside Sensei's design.

The catalog refresh writes a small local cache of the same resolved bindings used by Omarchy's Super+K panel. Mouse clicks are matched against that cache without delaying their original UI action. Every active binding with the matching description is stored and displayed, including Lua, keycode, and user-remapped alternatives.

Pre-2.0 installations are migrated under the same lock. Sensei folds `events.jsonl` once into the aggregate total and open tasks, atomically writes the new state, and only then deletes the old file.
