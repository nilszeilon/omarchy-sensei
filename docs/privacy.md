# Local data

Sensei persists only what the product displays or needs to finish its coaching loop:

- The lifetime number of shortcut uses
- Currently open tasks
- For each task: its semantic action name, current shortcut hints, offender count, and opening time

`state.json` can also contain a sub-second routing guard with semantic action IDs and timestamps. It expires after one second and is removed on the next coaching or diagnostic state update. Shortcut chords are not passed to or stored by the shortcut observer.

Sensei does not store an action history, individual shortcut uses, typed characters, raw keycodes, pointer coordinates, commands, window addresses, app-launch records, content, clipboard data, screenshots, or credentials. Semantic events are accepted from known Omarchy menu routes, Apps-provider launches, bar controls, and pointer-originated compositor focus transitions, then matched locally to one existing keyboard action. The short-lived focus helper receives only the previous and current window addresses and does not persist them. Unmatched or ambiguous events are discarded.

All state remains local in `$XDG_STATE_HOME/omarchy-sensei/state.json` or `~/.local/state/omarchy-sensei/state.json`, with mode `0600`. There is no network or telemetry client.

`omarchy-sensei setup` enables coaching. `pause` and `resume` control updates, `status` reports only the aggregate total and open-task count, and `clear` permanently deletes progress and tasks. A pre-2.0 `events.jsonl` file is compacted once during upgrade and deleted only after the replacement state is safely written.
