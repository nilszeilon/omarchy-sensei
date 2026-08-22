import datetime as dt
import contextlib
import io
import json
import os
import stat
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

    @staticmethod
    def menu_item(item_id, label, action, aliases=None):
        parent = item_id.rsplit(".", 1)[0] if "." in item_id else "root"
        return sensei.MenuItem(
            item_id, parent, "", "", label, "", "", action, aliases or [], "", ""
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

    def test_backup_preserves_private_file_mode_with_permissive_umask(self):
        target = Path(self.temp.name) / "private.conf"
        target.write_text("secret command\n")
        target.chmod(0o600)

        previous_umask = os.umask(0o000)
        try:
            sensei.backup_and_write(target, b"replacement\n", 0o644)
        finally:
            os.umask(previous_umask)

        backups = list(target.parent.glob("private.conf.sensei-backup-*"))
        self.assertEqual(len(backups), 1)
        self.assertEqual(backups[0].read_text(), "secret command\n")
        self.assertEqual(stat.S_IMODE(backups[0].stat().st_mode), 0o600)
        self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)

    def test_managed_integration_writes_are_idempotent(self):
        self.paths.hyprland_config.write_text(
            "-- User configuration\n\n\n-- Load Omarchy defaults.\nrequire(\"default.hypr.omarchy\")\n"
        )
        self.paths.menu_extension.write_text("{\n  // User menu entries go here.\n\n\n}\n")
        menu = sensei.MenuItem(
            "trigger.audit", "trigger", "", "", "Audit", "", "", "audit-command", [], "", ""
        )
        catalog = sensei.Catalog(
            [sensei.CatalogMatch(menu, sensei.Binding("Audit", ["SUPER + A", "SUPER ALT + A"]), "command-exact")],
            [],
            [],
        )

        reordered = sensei.Catalog(
            [sensei.CatalogMatch(menu, sensei.Binding("Audit", ["SUPER ALT + A", "SUPER + A"]), "command-exact")],
            [],
            [],
        )
        sensei.install_hypr_integration(self.paths)
        sensei.install_menu_integration(self.paths, reordered)
        first_hyprland = self.paths.hyprland_config.read_bytes()
        first_menu = self.paths.menu_extension.read_bytes()
        first_observer_inode = self.paths.sensei_lua.stat().st_ino

        sensei.install_hypr_integration(self.paths)
        sensei.install_menu_integration(self.paths, catalog)

        self.assertEqual(self.paths.hyprland_config.read_bytes(), first_hyprland)
        self.assertEqual(self.paths.menu_extension.read_bytes(), first_menu)
        self.assertNotIn("\n\n\n", self.paths.hyprland_config.read_text())
        self.assertNotIn("\n\n\n", self.paths.menu_extension.read_text())
        self.assertNotIn("\n  ,\n", self.paths.menu_extension.read_text())
        self.assertEqual(self.paths.sensei_lua.stat().st_ino, first_observer_inode)

    def test_menu_routes_match_theme_and_background_without_action_specific_rules(self):
        menu = [
            self.menu_item(
                "style.theme",
                "Theme",
                'theme=$(omarchy-theme-switcher); [[ -n $theme ]] && omarchy-theme-set "$theme"',
                ["theme", "themes"],
            ),
            self.menu_item(
                "style.background",
                "Background",
                'background=$(omarchy-theme-bg-switcher); [[ -n $background ]] && omarchy-theme-bg-set "$background"',
                ["background", "wallpaper"],
            ),
            self.menu_item(
                "install.style.theme",
                "Theme",
                "omarchy-launch-floating-terminal-with-presentation omarchy-theme-install",
            ),
            self.menu_item("remove.theme", "Theme", "omarchy-theme-remove"),
            self.menu_item("update.themes", "Extra Themes", "omarchy-theme-update"),
        ]
        bindings = [
            sensei.Binding(
                "Theme menu",
                ["SUPER SHIFT CTRL + SPACE"],
                "exec",
                "omarchy-menu toggle theme",
            ),
            sensei.Binding(
                "Background switcher",
                ["SUPER CTRL + SPACE"],
                "exec",
                "omarchy-menu toggle background",
            ),
        ]

        matches, unmatched = sensei.match_catalog(menu, bindings)

        by_id = {match.menu.id: match for match in matches}
        self.assertEqual(set(by_id), {"style.theme", "style.background"})
        self.assertEqual(by_id["style.theme"].binding.description, "Theme menu")
        self.assertEqual(by_id["style.theme"].confidence, "route-alias")
        self.assertEqual(by_id["style.theme"].evidence, "unique menu route 'theme'")
        self.assertEqual(by_id["style.background"].binding.description, "Background switcher")
        self.assertEqual(
            {item.id for item in unmatched},
            {"install.style.theme", "remove.theme", "update.themes"},
        )

    def test_menu_route_supports_public_omarchy_command_and_direct_item_id(self):
        item = self.menu_item("style.theme", "Theme", "choose-theme", ["theme"])
        binding = sensei.Binding(
            "Theme menu",
            ["SUPER SHIFT CTRL + SPACE"],
            "exec",
            "omarchy menu summon style.theme",
        )

        matches, unmatched = sensei.match_catalog([item], [binding])

        self.assertEqual(unmatched, [])
        self.assertEqual(matches[0].confidence, "route-id")
        self.assertEqual(matches[0].evidence, "unique menu route 'style.theme'")

    def test_duplicate_menu_alias_is_ambiguous_and_not_coached(self):
        menu = [
            self.menu_item("style.theme", "Theme", "choose-theme", ["theme"]),
            self.menu_item("personal.theme", "Personal Theme", "choose-personal-theme", ["theme"]),
        ]
        binding = sensei.Binding(
            "Theme menu",
            ["SUPER SHIFT CTRL + SPACE"],
            "exec",
            "omarchy-menu toggle theme",
        )

        matches, unmatched = sensei.match_catalog(menu, [binding])

        self.assertEqual(matches, [])
        self.assertEqual({item.id for item in unmatched}, {"style.theme", "personal.theme"})

    def test_unique_semantic_match_is_not_limited_to_builtin_namespaces(self):
        item = self.menu_item("personal.audit", "Audit", "personal-audit-command")
        binding = sensei.Binding("Audit", ["SUPER + A"], "lua", "audit")

        matches, unmatched = sensei.match_catalog([item], [binding])

        self.assertEqual(unmatched, [])
        self.assertEqual(matches[0].confidence, "phrase-exact")
        self.assertEqual(matches[0].evidence, "unique phrase 'audit'")

    def test_duplicate_semantic_identity_is_ambiguous_and_not_coached(self):
        menu = [
            self.menu_item("personal.audit", "Audit", "personal-audit-command"),
            self.menu_item("system.audit", "Audit", "system-audit-command"),
        ]
        binding = sensei.Binding("Audit", ["SUPER + A"], "lua", "audit")

        matches, unmatched = sensei.match_catalog(menu, [binding])

        self.assertEqual(matches, [])
        self.assertEqual({item.id for item in unmatched}, {"personal.audit", "system.audit"})

    def test_semantic_match_rejects_different_operations(self):
        menu = [
            self.menu_item(
                "install.service.signal",
                "Signal",
                "omarchy-launch-floating-terminal-with-presentation omarchy-install-signal",
            ),
            self.menu_item("update.hardware.audio", "Audio", "omarchy-update-audio"),
        ]
        bindings = [
            sensei.Binding("Signal", ["SUPER + S"], "exec", "signal-desktop"),
            sensei.Binding("Audio", ["SUPER CTRL + A"], "exec", "omarchy-menu toggle audio"),
            sensei.Binding("Notes", ["SUPER + N"], "exec", "omarchy-install-notes"),
        ]
        menu.append(self.menu_item("apps.notes", "Notes", "omarchy-launch-notes"))

        matches, unmatched = sensei.match_catalog(menu, bindings)

        self.assertEqual(matches, [])
        self.assertEqual(
            {item.id for item in unmatched},
            {"apps.notes", "install.service.signal", "update.hardware.audio"},
        )

    def test_route_match_preserves_remapped_shortcut_alternatives_in_wrapper(self):
        item = self.menu_item("style.theme", "Theme", "choose-theme", ["theme"])
        records = "\n".join(
            [
                "SUPER SHIFT CTRL + SPACE → Theme menu\texec\tomarchy-menu toggle theme",
                "SUPER + T → Theme menu\texec\tomarchy-menu toggle theme",
            ]
        )
        bindings = sensei.bindings_from_records(records)
        matches, unmatched = sensei.match_catalog([item], bindings)

        self.assertEqual(unmatched, [])
        self.assertEqual(
            matches[0].binding.shortcuts,
            ["SUPER + T", "SUPER SHIFT CTRL + SPACE"],
        )
        generated = sensei.menu_override_block(matches)
        self.assertIn("omarchy-sensei run", generated)
        self.assertIn("--title 'Theme menu'", generated)
        self.assertIn("--shortcut 'SUPER + T'", generated)
        self.assertIn("--shortcut 'SUPER SHIFT CTRL + SPACE'", generated)
        self.assertIn("'choose-theme'", generated)

    def test_menu_wrapper_uses_shared_concept_identity(self):
        item = self.menu_item("system.power", "Power", "poweroff")
        binding = sensei.annotate_binding_concepts([
            sensei.Binding("System menu", ["SUPER + ESCAPE"], "exec", "omarchy-menu toggle system"),
            sensei.Binding("Power menu", ["XF86PowerOff"], "exec", "omarchy-menu toggle system"),
        ])[1]

        generated = sensei.menu_override_block([
            sensei.CatalogMatch(item, binding, "command-exact")
        ])

        self.assertIn("--action 'binding-", generated)
        self.assertIn("--title 'System menu'", generated)
        self.assertIn("--shortcut 'XF86PowerOff'", generated)
        self.assertIn("--shortcut 'SUPER + ESCAPE'", generated)

    def test_route_created_task_is_cleared_by_matching_shortcut_identity(self):
        at = dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc)
        action = sensei.action_id("Theme menu")
        self.observe(
            action,
            "Theme menu",
            "menu",
            at,
            "SUPER SHIFT CTRL + SPACE",
            ["SUPER SHIFT CTRL + SPACE"],
        )
        state = sensei.StateStore(self.paths).read_modify(at)
        self.assertEqual([task.action for task in state.tasks], ["theme-menu"])

        self.observe(action, "Theme menu", "shortcut", at + dt.timedelta(seconds=2))
        state = sensei.StateStore(self.paths).read_modify(at + dt.timedelta(seconds=2))
        self.assertEqual(state.tasks, [])
        self.assertEqual(state.total_shortcuts, 1)

    def test_identical_commands_share_one_concept_and_all_shortcuts(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("System menu", ["SUPER + ESCAPE"], "exec", "omarchy-menu toggle system"),
            sensei.Binding("Power menu", ["XF86PowerOff"], "exec", "omarchy-menu toggle system"),
        ])

        self.assertEqual(bindings[0].concept_action, bindings[1].concept_action)
        self.assertEqual(bindings[0].concept_title, "System menu")
        self.assertEqual(bindings[0].shortcuts, ["XF86PowerOff", "SUPER + ESCAPE"])
        generated = sensei.sensei_lua(bindings)
        self.assertIn('["System menu"]', generated)
        self.assertIn('["Power menu"]', generated)
        self.assertIn(bindings[0].concept_action, generated)

    def test_workspace_dispatcher_variants_share_the_workspace_concept(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("Switch to workspace 1", ["SUPER + 1"], "lua", 'hl.dsp.focus({ workspace = "1" })'),
            sensei.Binding("Scroll active workspace forward", ["SUPER + mouse_down"], "lua", 'hl.dsp.focus({ workspace = "e+1" })'),
        ])

        self.assertEqual({binding.concept_action for binding in bindings}, {"workspace-switching"})
        self.assertEqual(
            bindings[0].shortcuts,
            ["SUPER + 1", "SUPER + mouse_down"],
        )

    def test_bar_resolution_prefers_named_identity_before_position(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("Bluetooth", ["SUPER CTRL + B"], "exec", "omarchy-shell shell toggle omarchy.bluetooth"),
            sensei.Binding("Bar panel 1", ["SUPER CTRL + 1"], "exec", "omarchy-shell shell toggle-panel-at right 1"),
            sensei.Binding("Personal status", ["SUPER CTRL + S"], "exec", "personal-status"),
        ])

        named = sensei.resolve_click_binding(
            bindings, "omarchy.bluetooth", 0, "right", 1, "Bluetooth"
        )
        manifest_named = sensei.resolve_click_binding(
            bindings, "io.example.status", 0, "right", 1, "Personal status"
        )
        positional = sensei.resolve_click_binding(
            bindings, "io.example.unknown", 0, "right", 1, "Unknown"
        )

        self.assertEqual(named.description, "Bluetooth")
        self.assertEqual(manifest_named.description, "Personal status")
        self.assertEqual(positional.description, "Bar panel 1")

    def test_workspace_click_resolves_to_shared_task_and_hint(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("Switch to workspace 4", ["SUPER + 4"], "lua", 'hl.dsp.focus({ workspace = "4" })'),
            sensei.Binding("Former workspace", ["SUPER + TAB"], "lua", 'hl.dsp.focus({ workspace = "previous" })'),
        ])

        clicked = sensei.resolve_click_binding(
            bindings, "omarchy.workspaces", 4, "left", 0
        )
        grouped = sensei.grouped_click_binding(bindings, 4, clicked)

        self.assertEqual(grouped.concept_action, "workspace-switching")
        self.assertEqual(grouped.concept_title, "Workspace switching")
        self.assertEqual(grouped.shortcuts, ["SUPER + TAB"])

    def test_parent_menu_route_resolves_alias_and_duplicate_command_variants(self):
        self.paths.default_menu.write_text(
            '{"system":{"label":"System","aliases":["power-menu"]},'
            '"system.lock":{"label":"Lock","action":"lock"}}'
        )
        self.paths.menu_extension.write_text("{}")
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("System menu", ["SUPER + ESCAPE"], "exec", "omarchy-menu toggle system"),
            sensei.Binding("Power menu", ["XF86PowerOff"], "exec", "omarchy menu summon power-menu"),
        ], sensei.load_merged_menu(self.paths, actions_only=False))

        resolved = sensei.resolve_menu_route_binding(self.paths, bindings, "system")

        self.assertIsNotNone(resolved)
        self.assertEqual(resolved.concept_action, bindings[0].concept_action)
        self.assertEqual(resolved.shortcuts, ["XF86PowerOff", "SUPER + ESCAPE"])

    def test_app_resolution_uses_roles_and_rejects_private_or_cwd_variants(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("Browser", ["SUPER + B"], "exec", "omarchy-launch-browser"),
            sensei.Binding("Browser (private)", ["SUPER ALT + B"], "exec", "omarchy-launch-browser --private"),
            sensei.Binding("File manager", ["SUPER + F"], "exec", "omarchy-launch-nautilus"),
            sensei.Binding("File manager (cwd)", ["SUPER ALT + F"], "exec", "omarchy-launch-nautilus-cwd"),
        ])
        entries = [
            sensei.DesktopEntry("chromium", "Chromium", "Web Browser", [], "/usr/bin/chromium %U"),
            sensei.DesktopEntry("org.gnome.Nautilus", "Files", "File Manager", [], "nautilus --new-window %U"),
        ]
        with mock.patch.object(sensei, "load_desktop_entries", return_value=entries), \
             mock.patch.object(sensei, "default_desktop_roles", return_value={"browser": "chromium"}):
            browser, browser_evidence = sensei.resolve_app_binding(bindings, "Chromium")
            files, files_evidence = sensei.resolve_app_binding(bindings, "Files")

        self.assertEqual(browser.description, "Browser")
        self.assertEqual(browser_evidence, "current default browser")
        self.assertEqual(files.description, "File manager")
        self.assertIn("nautilus", files_evidence)

    def test_focus_click_resolves_direction_from_live_window_geometry(self):
        bindings = sensei.annotate_binding_concepts([
            sensei.Binding("Focus on right window", ["SUPER + RIGHT"], "lua", 'hl.dsp.focus({ direction = "r" })'),
            sensei.Binding("Focus on below window", ["SUPER + DOWN"], "lua", 'hl.dsp.focus({ direction = "d" })'),
            sensei.Binding("Focus on next window", ["ALT + TAB"], "lua", "hl.dsp.window.cycle_next()"),
        ])
        clients = [
            {"address": "0xaaa", "monitor": 0, "at": [0, 0], "size": [500, 500]},
            {"address": "0xbbb", "monitor": 0, "at": [700, 40], "size": [500, 500]},
        ]

        resolved = sensei.resolve_focus_binding(bindings, "0xaaa", "0xbbb", clients)

        self.assertEqual(resolved.description, "Focus on right window")

    def test_stale_binding_snapshot_is_used_when_live_compositor_is_unavailable(self):
        cache = Path(self.temp.name) / ".cache" / "omarchy"
        cache.mkdir(parents=True)
        cached = cache / "keybindings-good.records"
        cached.write_text("\n".join([
            "SUPER + A → Alpha\texec\talpha",
            "SUPER + B → Beta\texec\tbeta",
            "SUPER + C → Gamma\texec\tgamma",
        ]))
        with mock.patch.object(Path, "home", return_value=Path(self.temp.name)), \
             mock.patch.dict(os.environ, {"XDG_CACHE_HOME": str(Path(self.temp.name) / ".cache")}, clear=False):
            snapshot = sensei.latest_cached_binding_records()

        self.assertIsNotNone(snapshot)
        self.assertEqual(sensei.binding_record_count(snapshot.data), 3)
        self.assertTrue(snapshot.source.startswith("cache:"))

    def test_full_live_binding_snapshot_remains_completely_classified(self):
        fixture = Path(__file__).parent / "fixtures" / "live-keybindings.records"
        records = fixture.read_text()

        bindings = sensei.bindings_from_records(records)
        annotated = sensei.annotate_binding_concepts(bindings)

        self.assertEqual(len(records.splitlines()), 229)
        self.assertEqual(len(bindings), 218)
        self.assertEqual(len({binding.concept_action for binding in annotated}), 191)
        self.assertTrue(all(binding.concept_action for binding in annotated))
        self.assertEqual(
            {binding.description for binding in bindings if not binding.dispatcher or not binding.argument},
            {"Universal copy", "Universal paste", "Universal cut", "Zoom in", "Reset zoom"},
        )

    def test_menu_integration_preserves_existing_entry_without_trailing_comma(self):
        self.paths.menu_extension.write_text(
            '{\n  "personal.action": {"label": "Personal", "action": "personal-command"}\n}\n'
        )
        menu = sensei.MenuItem(
            "trigger.audit", "trigger", "", "", "Audit", "", "", "audit-command", [], "", ""
        )
        catalog = sensei.Catalog(
            [sensei.CatalogMatch(menu, sensei.Binding("Audit", ["SUPER + A"]), "command-exact")],
            [],
            [],
        )

        sensei.install_menu_integration(self.paths, catalog)

        items = sensei.parse_menu_jsonc(self.paths.menu_extension.read_text())
        self.assertEqual([item.id for item in items], ["personal.action", "trigger.audit"])

    def test_pause_resume_clear_and_diagnostic_snapshot(self):
        at = dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc)
        self.observe("open-menu", "Open Menu", "mouse", at, "SUPER + SPACE")

        with mock.patch.object(sensei.Paths, "current", return_value=self.paths):
            sensei.main(["pause"])
            self.assertTrue(self.paths.paused.exists())

            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                sensei.main(["snapshot"])
            snapshot = json.loads(output.getvalue())
            self.assertTrue(snapshot["paused"])
            self.assertEqual(snapshot["tasks"][0]["action"], "open-menu")

            sensei.main(["resume"])
            self.assertFalse(self.paths.paused.exists())
            sensei.main(["clear"])
            self.assertFalse(self.paths.state.exists())


if __name__ == "__main__":
    unittest.main()
