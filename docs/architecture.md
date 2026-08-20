# Architecture

Omarchy Sensei is planned as two cooperating pieces:

1. A Quickshell bar widget and panel for hints, progress, controls, and the activity graph.
2. A small local companion service for normalizing semantic events, matching actions to shortcuts, and persisting aggregates.

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

SQLite is the likely local store, following the useful aggregation pattern in `nilszeilon/devstats`. Raw semantic events should have a short retention period; daily action counts and learned-shortcut totals can be retained for the lifetime graph.
