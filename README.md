# Omarchy Sensei

Turn mouse habits into keyboard practice.

Sensei has one loop:

1. Use a recognized Omarchy action through the mouse or menu.
2. Sensei opens a task containing the matching shortcut.
3. Perform that action once with the shortcut.
4. The task closes.

Using the mouse for that action again reopens the task. Sensei never records ordinary typing, clipboard contents, terminal text, passwords, or document contents.

## Install

```sh
omarchy plugin add https://github.com/nilszeilon/omarchy-sensei.git --enable
cd ~/.config/omarchy/plugins/io.github.nilszeilon.omarchy-sensei
./install.sh
```

The installer builds the local collector and connects recognized Omarchy shortcuts and matching menu actions. It never edits `/usr/share/omarchy`.

## Controls

Use arrows or `h/j/k/l` to move through tasks, `Tab`/`Shift+Tab` to switch bar panels, and `Esc` to close.

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
