import QtQuick
import Quickshell.Io

Item {
  id: root
  visible: false

  property var days: []
  property int maxCount: 0
  property var hint: null
  property bool paused: false
  property string error: ""

  function refresh() {
    if (!snapshotProcess.running) snapshotProcess.running = true
  }

  function applySnapshot(output) {
    try {
      var parsed = JSON.parse(String(output || "{}"))
      days = parsed.days || []
      maxCount = Number(parsed.maxCount || 0)
      hint = parsed.hint || null
      paused = parsed.paused === true
      error = ""
    } catch (e) {
      error = "Sensei could not read its local activity data."
    }
  }

  Component.onCompleted: refresh()

  Timer {
    interval: 60000
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
