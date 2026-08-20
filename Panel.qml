pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import Quickshell
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "io.github.nilszeilon.omarchy-sensei"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property color accent: Color.accent
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family
  property bool cursorActive: false
  property int selectedDayIndex: -1

  function todayIndex() {
    for (var i = 0; i < stats.shortcutDays.length; i++) {
      if (stats.shortcutDays[i] && stats.shortcutDays[i].today === true) return i
    }
    return Math.max(0, stats.shortcutDays.length - 1)
  }
  function selectDay(index) {
    if (index < 0 || index >= stats.shortcutDays.length) return
    var day = stats.shortcutDays[index]
    if (!day || String(day.date || "") === "") return
    selectedDayIndex = index
    cursorActive = true
  }
  function moveCursor(dx, dy) {
    if (!cursorActive || selectedDayIndex < 0) {
      selectDay(todayIndex())
      return
    }

    var delta = dx !== 0 ? dx : dy
    selectDay(Math.max(0, Math.min(6, selectedDayIndex + delta)))
  }
  function selectedDay() {
    return selectedDayIndex >= 0 && selectedDayIndex < stats.shortcutDays.length
      ? stats.shortcutDays[selectedDayIndex]
      : null
  }
  function selectedDayLabel() {
    var day = selectedDay()
    if (!day || !day.date) return "Use arrows or h/j/k/l to compare days"
    var mouseDay = stats.mouseDays[selectedDayIndex]
    return day.date + " · Mouse/menu " + Number(mouseDay ? mouseDay.count : 0)
      + " · Shortcuts " + Number(day.count || 0)
  }

  function open() {
    root.controller.show()
    stats.refresh()
    Qt.callLater(function() { root.selectDay(root.todayIndex()) })
  }
  function close() { root.controller.hide() }
  function switchPanel(direction) {
    if (root.bar && typeof root.bar.switchPanelFrom === "function")
      return root.bar.switchPanelFrom(root.hostWidget || root, direction)
    return false
  }
  function alpha(color, opacity) { return Qt.rgba(color.r, color.g, color.b, opacity) }
  function barHeight(count) {
    if (stats.maxCount <= 0) return 0
    return Math.max(count > 0 ? 3 : 0, Math.round(Style.space(48) * Number(count) / stats.maxCount))
  }

  SenseiModel { id: stats }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.hostWidget || root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(300))
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

        Text {
          width: parent.width
          text: "Omarchy Sensei"
          color: root.foreground
          font.family: root.fontFamily
          font.pixelSize: Style.font.subtitle
          font.bold: true
        }

        Column {
          width: parent.width
          spacing: Style.space(10)

          Text {
            text: "Mouse / menu actions · last 7 days"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            font.bold: true
          }

          Row {
            id: mouseChart
            width: parent.width
            height: Style.space(62)
            spacing: Style.space(4)

            Repeater {
              model: 7

              Rectangle {
                required property int index
                readonly property var day: index < stats.mouseDays.length ? stats.mouseDays[index] : null
                width: (mouseChart.width - mouseChart.spacing * 6) / 7
                height: mouseChart.height
                radius: Style.cornerRadius
                color: root.alpha(root.foreground, root.cursorActive && root.selectedDayIndex === index ? 0.1 : 0.04)
                border.width: root.cursorActive && root.selectedDayIndex === index ? 1 : 0
                border.color: root.foreground

                Rectangle {
                  anchors.bottom: dayName.top
                  anchors.bottomMargin: Style.space(4)
                  anchors.horizontalCenter: parent.horizontalCenter
                  width: Math.max(Style.space(8), parent.width - Style.space(12))
                  height: root.barHeight(parent.day ? Number(parent.day.count) : 0)
                  radius: 2
                  color: root.alpha(root.foreground, 0.55)
                }

                Text {
                  id: dayName
                  anchors.bottom: parent.bottom
                  anchors.horizontalCenter: parent.horizontalCenter
                  text: parent.day && parent.day.date
                    ? Qt.formatDate(new Date(parent.day.date + "T12:00:00"), "ddd").slice(0, 1)
                    : ""
                  color: root.foreground
                  opacity: 0.65
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.caption
                }

                MouseArea {
                  anchors.fill: parent
                  hoverEnabled: true
                  onPositionChanged: root.selectDay(parent.index)
                }
              }
            }
          }

          Text {
            text: "Omarchy shortcuts · last 7 days"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            font.bold: true
          }

          Row {
            id: shortcutChart
            width: parent.width
            height: Style.space(62)
            spacing: Style.space(4)

            Repeater {
              model: 7

              Rectangle {
                required property int index
                readonly property var day: index < stats.shortcutDays.length ? stats.shortcutDays[index] : null
                width: (shortcutChart.width - shortcutChart.spacing * 6) / 7
                height: shortcutChart.height
                radius: Style.cornerRadius
                color: root.alpha(root.foreground, root.cursorActive && root.selectedDayIndex === index ? 0.1 : 0.04)
                border.width: root.cursorActive && root.selectedDayIndex === index ? 1 : 0
                border.color: root.foreground

                Rectangle {
                  anchors.bottom: shortcutDayName.top
                  anchors.bottomMargin: Style.space(4)
                  anchors.horizontalCenter: parent.horizontalCenter
                  width: Math.max(Style.space(8), parent.width - Style.space(12))
                  height: root.barHeight(parent.day ? Number(parent.day.count) : 0)
                  radius: 2
                  color: root.accent
                }

                Text {
                  id: shortcutDayName
                  anchors.bottom: parent.bottom
                  anchors.horizontalCenter: parent.horizontalCenter
                  text: parent.day && parent.day.date
                    ? Qt.formatDate(new Date(parent.day.date + "T12:00:00"), "ddd").slice(0, 1)
                    : ""
                  color: root.foreground
                  opacity: 0.65
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.caption
                }

                MouseArea {
                  anchors.fill: parent
                  hoverEnabled: true
                  onPositionChanged: root.selectDay(parent.index)
                }
              }
            }
          }

          Text {
            width: parent.width
            text: root.selectedDayLabel()
            color: root.foreground
            opacity: 0.72
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }

        Rectangle {
          width: parent.width
          implicitHeight: lesson.implicitHeight + Style.space(24)
          radius: Style.cornerRadius
          color: root.alpha(root.foreground, 0.08)

          Column {
            id: lesson
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Style.space(12)
            spacing: Style.space(6)

            Text {
              width: parent.width
              text: stats.paused ? "Sensei is paused" : stats.hint ? "Your next lesson" : "Sensei is watching quietly"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              font.bold: true
            }

            Text {
              width: parent.width
              text: stats.error !== ""
                ? stats.error
                : stats.paused
                  ? "Resume with omarchy-sensei resume."
                : stats.hint
                  ? stats.hint.title + " was done the slow way " + stats.hint.slowUses
                    + (Number(stats.hint.slowUses) === 1 ? " time. Use " : " times. Use ")
                    + stats.hint.shortcut + "."
                  : "No unlearned shortcut yet."
              color: root.foreground
              opacity: 0.82
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
              wrapMode: Text.WordWrap
            }
          }
        }
      }
    }
  }
}
