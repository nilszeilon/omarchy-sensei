# Architecture

Sensei stores an append-only local stream of semantic actions. Each event contains an action identifier, trigger class, title, known shortcut, and timestamp. It never stores typed text.

The snapshot folds events in chronological order into open tasks:

- A recognized `mouse` or `menu` event opens the task for that action.
- Further slow uses increment the same task instead of creating duplicates.
- A matching `shortcut` event closes the task.
- A later slow event reopens it.

The Quickshell panel renders only these open tasks and refreshes every two seconds. Tasks are ordered by descending slow-use count, with the oldest task first when counts tie. The bounded list follows its keyboard cursor and supports arrows plus `h/j/k/l`. There are no scores, charts, skill trees, trials, notifications, or manually completed checkboxes. The required behavior itself is the completion action.

The integration observes every described Hyprland shortcut through one pass-through dispatcher. It dynamically merges Omarchy's default and user menu definitions, resolves the same bindings as Super+K, and instruments every menu leaf with a unique high-confidence command or semantic match. Ambiguous actions remain untouched and are visible through catalog diagnostics. There is no hardcoded action registry.

Generated menu wrappers preserve the complete merged item metadata and execute the original command byte-for-byte. A user-level path unit refreshes the catalog after menu or personal binding edits, and an Omarchy post-update hook refreshes it after upgrades.

Adding broader mouse observations still requires a reliable semantic mapping to a keyboard action; raw click or key logging is intentionally out of scope.

Before a menu or mouse event is stored, the collector reads the same resolved keybinding list used by Omarchy's Super+K panel. Every active binding with the matching description is stored and displayed, including Lua, keycode, and user-remapped alternatives.
