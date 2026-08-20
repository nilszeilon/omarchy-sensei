import QtQuick
import Quickshell.Io

Item {
  id: root
  visible: false

  property var mouseDays: []
  property var shortcutDays: []
  property int maxCount: 0
  property var hint: null
  property var branches: []
  property var trial: null
  property int xp: 0
  property int level: 1
  property bool paused: false
  property string error: ""

  function refresh() {
    if (!snapshotProcess.running) snapshotProcess.running = true
  }

  function applySnapshot(output) {
    try {
      var parsed = JSON.parse(String(output || "{}"))
      mouseDays = parsed.mouseDays || []
      shortcutDays = parsed.shortcutDays || []
      maxCount = Number(parsed.maxCount || 0)
      hint = parsed.hint || null
      branches = parsed.branches || []
      trial = parsed.trial || null
      xp = Number(parsed.xp || 0)
      level = Number(parsed.level || 1)
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
