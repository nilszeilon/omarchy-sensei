import QtQuick
import Quickshell.Io

Item {
  id: root
  visible: false

  property var tasks: []
  property bool paused: false
  property string error: ""

  function refresh() {
    if (!snapshotProcess.running) snapshotProcess.running = true
  }

  function applySnapshot(output) {
    try {
      var parsed = JSON.parse(String(output || "{}"))
      tasks = parsed.tasks || []
      paused = parsed.paused === true
      error = ""
    } catch (e) {
      error = "Sensei could not read its local tasks."
    }
  }

  Component.onCompleted: refresh()

  Timer {
    interval: 2000
    repeat: true
    running: true
    onTriggered: root.refresh()
  }

  Process {
    id: snapshotProcess
    command: ["omarchy-sensei", "snapshot"]
    onExited: function(exitCode) {
      if (exitCode !== 0) root.error = "Install the omarchy-sensei collector to begin tracking."
    }
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applySnapshot(text)
    }
  }
}
