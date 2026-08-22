import QtQuick
import Quickshell.Io

// A plugin service is loaded whenever Sensei's bar widget is enabled.  It
// bootstraps the user integration from the cloned checkout, so marketplace
// installation does not require a compiler or a second terminal command.
Item {
  id: root

  property var manifest: null
  property var shell: null
  property bool ready: false
  property string error: ""
  property string stderrText: ""
  property int refreshAttempts: 0
  property string pendingRoute: ""
  property string pendingAppName: ""

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

  readonly property var menuItem: {
    var loaders = root.shell ? root.shell.panelLoaders : null
    var loader = loaders ? loaders["omarchy.menu"] : null
    return loader && loader.item ? loader.item : null
  }

  readonly property var appLibrary: root.shell ? root.shell.appLibrary : null

  function observeRoute(route) {
    var value = String(route || "")
    if (!root.ready || !value || value === "root") return
    root.pendingRoute = value
    if (!routeProcess.running) {
      routeProcess.command = ["omarchy-sensei", "coach-route", "--route", root.pendingRoute]
      root.pendingRoute = ""
      routeProcess.running = true
    }
  }

  function observeAppLaunch() {
    if (!root.ready || !root.appLibrary) return
    var message = String(root.appLibrary.launchOsdMessage || "")
    if (message.indexOf("Launching ") !== 0) return
    var name = message.slice(10).replace(/…$/, "").trim()
    if (!name) return
    root.pendingAppName = name
    if (!appProcess.running) {
      appProcess.command = ["omarchy-sensei", "coach-app", "--name", root.pendingAppName]
      root.pendingAppName = ""
      appProcess.running = true
    }
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

  Timer {
    id: routeObservationTimer
    interval: 0
    repeat: false
    onTriggered: if (root.menuItem && root.menuItem.opened)
      root.observeRoute(root.menuItem.activeMenu)
  }

  Timer {
    id: appObservationTimer
    interval: 0
    repeat: false
    onTriggered: root.observeAppLaunch()
  }

  Connections {
    target: root.menuItem
    ignoreUnknownSignals: true
    function onActiveMenuChanged() { routeObservationTimer.restart() }
    function onOpenedChanged() { if (root.menuItem && root.menuItem.opened) routeObservationTimer.restart() }
  }

  Connections {
    target: root.appLibrary
    ignoreUnknownSignals: true
    function onLaunchSerialChanged() { appObservationTimer.restart() }
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
    id: routeProcess
    onExited: {
      if (!root.pendingRoute) return
      command = ["omarchy-sensei", "coach-route", "--route", root.pendingRoute]
      root.pendingRoute = ""
      running = true
    }
  }

  Process {
    id: appProcess
    onExited: {
      if (!root.pendingAppName) return
      command = ["omarchy-sensei", "coach-app", "--name", root.pendingAppName]
      root.pendingAppName = ""
      running = true
    }
  }

  Process {
    id: reloadProcess
    command: ["hyprctl", "reload"]
  }
}
