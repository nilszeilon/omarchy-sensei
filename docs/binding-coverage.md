# Binding coverage

## What the numbers mean

The release fixture contains 229 raw Super+K records, 218 unique described bindings, and 191 semantic concepts after duplicates and equivalent workspace/panel habits are collapsed.

The old catalog reported 24 “coached actions.” That number measured only static menu-leaf matches. It did not mean that the other 194 bindings were unknown: Sensei already wrapped their keyboard callbacks, but had no safe slow-action event that could open the corresponding task.

The coverage report now distinguishes those concerns:

- `observed`: a safe menu, Apps, bar, or compositor event can open the task;
- `matchable-unobserved`: the shortcut identity is understood, but the current desktop exposes no safe slow-action source;
- `missing-metadata`: Super+K exposes no dispatcher/argument for the binding;
- `keyboard-only`: the binding is a synthetic keyboard action without a semantic desktop source.

On the audited machine, 85 binding rows representing 62 concepts are observed, 126 are understood but unobserved, five lack dispatcher metadata, and two are keyboard-only. Counts are machine-specific because apps, plugins, bar layout, bindings, and remaps are all live inputs.

Run:

```sh
omarchy-sensei catalog --coverage
omarchy-sensei catalog --coverage --json
```

Every JSON row contains its status, event source, evidence, semantic action, title, dispatcher, argument, and all current shortcut alternatives. The report also records whether bindings came directly from the compositor or from the newest healthy Omarchy cache.

## Identity graph

Sensei builds its mapping from current desktop data instead of maintaining an action registry:

1. Parse the same resolved binding records displayed by Super+K.
2. Collapse exact dispatcher/argument duplicates and the deliberate workspace/panel families.
3. Parse menu route IDs and aliases from the merged default and user menu.
4. Resolve bar modules through command targets, manifest display names, or live panel position.
5. Resolve Apps launches through desktop ID, URL, executable, generic name, or configured default role.
6. Resolve pointer-driven focus through old/new compositor geometry.
7. Connect an event only when one semantic concept wins. Ambiguous matches are ignored.

Descriptions remain presentation data and a final unique fallback. Operation classes prevent an installer, updater, remover, private browser, current-directory launcher, or compose action from being confused with an ordinary app or menu action that happens to have a similar label.

## Safe event sources

Menu leaves retain their original metadata and command, wrapped only to record the slow use. Parent menu navigation is observed through the live menu's `activeMenu` identity, so aliases and future routes resolve without replacing parent rows or breaking navigation.

Apps-provider launches are observed through AppLibrary's launch serial. No process polling or app-specific list is required. Missing and ambiguous desktop entries remain dormant.

Bar clicks use Omarchy Shell's clickable-widget registry. Only left clicks on the bar surface qualify; panel contents, Sensei itself, and widgets without a unique keyboard equivalent are ignored.

Window focus uses mouse-only Hyprland press and release bindings with `non_consuming = true`. A task is emitted only when a real active-window transition occurs while that exact press is armed and the matching release follows. Ordinary clicks launch no process, keyboard focus changes cannot arm the observer, and the existing Super+mouse drag binding remains separate.

## Remaining work

The 126 matchable-but-unobserved rows are not a hardcoding backlog. They need a general semantic interaction source in Omarchy Shell—for example, shared controls publishing the command or module identity they executed. That would cover audio, brightness, media, notifications, and future panels without adding Sensei-specific cases.

Sensei intentionally does not infer mouse origin from state changes alone. Fullscreen, floating, window movement, media state, and similar transitions can also be caused by applications or keyboards; coaching them without origin evidence would produce false tasks. The success criterion is complete accounting with safe evidence, not a superficially larger matched number.
