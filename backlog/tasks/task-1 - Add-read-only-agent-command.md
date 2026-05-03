---
id: TASK-1
title: Add read-only agent command
status: In Progress
assignee: []
created_date: '2026-04-30 19:01'
updated_date: '2026-04-30 19:18'
labels: []
dependencies: []
priority: high
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implement a native Go agent loop for termask with a new agent command and strictly read-only tools for gathering project context and running allowlisted checks.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 termask exposes an agent command with provider, max-steps, file, and plain flags
- [x] #2 Agent loop supports provider-neutral JSON tool calls and final markdown answers
- [x] #3 Read-only tools list files, read files, search text, inspect git status/diff, and run only allowlisted Go checks
- [x] #4 Paths are sandboxed to the workspace and binary/oversized files are rejected
- [x] #5 Tests cover tool safety, loop behavior, CLI wiring, and existing regression suite passes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add failing tests for read-only tools and agent loop
2. Implement internal/agent toolset and provider-neutral loop
3. Add CLI command and history integration
4. Add CLI tests and update docs
5. Run full verification and update backlog notes/AC status
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented native read-only agent package, provider adapter, Cobra agent command, README docs, and tests. Verified with `go test ./...` and `go run ./cmd/termask agent --help`.

Fixed agent loop bug where models embedded a tool JSON object in explanatory text. Added regression test and now extract balanced JSON tool calls from text before treating a response as final.

Fixed one-shot permission deferral: agent now retries when a model asks permission for read-only tools, and prefetches list_files for direct current-folder file-list questions. Rebuilt and installed termask to ~/.local/bin/termask for manual testing.

Changed agent UX from one-shot terminal behavior to an interactive session: initial goals run first, then the prompt stays open for follow-ups; --plain and piped stdin remain one-shot. Agent turns now carry prior history into subsequent model calls.

Updated installed binary after interactive agent-session UX change. Verified with go test ./... and termask agent --help.
<!-- SECTION:NOTES:END -->
