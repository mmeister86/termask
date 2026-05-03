---
id: TASK-3
title: Rebuild termask tui as an opencode-inspired fullscreen interface
status: In Progress
assignee: []
created_date: '2026-05-03 14:26'
updated_date: '2026-05-03 14:36'
labels:
  - tui
  - feature
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Replace the current scanner-based termask tui with a Bubble Tea fullscreen interface inspired by opencode while preserving existing chat and agent commands.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 termask tui launches a fullscreen Bubble Tea interface with termask-branded opencode-style idle screen input panel hints and footer status
- [x] #2 TUI supports Chat and read-only Agent modes with Tab mode switching Enter send Alt+Enter or Ctrl+J newline Ctrl+P command palette and Esc/Ctrl+C quit behavior
- [x] #3 Slash commands /new /provider <name> /history and /quit continue to work inside the TUI
- [x] #4 Chat mode uses existing ask history flow and Agent mode reuses existing read-only agent streaming events
- [x] #5 Resize and narrow terminal rendering stay readable without broken layouts
- [x] #6 Automated tests cover TUI update logic rendering helpers fake chat runner and fake agent event handling
- [x] #7 go test ./... passes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add failing unit tests for TUI mode switching command palette slash commands rendering and fake chat/agent runners
2. Add Bubble Tea/Bubbles direct dependencies if needed and inspect local APIs
3. Replace internal/tui scanner loop with testable Bubble Tea model and runners
4. Wire Chat mode to ask.Run and Agent mode to agent.RunStream events
5. Polish responsive opencode-inspired termask rendering
6. Run gofmt and go test ./...
7. Update Backlog acceptance criteria and notes without marking Done
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented Bubble Tea fullscreen TUI for termask tui with termask-branded opencode-inspired idle screen input panel footer status and command hints.

Added Chat and read-only Agent modes with Tab switching Ctrl+P command palette Enter submit Alt+Enter/Ctrl+J newline and Esc/Ctrl+C quit behavior.

Preserved slash commands /new /provider <name> /history /quit and added internal /mode chat|agent commands for palette mode switching.

Chat mode uses ask.Run with session history and Agent mode reuses agent.RunStream via event-channel messages for thinking tool and answer updates.

Added internal/tui unit tests for mode switching palette slash commands multiline input fake chat runner agent events error handling and responsive rendering.

Verified go test ./... passes and go run ./cmd/termask tui --help succeeds. Manual interactive terminal testing is still recommended before closing TASK-3.

Final verification after provider-switch cleanup: go test ./... exits 0 across all packages. go run ./cmd/termask tui --help exits 0 and shows the TUI command help.
<!-- SECTION:NOTES:END -->
