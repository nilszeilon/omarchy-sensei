# Local data

Sensei persists only what the product displays or needs to finish its coaching loop:

- The lifetime number of shortcut uses
- Currently open tasks
- For each task: its semantic action name, current shortcut hints, offender count, and opening time

`state.json` can also contain a sub-second routing guard with semantic action IDs and timestamps. It expires after one second and is removed on the next panel refresh or coaching update. Shortcut chords are not passed to or stored by the shortcut observer.

Sensei does not store an action history, individual shortcut uses, typed characters, raw keycodes, pointer coordinates, commands, content, clipboard data, screenshots, or credentials. Clicks are accepted only from known Omarchy bar controls and matched locally to an existing keyboard action. Unmatched clicks are discarded.

All state remains local in `$XDG_STATE_HOME/omarchy-sensei/state.json` or `~/.local/state/omarchy-sensei/state.json`, with mode `0600`. There is no network or telemetry client.

`omarchy-sensei setup` enables coaching. `pause` and `resume` control updates, `status` reports only the aggregate total and open-task count, and `clear` permanently deletes progress and tasks. A pre-2.0 `events.jsonl` file is compacted once during upgrade and deleted only after the replacement state is safely written.
