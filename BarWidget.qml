import QtQuick
import qs.Commons
import qs.Ui

BarWidget {
  id: root
  moduleName: "io.github.nilszeilon.omarchy-sensei"

  readonly property bool opened: panelLoader.item
    ? panelLoader.item.opened === true
    : false
  readonly property bool popoutSwitchClosing: panelLoader.item
    ? panelLoader.item.popoutSwitchClosing === true
    : false

  function open() {
    if (panelLoader.item) panelLoader.item.open()
  }

  function close() {
    if (panelLoader.item) panelLoader.item.close()
  }

  function toggle() {
    if (panelLoader.item) panelLoader.item.toggle()
  }

  function closeForPopoutSwitch() {
    if (panelLoader.item) panelLoader.item.closeForPopoutSwitch()
  }

  function isObserverLeader() {
    if (!root.bar) return false
    var slots = root.bar.moduleSlots || []
    for (var i = 0; i < slots.length; i++) {
      var slot = slots[i]
      if (slot && slot.moduleName === root.moduleName && slot.activeItem)
        return slot.activeItem === root
    }
    return false
  }

  function slotForTarget(target) {
    if (!root.bar || !target) return null
    var slots = root.bar.moduleSlots || []
    for (var i = 0; i < slots.length; i++) {
      var slot = slots[i]
      if (!slot || !slot.activeItem) continue
      var item = target
      while (item) {
        if (item === slot.activeItem) return slot
        item = item.parent
      }
    }
    return null
  }

  function targetIsOnBar(target, slot) {
    if (!root.bar || !target || !slot) return false
    if (typeof root.bar.targetWindow !== "function" ||
        typeof root.bar.slotWindow !== "function" ||
        typeof root.bar.sameWindow !== "function") return false
    return root.bar.sameWindow(root.bar.targetWindow(target), root.bar.slotWindow(slot))
  }

  function panelIndexForSlot(slot) {
    if (!root.bar || !slot || slot.region !== "right" ||
        typeof root.bar.panelNavigationSlots !== "function") return 0
    var panels = root.bar.panelNavigationSlots(slot.region, root.bar.slotWindow(slot))
    for (var i = 0; i < panels.length; i++) if (panels[i] === slot) return i + 1
    return 0
  }

  function workspaceForTarget(target, moduleId) {
    if (String(moduleId || "").toLowerCase().indexOf("workspace") < 0) return 0
    try {
      var workspace = Number(target.modelData)
      if (workspace >= 1 && workspace <= 10 && Math.floor(workspace) === workspace) return workspace
    } catch (e) {
    }
    return 0
  }

  function observeBarClick(target, buttonCode) {
    if (buttonCode !== Qt.LeftButton || !root.observerLeader) return
    var slot = root.slotForTarget(target)
    if (!slot || slot.moduleName === root.moduleName || !root.targetIsOnBar(target, slot)) return

    var command = "omarchy-sensei coach-click --module " + Util.shellQuote(String(slot.moduleName))
      + " --region " + Util.shellQuote(String(slot.region || ""))
    var workspace = root.workspaceForTarget(target, slot.moduleName)
    var panelIndex = root.panelIndexForSlot(slot)
    if (workspace > 0) command += " --workspace " + workspace
    if (panelIndex > 0) command += " --panel-index " + panelIndex
    root.bar.run(command)
  }

  readonly property bool observerLeader: {
    // Reading moduleSlots here makes leadership follow monitor/bar rebuilds.
    var serial = root.bar ? root.bar.moduleSlots : []
    return serial && root.isObserverLeader()
  }
  readonly property var observedClickTargets: root.observerLeader && root.bar
    ? root.bar.clickTargets : []

  function injectPanel() {
    if (!panelLoader.item) return
    panelLoader.item.bar = root.bar
    panelLoader.item.anchorItem = button
    panelLoader.item.hostWidget = root
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onBarChanged: injectPanel()

  Loader {
    id: panelLoader
    active: true
    source: Qt.resolvedUrl("Panel.qml")
    visible: false
    onLoaded: {
      root.injectPanel()
      Qt.callLater(root.injectPanel)
    }
  }

  Repeater {
    model: root.observedClickTargets

    delegate: Item {
      id: clickObserver
      required property var modelData
      visible: false
      width: 0
      height: 0

      Connections {
        target: clickObserver.modelData
        function onPressed(buttonCode) { root.observeBarClick(clickObserver.modelData, buttonCode) }
      }
    }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: "⌨"
    tooltipText: "Open Omarchy Sensei"
    onPressed: function(buttonCode) {
      if (buttonCode === Qt.LeftButton) root.toggle()
    }
  }
}
