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


if __name__ == "__main__":
    unittest.main()
