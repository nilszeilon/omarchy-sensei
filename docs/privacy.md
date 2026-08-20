# Privacy contract

Omarchy Sensei must be useful without becoming a keylogger.

## Never collect

- Typed characters or raw keycodes
- Clipboard contents
- Terminal commands or terminal output
- Document, message, or browser contents
- Passwords or authentication input
- Pointer coordinates or screenshots
- Raw pointer-button streams or clicks inside application/panel content

## Allowed local signals

- A known semantic action, such as `launch_browser`
- A semantic Omarchy bar module, workspace number, or keyboard panel position while resolving a click locally
- A coarse trigger class: shortcut, menu, mouse, command, or agent
- The known shortcut associated with that action
- Timing needed to estimate friction and time saved
- Aggregate counters and recommendation outcomes

## User control

Collection must be opt-in. The collector must expose pause and complete-data deletion controls. Any future agent handoff must show the exact context bundle and require confirmation before anything leaves the machine.

Setup opts in through `omarchy-sensei setup`. `pause`, `resume`, `status`, and `clear` provide local control. No network client or telemetry endpoint exists in the collector. Unmatched clicks are discarded rather than stored.
