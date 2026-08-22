#!/usr/bin/env python3
"""Omarchy Sensei's dependency-free runtime and integration helper.

The script deliberately uses only Python's standard library.  It is copied to
~/.local/bin by ``setup`` for compatibility with existing generated bindings,
but the plugin can also run it directly from its git checkout.
"""

from __future__ import annotations

import argparse
import configparser
import dataclasses
import datetime as dt
import errno
import fcntl
import hashlib
import json
import os
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile
import time
import urllib.parse
from pathlib import Path
from typing import Any, Callable, Iterable


STATE_VERSION = 1
DUPLICATE_WINDOW_MS = 100
MENU_CONSEQUENCE_MS = 1000
HYPR_START = "-- BEGIN OMARCHY SENSEI (managed by omarchy-sensei setup)"
HYPR_END = "-- END OMARCHY SENSEI"
MENU_START = "// BEGIN OMARCHY SENSEI (managed by omarchy-sensei setup)"
MENU_END = "// END OMARCHY SENSEI"
WORDS_PATTERN = re.compile(r"[A-Za-z0-9]+")
TRAILING_COMMA = re.compile(r",(\s*[}\]])")
SEMANTIC_OPERATION_CLASSES = {
    "add": "install",
    "delete": "remove",
    "disable": "disable",
    "edit": "edit",
    "enable": "enable",
    "install": "install",
    "remove": "remove",
    "reset": "reset",
    "restore": "reset",
    "setup": "setup",
    "uninstall": "remove",
    "update": "update",
}


def now_utc() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


def parse_time(value: str | None) -> dt.datetime:
    if not value:
        return dt.datetime.fromtimestamp(0, dt.timezone.utc)
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return dt.datetime.fromtimestamp(0, dt.timezone.utc)
    return parsed if parsed.tzinfo else parsed.replace(tzinfo=dt.timezone.utc)


def iso_time(value: dt.datetime) -> str:
    return value.astimezone(dt.timezone.utc).isoformat().replace("+00:00", "Z")


@dataclasses.dataclass
class Paths:
    home: Path
    state_dir: Path
    state: Path
    state_lock: Path
    legacy_events: Path
    binding_cache: Path
    paused: Path
    hyprland_config: Path
    sensei_lua: Path
    menu_extension: Path
    default_menu: Path
    local_binary: Path
    refresh_service: Path
    refresh_path: Path
    post_update_hook: Path

    @classmethod
    def current(cls) -> "Paths":
        home = Path.home()
        state_root = Path(os.environ.get("XDG_STATE_HOME", home / ".local" / "state"))
        state_dir = state_root / "omarchy-sensei"
        return cls(
            home=home,
            state_dir=state_dir,
            state=state_dir / "state.json",
            state_lock=state_dir / "state.lock",
            legacy_events=state_dir / "events.jsonl",
            binding_cache=state_dir / "bindings.json",
            paused=state_dir / "paused",
            hyprland_config=home / ".config" / "hypr" / "hyprland.lua",
            sensei_lua=home / ".config" / "hypr" / "sensei.lua",
            menu_extension=home / ".config" / "omarchy" / "extensions" / "omarchy-menu.jsonc",
            default_menu=Path("/usr/share/omarchy/default/omarchy/omarchy-menu.jsonc"),
            local_binary=home / ".local" / "bin" / "omarchy-sensei",
            refresh_service=home / ".config" / "systemd" / "user" / "omarchy-sensei-refresh.service",
            refresh_path=home / ".config" / "systemd" / "user" / "omarchy-sensei-refresh.path",
            post_update_hook=home / ".config" / "omarchy" / "hooks" / "post-update.d" / "omarchy-sensei",
        )


@dataclasses.dataclass
class Observation:
    observed_at: dt.datetime
    action: str
    title: str
    trigger: str
    shortcut: str = ""
    shortcuts: list[str] = dataclasses.field(default_factory=list)


@dataclasses.dataclass
class Task:
    action: str
    title: str
    shortcuts: list[str]
    opened_at: dt.datetime
    slow_uses: int

    @classmethod
    def from_json(cls, value: dict[str, Any]) -> "Task":
        return cls(
            action=str(value.get("action", "")),
            title=str(value.get("title", "")),
            shortcuts=[str(item) for item in value.get("shortcuts", [])],
            opened_at=parse_time(value.get("openedAt")),
            slow_uses=int(value.get("slowUses", 0)),
        )

    def to_json(self) -> dict[str, Any]:
        return {
            "action": self.action,
            "title": self.title,
            "shortcuts": self.shortcuts,
            "openedAt": iso_time(self.opened_at),
            "slowUses": self.slow_uses,
        }


@dataclasses.dataclass
class SenseiState:
    version: int = STATE_VERSION
    total_shortcuts: int = 0
    tasks: list[Task] = dataclasses.field(default_factory=list)
    last_shortcut_at: int = 0
    recent_shortcut_at: dict[str, int] = dataclasses.field(default_factory=dict)

    @classmethod
    def from_json(cls, value: dict[str, Any]) -> "SenseiState":
        version = int(value.get("version", 0))
        if version != STATE_VERSION:
            raise ValueError(f"unsupported state version {version}")
        total = int(value.get("totalShortcuts", 0))
        if total < 0:
            raise ValueError("shortcut total cannot be negative")
        return cls(
            version=version,
            total_shortcuts=total,
            tasks=[Task.from_json(item) for item in value.get("tasks", [])],
            last_shortcut_at=int(value.get("lastShortcutAt", 0)),
            recent_shortcut_at={str(k): int(v) for k, v in value.get("recentShortcutAt", {}).items()},
        )

    def to_json(self) -> dict[str, Any]:
        value: dict[str, Any] = {
            "version": STATE_VERSION,
            "totalShortcuts": self.total_shortcuts,
            "tasks": [task.to_json() for task in self.tasks],
        }
        if self.last_shortcut_at:
            value["lastShortcutAt"] = self.last_shortcut_at
        if self.recent_shortcut_at:
            value["recentShortcutAt"] = self.recent_shortcut_at
        return value


def write_atomic(path: Path, data: bytes, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary_name = tempfile.mkstemp(prefix=".sensei-", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        os.fchmod(fd, mode)
        with os.fdopen(fd, "wb") as output:
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def write_if_changed(path: Path, data: bytes, mode: int) -> None:
    try:
        current = path.read_bytes()
        current_mode = stat.S_IMODE(path.stat().st_mode)
    except FileNotFoundError:
        current = None
        current_mode = None
    if current == data:
        if current_mode != mode:
            path.chmod(mode)
        return
    write_atomic(path, data, mode)


def backup_and_write(path: Path, data: bytes, mode: int) -> None:
    try:
        original_mode = stat.S_IMODE(path.stat().st_mode)
        current = path.read_bytes()
    except FileNotFoundError:
        current = None
        original_mode = None
    if current == data:
        return
    if current is not None:
        assert original_mode is not None
        stamp = dt.datetime.now().strftime("%Y%m%d-%H%M%S")
        backup = path.with_name(f"{path.name}.sensei-backup-{stamp}")
        write_atomic(backup, current, original_mode)
    write_atomic(path, data, original_mode if original_mode is not None else mode)


def load_state_file(path: Path) -> tuple[SenseiState, bool]:
    try:
        value = json.loads(path.read_text())
    except FileNotFoundError:
        return SenseiState(), False
    except json.JSONDecodeError as error:
        raise ValueError(f"read state: {error}") from error
    return SenseiState.from_json(value), True


def file_exists(path: Path) -> bool:
    return path.exists()


def prune_transient_state(state: SenseiState, when: dt.datetime) -> bool:
    now_ms = int(when.timestamp() * 1000)
    changed = False
    if state.last_shortcut_at:
        elapsed = now_ms - state.last_shortcut_at
        if elapsed < 0 or elapsed >= DUPLICATE_WINDOW_MS:
            state.last_shortcut_at = 0
            changed = True
    for action, recent in list(state.recent_shortcut_at.items()):
        elapsed = now_ms - recent
        if elapsed < 0 or elapsed >= MENU_CONSEQUENCE_MS:
            del state.recent_shortcut_at[action]
            changed = True
    return changed


def migrate_legacy_file(path: Path) -> SenseiState:
    events: list[dict[str, Any]] = []
    with path.open() as source:
        for line in source:
            if line.strip():
                events.append(json.loads(line))
    events.sort(key=lambda value: parse_time(value.get("occurredAt")))
    state = SenseiState()
    open_tasks: dict[str, Task] = {}
    recent: dict[str, dt.datetime] = {}
    last_chord: dict[str, dt.datetime] = {}
    known_shortcuts: dict[str, list[str]] = {}
    for event in events:
        observation = normalize_observation(Observation(
            observed_at=parse_time(event.get("occurredAt")),
            action=str(event.get("action", "")),
            title=str(event.get("title", "")),
            trigger=str(event.get("trigger", "")),
            shortcut=str(event.get("shortcut", "")),
            shortcuts=[str(item) for item in event.get("shortcuts", [])],
        ))
        if observation.trigger == "shortcut" and observation.shortcut and not is_grouped_action(observation.action):
            known_shortcuts[observation.action] = merge_shortcuts(known_shortcuts.get(observation.action, []), observation.shortcut)
        if not observation.action:
            continue
        if observation.trigger == "shortcut":
            key = canonical_shortcut(observation.shortcut) or observation.action
            previous = last_chord.get(key)
            if previous is None or (observation.observed_at - previous).total_seconds() < 0 or (observation.observed_at - previous).total_seconds() * 1000 >= DUPLICATE_WINDOW_MS:
                state.total_shortcuts += 1
            last_chord[key] = observation.observed_at
            recent[observation.action] = observation.observed_at
            open_tasks.pop(observation.action, None)
            continue
        if observation.trigger not in {"menu", "mouse"}:
            continue
        if observation.trigger == "menu" and observation.action in recent:
            elapsed = (observation.observed_at - recent[observation.action]).total_seconds() * 1000
            if 0 <= elapsed < MENU_CONSEQUENCE_MS:
                continue
        shortcuts = observation_shortcuts(observation)
        if not shortcuts:
            continue
        task = open_tasks.get(observation.action)
        if task is None:
            task = Task(observation.action, observation.title, [], observation.observed_at, 0)
            open_tasks[observation.action] = task
        task.title = observation.title
        task.shortcuts = merge_shortcuts(shortcuts, *known_shortcuts.get(observation.action, []))
        task.slow_uses += 1
    state.tasks = sorted(open_tasks.values(), key=lambda task: (-task.slow_uses, task.opened_at))
    return state


class StateStore:
    def __init__(self, paths: Paths):
        self.paths = paths

    def locked(self):
        self.paths.state_dir.mkdir(parents=True, exist_ok=True)
        lock = self.paths.state_lock.open("a+")
        os.chmod(lock.fileno(), 0o600)
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        return lock

    def read_modify(self, when: dt.datetime, update: Callable[[SenseiState], bool] | None = None, ensure_file: bool = False) -> SenseiState:
        with self.locked() as lock:
            try:
                state, exists = load_state_file(self.paths.state)
                migrated = False
                if not exists and self.paths.legacy_events.exists():
                    state = migrate_legacy_file(self.paths.legacy_events)
                    migrated = True
                changed = prune_transient_state(state, when)
                if update is not None:
                    changed = bool(update(state)) or changed
                if ensure_file and not exists:
                    changed = True
                if migrated or changed:
                    write_atomic(self.paths.state, (json.dumps(state.to_json(), indent=2, ensure_ascii=False) + "\n").encode(), 0o600)
                if self.paths.legacy_events.exists() and (exists or migrated):
                    self.paths.legacy_events.unlink(missing_ok=True)
                return state
            finally:
                fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                lock.close()

    def clear(self) -> None:
        with self.locked() as lock:
            try:
                self.paths.state.unlink(missing_ok=True)
                self.paths.legacy_events.unlink(missing_ok=True)
            finally:
                fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                lock.close()


def level_progress(total: int) -> dict[str, Any]:
    total = max(0, total)
    level, level_start, requirement = 1, 0, 10
    while total >= level_start + requirement:
        level_start += requirement
        level += 1
        requirement = (requirement * 3 + 1) // 2
    current = total - level_start
    return {
        "totalShortcuts": total,
        "level": level,
        "nextLevel": level + 1,
        "shortcutsInLevel": current,
        "shortcutsForLevel": requirement,
        "shortcutsRemaining": requirement - current,
        "progress": current / requirement,
    }


def snapshot_from_state(state: SenseiState, paused: bool) -> dict[str, Any]:
    tasks = sorted(state.tasks, key=lambda task: (-task.slow_uses, task.opened_at))
    return {
        "tasks": [task.to_json() for task in tasks],
        "level": level_progress(state.total_shortcuts),
        "paused": paused,
    }


def is_paused(paths: Paths) -> bool:
    return paths.paused.exists()


def update_coaching_state(paths: Paths, observation: Observation) -> None:
    if is_paused(paths):
        return
    store = StateStore(paths)

    def update(state: SenseiState) -> bool:
        apply_observation(state, observation)
        return True

    store.read_modify(observation.observed_at, update, ensure_file=True)


def initialize_state(paths: Paths) -> None:
    StateStore(paths).read_modify(now_utc(), ensure_file=True)


def apply_observation(state: SenseiState, observation: Observation) -> None:
    observation = normalize_observation(observation)
    if not observation.action:
        return
    when = observation.observed_at if observation.observed_at else now_utc()
    prune_transient_state(state, when)
    now_ms = int(when.timestamp() * 1000)
    if observation.trigger == "shortcut":
        elapsed = now_ms - state.last_shortcut_at
        if not state.last_shortcut_at or elapsed < 0 or elapsed >= DUPLICATE_WINDOW_MS:
            state.total_shortcuts += 1
        state.last_shortcut_at = now_ms
        state.recent_shortcut_at[observation.action] = now_ms
        state.tasks = [task for task in state.tasks if task.action != observation.action]
        return
    if observation.trigger == "menu" and observation.action in state.recent_shortcut_at:
        elapsed = now_ms - state.recent_shortcut_at[observation.action]
        if 0 <= elapsed < MENU_CONSEQUENCE_MS:
            return
    shortcuts = observation_shortcuts(observation)
    if not shortcuts:
        return
    for task in state.tasks:
        if task.action == observation.action:
            task.title = observation.title
            task.shortcuts = merge_shortcuts(shortcuts)
            task.slow_uses += 1
            return
    state.tasks.append(Task(observation.action, observation.title, shortcuts, when, 1))


def normalize_observation(observation: Observation) -> Observation:
    if is_workspace_description(observation.title) or observation.action.startswith("switch-to-workspace-") or observation.action in {"next-workspace", "previous-workspace", "former-workspace"}:
        observation.action = "workspace-switching"
        observation.title = "Workspace switching"
        if observation.trigger != "shortcut":
            observation.shortcut = "SUPER + TAB"
            observation.shortcuts = ["SUPER + TAB"]
    elif is_panel_description(observation.title) or observation.action.startswith("bar-panel-"):
        observation.action = "bar-panels"
        observation.title = "Bar panels"
    return observation


def is_workspace_description(description: str) -> bool:
    value = description.strip().lower()
    return value in {
        "workspace switching",
        "next workspace",
        "previous workspace",
        "former workspace",
        "scroll active workspace forward",
        "scroll active workspace backward",
    } or value.startswith("switch to workspace ")


def is_panel_description(description: str) -> bool:
    value = description.strip().lower()
    if value == "bar panels":
        return True
    suffix = value.removeprefix("bar panel ")
    return suffix != value and bool(suffix) and suffix.isdigit()


def is_grouped_action(action: str) -> bool:
    return action in {"workspace-switching", "bar-panels"}


def canonical_shortcut(shortcut: str) -> str:
    return " ".join(shortcut.upper().replace("+", " ").split())


def merge_shortcuts(existing: Iterable[str] | None, *additions: str) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for shortcut in list(existing or []) + list(additions):
        value = str(shortcut).strip()
        key = canonical_shortcut(value)
        if value and key not in seen:
            result.append(value)
            seen.add(key)
    return sorted(result, key=lambda value: (len(canonical_shortcut(value).split()), canonical_shortcut(value)))


def observation_shortcuts(observation: Observation) -> list[str]:
    return merge_shortcuts(observation.shortcuts, observation.shortcut)


@dataclasses.dataclass
class MenuItem:
    id: str
    parent: str
    icon: str
    icon_font: str
    label: str
    title: str
    description: str
    action: str
    aliases: list[str]
    when: str
    checked: str

    def to_json(self) -> dict[str, Any]:
        value: dict[str, Any] = {"id": self.id, "parent": self.parent, "label": self.label, "action": self.action}
        for key, field in (("icon", "icon"), ("iconFont", "icon_font"), ("title", "title"), ("description", "description"), ("when", "when"), ("checked", "checked")):
            current = getattr(self, field)
            if current:
                value[key] = current
        if self.aliases:
            value["aliases"] = self.aliases
        return value


@dataclasses.dataclass
class Binding:
    description: str
    shortcuts: list[str]
    dispatcher: str = ""
    argument: str = ""
    concept_action: str = ""
    concept_title: str = ""

    def to_json(self) -> dict[str, Any]:
        value: dict[str, Any] = {"description": self.description, "shortcuts": self.shortcuts}
        if self.dispatcher:
            value["dispatcher"] = self.dispatcher
        if self.argument:
            value["argument"] = self.argument
        if self.concept_action:
            value["conceptAction"] = self.concept_action
        if self.concept_title:
            value["conceptTitle"] = self.concept_title
        return value


@dataclasses.dataclass
class DesktopEntry:
    desktop_id: str
    name: str
    generic_name: str
    keywords: list[str]
    command: str


def desktop_entry_paths() -> list[Path]:
    home = Path.home()
    data_home = Path(os.environ.get("XDG_DATA_HOME", home / ".local" / "share"))
    data_dirs = [Path(value) for value in os.environ.get("XDG_DATA_DIRS", "/usr/local/share:/usr/share").split(":") if value]
    return [data_home / "applications", *(path / "applications" for path in data_dirs)]


def load_desktop_entries() -> list[DesktopEntry]:
    entries: dict[str, DesktopEntry] = {}
    for directory in desktop_entry_paths():
        try:
            files = sorted(directory.glob("*.desktop"))
        except OSError:
            continue
        for path in files:
            desktop_id = path.stem
            if desktop_id in entries:
                continue
            parser = configparser.ConfigParser(interpolation=None, strict=False)
            parser.optionxform = str
            try:
                parser.read(path, encoding="utf-8")
                value = parser["Desktop Entry"]
            except (OSError, KeyError, configparser.Error):
                continue
            if value.get("Hidden", "false").casefold() == "true" or value.get("NoDisplay", "false").casefold() == "true":
                continue
            name = value.get("Name", "").strip()
            command = value.get("Exec", "").strip()
            if not name or not command:
                continue
            keywords = [item.strip() for item in value.get("Keywords", "").split(";") if item.strip()]
            entries[desktop_id] = DesktopEntry(
                desktop_id,
                name,
                value.get("GenericName", "").strip(),
                keywords,
                command,
            )
    return list(entries.values())


def normalized_url(value: str) -> str:
    try:
        parsed = urllib.parse.urlsplit(value.strip("'\""))
    except ValueError:
        return ""
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return ""
    path = parsed.path.rstrip("/") or "/"
    return urllib.parse.urlunsplit((parsed.scheme.casefold(), parsed.netloc.casefold(), path, parsed.query, ""))


def command_urls(command: str) -> set[str]:
    result: set[str] = set()
    for value in re.findall(r"https?://[^\s'\"]+", command):
        normalized = normalized_url(value)
        if normalized:
            result.add(normalized)
    return result


COMMAND_ATOM_STOPWORDS = {
    "app", "bash", "command", "exec", "focus", "gtk-launch", "launch",
    "omarchy", "or", "setsid", "shell", "systemd-run", "terminal", "tui",
    "uwsm-app", "webapp", "xdg-terminal-exec",
}


def command_atoms(command: str) -> set[str]:
    try:
        fields = shlex.split(command)
    except ValueError:
        fields = command.split()
    result: set[str] = set()
    for field in fields:
        if field.startswith("-") or field.startswith("%") or "://" in field or "=" in field:
            continue
        value = Path(field).name.casefold()
        if value.endswith(".desktop"):
            value = value[:-8]
        for atom in re.findall(r"[a-z0-9][a-z0-9.+]*", value.replace("_", "-")):
            if len(atom) >= 3 and atom not in COMMAND_ATOM_STOPWORDS:
                result.add(atom)
    return result


def binding_role(binding: Binding) -> str:
    if binding.dispatcher.casefold() != "exec":
        return ""
    try:
        fields = shlex.split(binding.argument)
    except ValueError:
        return ""
    if not fields:
        return ""
    head = Path(fields[0]).name.casefold()
    for role in ("browser", "editor", "terminal"):
        if head == f"omarchy-launch-{role}":
            return role
        if fields[:3] == ["omarchy", "launch", role]:
            return role
    return ""


def default_desktop_roles() -> dict[str, str]:
    roles: dict[str, str] = {}
    for role in ("browser", "editor", "terminal"):
        command = shutil.which(f"omarchy-default-{role}")
        if not command:
            continue
        try:
            value = subprocess.run([command], capture_output=True, text=True, timeout=2).stdout.strip()
        except (OSError, subprocess.SubprocessError):
            continue
        if value:
            roles[role] = Path(value).name.casefold().removesuffix(".desktop")
    return roles


def desktop_entry_roles(entry: DesktopEntry, defaults: dict[str, str]) -> set[str]:
    identities = {entry.desktop_id.casefold(), literal_phrase(entry.name), *command_atoms(entry.command)}
    return {role for role, value in defaults.items() if value in identities}


def binding_is_app_launch(binding: Binding) -> bool:
    if binding.dispatcher.casefold() != "exec":
        return False
    try:
        fields = shlex.split(binding.argument)
    except ValueError:
        return False
    if not fields:
        return False
    head = Path(fields[0]).name.casefold()
    return head.startswith("omarchy-launch-") or head in {"omarchy-agent", "omacalc", "uwsm-app"} or fields[:2] == ["omarchy", "launch"]


APP_VARIANT_WORDS = {"cwd", "new", "post", "private"}


def app_variant_words(*values: str) -> set[str]:
    words = set().union(*(set(WORDS_PATTERN.findall(value.casefold())) for value in values))
    return words & APP_VARIANT_WORDS


def app_binding_score(entry: DesktopEntry, binding: Binding, defaults: dict[str, str]) -> tuple[int, str]:
    if not binding_is_app_launch(binding):
        return 0, ""
    binding_variants = app_variant_words(binding.description, binding.argument)
    entry_variants = app_variant_words(entry.name, entry.generic_name, entry.command)
    if not binding_variants.issubset(entry_variants):
        return 0, ""
    urls = command_urls(entry.command) & command_urls(binding.argument)
    if urls:
        return 500, f"identical URL {sorted(urls)[0]!r}"
    role = binding_role(binding)
    if role and role in desktop_entry_roles(entry, defaults):
        return 450, f"current default {role}"
    atoms = command_atoms(entry.command) & command_atoms(binding.argument)
    if atoms:
        return 400, f"unique executable {sorted(atoms)[0]!r}"
    binding_name = literal_phrase(binding.description)
    if binding_name == literal_phrase(entry.name):
        return 350, f"exact app name {entry.name!r}"
    if entry.generic_name and binding_name == literal_phrase(entry.generic_name):
        return 300, f"exact generic name {entry.generic_name!r}"
    if binding_name and binding_name in {literal_phrase(value) for value in entry.keywords}:
        return 250, f"exact desktop keyword {binding.description!r}"
    return 0, ""


def resolve_app_binding(bindings: list[Binding], name: str, desktop_id: str = "") -> tuple[Binding | None, str]:
    selected = []
    wanted_name = literal_phrase(name)
    wanted_id = desktop_id.casefold().removesuffix(".desktop")
    for entry in load_desktop_entries():
        if wanted_id and entry.desktop_id.casefold() == wanted_id:
            selected.append(entry)
        elif not wanted_id and literal_phrase(entry.name) == wanted_name:
            selected.append(entry)
    if not selected:
        return None, "no installed desktop entry"
    defaults = default_desktop_roles()
    candidates: list[tuple[int, str, Binding]] = []
    for entry in selected:
        for binding in bindings:
            score, evidence = app_binding_score(entry, binding, defaults)
            if score:
                candidates.append((score, evidence, binding))
    if not candidates:
        return None, "no binding identity"
    best = max(score for score, _, _ in candidates)
    winners: dict[str, tuple[Binding, str]] = {}
    for score, evidence, binding in candidates:
        if score == best:
            winners[binding_action(binding)] = (binding, evidence)
    if len(winners) != 1:
        return None, "ambiguous binding identity"
    return next(iter(winners.values()))


@dataclasses.dataclass
class CatalogMatch:
    menu: MenuItem
    binding: Binding
    confidence: str
    evidence: str = ""

    def to_json(self) -> dict[str, Any]:
        value = {"menu": self.menu.to_json(), "binding": self.binding.to_json(), "confidence": self.confidence}
        if self.evidence:
            value["evidence"] = self.evidence
        return value


@dataclasses.dataclass
class Catalog:
    matches: list[CatalogMatch]
    unmatched_menu: list[MenuItem]
    unmatched_bindings: list[Binding]
    binding_source: str = "live"
    binding_generated_at: str = ""

    def to_json(self) -> dict[str, Any]:
        value = {
            "matches": [item.to_json() for item in self.matches],
            "unmatchedMenu": [item.to_json() for item in self.unmatched_menu],
            "unmatchedBindings": [item.to_json() for item in self.unmatched_bindings],
        }
        value["bindingSource"] = self.binding_source
        if self.binding_generated_at:
            value["bindingGeneratedAt"] = self.binding_generated_at
        return value


@dataclasses.dataclass
class BindingRecords:
    data: str
    source: str
    generated_at: str = ""


def parse_menu_jsonc_all(data: str) -> list[MenuItem]:
    kept = "\n".join(line for line in data.splitlines() if not line.strip().startswith("//"))
    clean = TRAILING_COMMA.sub(r"\1", kept)
    if not clean.strip():
        return []
    raw = json.loads(clean)
    if "items" in raw:
        raw = raw["items"]
    result: list[MenuItem] = []
    for item_id, value in sorted(raw.items()):
        parent = item_id.rsplit(".", 1)[0] if "." in item_id else "root"
        if value.get("parent") is not None:
            parent = value["parent"]
        label = value.get("label") or item_id
        result.append(MenuItem(item_id, parent, value.get("icon", ""), value.get("iconFont", ""), label, value.get("title", ""), value.get("description", ""), value.get("action", ""), decode_aliases(value.get("aliases")), value.get("when", ""), value.get("checked", "")))
    return result


def parse_menu_jsonc(data: str) -> list[MenuItem]:
    return [item for item in parse_menu_jsonc_all(data) if item.action]


def decode_aliases(value: Any) -> list[str]:
    if isinstance(value, list):
        return [str(item) for item in value]
    if isinstance(value, str) and value:
        return [value]
    return []


def load_merged_menu(paths: Paths, actions_only: bool = True) -> list[MenuItem]:
    defaults = parse_menu_jsonc_all(paths.default_menu.read_text())
    try:
        user_text = paths.menu_extension.read_text()
    except FileNotFoundError:
        user_text = ""
    user = parse_menu_jsonc_all(strip_managed_block(user_text, MENU_START, MENU_END)) if user_text else []
    merged: dict[str, MenuItem] = {}
    order: list[str] = []
    for item in defaults + user:
        if item.id not in merged:
            order.append(item.id)
        merged[item.id] = item
    result = [merged[item_id] for item_id in order]
    return [item for item in result if item.action] if actions_only else result


def binding_record_count(data: str) -> int:
    return sum(1 for line in data.splitlines() if "→" in line)


def latest_cached_binding_records() -> BindingRecords | None:
    home = Path.home()
    cache_root = Path(os.environ.get("XDG_CACHE_HOME", home / ".cache")) / "omarchy"
    candidates: list[Path] = []
    try:
        candidates = sorted(
            cache_root.glob("keybindings-*.records"),
            key=lambda path: path.stat().st_mtime,
            reverse=True,
        )
    except OSError:
        return None
    for candidate in candidates:
        try:
            data = candidate.read_text()
            modified = dt.datetime.fromtimestamp(candidate.stat().st_mtime, dt.timezone.utc)
        except OSError:
            continue
        # Omarchy always appends two static web-app records. A file containing
        # only those records is the fallout of an unavailable compositor, not
        # a useful binding snapshot.
        if binding_record_count(data) > 2 and "\t" in data:
            return BindingRecords(data, f"cache:{candidate}", iso_time(modified))
    return None


def resolved_keybinding_snapshot() -> BindingRecords:
    path = shutil.which("omarchy-menu-keybindings")
    if not path:
        raise FileNotFoundError("omarchy-menu-keybindings")
    script = Path(path).read_text()
    marker = '\nif [[ $1 == "--print"'
    index = script.rfind(marker)
    if index < 0:
        process = subprocess.run([path, "--print"], capture_output=True, text=True, check=True)
        return BindingRecords(process.stdout, "print")
    source = script[:index] + "\noutput_binding_records_uncached\n"
    process = subprocess.run(["bash"], input=source, capture_output=True, text=True)
    if process.returncode == 0 and binding_record_count(process.stdout) > 2:
        return BindingRecords(process.stdout, "live", iso_time(now_utc()))
    cached = latest_cached_binding_records()
    if cached is not None:
        return cached
    # During shell startup or a reload the compositor may be unavailable.
    # Fall back to Omarchy's cache-backed normal command in that case.
    process = subprocess.run([path, "--print"], capture_output=True, text=True, check=True)
    return BindingRecords(process.stdout, "print")


def resolved_keybinding_records() -> str:
    return resolved_keybinding_snapshot().data


def bindings_from_records(data: str) -> list[Binding]:
    by_description: dict[str, Binding] = {}
    order: list[str] = []
    for line in data.splitlines():
        fields = line.split("\t")
        if "→" not in fields[0]:
            continue
        shortcut, description = (part.strip() for part in fields[0].split("→", 1))
        if not shortcut or not description:
            continue
        key = normalized_phrase(description)
        binding = by_description.get(key)
        if binding is None:
            binding = Binding(description, [])
            by_description[key] = binding
            order.append(key)
        binding.shortcuts = merge_shortcuts(binding.shortcuts, shortcut)
        if len(fields) > 1 and not binding.dispatcher:
            binding.dispatcher = fields[1].strip()
        if len(fields) > 2 and not binding.argument:
            binding.argument = "\t".join(fields[2:]).strip()
    return [by_description[key] for key in order]


def load_catalog(paths: Paths) -> Catalog:
    menu_all = load_merged_menu(paths, actions_only=False)
    menu = [item for item in menu_all if item.action]
    snapshot = resolved_keybinding_snapshot()
    bindings = annotate_binding_concepts(bindings_from_records(snapshot.data), menu_all)
    matches, unmatched_menu = match_catalog(menu, bindings)
    matched_bindings = {normalized_phrase(match.binding.description) for match in matches}
    unmatched_bindings = [binding for binding in bindings if normalized_phrase(binding.description) not in matched_bindings]
    return Catalog(matches, unmatched_menu, unmatched_bindings, snapshot.source, snapshot.generated_at)


def match_catalog(menu: list[MenuItem], bindings: list[Binding]) -> tuple[list[CatalogMatch], list[MenuItem]]:
    route_owners = identity_owners(menu, menu_routes)
    phrase_owners = identity_owners(menu, menu_phrases)
    token_owners = identity_owners(menu, lambda item: [token_identity(tokens) for tokens in strong_semantic_token_sets(item)])
    matches: list[CatalogMatch] = []
    unmatched: list[MenuItem] = []
    for item in menu:
        binding, confidence, evidence = match_menu_item(
            item,
            bindings,
            route_owners,
            phrase_owners,
            token_owners,
        )
        if binding is None:
            unmatched.append(item)
        else:
            matches.append(CatalogMatch(item, binding, confidence, evidence))
    return matches, unmatched


def identity_owners(
    menu: list[MenuItem],
    identities: Callable[[MenuItem], Iterable[str]],
) -> dict[str, set[str]]:
    owners: dict[str, set[str]] = {}
    for item in menu:
        for identity in identities(item):
            if identity:
                owners.setdefault(identity, set()).add(item.id)
    return owners


def menu_routes(item: MenuItem) -> list[str]:
    return unique_strings([item.id, *item.aliases], normalize_route)


def menu_phrases(item: MenuItem) -> list[str]:
    return unique_strings(
        [item.label, item.title, item.description, *item.aliases],
        normalized_phrase,
    )


def unique_strings(values: Iterable[str], normalize: Callable[[str], str]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        current = normalize(value)
        if current and current not in seen:
            result.append(current)
            seen.add(current)
    return result


def normalize_route(value: str) -> str:
    return value.strip().casefold()


def token_identity(tokens: set[str]) -> str:
    return "\x1f".join(sorted(tokens))


def binding_menu_route(binding: Binding) -> str:
    if binding.dispatcher.casefold() != "exec":
        return ""
    try:
        fields = shlex.split(binding.argument)
    except ValueError:
        return ""
    if not fields:
        return ""
    if Path(fields[0]).name == "omarchy-menu" and len(fields) in {2, 3} and fields[1] in {"toggle", "summon"}:
        return normalize_route(fields[2] if len(fields) == 3 else "root")
    if Path(fields[0]).name == "omarchy" and len(fields) in {3, 4} and fields[1:3] in (["menu", "toggle"], ["menu", "summon"]):
        return normalize_route(fields[3] if len(fields) == 4 else "root")
    return ""


def resolve_menu_route_binding(paths: Paths, bindings: list[Binding], route: str) -> Binding | None:
    requested = normalize_route(route or "root")
    menu = load_merged_menu(paths, actions_only=False)
    owners = identity_owners(menu, menu_routes)
    if requested == "root":
        item_id = "root"
    else:
        item_ids = owners.get(requested, set())
        if len(item_ids) != 1:
            return None
        item_id = next(iter(item_ids))

    candidates: dict[str, Binding] = {}
    for binding in bindings:
        target = binding_menu_route(binding)
        if not target:
            continue
        if target == "root":
            target_id = "root"
        else:
            target_ids = owners.get(target, set())
            if len(target_ids) != 1:
                continue
            target_id = next(iter(target_ids))
        if target_id == item_id:
            candidates[binding_action(binding)] = binding
    return next(iter(candidates.values())) if len(candidates) == 1 else None


def unique_binding_candidates(
    candidates: Iterable[tuple[Binding, str]],
) -> tuple[Binding | None, str]:
    unique: dict[int, tuple[Binding, str]] = {}
    for binding, evidence in candidates:
        unique[id(binding)] = (binding, evidence)
    if len(unique) != 1:
        return None, ""
    return next(iter(unique.values()))


def semantic_operations(*values: str) -> set[str]:
    words = set().union(*(token_set(value) for value in values))
    return {SEMANTIC_OPERATION_CLASSES[word] for word in words if word in SEMANTIC_OPERATION_CLASSES}


def semantic_operations_match(item: MenuItem, binding: Binding) -> bool:
    item_operations = semantic_operations(item.id, item.action)
    binding_operations = semantic_operations(binding.description, binding.argument)
    return item_operations == binding_operations


def match_menu_item(
    item: MenuItem,
    bindings: list[Binding],
    route_owners: dict[str, set[str]],
    phrase_owners: dict[str, set[str]],
    token_owners: dict[str, set[str]],
) -> tuple[Binding | None, str, str]:
    command = normalized_command(item.action)
    if command:
        exact = [binding for binding in bindings if binding.dispatcher == "exec" and normalized_command(binding.argument) == command]
        if len(exact) == 1:
            return exact[0], "command-exact", "identical exec command"

    route_candidates: list[tuple[Binding, str]] = []
    for binding in bindings:
        route = binding_menu_route(binding)
        if route and route_owners.get(route) == {item.id}:
            route_candidates.append((binding, route))
    binding, route = unique_binding_candidates(route_candidates)
    if binding is not None:
        confidence = "route-id" if route == normalize_route(item.id) else "route-alias"
        return binding, confidence, f"unique menu route {route!r}"

    phrase_candidates: list[tuple[Binding, str]] = []
    for phrase in menu_phrases(item):
        if phrase_owners.get(phrase) != {item.id}:
            continue
        for binding in bindings:
            if normalized_phrase(binding.description) == phrase and semantic_operations_match(item, binding):
                phrase_candidates.append((binding, phrase))
    binding, phrase = unique_binding_candidates(phrase_candidates)
    if binding is not None:
        return binding, "phrase-exact", f"unique phrase {phrase!r}"

    token_candidates: list[tuple[Binding, str]] = []
    for tokens in strong_semantic_token_sets(item):
        identity = token_identity(tokens)
        if not identity or token_owners.get(identity) != {item.id}:
            continue
        for binding in bindings:
            if token_set(binding.description) == tokens and semantic_operations_match(item, binding):
                token_candidates.append((binding, " ".join(sorted(tokens))))
    binding, tokens = unique_binding_candidates(token_candidates)
    if binding is not None:
        return binding, "token-exact", f"unique tokens {tokens!r}"
    return None, "", ""


def strong_semantic_token_sets(item: MenuItem) -> list[set[str]]:
    values = [item.label, item.title, item.description, *item.aliases]
    segments = item.id.split(".")
    values.extend(" ".join(segments[start:]) for start in range(len(segments)))
    return [tokens for value in values if (tokens := token_set(value))]


def normalized_command(value: str) -> str:
    return " ".join(value.strip().split())


def normalized_phrase(value: str) -> str:
    return " ".join(normalized_words(value))


def normalized_words(value: str) -> list[str]:
    result = []
    for word in WORDS_PATTERN.findall(value.lower()):
        if len(word) > 3 and word.endswith("s") and not word.endswith("ss"):
            word = word[:-1]
        result.append(word)
    return result


def token_set(value: str) -> set[str]:
    return set(normalized_words(value))


def action_id(description: str) -> str:
    return "-".join(WORDS_PATTERN.findall(description.lower()))


def literal_phrase(value: str) -> str:
    return " ".join(WORDS_PATTERN.findall(value.casefold()))


def binding_command_key(binding: Binding) -> str:
    dispatcher = binding.dispatcher.strip().casefold()
    argument = normalized_command(binding.argument)
    return f"{dispatcher}\0{argument}" if dispatcher and argument else ""


def binding_is_workspace_switch(binding: Binding) -> bool:
    if is_workspace_description(binding.description):
        return True
    if binding.dispatcher.casefold() != "lua":
        return False
    return bool(re.search(r"hl\.dsp\.focus\s*\(\s*\{[^}]*\bworkspace\s*=", binding.argument))


def binding_group_key(binding: Binding, route_owners: dict[str, set[str]] | None = None) -> str:
    if binding_is_workspace_switch(binding):
        return "family:workspace-switching"
    if is_panel_description(binding.description):
        return "family:bar-panels"
    route = binding_menu_route(binding)
    if route and route_owners is not None:
        if route == "root":
            return "menu-route:root"
        owners = route_owners.get(route, set())
        if len(owners) == 1:
            return "menu-route:" + next(iter(owners))
    command = binding_command_key(binding)
    if command:
        return "command:" + command
    return "description:" + literal_phrase(binding.description)


def annotate_binding_concepts(bindings: list[Binding], menu: list[MenuItem] | None = None) -> list[Binding]:
    route_owners = identity_owners(menu, menu_routes) if menu is not None else None
    groups: dict[str, list[Binding]] = {}
    for binding in bindings:
        groups.setdefault(binding_group_key(binding, route_owners), []).append(binding)

    concepts: dict[str, tuple[str, str, list[str]]] = {}
    for key, members in groups.items():
        if key == "family:workspace-switching":
            action, title = "workspace-switching", "Workspace switching"
        elif key == "family:bar-panels":
            action, title = "bar-panels", "Bar panels"
        elif (key.startswith("command:") or key.startswith("menu-route:")) and len(members) > 1:
            digest = hashlib.sha256(key.encode()).hexdigest()[:12]
            action, title = f"binding-{digest}", members[0].description
        else:
            action, title = action_id(members[0].description), members[0].description
        shortcuts: list[str] = []
        for member in members:
            shortcuts = merge_shortcuts(shortcuts, *member.shortcuts)
        concepts[key] = action, title, shortcuts

    result: list[Binding] = []
    for binding in bindings:
        action, title, shortcuts = concepts[binding_group_key(binding, route_owners)]
        result.append(dataclasses.replace(
            binding,
            shortcuts=shortcuts,
            concept_action=action,
            concept_title=title,
        ))
    return result


def binding_action(binding: Binding) -> str:
    return binding.concept_action or action_id(binding.description)


def binding_title(binding: Binding) -> str:
    return binding.concept_title or binding.description


def strip_managed_block(content: str, start: str, end: str) -> str:
    start_index = content.find(start)
    if start_index < 0:
        return content
    line_start = content.rfind("\n", 0, start_index) + 1
    end_index = content.find(end, start_index)
    if end_index < 0:
        return content
    end_index += len(end)
    if end_index < len(content) and content[end_index] == "\n":
        end_index += 1
    return content[:line_start] + content[end_index:]


def shell_quote(value: str) -> str:
    return "'" + value.replace("'", "'\\''") + "'"


def unique_catalog_bindings(catalog: Catalog) -> list[Binding]:
    result: list[Binding] = []
    seen: set[str] = set()
    for binding in [*(match.binding for match in catalog.matches), *catalog.unmatched_bindings]:
        key = normalized_phrase(binding.description)
        if key not in seen:
            result.append(binding)
            seen.add(key)
    return result


def lua_string(value: str) -> str:
    return json.dumps(str(value), ensure_ascii=False)


def sensei_lua(bindings: list[Binding] | None = None) -> str:
    concept_lines = []
    for binding in bindings or []:
        concept_lines.append(
            "    [" + lua_string(binding.description) + "] = { action = "
            + lua_string(binding_action(binding)) + ", title = "
            + lua_string(binding_title(binding)) + " },"
        )
    concept_table = "\n".join(concept_lines)
    return f'''-- Generated by omarchy-sensei setup. Re-run setup instead of editing this file.
if hl and not _G.omarchy_sensei_original_hl_bind then
  _G.omarchy_sensei_original_hl_bind = hl.bind
  local coaching_by_description = {{
{concept_table}
  }}
  local function slug(value)
    return tostring(value):lower():gsub("[^%w]+", "-"):gsub("^-", ""):gsub("-$", "")
  end
  local function quote(value)
    return "'" .. tostring(value):gsub("'", "'\\\\''") .. "'"
  end
  local function coaching_identity(description)
    local text = tostring(description or "")
    local known = coaching_by_description[text]
    if known then return known.action, known.title end
    if text:match("^Switch to workspace %d+$") or text == "Next workspace"
      or text == "Previous workspace" or text == "Former workspace"
      or text == "Scroll active workspace forward"
      or text == "Scroll active workspace backward" then
      return "workspace-switching", "Workspace switching"
    end
    if text:match("^Bar panel %d+$") then
      return "bar-panels", "Bar panels"
    end
    return slug(text), text
  end
  function hl.bind(keys, dispatcher, options)
    local original = _G.omarchy_sensei_original_hl_bind
    local description = options and (options.description or options.desc)
    local key_text = tostring(keys or "")
    local source_scan = type(dispatcher) == "table" and dispatcher.__omarchy_dispatcher
    if source_scan or not description or key_text:find("mouse", 1, true) or key_text:find("switch:", 1, true) then
      return original(keys, dispatcher, options)
    end
    local action, title = coaching_identity(description)
    local command = "omarchy-sensei complete --action " .. quote(action)
      .. " --title " .. quote(title)
    return original(keys, function()
      hl.dispatch(dispatcher)
      hl.exec_cmd(command)
    end, options)
  end

  -- Observe only completed left clicks that coincide with a real active-window
  -- transition. The binding is non-consuming: Hyprland still delivers the
  -- original click unchanged, and ordinary clicks launch no process.
  if hl.on and hl.timer and hl.get_active_window then
    local last_active = ""
    local pointer_down = nil
    local recent_focus = nil
    local pointer_serial = 0
    local function window_address(window)
      if not window or not window.address then return "" end
      return tostring(window.address)
    end
    local ok, active = pcall(hl.get_active_window)
    if ok then last_active = window_address(active) end
    hl.bind("mouse:272", function()
      pointer_serial = pointer_serial + 1
      local serial = pointer_serial
      pointer_down = {{ serial = serial }}
      recent_focus = nil
      hl.timer(function()
        if pointer_down and pointer_down.serial == serial then pointer_down = nil end
        if recent_focus and recent_focus.serial == serial then recent_focus = nil end
      end, {{ timeout = 800, type = "oneshot" }})
    end, {{ non_consuming = true }})
    hl.on("window.active", function(window)
      local next_active = window_address(window)
      if pointer_down and last_active ~= "" and next_active ~= "" and next_active ~= last_active then
        recent_focus = {{ from = last_active, to = next_active, serial = pointer_down.serial }}
      end
      last_active = next_active
    end)
    hl.bind("mouse:272", function()
      local transition = recent_focus
      pointer_down = nil
      recent_focus = nil
      if transition then
        hl.exec_cmd("omarchy-sensei coach-focus --from " .. quote(transition.from)
          .. " --to " .. quote(transition.to))
      end
    end, {{ release = true, non_consuming = true }})
  end
end
'''


def menu_override_block(matches: list[CatalogMatch]) -> str:
    lines = []
    for match in matches:
        shortcuts = merge_shortcuts(match.binding.shortcuts)
        if not shortcuts:
            continue
        shortcut_args = "".join(" --shortcut " + shell_quote(shortcut) for shortcut in shortcuts)
        command = (
            "omarchy-sensei run --action " + shell_quote(binding_action(match.binding))
            + " --title " + shell_quote(binding_title(match.binding))
            + shortcut_args + " -- " + shell_quote(match.menu.action)
        )
        item = match.menu.to_json()
        # The menu extension uses the map key as the item's id; Omarchy's
        # native override shape intentionally omits a duplicate ``id`` field.
        item.pop("id", None)
        item["action"] = command
        lines.append(f"  {json.dumps(match.menu.id, ensure_ascii=False)}: {json.dumps(item, ensure_ascii=False, separators=(',', ':'))},")
    return "\n".join(lines) + ("\n" if lines else "")


def install_binding_cache(paths: Paths, catalog: Catalog) -> None:
    bindings: list[Binding] = []
    seen: set[str] = set()
    for match in catalog.matches:
        key = normalized_phrase(match.binding.description)
        if key not in seen:
            bindings.append(match.binding)
            seen.add(key)
    for binding in catalog.unmatched_bindings:
        key = normalized_phrase(binding.description)
        if key not in seen:
            bindings.append(binding)
            seen.add(key)
    bindings.sort(key=lambda binding: normalized_phrase(binding.description))
    write_if_changed(paths.binding_cache, (json.dumps([item.to_json() for item in bindings], ensure_ascii=False) + "\n").encode(), 0o600)


def load_binding_cache(path: Path) -> list[Binding]:
    value = json.loads(path.read_text())
    return [Binding(
        str(item.get("description", "")),
        [str(x) for x in item.get("shortcuts", [])],
        str(item.get("dispatcher", "")),
        str(item.get("argument", "")),
        str(item.get("conceptAction", "")),
        str(item.get("conceptTitle", "")),
    ) for item in value]


def binding_with_description(bindings: list[Binding], description: str) -> Binding | None:
    return next((binding for binding in bindings if binding.description.strip().lower() == description.strip().lower()), None)


def binding_with_literal_description(bindings: list[Binding], description: str) -> Binding | None:
    phrase = literal_phrase(description)
    candidates = [binding for binding in bindings if literal_phrase(binding.description) == phrase]
    return candidates[0] if len(candidates) == 1 else None


def binding_targets_module(binding: Binding, module: str) -> bool:
    if binding.dispatcher.lower() != "exec" or not module:
        return False
    found_shell = found_action = found_module = False
    for field in binding.argument.split():
        field = field.strip("'\"")
        if field.endswith("omarchy-shell"):
            found_shell = True
        elif field in {"toggle", "summon", "open", "show"}:
            found_action = True
        elif field == module:
            found_module = True
    return found_shell and found_action and found_module


def resolve_click_binding(bindings: list[Binding], module: str, workspace: int, region: str, panel_index: int, module_title: str = "") -> Binding | None:
    if workspace > 0 and "workspace" in module.lower():
        return binding_with_description(bindings, f"Switch to workspace {workspace}")
    for binding in bindings:
        if binding_targets_module(binding, module):
            return binding
    if module_title:
        named = binding_with_literal_description(bindings, module_title)
        if named is not None:
            return named
    if region.lower() == "right" and panel_index > 0:
        return binding_with_description(bindings, f"Bar panel {panel_index}")
    return None


def grouped_click_binding(bindings: list[Binding], workspace: int, binding: Binding) -> Binding:
    if workspace > 0:
        return Binding(
            "Workspace switching",
            ["SUPER + TAB"],
            concept_action="workspace-switching",
            concept_title="Workspace switching",
        )
    if not is_panel_description(binding.description):
        return binding
    hint = binding_with_description(bindings, "Bar panel 1") or binding
    return dataclasses.replace(
        hint,
        description="Bar panels",
        concept_action="bar-panels",
        concept_title="Bar panels",
    )


def install_self(paths: Paths) -> None:
    source = Path(__file__).resolve()
    if paths.local_binary.exists() and source == paths.local_binary.resolve():
        return
    paths.local_binary.parent.mkdir(parents=True, exist_ok=True)
    write_if_changed(paths.local_binary, source.read_bytes(), 0o755)


def install_hypr_integration(paths: Paths, catalog: Catalog | None = None) -> None:
    content = paths.hyprland_config.read_text()
    clean = strip_managed_block(content, HYPR_START, HYPR_END)
    needle = "-- Load Omarchy defaults."
    index = clean.find(needle)
    if index < 0:
        raise ValueError("could not find the Omarchy defaults marker in hyprland.lua")
    before = clean[:index].rstrip()
    after = clean[index:].lstrip()
    block = HYPR_START + '\nrequire("default.hypr.helpers")\nrequire("hypr.sensei")\n' + HYPR_END
    updated = before + "\n\n" + block + "\n\n" + after
    backup_and_write(paths.hyprland_config, updated.encode(), 0o644)
    bindings = unique_catalog_bindings(catalog) if catalog is not None else []
    write_if_changed(paths.sensei_lua, sensei_lua(bindings).encode(), 0o644)


def install_menu_integration(paths: Paths, catalog: Catalog) -> None:
    try:
        content = paths.menu_extension.read_text()
    except FileNotFoundError:
        content = "{\n}\n"
    clean = strip_managed_block(content, MENU_START, MENU_END)
    open_index, close_index = clean.find("{"), clean.rfind("}")
    if open_index < 0 or close_index <= open_index:
        raise ValueError("menu extension is not a JSONC object")
    block = menu_override_block(catalog.matches)
    before = clean[:close_index].rstrip()
    after = clean[close_index:].lstrip()
    active_body = "\n".join(
        line for line in clean[open_index + 1:close_index].splitlines()
        if not line.strip().startswith("//")
    ).strip()
    separator = ""
    if active_body and not active_body.endswith(","):
        separator = "\n  ,"
    updated = before + separator + "\n\n  " + MENU_START + "\n" + block + "  " + MENU_END + "\n" + after
    backup_and_write(paths.menu_extension, updated.encode(), 0o644)


def install_refresh_watcher(paths: Paths) -> None:
    service = """[Unit]
Description=Refresh the Omarchy Sensei coaching catalog

[Service]
Type=oneshot
ExecStartPre=/usr/bin/sleep 1
ExecStart=%h/.local/bin/omarchy-sensei refresh
"""
    path_unit = f"""[Unit]
Description=Watch Omarchy actions and keybindings for Sensei

[Path]
PathChanged={paths.default_menu}
PathChanged=%h/.config/omarchy/extensions/omarchy-menu.jsonc
PathChanged=%h/.config/hypr/bindings.lua
Unit=omarchy-sensei-refresh.service

[Install]
WantedBy=default.target
"""
    hook = """#!/usr/bin/env bash
if ! omarchy-sensei refresh; then
  logger -t omarchy-sensei "Catalog refresh skipped after Omarchy update"
fi
"""
    backup_and_write(paths.refresh_service, service.encode(), 0o644)
    backup_and_write(paths.refresh_path, path_unit.encode(), 0o644)
    backup_and_write(paths.post_update_hook, hook.encode(), 0o755)
    # The watcher is an optimization.  A minimal session (or a shell started
    # before the user bus is ready) can still use Sensei without systemd.
    subprocess.run(["systemctl", "--user", "daemon-reload"], check=False, capture_output=True)
    subprocess.run(["systemctl", "--user", "enable", "--now", "omarchy-sensei-refresh.path"], check=False, capture_output=True)


def setup_integration(paths: Paths) -> Catalog:
    install_self(paths)
    catalog = load_catalog(paths)
    install_hypr_integration(paths, catalog if catalog.matches else None)
    # Quickshell can load the service while Hyprland is still publishing its
    # bindings.  Do not replace a useful catalog with an empty startup result;
    # Service.qml retries ``refresh`` once the compositor is ready.
    if catalog.matches:
        install_menu_integration(paths, catalog)
        install_binding_cache(paths, catalog)
    install_refresh_watcher(paths)
    return catalog


def refresh_integration(paths: Paths) -> Catalog:
    catalog = load_catalog(paths)
    if not catalog.matches:
        raise ValueError("Omarchy bindings are not ready yet")
    install_hypr_integration(paths, catalog)
    install_menu_integration(paths, catalog)
    install_binding_cache(paths, catalog)
    return catalog


def remove_managed_block(path: Path, start: str, end: str) -> None:
    try:
        content = path.read_text()
    except FileNotFoundError:
        return
    updated = strip_managed_block(content, start, end)
    if updated != content:
        backup_and_write(path, updated.encode(), 0o644)


def uninstall_integration(paths: Paths) -> None:
    remove_managed_block(paths.hyprland_config, HYPR_START, HYPR_END)
    remove_managed_block(paths.menu_extension, MENU_START, MENU_END)
    subprocess.run(["systemctl", "--user", "disable", "--now", "omarchy-sensei-refresh.path"], check=False, capture_output=True)
    for path in (paths.refresh_path, paths.refresh_service, paths.post_update_hook, paths.sensei_lua, paths.binding_cache, paths.local_binary):
        path.unlink(missing_ok=True)
    subprocess.run(["systemctl", "--user", "daemon-reload"], check=False)


def integration_installed(paths: Paths) -> bool:
    try:
        return HYPR_START in paths.hyprland_config.read_text()
    except FileNotFoundError:
        return False


def resolve_current_shortcuts(title: str, fallback: str | list[str]) -> list[str]:
    fallbacks = [fallback] if isinstance(fallback, str) else fallback
    try:
        output = subprocess.run(["omarchy-menu-keybindings", "--print"], capture_output=True, text=True, check=True).stdout
    except (OSError, subprocess.CalledProcessError):
        return merge_shortcuts([], *fallbacks)
    shortcuts = []
    for line in output.splitlines():
        if "→" not in line:
            continue
        key, action = (part.strip() for part in line.split("→", 1))
        if action.lower() == title.lower():
            shortcuts = merge_shortcuts(shortcuts, key)
    return merge_shortcuts(shortcuts, *fallbacks)


def command_complete(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--action", required=True)
    parser.add_argument("--title", required=True)
    value = parser.parse_args(args)
    update_coaching_state(paths, Observation(now_utc(), value.action.strip(), value.title.strip(), "shortcut"))


def command_run(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--action", required=True)
    parser.add_argument("--title", required=True)
    parser.add_argument("--shortcut", action="append", required=True)
    value, command = parser.parse_known_args(args)
    if command[:1] == ["--"]:
        command = command[1:]
    if len(command) != 1:
        raise ValueError("run requires one command after --")
    shortcuts = resolve_current_shortcuts(
        value.title.strip(),
        [shortcut.strip() for shortcut in value.shortcut if shortcut.strip()],
    )
    update_coaching_state(paths, Observation(now_utc(), value.action.strip(), value.title.strip(), "menu", shortcuts[0], shortcuts))
    completed = subprocess.run(["bash", "-lc", command[0]], stdin=sys.stdin, stdout=sys.stdout, stderr=sys.stderr)
    raise SystemExit(completed.returncode)


def command_coach_click(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--module", required=True)
    parser.add_argument("--module-title", default="")
    parser.add_argument("--workspace", type=int, default=0)
    parser.add_argument("--region", default="")
    parser.add_argument("--panel-index", type=int, default=0)
    value = parser.parse_args(args)
    try:
        bindings = load_binding_cache(paths.binding_cache)
    except (OSError, json.JSONDecodeError):
        return
    binding = resolve_click_binding(
        bindings,
        value.module.strip(),
        value.workspace,
        value.region.strip(),
        value.panel_index,
        value.module_title.strip(),
    )
    if binding is None or not binding.shortcuts:
        return
    binding = grouped_click_binding(bindings, value.workspace, binding)
    update_coaching_state(paths, Observation(
        now_utc(),
        binding_action(binding),
        binding_title(binding),
        "mouse",
        binding.shortcuts[0],
        binding.shortcuts,
    ))


def command_coach_route(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--route", required=True)
    value = parser.parse_args(args)
    try:
        bindings = load_binding_cache(paths.binding_cache)
        binding = resolve_menu_route_binding(paths, bindings, value.route.strip())
    except (OSError, json.JSONDecodeError, ValueError):
        return
    if binding is None or not binding.shortcuts:
        return
    update_coaching_state(paths, Observation(
        now_utc(),
        binding_action(binding),
        binding_title(binding),
        "menu",
        binding.shortcuts[0],
        binding.shortcuts,
    ))


def command_coach_app(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--name", required=True)
    parser.add_argument("--desktop-id", default="")
    value = parser.parse_args(args)
    try:
        bindings = load_binding_cache(paths.binding_cache)
    except (OSError, json.JSONDecodeError):
        return
    binding, _ = resolve_app_binding(bindings, value.name.strip(), value.desktop_id.strip())
    if binding is None or not binding.shortcuts:
        return
    update_coaching_state(paths, Observation(
        now_utc(),
        binding_action(binding),
        binding_title(binding),
        "menu",
        binding.shortcuts[0],
        binding.shortcuts,
    ))


def normalized_window_address(value: str) -> str:
    return value.strip().casefold().removeprefix("0x").lstrip("0") or "0"


def client_center(client: dict[str, Any]) -> tuple[float, float]:
    at = client.get("at", [0, 0])
    size = client.get("size", [0, 0])
    try:
        return float(at[0]) + float(size[0]) / 2, float(at[1]) + float(size[1]) / 2
    except (IndexError, TypeError, ValueError):
        return 0, 0


def resolve_focus_binding(bindings: list[Binding], before: str, after: str, clients: list[dict[str, Any]]) -> Binding | None:
    by_address = {
        normalized_window_address(str(client.get("address", ""))): client
        for client in clients
        if client.get("address")
    }
    previous = by_address.get(normalized_window_address(before))
    current = by_address.get(normalized_window_address(after))
    description = "Focus on next window"
    if previous is not None and current is not None:
        previous_monitor = previous.get("monitor")
        current_monitor = current.get("monitor")
        if previous_monitor != current_monitor:
            try:
                description = "Focus on next monitor" if int(current_monitor) > int(previous_monitor) else "Focus on previous monitor"
            except (TypeError, ValueError):
                description = "Focus on next monitor"
        else:
            previous_x, previous_y = client_center(previous)
            current_x, current_y = client_center(current)
            dx, dy = current_x - previous_x, current_y - previous_y
            if abs(dx) >= abs(dy) and abs(dx) >= 8:
                description = "Focus on right window" if dx > 0 else "Focus on left window"
            elif abs(dy) >= 8:
                description = "Focus on below window" if dy > 0 else "Focus on above window"
    return binding_with_description(bindings, description)


def command_coach_focus(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--from", dest="before", required=True)
    parser.add_argument("--to", dest="after", required=True)
    value = parser.parse_args(args)
    if normalized_window_address(value.before) == normalized_window_address(value.after):
        return
    try:
        bindings = load_binding_cache(paths.binding_cache)
    except (OSError, json.JSONDecodeError):
        return
    clients: list[dict[str, Any]] = []
    try:
        process = subprocess.run(["hyprctl", "-j", "clients"], capture_output=True, text=True, timeout=2)
        decoded = json.loads(process.stdout) if process.returncode == 0 else []
        if isinstance(decoded, list):
            clients = decoded
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        pass
    binding = resolve_focus_binding(bindings, value.before, value.after, clients)
    if binding is None or not binding.shortcuts:
        return
    update_coaching_state(paths, Observation(
        now_utc(),
        binding_action(binding),
        "Window focus",
        "mouse",
        binding.shortcuts[0],
        binding.shortcuts,
    ))


def active_bar_modules(paths: Paths) -> list[str]:
    config = paths.home / ".config" / "omarchy" / "shell.json"
    try:
        value = json.loads(config.read_text())
    except (OSError, json.JSONDecodeError):
        return []
    layout = value.get("bar", {}).get("layout", {})
    result: list[str] = []
    for region in ("left", "center", "right"):
        for entry in layout.get(region, []):
            if isinstance(entry, dict) and entry.get("id"):
                result.append(str(entry["id"]))
    return result


def installed_plugin_titles(paths: Paths) -> dict[str, str]:
    roots = [paths.home / ".config" / "omarchy" / "plugins", Path("/usr/share/omarchy/shell/plugins")]
    result: dict[str, str] = {}
    for root in roots:
        try:
            manifests = root.rglob("manifest.json")
        except OSError:
            continue
        for manifest in manifests:
            try:
                value = json.loads(manifest.read_text())
            except (OSError, json.JSONDecodeError):
                continue
            plugin_id = str(value.get("id", ""))
            bar = value.get("barWidget", {}) if isinstance(value.get("barWidget", {}), dict) else {}
            title = str(bar.get("displayName") or value.get("name") or "")
            if plugin_id and title and plugin_id not in result:
                result[plugin_id] = title
    return result


def app_observed_actions(bindings: list[Binding]) -> dict[str, str]:
    defaults = default_desktop_roles()
    actions: dict[str, str] = {}
    for entry in load_desktop_entries():
        candidates: list[tuple[int, str, Binding]] = []
        for binding in bindings:
            score, evidence = app_binding_score(entry, binding, defaults)
            if score:
                candidates.append((score, evidence, binding))
        if not candidates:
            continue
        best = max(score for score, _, _ in candidates)
        winners: dict[str, tuple[Binding, str]] = {}
        for score, evidence, binding in candidates:
            if score == best:
                winners[binding_action(binding)] = (binding, evidence)
        if len(winners) == 1:
            binding, evidence = next(iter(winners.values()))
            actions[binding_action(binding)] = f"Apps provider: {entry.name!r}, {evidence}"
    return actions


def binding_coverage(paths: Paths, catalog: Catalog) -> dict[str, Any]:
    bindings = unique_catalog_bindings(catalog)
    matched = {normalized_phrase(match.binding.description): match for match in catalog.matches}
    modules = active_bar_modules(paths)
    titles = installed_plugin_titles(paths)
    app_actions = app_observed_actions(bindings)
    rows: list[dict[str, Any]] = []
    for binding in bindings:
        key = normalized_phrase(binding.description)
        status = "matchable-unobserved"
        source = ""
        evidence = "structured dispatcher and argument"
        if key in matched:
            status, source = "observed", "menu-action"
            evidence = matched[key].evidence or matched[key].confidence
        elif binding_is_workspace_switch(binding):
            status, source, evidence = "observed", "bar-workspace", "workspace semantic identity"
        elif is_panel_description(binding.description):
            status, source, evidence = "observed", "bar-panel", "live positional panel identity"
        elif binding.description in {
            "Focus on next window",
            "Focus on next monitor",
            "Focus on previous monitor",
            "Focus on below window",
            "Focus on left window",
            "Focus on right window",
            "Focus on above window",
        }:
            status, source, evidence = "observed", "compositor-focus", "non-consuming pointer focus transition"
        else:
            targeted = next((module for module in modules if binding_targets_module(binding, module)), "")
            titled = next((module for module in modules if literal_phrase(titles.get(module, "")) == literal_phrase(binding.description)), "")
            if targeted:
                status, source, evidence = "observed", "bar-module", f"exact shell module {targeted!r}"
            elif titled:
                status, source, evidence = "observed", "bar-manifest", f"exact manifest title for {titled!r}"
            elif binding_menu_route(binding):
                resolved = resolve_menu_route_binding(paths, bindings, binding_menu_route(binding))
                if resolved is not None:
                    status, source, evidence = "observed", "menu-route", f"unique route {binding_menu_route(binding)!r}"
            elif binding_action(binding) in app_actions:
                status, source, evidence = "observed", "apps-provider", app_actions[binding_action(binding)]
            elif not binding.dispatcher or not binding.argument:
                status, evidence = "missing-metadata", "Super+K exposes no dispatcher argument"
            elif binding.dispatcher.casefold() == "sendshortcut":
                status, evidence = "keyboard-only", "synthetic shortcut has no semantic desktop source"
        row = binding.to_json()
        row.update({
            "status": status,
            "source": source,
            "evidence": evidence,
            "conceptAction": binding_action(binding),
            "conceptTitle": binding_title(binding),
        })
        rows.append(row)

    status_counts: dict[str, int] = {}
    for row in rows:
        status_counts[row["status"]] = status_counts.get(row["status"], 0) + 1
    concepts = {row["conceptAction"] for row in rows}
    observed_concepts = {row["conceptAction"] for row in rows if row["status"] == "observed"}
    return {
        "generatedAt": iso_time(now_utc()),
        "bindingSource": catalog.binding_source,
        "bindingGeneratedAt": catalog.binding_generated_at,
        "bindings": len(rows),
        "concepts": len(concepts),
        "observedConcepts": len(observed_concepts),
        "statusCounts": status_counts,
        "rows": rows,
    }


def coverage_summary(coverage: dict[str, Any]) -> str:
    return (
        f"{coverage['bindings']} bindings / {coverage['concepts']} concepts; "
        f"{coverage['observedConcepts']} observed concepts"
    )


def print_catalog(paths: Paths, args: list[str]) -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--unmatched", action="store_true")
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--coverage", action="store_true")
    value = parser.parse_args(args)
    catalog = load_catalog(paths)
    if value.coverage:
        coverage = binding_coverage(paths, catalog)
        if value.json:
            print(json.dumps(coverage, ensure_ascii=False))
        else:
            counts = ", ".join(f"{key}={count}" for key, count in sorted(coverage["statusCounts"].items()))
            print(f"{coverage_summary(coverage)} ({counts})")
            print(f"Binding source: {coverage['bindingSource']}")
        return
    if value.json:
        print(json.dumps(catalog.to_json(), ensure_ascii=False))
        return
    if not value.unmatched:
        for match in catalog.matches:
            reason = match.confidence + (f": {match.evidence}" if match.evidence else "")
            print(f"✓ {match.menu.id:<38} → {match.binding.description:<32} {' / '.join(match.binding.shortcuts)} [{reason}]")
    for item in catalog.unmatched_menu:
        print(f"· {item.id:<38} (no shortcut match for {item.label!r})")
    if value.unmatched:
        for binding in catalog.unmatched_bindings:
            print(f"⌨ {binding.description:<38} (no matching menu action; {' / '.join(binding.shortcuts)})")
    else:
        print(
            f"\n{len(catalog.matches)} menu-leaf matches; "
            f"{len(catalog.unmatched_menu)} menu leaves without a shortcut; "
            f"{len(catalog.unmatched_bindings)} bindings without a menu-leaf match."
        )


def doctor(paths: Paths) -> None:
    catalog = load_catalog(paths)
    if not catalog.matches:
        raise ValueError("catalog has no coached actions")
    if not integration_installed(paths):
        raise ValueError("Hyprland integration is not installed")
    try:
        menu = paths.menu_extension.read_text()
    except OSError:
        menu = ""
    if MENU_START not in menu:
        raise ValueError("generated menu integration is not installed")
    try:
        observer = paths.sensei_lua.read_text()
    except OSError:
        observer = ""
    if "hl.dispatch(dispatcher)" not in observer:
        raise ValueError("generic shortcut observer is not installed")
    if 'hl.on("window.active"' not in observer or "non_consuming = true" not in observer:
        raise ValueError("non-consuming pointer focus observer is not installed")
    try:
        bindings = load_binding_cache(paths.binding_cache)
    except (OSError, json.JSONDecodeError):
        bindings = []
    if not bindings:
        raise ValueError("semantic click binding cache is not installed")
    if any(not binding_action(binding) for binding in bindings):
        raise ValueError("binding cache contains an action without semantic identity")
    watcher = subprocess.run(["systemctl", "--user", "is-active", "omarchy-sensei-refresh.path"], capture_output=True, text=True)
    if watcher.returncode or watcher.stdout.strip() != "active":
        raise ValueError("catalog refresh watcher is not active")
    config = subprocess.run(["hyprctl", "configerrors"], capture_output=True, text=True)
    if config.returncode or config.stdout.strip():
        raise ValueError("Hyprland reports configuration errors")
    coverage = binding_coverage(paths, catalog)
    print(f"Sensei is healthy: {coverage_summary(coverage)}, refresh watcher active.")


def main(argv: list[str] | None = None) -> int:
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv:
        raise ValueError("usage: sensei.py <setup|refresh|catalog|doctor|uninstall|complete|coach-click|coach-route|coach-app|coach-focus|run|snapshot|pause|resume|clear|status>")
    paths = Paths.current()
    command, args = argv[0], argv[1:]
    if command == "setup":
        initialize_state(paths)
        catalog = setup_integration(paths)
        coverage = binding_coverage(paths, catalog)
        print(f"Omarchy Sensei coaching is installed ({coverage_summary(coverage)}). Run `hyprctl reload` to activate it.")
    elif command == "refresh":
        catalog = refresh_integration(paths)
        coverage = binding_coverage(paths, catalog)
        print(f"Sensei catalog refreshed: {coverage_summary(coverage)}.")
    elif command == "catalog":
        print_catalog(paths, args)
    elif command == "doctor":
        doctor(paths)
    elif command == "uninstall":
        uninstall_integration(paths)
        print("Omarchy Sensei coaching was removed. Your progress and open tasks were kept.")
    elif command == "complete":
        command_complete(paths, args)
    elif command == "coach-click":
        command_coach_click(paths, args)
    elif command == "coach-route":
        command_coach_route(paths, args)
    elif command == "coach-app":
        command_coach_app(paths, args)
    elif command == "coach-focus":
        command_coach_focus(paths, args)
    elif command == "run":
        command_run(paths, args)
    elif command == "snapshot":
        state = StateStore(paths).read_modify(now_utc())
        print(json.dumps(snapshot_from_state(state, is_paused(paths)), ensure_ascii=False))
    elif command == "pause":
        paths.state_dir.mkdir(parents=True, exist_ok=True)
        paths.paused.write_text("paused\n")
        os.chmod(paths.paused, 0o600)
    elif command == "resume":
        paths.paused.unlink(missing_ok=True)
    elif command == "clear":
        StateStore(paths).clear()
    elif command == "status":
        state = StateStore(paths).read_modify(now_utc())
        print(json.dumps({"paused": is_paused(paths), "totalShortcuts": state.total_shortcuts, "openTasks": len(state.tasks), "installed": integration_installed(paths)}))
    else:
        raise ValueError(f"unknown command: {command}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"omarchy-sensei: {error}", file=sys.stderr)
        raise SystemExit(1)
