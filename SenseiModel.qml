import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  visible: false

  property var tasks: []
  property var level: ({
    totalShortcuts: 0,
    level: 1,
    nextLevel: 2,
    shortcutsInLevel: 0,
    shortcutsForLevel: 10,
    shortcutsRemaining: 10,
    progress: 0
  })
  property bool paused: false
  property string error: ""

  readonly property string stateHome: {
    var configured = Quickshell.env("XDG_STATE_HOME")
    return configured ? String(configured) : String(Quickshell.env("HOME")) + "/.local/state"
  }
  readonly property string stateDir: stateHome + "/omarchy-sensei"
  readonly property string statePath: stateDir + "/state.json"
  readonly property string pausedPath: stateDir + "/paused"

  function refresh() {
    stateFile.reload()
    pausedFile.reload()
  }

  function levelProgress(total) {
    total = Math.max(0, Number(total) || 0)
    var level = 1
    var levelStart = 0
    var requirement = 10
    while (total >= levelStart + requirement) {
      levelStart += requirement
      level += 1
      requirement = Math.floor((requirement * 3 + 1) / 2)
    }
    var current = total - levelStart
    return {
      totalShortcuts: total,
      level: level,
      nextLevel: level + 1,
      shortcutsInLevel: current,
      shortcutsForLevel: requirement,
      shortcutsRemaining: requirement - current,
      progress: current / requirement
    }
  }

  function resetState() {
    tasks = []
    level = levelProgress(0)
    error = ""
  }

  function applyState(output) {
    try {
      var parsed = JSON.parse(String(output || "{}"))
      var nextTasks = Array.isArray(parsed.tasks) ? parsed.tasks.slice() : []
      nextTasks.sort(function(left, right) {
        var offenderOrder = Number(right.slowUses || 0) - Number(left.slowUses || 0)
        if (offenderOrder !== 0) return offenderOrder
        return String(left.openedAt || "").localeCompare(String(right.openedAt || ""))
      })
      tasks = nextTasks
      level = levelProgress(parsed.totalShortcuts)
      error = ""
    } catch (e) {
      error = "Sensei could not read its local tasks."
    }
  }

  Component.onCompleted: Qt.callLater(root.refresh)

  // Atomic state replacement and pause-file creation can detach a direct file
  // watch. Watching the stable directory ensures both files are reloaded and
  // their watches are re-established after every relevant change.
  FileView {
    id: stateDirectory
    path: root.stateDir
    watchChanges: true
    printErrors: false
    onFileChanged: root.refresh()
  }

  FileView {
    id: stateFile
    path: root.statePath
    watchChanges: true
    printErrors: false
    onLoaded: root.applyState(text())
    onLoadFailed: root.resetState()
    onFileChanged: reload()
  }

  FileView {
    id: pausedFile
    path: root.pausedPath
    watchChanges: true
    printErrors: false
    onLoaded: root.paused = true
    onLoadFailed: root.paused = false
    onFileChanged: reload()
  }
}
