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
  property int refreshAttempts: 0

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

  function refreshCatalog() {
    if (!root.helperPath || refreshProcess.running || root.refreshAttempts >= 30) return
    root.refreshAttempts += 1
    refreshProcess.command = ["python3", root.helperPath, "refresh"]
    refreshProcess.running = true
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
      refreshTimer.start()
    }
  }

  // Hyprland's live binding catalog may not be ready at the instant the shell
  // loads third-party services. Retry for up to five minutes; refresh refuses
  // to overwrite the last useful catalog with an empty startup result.
  Timer {
    id: refreshTimer
    interval: 10000
    repeat: false
    onTriggered: root.refreshCatalog()
  }

  Process {
    id: refreshProcess
    onExited: function(exitCode) {
      if (exitCode === 0) {
        root.error = ""
        return
      }
      if (root.refreshAttempts < 30) refreshTimer.start()
    }
  }

  Process {
    id: reloadProcess
    command: ["hyprctl", "reload"]
  }
}
