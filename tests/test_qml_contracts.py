import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
import sensei  # noqa: E402


class QmlContractTests(unittest.TestCase):
    def test_panel_model_is_file_driven_without_polling_processes(self):
        model = (ROOT / "SenseiModel.qml").read_text()

        self.assertIn("FileView {", model)
        self.assertIn("watchChanges: true", model)
        self.assertNotIn("Timer {", model)
        self.assertNotIn("Process {", model)
        self.assertNotIn("omarchy-sensei\", \"snapshot", model)

    def test_shortcut_action_dispatches_before_async_coaching(self):
        observer = sensei.sensei_lua()
        dispatch = observer.index("hl.dispatch(dispatcher)")
        coaching = observer.index("hl.exec_cmd(command)", dispatch)

        self.assertLess(dispatch, coaching)

    def test_mouse_focus_observer_is_non_consuming_and_mouse_only(self):
        observer = sensei.sensei_lua()

        self.assertEqual(observer.count('hl.bind("mouse:272"'), 2)
        self.assertIn("non_consuming = true", observer)
        self.assertIn('hl.on("window.active"', observer)
        self.assertIn("pointer_down", observer)
        self.assertLess(observer.index("pointer_down = { serial = serial }"), observer.index('hl.on("window.active"'))
        self.assertNotIn('hl.bind("catch_all"', observer)

    def test_service_observes_menu_routes_and_apps_without_polling(self):
        service = (ROOT / "Service.qml").read_text()
        bar = (ROOT / "BarWidget.qml").read_text()

        self.assertIn("onActiveMenuChanged", service)
        self.assertIn("onLaunchSerialChanged", service)
        self.assertEqual(service.count("ignoreUnknownSignals: true"), 2)
        self.assertIn('"coach-route"', service)
        self.assertIn('"coach-app"', service)
        self.assertIn("metadataFor", bar)
        self.assertIn("--module-title", bar)


if __name__ == "__main__":
    unittest.main()
