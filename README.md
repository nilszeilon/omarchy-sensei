# Omarchy Sensei

Build keyboard-first Omarchy habits, one shortcut at a time.

Omarchy Sensei is a privacy-first coaching plugin for [Omarchy Quattro](https://github.com/basecamp/omarchy). It aims to notice repeated, slow interaction patterns, teach the matching keyboard shortcut, and celebrate the habits that stick.

> [!IMPORTANT]
> This repository is an early functional shell-plugin scaffold. The panel works, but observation, recommendations, and progress tracking are not implemented yet.

## Principles

- Collect semantic actions, not typed characters.
- Keep history local by default.
- Explain every observation and recommendation.
- Let the user pause, inspect, export, or delete all data.
- Never inspect clipboard contents, terminal text, passwords, or document contents.

## Planned experience

1. Observe an action such as opening the browser from a menu.
2. Match it to an existing Omarchy shortcut such as `SUPER + B`.
3. Wait for repetition before offering a quiet, rate-limited hint.
4. Detect adoption and record the shortcut as learned.
5. Show streaks, time saved, and a GitHub-style keyboard-first activity graph.
6. For actions without shortcuts, create a reviewable, privacy-safe context bundle that an agent can use to propose one.

## Install

```sh
omarchy plugin add https://github.com/nilszeilon/omarchy-sensei.git --enable
```

Move it to the right section of the bar if needed:

```sh
omarchy bar move io.github.nilszeilon.omarchy-sensei --section right
```

## Development

```sh
omarchy plugin validate .
qmllint -I "$OMARCHY_PATH/shell" BarWidget.qml Panel.qml
```

Saved changes in an installed user plugin reload automatically. To force discovery:

```sh
omarchy-shell shell rescanPlugins
```

## Remove

```sh
omarchy plugin remove io.github.nilszeilon.omarchy-sensei
```

See [the architecture](docs/architecture.md) and [privacy contract](docs/privacy.md) for the proposed MVP boundaries.

## License

MIT
