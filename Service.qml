import QtQuick
import Quickshell.Io

// A plugin service is loaded whenever Sensei's bar widget is enabled.  It
// bootstraps the user integration from the cloned checkout, so marketplace
// installation does not require a compiler or a second terminal command.
Item {
  id: root

  property var manifest: null
  property bool ready: false
  property string error: ""
  property string stderrText: ""

  readonly property string sourceDir: manifest && manifest.__sourceDir
    ? String(manifest.__sourceDir)
    : ""
  readonly property string helperPath: sourceDir ? sourceDir + "/sensei.py" : ""

  function setup() {
    if (!root.helperPath || setupProcess.running) return
    root.ready = false
    root.error = ""
    root.stderrText = ""
    setupProcess.command = ["python3", root.helperPath, "setup"]
    setupProcess.running = true
  }

  Component.onCompleted: Qt.callLater(root.setup)

  Process {
    id: setupProcess
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.stderrText = String(text || "")
    }

    onExited: function(exitCode) {
      if (exitCode !== 0) {
        root.error = root.stderrText.trim() || "Sensei could not install its local coaching integration."
        return
      }
      root.ready = true
      reloadProcess.running = true
    }
  }

  Process {
    id: reloadProcess
    command: ["hyprctl", "reload"]
  }
}
