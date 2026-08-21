# Architecture

Sensei stores an append-only local stream of semantic actions. Each event contains an action identifier, trigger class, title, known shortcut, and timestamp. It never stores typed text.

The snapshot folds events in chronological order into open tasks:

- A recognized `mouse` or `menu` event opens the task for that action.
- Further slow uses increment the same task instead of creating duplicates.
- A matching `shortcut` event closes the task.
- A later slow event reopens it.

The fold also normalizes equivalent habits without rewriting history. Numbered, next, previous, and former workspace actions share the `workspace-switching` identity. Positional `Bar panel N` actions share `bar-panels`. Directly named panels retain their own identity, so a positional panel shortcut cannot accidentally complete a named Bluetooth, Audio, or Network task.

The Quickshell panel renders the lifetime level plus open tasks and refreshes every two seconds. Tasks are ordered by descending slow-use count, with the oldest task first when counts tie. The bounded list follows its keyboard cursor and supports arrows plus `h/j/k/l`. There are no charts, skill trees, trials, notifications, or manually completed checkboxes. The required behavior itself is the completion action.

The same snapshot counts lifetime shortcut invocations and derives a level locally. Level 1 requires 10 uses; each next per-level requirement is `ceil(previous × 1.5)`. Simultaneous records for the same chord are deduplicated within 100ms so one physical shortcut that dispatches multiple actions counts once. The level card is a read-only total and progress display; it does not alter coaching tasks.

The integration observes every described Hyprland shortcut through one pass-through dispatcher. It dynamically merges Omarchy's default and user menu definitions, resolves the same bindings as Super+K, and instruments every menu leaf with a unique high-confidence command or semantic match. Ambiguous actions remain untouched and are visible through catalog diagnostics. There is no hardcoded action registry.

The Sensei bar widget subscribes to Omarchy Shell's existing clickable-widget registry. It resolves the clicked button to its owning bar module while the semantic object is still available. Workspace widgets expose their workspace number directly. Panel widgets first match exact `omarchy-shell ... <module-id>` binding metadata, then fall back to the live right-side `Bar panel N` position. Only left clicks on the bar surface are eligible; clicks inside an open panel, clicks on Sensei itself, and widgets without a keyboard equivalent are ignored. One Sensei instance leads observation across multi-monitor copies to prevent duplicate events.

Generated menu wrappers preserve the complete merged item metadata and execute the original command byte-for-byte. A user-level path unit refreshes the catalog after menu or personal binding edits, and an Omarchy post-update hook refreshes it after upgrades.

Broader mouse observations still require a reliable semantic mapping to a keyboard action; raw click or key logging is intentionally out of scope.

The catalog refresh writes a small local cache of the same resolved bindings used by Omarchy's Super+K panel. Mouse clicks are matched against that cache without delaying their original UI action. Every active binding with the matching description is stored and displayed, including Lua, keycode, and user-remapped alternatives.
