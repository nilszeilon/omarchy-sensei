# Architecture

Omarchy Sensei is planned as two cooperating pieces:

1. A Quickshell bar widget and panel containing the keyboard-first activity graph and one lesson.
2. A small local companion CLI for normalizing semantic events, matching actions to shortcuts, and persisting local history.

## Automatic observation

`omarchy-sensei setup` installs two user-owned integrations:

- A small Lua module loaded before Omarchy's default bindings wraps `o.bind`. Every described keyboard binding keeps its original behavior and gains a second semantic recording action. Mouse bindings, switches, and undescribed keys are ignored.
- Managed overrides in `~/.config/omarchy/extensions/omarchy-menu.jsonc` wrap menu actions that have known shortcut equivalents. The original action still runs after the local event is recorded.

Both integrations are delimited by managed markers, create timestamped backups, and can be removed without discarding unrelated user configuration. Packaged files under `/usr/share/omarchy` remain untouched.

## Event model

The minimum useful event describes intent without content:

```json
{
  "occurredAt": "2026-08-20T12:00:00Z",
  "action": "launch_browser",
  "trigger": "menu",
  "shortcut": "SUPER+B",
  "durationMs": 1850
}
```

The MVP should prefer explicit Omarchy and Hyprland event sources over global input capture. Mouse coordinates and key values are not part of the event model.

## Recommendation loop

- Recommend the highest-impact observed slow action as soon as it has a known shortcut.
- Rate-limit hints and provide snooze and disable controls.
- Count a shortcut as learned only after repeat use across multiple sessions.
- Estimate time saved from measured local baselines and label the number as an estimate.
- Require user review before sending any context to an agent.

## Storage

The first release uses a private JSON Lines event log so the complete data path stays inspectable. The graph counts only actions triggered by shortcuts. The lesson is the menu or mouse action with the largest gap between slow and shortcut uses; it appears after the first observed slow use, and five shortcut uses marks it learned. Observation can be paused, resumed, inspected, or cleared from the CLI.
