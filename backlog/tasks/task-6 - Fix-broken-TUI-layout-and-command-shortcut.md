---
id: TASK-6
title: Fix broken TUI layout and command shortcut
status: In Progress
assignee: []
created_date: '2026-05-04 13:29'
updated_date: '2026-05-04 14:03'
labels: []
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Investigate and fix the TUI rendering artifacts shown as gray bars, center the bottom input, and restore ctrl+p command handling.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The TUI no longer renders stray gray bars beside the logo or around the input area
- [x] #2 The bottom input is horizontally centered/aligned as intended across terminal widths
- [x] #3 ctrl+p opens or triggers the commands behavior again
- [x] #4 Relevant automated tests or reproducible verification cover the regression
- [x] #5 Transcript output can be scrolled after content exceeds the viewport
- [x] #6 Transcript message spacing is compact enough for terminal reading
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Reproduce and inspect current TUI rendering tests
2. Trace layout width/background handling for splash and input
3. Add failing regression tests for gray bars/centering and ctrl+p
4. Implement focused fix
5. Run targeted and full Go tests
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Created TASK-6 and confirmed existing internal/tui tests pass after allowing Go dependency resolution. Current tests do not cover styled ANSI reset gaps or Update-level ctrl+p handling.

Added regression coverage for ANSI reset gaps, input hint centering, and ctrl+p KeyPressMsg handling. Implemented explicit screen padding backgrounds, input border background, centered input hints, and normalized ctrl-key matching.

New follow-up: streamed transcript output is not scrollable and message spacing is too loose. Investigating transcript rendering and scroll input handling before changing code.

Added transcript scroll and spacing regression tests. Implemented transcript scroll offset, PageUp/PageDown/Home/End handling, mouse wheel support, compact line rendering, and bottom-follow behavior for new submissions.

Follow-up after manual test: transcript is scrollable, but assistant output still has huge vertical gaps. Investigating Markdown rendering and transcript container height/background behavior.

Manual retest showed scrolling works but Markdown/codeblock output still had huge vertical gaps. Added a regression for compact fenced-code rendering in the TUI transcript, removed Glamour from TUI transcript rendering, and stopped forcing transcript containers to fill their full height.
<!-- SECTION:NOTES:END -->
