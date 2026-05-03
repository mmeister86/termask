---
id: TASK-2
title: Add streaming agent events and responses
status: In Progress
assignee: []
created_date: '2026-04-30 19:25'
updated_date: '2026-04-30 19:44'
labels: []
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Add a streaming event layer for the native read-only agent so interactive sessions show model activity, tool execution, and final answer deltas in a modern agent-style terminal UX.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Agent exposes a streaming run API with step, tool, answer delta, done, and error events
- [x] #2 Interactive agent sessions render live thinking/tool status and streamed final answer text
- [x] #3 Plain mode remains script-friendly without status lines
- [x] #4 Tool-call JSON is not leaked as visible assistant output
- [x] #5 Tests cover event ordering, streamed final answers, tool events, and plain-mode behavior
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add failing tests for streaming agent events and answer deltas
2. Implement agent event types and RunStream while preserving Run
3. Wire interactive CLI to render streaming status and answer output
4. Keep --plain one-shot output clean
5. Verify full suite, install binary, and update backlog notes
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented streaming agent event layer with model/tool/answer events, wired interactive sessions to render thinking/tool status and answer deltas, kept --plain one-shot behavior clean, and documented the streaming UX. Verified with go test ./... and go run ./cmd/termask agent --help; installed updated binary with make install.

Fixed follow-up quality regression: when the model deflects with "already listed/explained" plus an offer instead of answering, the loop now retries with a direct follow-up instruction. Also fixed streamed answer chunks to preserve original whitespace/Markdown instead of collapsing bullets into one line.

Fixed install/runtime issue where copying over ~/.local/bin/termask in place could leave the installed executable wedged. Makefile install now copies to a temporary file and atomically mv -f replaces the target. Reinstalled and verified ~/.local/bin/termask agent --help exits successfully.

Fixed repeated-tool loop: identical tool calls are now detected by signature and not executed again. The loop instructs the model to answer with existing context instead, preventing repeated list_files calls from exhausting max steps on project-overview follow-ups.

Fixed direct file-list intent overreach: when the user asks which files are in the current folder, termask now uses the read-only list_files result to produce a deterministic file list directly instead of passing the result back through the model, preventing project-analysis answers to simple listing questions.
<!-- SECTION:NOTES:END -->
