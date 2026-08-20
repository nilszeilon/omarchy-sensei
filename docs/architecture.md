# Architecture

Sensei stores an append-only local stream of semantic actions. Each event contains an action identifier, trigger class, title, known shortcut, and timestamp. It never stores typed text.

The snapshot folds events in chronological order into open tasks:

- A recognized `mouse` or `menu` event opens the task for that action.
- Further slow uses increment the same task instead of creating duplicates.
- A matching `shortcut` event closes the task.
- A later slow event reopens it.

The Quickshell panel renders only these open tasks and refreshes every two seconds. Tasks are ordered by descending slow-use count, with the oldest task first when counts tie. The bounded list follows its keyboard cursor and supports arrows plus `h/j/k/l`. There are no scores, charts, skill trees, trials, notifications, or manually completed checkboxes. The required behavior itself is the completion action.

The current integration observes semantic Omarchy shortcuts plus explicitly matched menu actions. Adding broader mouse observations requires a reliable semantic mapping to a keyboard action; raw click logging is intentionally out of scope.
