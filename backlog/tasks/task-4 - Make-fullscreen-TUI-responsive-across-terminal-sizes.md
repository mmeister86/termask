---
id: TASK-4
title: Make fullscreen TUI responsive across terminal sizes
status: In Progress
assignee: []
created_date: '2026-05-03 14:39'
updated_date: '2026-05-03 20:09'
labels:
  - tui
  - responsive
  - bugfix
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Fix the new Bubble Tea fullscreen TUI so it fills the actual terminal viewport without leaving blank uncovered areas and adapts logo input footer and transcript layout across narrow wide short and tall terminals.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 TUI background fills the full terminal width and height in fullscreen alt-screen mode without right-side or bottom uncovered areas
- [x] #2 Idle screen adapts vertical spacing so the logo prompt input and footer remain visible on short terminals
- [x] #3 Input panel footer and hints stay within the viewport on narrow terminals without wrapping into broken overlapping rows
- [x] #4 Wide terminals keep the experience centered and readable without stretching controls excessively
- [x] #5 Automated tests cover responsive render output for narrow wide short and tall terminal sizes
- [x] #6 go test ./... passes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add failing responsive render tests for full-viewport fill and short/narrow/wide layouts
2. Refactor render sizing so outer canvas always uses terminal dimensions while inner content remains constrained
3. Adapt idle screen vertical allocation and input/footer wrapping for short and narrow terminals
4. Run gofmt and go test ./...
5. Check Backlog acceptance criteria without marking Done
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Refactored TUI rendering so the outer canvas always uses the actual terminal width and height while the inner content remains centered and capped for readability.

Added compact idle behavior for short terminals so the large block logo is replaced by a small termask mark and prompt input footer remain visible.

Adjusted hint text footer sizing and input panel sizing for narrow terminals to avoid broken wrapping and overlapping rows.

Added responsive regression tests for wide viewport fill short terminal compact layout and narrow viewport bounds.

Verified go test ./... exits 0.

Fixed follow-up bug from manual testing: normal printable keypresses were being swallowed because all tea.KeyPressMsg values returned from the shortcut handler before textarea.Update could process them.

Set explicit Bubble Tea View background and foreground colors so ANSI resets do not reveal the terminal default background as flecked patches.

Styled the textarea focused and blurred states with the input background color so the input panel does not show mismatched default-background blocks.

Rebuilt the tracked ./termask binary with make build after the fix.

Final verification after input/background fix: go test ./... exits 0.
<!-- SECTION:NOTES:END -->
