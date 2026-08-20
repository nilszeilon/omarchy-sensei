# Privacy contract

Omarchy Sensei must be useful without becoming a keylogger.

## Never collect

- Typed characters or raw keycodes
- Clipboard contents
- Terminal commands or terminal output
- Document, message, or browser contents
- Passwords or authentication input
- Pointer coordinates or screenshots

## Allowed local signals

- A known semantic action, such as `launch_browser`
- A coarse trigger class: shortcut, menu, mouse, command, or agent
- The known shortcut associated with that action
- Timing needed to estimate friction and time saved
- Aggregate counters and recommendation outcomes

## User control

Collection must be opt-in. The UI must expose pause, retention, export, and delete controls. Agent handoff must show the exact context bundle and require confirmation before anything leaves the machine.
