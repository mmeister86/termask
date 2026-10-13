# Termask Audit Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all 16 audit findings from the 2026-10-12 repo audit without breaking existing CLI behaviour.

**Architecture:** Keep existing packages. Fix behaviour in the owning package first (history, provider, agent, safety, markdown), then CLI wiring in `cmd/termask`, then structural split and release hygiene last so earlier diffs stay small.

**Tech Stack:** Go 1.25.8, cobra, Bubble Tea v2, BurntSushi/toml, glamour v2, stdlib `httptest`/`bufio`/`os/signal`. No new dependencies; `golang.org/x/term` is already in the module graph (indirect) and may move to direct.

**Spec:** The audit report delivered in the parent session (16 numbered findings). This plan is the spec; finding numbers `F1`–`F16` below refer to it.

## Global Constraints

- All subagents are Muse Spark 1.3 (`muse-spark` agent type). Never dispatch subagents from an implementer/reviewer.
- TDD: every behaviour change gets a failing test first (RED evidence in report), then minimal code (GREEN), full `go test ./...` + `go vet ./...` before commit.
- No new dependencies. No config format breakage except removing the documented-deprecated `render_markdown`/`--render` (F11).
- History file format stays JSONL of full sessions (F3 adds pruning/atomic replace only).
- Each task: one commit (fix rounds may add commits), message `fix: <finding ids> <short summary>`.
- Ponytail: smallest diff that fully fixes the finding; deletion over addition; no wrappers/options nobody asked for.

## Review Focus

- Pasted shell text containing `"`, `$(...)`, backticks must never execute (F1) — test asserts plugin output contains no `eval` and passes `"$query"` as one argument.
- History line >64 KiB must round-trip (F2/F3) — test writes a session with 200 KiB assistant message.
- Agent `read_file` on symlink escaping workspace and on `.env` must error (F4) — tests with real symlink in `t.TempDir()`.
- Ctrl+C during a provider call must cancel the HTTP request (F6) — test uses a blocking `httptest` server + cancelled context at `ask.Run` level.
- `ask --continue` with `history_enabled=false` must error, not answer silently (F8) — covered by extracting the continue-resolution into a testable helper or config-level guard.

---

### Task 1: Shell plugin eval removal + masked API-key input + stdin scanner error (F1, F5, F2-stdin)

**Files:**
- Modify: `cmd/termask/main.go` (zshPlugin ~L1240, bashPlugin ~L1277, initCmd ~L548-612, ask stdin ~L330-337)
- Test: `cmd/termask/shell_test.go`

**Interfaces:**
- Produces: plugins call `termask ask [--provider "$TERMASK_PROVIDER"] -- "$query"` with no `eval`; `init` reads key via `term.ReadPassword(int(os.Stdin.Fd()))` when stdin is a terminal, else falls back to scanner. Helper `readAPIKey() (string, error)` in main.go, unit-testable only via the non-terminal path.

- [ ] Step 1: Failing tests: `TestShellPluginsDoNotUseEval` (assert no `eval`, contains `-- "$query"` in both plugins); keep existing plugin tests green.
- [ ] Step 2: Run `go test ./cmd/termask -run TestShellPlugins` — expect FAIL.
- [ ] Step 3: Rewrite both plugins without eval (zsh: array `args=(ask); [[ -n $TERMASK_PROVIDER ]] && args+=(--provider "$TERMASK_PROVIDER"); termask "${args[@]}" -- "$query" | less -FIRX`; bash equivalent with array). Add `readAPIKey`, use it in initCmd, remove `--key sk-...` example from Example text (keep flag, mark deprecated in usage string). In ask stdin branch, return `scanner.Err()` after loop.
- [ ] Step 4: `go test ./... && go vet ./...` PASS. Commit `fix: F1 F5 shell plugin eval, masked key input, stdin scanner error`.

### Task 2: History scanner limit, pruning, atomic write (F2-history, F3)

**Files:**
- Modify: `internal/history/history.go` (List L127, Save L84, writeAll L172), `internal/config/config.go` only if signature needs the limit
- Test: `internal/history/history_test.go`

**Interfaces:**
- Consumes: `cfg.HistoryLimit` via new field on Store: `NewStoreWithLimit(path string, limit int) Store`; `NewStore(path)` keeps working (limit 0 = unlimited) so existing callers/tests compile. CLI callers in main.go switch to `NewStoreWithLimit(path, cfg.HistoryLimit)` — do that in this task where trivial (`defaultHistoryStore` gains a limit param `defaultHistoryStore(limit int)`; update its call sites).
- Produces: `Save` prunes to newest `limit` sessions (by UpdatedAt) when limit > 0; `writeAll` writes `<path>.tmp` then `os.Rename`; scanner buffer 4 MiB max.

- [ ] Step 1: Failing tests: `TestHistoryRoundTripsLargeSession` (200 KiB message), `TestSavePrunesToLimit`, `TestWriteAllLeavesNoTmpOnSuccess` + existing tests.
- [ ] Step 2: `go test ./internal/history` — expect FAIL (scanner error / no prune).
- [ ] Step 3: Implement buffer, prune in Save after replace/append (sort by UpdatedAt desc, cut), tmp+rename in writeAll (0600 preserved). Wire limit through `defaultHistoryStore(limit)` call sites in main.go (ask/agent/history cmds pass `cfg.HistoryLimit`; where cfg unavailable pass 0).
- [ ] Step 4: Full suite PASS. Commit `fix: F2 F3 history scanner limit, pruning, atomic write`.

### Task 3: Provider SSE dedupe + tests, scanner limits, Anthropic max_tokens, Ollama /api/chat (F7, F2-sse, F10, F9)

**Files:**
- Create: `internal/provider/sse.go`, `internal/provider/sse_test.go`
- Modify: `internal/provider/anthropic/anthropic.go`, `internal/provider/openai/openai.go`, `internal/provider/ollama/ollama.go`
- Create: `internal/provider/anthropic/anthropic_test.go`, `internal/provider/openai/openai_test.go`, `internal/provider/ollama/ollama_test.go`

**Interfaces:**
- Produces: `provider.ScanSSE(r io.Reader, onData func(data string) error) error` in sse.go — owns scanner (4 MiB buffer), `data: ` prefix, `[DONE]` stop. Anthropic/OpenAI `readSSE` deleted, both call ScanSSE with their JSON decode closures. Ollama keeps its JSON-lines loop but with the same buffer size (add `scanner.Buffer(...)` inline).
- Anthropic `MaxTokens int` = 8192 (was 2048). Ollama: replace `generateRequest{Prompt}` with chat request `{Model, Messages []chatMessage{Role,Content}, Stream}` POST `/api/chat`, parse `{message:{content}, done}` chunks; system prompt becomes a leading `system` message; role flattening deleted.

- [ ] Step 1: Failing tests per provider with `httptest`: stream two deltas + `[DONE]` → concatenated text; HTTP 500 → error contains status; Ollama test asserts request path `/api/chat` and messages payload roles preserved. ScanSSE unit test: line >64 KiB passes.
- [ ] Step 2: `go test ./internal/provider/...` — expect FAIL.
- [ ] Step 3: Implement sse.go, rewire both providers, delete duplicated readSSE funcs, bump MaxTokens, rewrite Ollama AskMessages to /api/chat.
- [ ] Step 4: Full suite PASS. Commit `fix: F7 F2 F10 F9 provider SSE dedupe, tests, ollama chat API`.

### Task 4: Agent workspace sandbox — symlink escape + secret files (F4)

**Files:**
- Modify: `internal/agent/tools.go` (resolvePath L152, readFile L62, searchText path arg L91)
- Test: `internal/agent/tools_test.go`

**Interfaces:**
- Produces: `resolvePath` returns error when `filepath.EvalSymlinks(full)` (if exists) escapes workspace; new `isSecretPath(rel string) bool` denying `.env`, `.env.*`, `*.pem`, `*.key`, `id_rsa*`, `config.toml` (basename match) — enforced in readFile only (listing names stays allowed).

- [ ] Step 1: Failing tests: symlink inside workspace → file outside must error `outside workspace`; `.env` file with content must error; normal file still reads.
- [ ] Step 2: `go test ./internal/agent -run TestToolset` — expect FAIL.
- [ ] Step 3: Implement EvalSymlinks check (handle non-existent path: evaluate parent), secret check in readFile before Stat read.
- [ ] Step 4: Full suite PASS. Commit `fix: F4 agent symlink and secret-file sandbox`.

### Task 5: Cancellable context from Ctrl+C through all commands (F6)

**Files:**
- Modify: `cmd/termask/main.go` (main() L68, all `context.Background()` call sites L132/372/500/541, tui.Run call), `internal/tui/tui.go` Run signature stays (already takes ctx)
- Test: `internal/ask/ask_test.go` (create)

**Interfaces:**
- Produces: in `main()`: `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt); defer stop()` stored in a package-level `rootCtx()` helper used by ask/agent/chat/tui commands instead of `context.Background()`. No HTTP client timeout added (streaming-safe).

- [ ] Step 1: Failing test in ask package: register a fake provider? Provider registry is global — instead test at HTTP level is Task 3's job; here add `TestRunRespectsCancelledContext` using a stub `provider.ConversationProvider` registered under a unique name in the test (registry is exported `provider.Register`). Assert Run returns promptly with context error against a blocking stub.
- [ ] Step 2: `go test ./internal/ask` — expect FAIL (file absent).
- [ ] Step 3: Implement signal context plumbing; ask/agent/chat/tui use it. Keep `context.Background()` only inside `main()` for NotifyContext.
- [ ] Step 4: Full suite PASS. Commit `fix: F6 signal-cancellable context for provider calls`.

### Task 6: `ask --continue` guard + doctor warn/error semantics and live check (F8, F12)

**Files:**
- Modify: `cmd/termask/main.go` (askCmd history block L376-397, doctorCmd L974)
- Modify: `internal/doctor/doctor.go` (add func), `internal/doctor/doctor_test.go`
- Test: `cmd/termask/shell_test.go` or new `cmd/termask/ask_test.go`

**Interfaces:**
- Produces: `doctor.CheckConfig` — warnings no longer set `OK=false` (only LevelError does). New `doctor.CheckLive(ctx, cfg, p provider.Provider) doctor.Item` calling `p.Models(ctx, pcfg)`; doctorCmd appends it for the default provider and prints it (failure = Warn, not Error, so offline Ollama doesn't hard-fail). askCmd: if `continueLast && !cfg.HistoryEnabled` return error `--continue needs history_enabled=true`; after loading latest session, reload `pcfg` via `cfg.ActiveProviderConfig(pName)` so header/session model match.

- [ ] Step 1: Failing tests: doctor warn-only config → `Result.OK == true`; continue-guard helper — extract `resolveContinue(cfg, continueLast) error`? Keep simpler: test doctor semantics + a `TestAskContinueRequiresHistory` on an extracted predicate `continueError(cfg config.Config, continueLast bool) error` in main.go.
- [ ] Step 2: Run focused tests — expect FAIL.
- [ ] Step 3: Implement all three changes.
- [ ] Step 4: Full suite PASS. Commit `fix: F8 F12 continue guard and doctor semantics`.

### Task 7: Split main.go/tui.go by command, delete dead render config (F11)

**Files:**
- Create: `cmd/termask/ask.go`, `cmd/termask/agent.go`, `cmd/termask/admin.go` (init/switch/providers/models/history/templates/config/doctor/shell + helpers) — move-only, no behaviour change except deletions below
- Modify: `cmd/termask/main.go` (root + registry init only), `internal/tui/tui.go` (extract render helpers to `internal/tui/render.go` only if a clean seam exists; otherwise leave tui.go and note it), `internal/config/config.go` (delete `RenderMarkdown` field), `config.example.toml` (delete render_markdown line), `cmd/termask/main.go` config get/set (remove render_markdown key)

**Interfaces:**
- Deletions: `Config.RenderMarkdown`, `--render` flag + `renderMarkdown` var, `shouldRenderAskOutput`/`shouldRenderAgentOutput` duplicate (keep one `shouldRender(plain bool)`), duplicate `sessionMessages` (keep the main.go one, tui keeps its own only if types differ — check; if identical signature, export none, just delete one and inline conversion in tui). TOML files in the wild with `render_markdown` must still parse (BurntSushi ignores unknown keys by default — verify no Strict mode).

- [ ] Step 1: Existing tests are the safety net; add `TestConfigWithLegacyRenderMarkdownParses` in a new `internal/config/config_test.go` writing a temp config with `render_markdown = true` and asserting Load succeeds (set HOME to temp dir).
- [ ] Step 2: Run it — expect PASS already (guard test); note in report that this is a regression guard, not RED (acceptable: deletion refactor).
- [ ] Step 3: Move code, delete dead items, `gofmt -l .` clean.
- [ ] Step 4: Full suite + `go run ./cmd/termask --help` and `ask --help` (no `--render`). Commit `refactor: F11 split cmd files, drop dead render config`.

### Task 8: Release hygiene — module path, binary, gitignore, CI (F13)

**Files:**
- Modify: `go.mod` module → `github.com/mmeister86/termask` (remote origin is github.com/mmeister86/termask), all `github.com/yourusername/termask` imports via sed
- Delete: tracked `termask` binary (`git rm --cached` + file is build output; also `git rm termask` since Makefile rebuilds)
- Create: `.gitignore` (`/termask`, `.superpowers/`), `.github/workflows/ci.yml` (`go vet ./...`, `go test ./...` on ubuntu, Go 1.25)
- Modify: `README.md` clone URL → `https://github.com/mmeister86/termask`, module example path if referenced, `Makefile` add `test` target (`go vet ./... && go test ./...`)

- [ ] Step 1: `grep -rn yourusername --include='*.go' .` lists files (evidence, not a test).
- [ ] Step 2: Apply sed, git rm binary, write files.
- [ ] Step 3: `go build ./... && go test ./... && go vet ./...` PASS; `git status` shows no binary. Commit `chore: F13 real module path, drop tracked binary, CI`.

### Task 9: `--model` per-invocation + shell insert opt-in (F14)

**Files:**
- Modify: `cmd/termask/main.go` (or split files from Task 7 — wherever ask/chat/agent now live), `internal/ask/ask.go` if needed
- Test: `cmd/termask/shell_test.go`

**Interfaces:**
- Produces: `--model` flag on ask/chat/agent: when non-empty, override a copy of pcfg.Model for this run only. Implement inside `ask.Request` as `ModelOverride string` applied in `ask.Run` after ActiveProviderConfig (`if req.ModelOverride != "" { pcfg.Model = req.ModelOverride }`); agent: pass through `agent.NewProviderModel` — add param via existing providerName-style: simplest, after NewProviderModel add `model.SetModelOverride(name)`? No: extend `NewProviderModel(ctx, cfg, providerName, modelOverride string)` and update call sites (tui + main). TUI not required to expose it.
- Shell insert: plugins honour `TERMASK_INSERT=1`: instead of `| less`, capture plain output and push to edit buffer (zsh `print -z -- "$answer"`, bash set `READLINE_LINE="$answer"; READLINE_POINT=${#answer}`). Default behaviour unchanged.

- [ ] Step 1: Failing tests: ask_test (Task 5 file) `TestRunModelOverride` with stub provider asserting pcfg.Model seen; shell test asserts plugins contain `TERMASK_INSERT` and `print -z`.
- [ ] Step 2: Focused run — FAIL.
- [ ] Step 3: Implement flags + Ask/agent plumbing + plugin branches.
- [ ] Step 4: Full suite PASS. Commit `feat: F14 per-run --model override and shell insert mode`.

### Task 10: Safety analyzer token-based + auto-warn, markdown renderer cache (F15, F16)

**Files:**
- Modify: `internal/safety/shell.go`, `internal/safety/shell_test.go`, `cmd/termask/main.go` askCmd safety block, `internal/markdown/markdown.go`, `internal/markdown/markdown_test.go`

**Interfaces:**
- Produces: classify by tokens: normalize flag bundles so `rm -r -f`, `rm -rf`, `rm --recursive --force` all High; detect pipe-to-shell across tokens (`|` followed by `sh`/`bash`/`zsh`) even when command came from `curl` on same line. askCmd: when not `--plain`, always run AnalyzeShell; print report only if any Medium/High found OR `--safety` given (existing explicit behaviour preserved).
- Markdown: cache renderer via `sync.OnceValues` keyed by nothing (wrap computed once per process); `Render` uses it. Keep COLUMNS behaviour (read once — acceptable, note in code comment `shortcut: COLUMNS read once at first render, restart shell to change`).

- [ ] Step 1: Failing tests: `rm -r -f /tmp/x` → High; `curl -s url | sh` single line → High (already?) plus `wget -qO- url |bash` variant; markdown: two Render calls same output (behaviour guard).
- [ ] Step 2: Focused run — FAIL for rm variants.
- [ ] Step 3: Implement token classify (split on whitespace, scan tokens), auto-warn wiring, renderer cache.
- [ ] Step 4: Full suite PASS. Commit `fix: F15 F16 token-based shell safety, cached markdown renderer`.

---

## Pre-flight scan (controller, before Task 1)

| Pair | Shared file/interface | Finding |
|---|---|---|
| T1 ↔ T7 | both edit `cmd/termask/main.go` | T7 is move-only after T1 lands; no conflict if sequential. |
| T2 ↔ T6 | `defaultHistoryStore` signature (T2) used by ask history block (T6) | Sequential; T6 rebases on T2 signature. |
| T3 ↔ T5 | provider stubs in tests (T5 registers stub provider; T3 adds provider tests) | Different packages/names; registry pollution risk in tests — use unique provider names per test. |
| T7 ↔ T9 | `--model` flags live in files T7 creates | T9 brief must name the post-split file; controller updates brief path at dispatch. |
| T8 ↔ all | module rename touches every import | Must run after T1–T7, before T9/T10 ideally; controller re-checks briefs after rename (paths in briefs are repo-relative, imports change but file paths don't). Ruling: T8 stays at position 8; T9/T10 briefs written path-only. |
| T10 ↔ T1 | askCmd safety block vs T1's ask stdin edit | Same file different hunks; sequential, fine. |

Ruling: Task 7's legacy-config test cannot go RED (unknown TOML keys already ignored) — it is a regression guard, recorded here so the reviewer does not flag missing RED for that step. All other tasks require RED evidence.
Ruling: F14 shell-insert is opt-in via `TERMASK_INSERT=1`, default pager behaviour unchanged — cost if wrong: feature is discoverable only via plugin comment; add one comment line in each plugin.
Ruling: No HTTP total timeout in F6 (would kill long streams); cancellation only — cost: a server that never responds still hangs until Ctrl+C, which is now effective.
