---
id: TASK-5
title: Stream chat responses in chat and TUI
status: In Progress
assignee: []
created_date: '2026-05-03 19:51'
updated_date: '2026-05-03 19:54'
labels: []
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Add visible response streaming for the interactive scanner-based chat command and the Bubble Tea TUI chat mode while leaving ask behavior unchanged.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 termask chat passes a live writer to ask.Run so provider deltas are shown before the full response completes
- [x] #2 TUI chat mode receives assistant deltas through Bubble Tea messages and appends them to the active assistant transcript item live
- [x] #3 Chat and TUI still save complete assistant responses to history after completion
- [x] #4 ask command behavior remains unchanged
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add focused failing tests for TUI chat delta handling and completion/history preservation
2. Add a small streaming message path for TUI chat without changing ask command behavior
3. Pass a live stdout writer from termask chat into ask.Run and avoid duplicate final rendering
4. Run targeted tests and full Go test suite
5. Update acceptance criteria and notes while leaving task open for user confirmation
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented TUI chat streaming with a Bubble Tea message writer that forwards ask.Run output deltas into transcript updates, then normalizes the final assistant text for history persistence without duplicate transcript output.

Updated termask chat to pass os.Stdout as ask.Run's live writer and skip final Markdown re-rendering, preserving ask command behavior.

Verified with go test ./internal/tui and go test ./....
<!-- SECTION:NOTES:END -->
