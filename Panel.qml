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
    for (var i = 0; i < stats.days.length; i++) {
      if (stats.days[i] && stats.days[i].today === true) return i
    }
    return Math.max(0, stats.days.length - 1)
  }
  function selectDay(index) {
    if (index < 0 || index >= stats.days.length) return
    var day = stats.days[index]
    if (!day || String(day.date || "") === "") return
    selectedDayIndex = index
    cursorActive = true
  }
  function moveCursor(dx, dy) {
    if (!cursorActive || selectedDayIndex < 0) {
      selectDay(todayIndex())
      return
    }

    var week = Math.floor(selectedDayIndex / 7)
    var weekday = selectedDayIndex % 7
    var nextWeek = Math.max(0, Math.min(52, week + dx))
    var nextWeekday = Math.max(0, Math.min(6, weekday + dy))
    var nextIndex = nextWeek * 7 + nextWeekday
    if (nextIndex < stats.days.length && stats.days[nextIndex]
        && String(stats.days[nextIndex].date || "") !== "") {
      selectDay(nextIndex)
    }
  }
  function selectedDay() {
    return selectedDayIndex >= 0 && selectedDayIndex < stats.days.length
      ? stats.days[selectedDayIndex]
      : null
  }
  function selectedDayLabel() {
    var day = selectedDay()
    if (!day || !day.date) return "Use arrows or h/j/k/l to explore the graph"
    var count = Number(day.count || 0)
    return day.date + " · " + count + (count === 1 ? " keyboard action" : " keyboard actions")
  }

  function open() {
    root.controller.show()
    stats.refresh()
  }
  function close() { root.controller.hide() }
  function switchPanel(direction) {
    if (root.bar && typeof root.bar.switchPanelFrom === "function")
      return root.bar.switchPanelFrom(root.hostWidget || root, direction)
    return false
  }
  function alpha(color, opacity) { return Qt.rgba(color.r, color.g, color.b, opacity) }
  function intensity(count) {
    if (count <= 0 || stats.maxCount <= 0) return 0
    var ratio = count / stats.maxCount
    if (ratio <= 0.25) return 0.28
    if (ratio <= 0.5) return 0.48
    if (ratio <= 0.75) return 0.7
    return 1
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
          spacing: Style.space(6)

          Text {
            text: "Keyboard-first activity"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            font.bold: true
          }

          Row {
            id: heatmap
            width: parent.width - Style.space(16)
            readonly property real cellGap: 2
            readonly property real cellSize: Math.max(3, Math.floor((width - (52 * cellGap)) / 53))
            spacing: cellGap

            Repeater {
              model: 53

              Column {
                id: weekColumn
                required property int index
                readonly property int weekIndex: index
                spacing: heatmap.cellGap

                Repeater {
                  model: 7

                  Rectangle {
                    required property int index
                    readonly property int dayIndex: weekColumn.weekIndex * 7 + index
                    readonly property var day: dayIndex < stats.days.length ? stats.days[dayIndex] : null
                    readonly property bool hasCursor: root.cursorActive && root.selectedDayIndex === dayIndex
                    width: heatmap.cellSize
                    height: heatmap.cellSize
                    radius: 1.5
                    color: day && day.count > 0
                      ? root.alpha(root.accent, root.intensity(Number(day.count)))
                      : root.alpha(root.foreground, 0.1)
                    border.width: hasCursor ? 2 : day && day.today ? 1 : 0
                    border.color: hasCursor ? root.accent : root.foreground

                    ToolTip.visible: hover.hovered && day !== null && String(day.date || "") !== ""
                    ToolTip.text: day && day.date ? day.date + " · " + day.count + " keyboard actions" : ""

                    HoverHandler {
                      id: hover
                      onHoveredChanged: if (hovered) root.selectDay(parent.dayIndex)
                    }
                  }
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
