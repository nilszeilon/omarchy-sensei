# Omarchy Sensei

Turn mouse habits into keyboard practice.

Sensei has one loop:

1. Click a workspace, a keyboard-accessible bar panel, or an Omarchy menu action with a confidently matching shortcut.
2. Sensei opens a task containing every matching shortcut shown by Omarchy's Super+K panel.
3. Perform that action once with the shortcut.
4. The task closes.

Using the mouse for that action again reopens the task. Sensei never records ordinary typing, clipboard contents, terminal text, passwords, or document contents.

## Install

```sh
omarchy plugin add https://github.com/nilszeilon/omarchy-sensei.git --enable
cd ~/.config/omarchy/plugins/io.github.nilszeilon.omarchy-sensei
./install.sh
```

The installer builds the local collector, derives a coaching catalog from Omarchy's live menu and Super+K bindings, and connects every confident match. Sensei also observes Omarchy Shell's semantic bar-button registry: workspace clicks retain their workspace number, built-in panels use their direct shortcut, and custom panels use the live `Bar panel N` shortcut. There is no hardcoded panel or action list, and it never edits `/usr/share/omarchy`.

Task hints resolve against the active Hyprland bindings, so user remaps take precedence over Omarchy defaults. Clicks without a keyboard equivalent are ignored.

Equivalent habits share one task: every numbered workspace click contributes to `Workspace switching`, and `Super+Tab` or any numbered workspace shortcut completes it. Panels without a named shortcut contribute to `Bar panels`; any positional bar-panel shortcut completes that shared task. Named panels such as Bluetooth stay independent and require their named shortcut.

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

Events stay in `$XDG_STATE_HOME/omarchy-sensei/` or `~/.local/state/omarchy-sensei/`.

## Remove

```sh
omarchy-sensei uninstall
hyprctl reload
omarchy plugin remove io.github.nilszeilon.omarchy-sensei
```

## License

MIT
