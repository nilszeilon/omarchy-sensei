# Omarchy Sensei

Turn mouse habits into keyboard practice.

![Omarchy Sensei showing ranked keyboard practice tasks](preview.png)

Sensei has one loop:

1. Click a workspace, a keyboard-accessible bar panel, or an Omarchy menu action with a confidently matching shortcut.
2. Sensei opens a task containing every matching shortcut shown by Omarchy's Super+K panel.
3. Perform that action once with the shortcut.
4. The task closes.

Using the mouse for that action again reopens the task. The worst habit stays at the top until you perform it with the keyboard.

## Install

Requires Omarchy Quattro and either Go 1.24+ or `mise`. Installation uses only user-level files and services; it never requires `sudo`.

```sh
omarchy plugin add https://github.com/nilszeilon/omarchy-sensei.git --enable
cd ~/.config/omarchy/plugins/io.github.nilszeilon.omarchy-sensei
./install.sh
```

The installer builds the local helper, derives a coaching catalog from Omarchy's live menu and Super+K bindings, and connects every confident match. Sensei also uses Omarchy Shell's semantic bar-button registry: workspace clicks retain their workspace number, built-in panels use their direct shortcut, and custom panels use the live `Bar panel N` shortcut. There is no hardcoded panel or action list, and it never edits `/usr/share/omarchy`.

Setup installs the helper in `~/.local/bin`, adds clearly marked managed blocks to the user Hyprland and Omarchy menu configurations, and enables a user-level systemd path unit that refreshes shortcut hints after remaps or Omarchy updates. Existing configuration is backed up before a managed block changes.

Task hints resolve against the active Hyprland bindings, so user remaps take precedence over Omarchy defaults. Clicks without a keyboard equivalent are ignored.

Equivalent habits share one task: every numbered workspace click contributes to `Workspace switching`, and `Super+Tab` or any numbered workspace shortcut completes it. Panels without a named shortcut contribute to `Bar panels`; any positional bar-panel shortcut completes that shared task. Named panels such as Bluetooth stay independent and require their named shortcut.

The panel also shows a lifetime shortcut level. Level 1 takes 10 shortcut uses; each following level requires 50% more than the previous one, rounded up.

Sensei stores no action or keypress history. Its private local state contains only the lifetime shortcut total and currently open tasks. Task records contain the action name, current shortcut hints, offender count, and opening time. A sub-second duplicate guard is discarded automatically. Nothing is sent over the network.

The catalog refreshes automatically after menu or personal binding changes and after Omarchy updates. Inspect its decisions with:

```sh
omarchy-sensei catalog
omarchy-sensei catalog --unmatched
omarchy-sensei catalog --json
omarchy-sensei doctor
```

## Controls

Tasks are ordered by their slow-use count, so the worst offender is always first. Use arrows or `h/j/k/l` to move through the scrollable list, `Tab`/`Shift+Tab` to switch bar panels, and `Esc` to close.

```sh
omarchy-sensei status
omarchy-sensei pause
omarchy-sensei resume
omarchy-sensei clear
```

State stays in `$XDG_STATE_HOME/omarchy-sensei/state.json` or `~/.local/state/omarchy-sensei/state.json`, with mode `0600`. `clear` permanently deletes progress and open tasks. Upgrading from a pre-2.0 release compacts the old history once and then permanently deletes `events.jsonl`.

## All clear

![Omarchy Sensei after every practice task has been completed](docs/screenshots/all-clear.png)

## Remove

```sh
# Optional: permanently delete progress before removing the helper.
omarchy-sensei clear

omarchy-sensei uninstall
hyprctl reload
omarchy plugin remove io.github.nilszeilon.omarchy-sensei
```

Uninstall removes the helper, generated integration, managed configuration blocks, binding cache, refresh unit, and update hook. Omit `clear` if you want to keep your compact progress file for a later reinstall.

## License

MIT
