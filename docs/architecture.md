# Architecture

Omarchy Sensei is planned as two cooperating pieces:

1. A Quickshell bar widget and panel containing the keyboard-first activity graph and one lesson.
2. A small local companion CLI for normalizing semantic events, matching actions to shortcuts, and persisting local history.

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

- Require repeated observations before recommending anything.
- Rate-limit hints and provide snooze and disable controls.
- Count a shortcut as learned only after repeat use across multiple sessions.
- Estimate time saved from measured local baselines and label the number as an estimate.
- Require user review before sending any context to an agent.

## Storage

The first slice uses a private JSON Lines event log so the complete data path stays inspectable. It can move to SQLite when automatic sources and retention controls land. The graph counts only actions triggered by shortcuts. The lesson is the repeated menu or mouse action with the largest gap between slow and shortcut uses; five shortcut uses marks it learned.
