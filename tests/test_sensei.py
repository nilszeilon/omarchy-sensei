import datetime as dt
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import sensei  # noqa: E402


class SenseiTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        state_dir = root / "state"
        self.paths = sensei.Paths(
            home=root,
            state_dir=state_dir,
            state=state_dir / "state.json",
            state_lock=state_dir / "state.lock",
            legacy_events=state_dir / "events.jsonl",
            binding_cache=state_dir / "bindings.json",
            paused=state_dir / "paused",
            hyprland_config=root / "hyprland.lua",
            sensei_lua=root / "sensei.lua",
            menu_extension=root / "menu.jsonc",
            default_menu=root / "default-menu.jsonc",
            local_binary=root / "bin" / "omarchy-sensei",
            refresh_service=root / "refresh.service",
            refresh_path=root / "refresh.path",
            post_update_hook=root / "hook",
        )

    def tearDown(self):
        self.temp.cleanup()

    def observe(self, action, title, trigger, at, shortcut="", shortcuts=None):
        sensei.update_coaching_state(
            self.paths,
            sensei.Observation(at, action, title, trigger, shortcut, shortcuts or []),
        )

    def test_mouse_task_and_shortcut_completion(self):
        at = dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc)
        self.observe("open-menu", "Open Menu", "mouse", at, "SUPER + SPACE", ["SUPER + SPACE"])
        state = sensei.StateStore(self.paths).read_modify(at)
        self.assertEqual(len(state.tasks), 1)
        self.assertEqual(state.tasks[0].slow_uses, 1)

        self.observe("open-menu", "Open Menu", "shortcut", at + dt.timedelta(seconds=1))
        state = sensei.StateStore(self.paths).read_modify(at + dt.timedelta(seconds=1))
        self.assertEqual(state.tasks, [])
        self.assertEqual(state.total_shortcuts, 1)

    def test_duplicate_shortcut_is_not_counted(self):
        at = dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc)
        self.observe("open-menu", "Open Menu", "shortcut", at)
        self.observe("open-menu", "Open Menu", "shortcut", at + dt.timedelta(milliseconds=50))
        state = sensei.StateStore(self.paths).read_modify(at + dt.timedelta(milliseconds=50))
        self.assertEqual(state.total_shortcuts, 1)

    def test_workspace_and_panel_grouping(self):
        at = dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc)
        self.observe("switch-to-workspace-3", "Switch to workspace 3", "mouse", at, "SUPER + 3")
        self.observe("switch-to-workspace-5", "Switch to workspace 5", "mouse", at + dt.timedelta(seconds=1), "SUPER + 5")
        self.observe("bar-panel-2", "Bar panel 2", "mouse", at + dt.timedelta(seconds=2), "SUPER CTRL + 2")
        state = sensei.StateStore(self.paths).read_modify(at + dt.timedelta(seconds=2))
        self.assertEqual({task.action for task in state.tasks}, {"workspace-switching", "bar-panels"})
        workspace = next(task for task in state.tasks if task.action == "workspace-switching")
        self.assertEqual(workspace.shortcuts, ["SUPER + TAB"])

    def test_level_progression(self):
        self.assertEqual(sensei.level_progress(0)["level"], 1)
        self.assertEqual(sensei.level_progress(9)["level"], 1)
        self.assertEqual(sensei.level_progress(10)["level"], 2)
        self.assertEqual(sensei.level_progress(25)["level"], 3)

    def test_legacy_history_migrates_to_compact_state(self):
        self.paths.state_dir.mkdir(parents=True)
        events = [
            {"occurredAt": "2026-01-01T00:00:00Z", "action": "open-menu", "title": "Open Menu", "trigger": "mouse", "shortcut": "SUPER + SPACE"},
            {"occurredAt": "2026-01-01T00:00:01Z", "action": "open-menu", "title": "Open Menu", "trigger": "shortcut"},
        ]
        self.paths.legacy_events.write_text("".join(json.dumps(event) + "\n" for event in events))
        state = sensei.StateStore(self.paths).read_modify(dt.datetime(2026, 1, 1, 0, 0, 2, tzinfo=dt.timezone.utc))
        self.assertEqual(state.total_shortcuts, 1)
        self.assertFalse(self.paths.legacy_events.exists())
        self.assertTrue(self.paths.state.exists())

    def test_empty_startup_refresh_preserves_last_binding_catalog(self):
        self.paths.state_dir.mkdir(parents=True)
        previous = '[{"description":"Terminal","shortcuts":["SUPER + RETURN"]}]\n'
        self.paths.binding_cache.write_text(previous)

        empty = sensei.Catalog([], [], [])
        with mock.patch.object(sensei, "load_catalog", return_value=empty):
            with self.assertRaisesRegex(ValueError, "bindings are not ready"):
                sensei.refresh_integration(self.paths)

        self.assertEqual(self.paths.binding_cache.read_text(), previous)


if __name__ == "__main__":
    unittest.main()
