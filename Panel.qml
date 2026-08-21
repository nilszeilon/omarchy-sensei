pragma ComponentBehavior: Bound

import QtQuick
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "io.github.nilszeilon.omarchy-sensei"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  property int selectedIndex: 0
  property bool cursorActive: false
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property color accent: Color.accent
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family

  function open() {
    root.controller.show()
    root.selectedIndex = 0
    root.cursorActive = stats.tasks.length > 0
    stats.refresh()
  }
  function close() { root.controller.hide() }
  function switchPanel(direction) {
    if (root.bar && typeof root.bar.switchPanelFrom === "function")
      return root.bar.switchPanelFrom(root.hostWidget || root, direction)
    return false
  }
  function moveCursor(dx, dy) {
    if (stats.tasks.length === 0) return
    root.cursorActive = true
    var delta = dy !== 0 ? dy : dx
    root.selectedIndex = Math.max(0, Math.min(stats.tasks.length - 1, root.selectedIndex + delta))
  }
  function ensureSelectedVisible() {
    if (!taskScroll.visible) return
    var card = taskRepeater.itemAt(root.selectedIndex)
    if (!card) return
    if (card.y < taskScroll.contentY) {
      taskScroll.contentY = card.y
    } else if (card.y + card.height > taskScroll.contentY + taskScroll.height) {
      taskScroll.contentY = card.y + card.height - taskScroll.height
    }
  }
  onSelectedIndexChanged: Qt.callLater(root.ensureSelectedVisible)
  function alpha(color, opacity) { return Qt.rgba(color.r, color.g, color.b, opacity) }

  SenseiModel { id: stats }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.hostWidget || root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(270))
    contentHeight: panel.fittedContentHeight(content.implicitHeight)

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onMoveRequested: function(dx, dy) { root.moveCursor(dx, dy) }
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }

      Column {
        id: content
        width: parent.width
        spacing: Style.space(14)

        Row {
          width: parent.width

          Column {
            width: parent.width - taskCount.implicitWidth
            spacing: Style.space(2)

            Text {
              text: "Sensei Tasks"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.subtitle
              font.bold: true
            }
            Text {
              text: "Use the shortcut to complete the task"
              color: root.foreground
              opacity: 0.65
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }

          Text {
            id: taskCount
            text: String(stats.tasks.length)
            color: stats.tasks.length > 0 ? root.accent : root.foreground
            opacity: stats.tasks.length > 0 ? 1 : 0.55
            font.family: root.fontFamily
            font.pixelSize: Style.font.title
            font.bold: true
          }
        }

        Rectangle {
          width: parent.width
          implicitHeight: levelContent.implicitHeight + Style.space(24)
          radius: Style.cornerRadius
          color: root.alpha(root.accent, 0.075)
          border.width: 1
          border.color: root.alpha(root.accent, 0.22)

          Column {
            id: levelContent
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Style.space(12)
            spacing: Style.space(8)

            Column {
              anchors.horizontalCenter: parent.horizontalCenter
              spacing: 0

              Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: String(stats.level.totalShortcuts || 0)
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.display
                font.bold: true
              }

              Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: "TOTAL SHORTCUTS USED"
                color: root.foreground
                opacity: 0.58
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                font.bold: true
                font.letterSpacing: 0.7
              }
            }

            Row {
              width: parent.width
              spacing: Style.space(8)

              Text {
                id: currentLevel
                text: "LVL " + String(stats.level.level || 1)
                color: root.accent
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                font.bold: true
                anchors.verticalCenter: parent.verticalCenter
              }

              PanelSlider {
                bar: root.bar
                width: parent.width - currentLevel.implicitWidth - nextLevel.implicitWidth - parent.spacing * 2
                value: stats.level.progress || 0
                minimum: 0
                maximum: 1
                enabled: false
                opacity: 1
                fillColor: root.accent
                knobColor: root.accent
                trackColor: root.alpha(root.foreground, 0.14)
                trackHeight: Math.max(4, Style.space(5))
                knobSize: Math.max(12, Style.space(12))
              }

              Text {
                id: nextLevel
                text: "LVL " + String(stats.level.nextLevel || 2)
                color: root.foreground
                opacity: 0.52
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                font.bold: true
                anchors.verticalCenter: parent.verticalCenter
              }
            }

            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: String(stats.level.shortcutsRemaining || 0) + " shortcuts to level " + String(stats.level.nextLevel || 2)
              color: root.foreground
              opacity: 0.58
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }
        }

        Flickable {
          id: taskScroll
          width: parent.width
          height: Math.min(taskList.implicitHeight, Style.space(360))
          contentWidth: width
          contentHeight: taskList.implicitHeight
          boundsBehavior: Flickable.StopAtBounds
          clip: true
          visible: stats.tasks.length > 0

          Column {
            id: taskList
            width: taskScroll.width
            spacing: Style.space(8)

            Repeater {
              id: taskRepeater
              model: stats.tasks

              Rectangle {
                id: taskCard
                required property int index
                required property var modelData
                readonly property bool selected: root.cursorActive && root.selectedIndex === index
                width: taskList.width
                implicitHeight: taskContent.implicitHeight + Style.space(24)
                radius: Style.cornerRadius
                color: selected ? root.alpha(root.accent, 0.12) : root.alpha(root.foreground, 0.055)
                border.width: selected ? 1 : 0
                border.color: root.alpha(root.accent, 0.75)

                Column {
                  id: taskContent
                  anchors.left: parent.left
                  anchors.right: parent.right
                  anchors.top: parent.top
                  anchors.margins: Style.space(12)
                  spacing: Style.space(7)

                  Row {
                    width: parent.width

                    Text {
                      width: parent.width - slowCount.implicitWidth
                      text: taskCard.modelData.title
                      color: root.foreground
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.body
                      font.bold: true
                      elide: Text.ElideRight
                    }
                    Text {
                      id: slowCount
                      text: taskCard.modelData.slowUses > 1 ? "×" + taskCard.modelData.slowUses : ""
                      color: root.foreground
                      opacity: 0.5
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.caption
                    }
                  }

                  Column {
                    width: parent.width
                    spacing: Style.space(5)

                    Repeater {
                      model: taskCard.modelData.shortcuts || []

                      Rectangle {
                        required property string modelData
                        width: taskContent.width
                        implicitHeight: shortcutText.implicitHeight + Style.space(16)
                        radius: Style.cornerRadius
                        color: root.alpha(root.accent, 0.15)

                        Text {
                          id: shortcutText
                          anchors.centerIn: parent
                          width: parent.width - Style.space(16)
                          text: parent.modelData
                          color: root.accent
                          font.family: root.fontFamily
                          font.pixelSize: Style.font.body
                          font.bold: true
                          horizontalAlignment: Text.AlignHCenter
                          wrapMode: Text.Wrap
                        }
                      }
                    }
                  }

                  Text {
                    width: parent.width
                    text: "Do it once with the keyboard to close this task."
                    color: root.foreground
                    opacity: 0.65
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.caption
                    wrapMode: Text.WordWrap
                  }
                }

                MouseArea {
                  anchors.fill: parent
                  hoverEnabled: true
                  onPositionChanged: {
                    root.selectedIndex = taskCard.index
                    root.cursorActive = true
                  }
                }
              }
            }
          }
        }

        Rectangle {
          visible: stats.tasks.length === 0
          width: parent.width
          implicitHeight: emptyContent.implicitHeight + Style.space(32)
          radius: Style.cornerRadius
          color: root.alpha(root.foreground, 0.05)
          border.width: 1
          border.color: root.alpha(root.foreground, 0.12)

          Column {
            id: emptyContent
            anchors.centerIn: parent
            spacing: Style.space(6)

            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: "✓"
              color: root.accent
              font.family: root.fontFamily
              font.pixelSize: Style.font.display
              font.bold: true
            }
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: "All clear"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              font.bold: true
            }
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: "No keyboard practice waiting."
              color: root.foreground
              opacity: 0.65
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }
        }

        Text {
          visible: stats.error !== "" || stats.paused
          width: parent.width
          text: stats.error !== "" ? stats.error : "Sensei is paused."
          color: root.foreground
          opacity: 0.7
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
          wrapMode: Text.WordWrap
        }
      }
    }
  }
}
