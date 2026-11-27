# Create a runner-agnostic `hybrid` user review profile with an agy adversarial peer

## Overview

Current `master` (`b04e45a`) already contains the native agy executor from `#60` and the shipped `comprehensive-plus` example from `#61`. The remaining goal is to install the updated local `revmux` binary and create a new runner-agnostic user profile named `hybrid` that combines the existing four Pi agents with a fifth `agy` adversarial peer, while keeping `~/.config/revmux/prompts/profiles/pi.md` untouched as a pure four-agent Pi fallback.

Scope:

- verify the current agy implementation, run the full Go/script baseline tests, build `.bin/revmux`, and update `/home/sasha/.local/bin/revmux` so the installed binary on `PATH` supports `agy` before enabling `hybrid`;
- create `/home/sasha/.config/revmux/prompts/profiles/hybrid.md` copied from the existing user `pi.md` with one added `adversarial-agy` roster entry;
- switch default profile in `/home/sasha/.config/revmux/config` from `profile = pi` to `profile = hybrid`, keeping `~/.config/revmux/prompts/profiles/pi.md` untouched as a pure Pi fallback;
- preserve the existing Pi runner, Kimi, GPT and GLM assignments and leave the persistent `max-parallel` configuration unchanged; pin only the smoke run to `--max-parallel=4`;
- prove the mixed five-source `hybrid` review end to end.

Acceptance criteria:

- the full automated baseline (`make test`, `go vet ./...`, `make build`) passes before any live user config is modified;
- `/home/sasha/.local/bin/revmux` points to the newly built binary and `revmux config` reports `agy` in the executor vocabulary;
- `revmux config` resolves default `profile = hybrid` (`source == "user"`), leaves `pi` untouched at four Pi agents, and resolves `hybrid` with five agents including `adversarial-agy` (executor `agy`, model `gemini-4-neon`, effort `high`);
- a real `hybrid` review accepts exit `0` or `1`, reports five expected and five reported sources, has no degraded sources, and records token usage plus a non-empty successful agy attempt, whether it succeeds initially or on retry.

Out of scope: executor rewrites, production code edits (including pre-existing `pi.go` lint/bug fixes), new embedded profiles, speculative rate-limit handling, changes to the existing `pi` profile or other models, raising `max-parallel`, commits and merges.

## Context

- `app/executor/agy.go`, `app/executor/agy_test.go` and `app/executor/testdata/agy-clean.jsonl` already implement and test the native stream-json/json-schema protocol.
- `app/prompt/roster.go` already accepts `claude`, `codex`, `agy` and `pi`; `app/revmux/main.go` already dispatches `agy` and applies its longer default idle timeout.
- `app/prompt/defaults/prompts/profiles/comprehensive-plus.md` is the working example of a second adversarial peer on agy.
- `.claude/rules/executor.md` documents the agy protocol, permissions and model grammar. The correct spelling is `agy/gemini-4-neon:high`: the bare model family plus `--effort`. Do **not** use `agy/gemini-4-neon-high:high`; a suffixed model combined with an effort can be rejected.
- `/home/sasha/.local/bin/revmux` is currently a standalone binary copy from `pi-executor` (`3dd938e`) whose executor vocabulary is `["claude", "codex", "pi"]`, and `~/.config/revmux/config` sets `profile = pi`. Updating `/home/sasha/.local/bin/revmux` before adding `hybrid.md` is required so ordinary `revmux` invocations outside this repo do not fail when loading the user prompt tree (which parses every profile in `~/.config/revmux/prompts/profiles/`).
- `/home/sasha/.local/bin/agy` is a wrapper script that exports `HTTP_PROXY`/`HTTPS_PROXY=http://127.0.0.1:2080` and invokes `/usr/bin/mise which agy`. Consequently, `agy --version`, `preflight.sh`, and the live smoke run must run outside a network/filesystem sandbox with `mise` and the local proxy `127.0.0.1:2080` reachable.
- `~/.config/revmux/prompts/profiles/pi.md` overrides the embedded `pi` profile with four Pi agents (`bugs+impl`, `arch+quality`, `docs+tests`, `adversarial`) plus GLM synthesis/verify. It remains untouched as a pure-Pi fallback; the new `~/.config/revmux/prompts/profiles/hybrid.md` starts from its exact content and adds `adversarial-agy`.
- The current built-in default is `max-parallel=6`, and the user configuration does not override it. The smoke invocation explicitly uses `--max-parallel=4`, so the fifth agent waits for a slot during this acceptance run; persistent configuration remains unchanged.
- `make lint` is deliberately omitted as a gate for this configuration-only plan: on the current `master` baseline, `golangci-lint` (in `~/go/bin`, not on `PATH`) flags a pre-existing `nestif` issue in `app/executor/pi.go:259`, and `shellcheck` is not installed on this host. Those are tracked as separate follow-ups in `Post-Completion`.

## Development Approach

- Use configuration-first validation: run all automated repository gates (`make test`, `go vet ./...`, `make build`) and install the updated binary **before** modifying any live user config or profile.
- Complete tasks in order and do not advance with a failing validation gate.
- Do not modify production code unless a baseline failure is independently reproduced and the user approves a scope change.
- If live acceptance fails and cannot be immediately resolved, restore `/home/sasha/.config/revmux/config` from its `/tmp` backup (back to `profile = pi`) and remove `/home/sasha/.config/revmux/prompts/profiles/hybrid.md` so the live configuration does not remain in an unverified state.
- Update this plan if execution uncovers a verified gap; do not silently turn the smoke run into an executor redesign.

## Testing Strategy

- Run the full repository test suite (`make test`, covering both Go race/coverage tests and skill script tests), `go vet ./...`, and `make build` before modifying user config.
- Verify the shipped `comprehensive-plus` roster, the unchanged user `pi` roster, the new `hybrid` roster, and the resolved default `profile = hybrid` knob through machine-readable `revmux config` output from the installed `revmux` binary.
- Run one real mixed `hybrid` review pinned to the small commit range `54378e3..b04e45a` outside any network/filesystem sandbox; validate its JSON report and archived agy stream.
- Human judgment of review quality is recorded separately rather than inferred from a successful process exit.

## Solution Overview

Reuse the current native agy executor, install the current `master` binary into `~/.local/bin/revmux`, create the runner-agnostic user profile `hybrid` with an independent `agy` adversarial peer, and set `profile = hybrid` in `~/.config/revmux/config`. This follows the shipped `comprehensive-plus` pattern, leaves `pi` intact as a four-agent Pi fallback, and allows mixing future runners under `hybrid` without renaming the profile again.

Rejected alternatives:

- keeping the mixed profile named `pi`: ties the profile name to a single runner even though it now combines multiple executors (`pi`, `agy`, and potentially others later);
- implementing another agy executor: redundant with `#60` and risks diverging from upstream;
- adding a new embedded profile: unnecessary because user-layer `~/.config/revmux/prompts/profiles/hybrid.md` is the intended personal default;
- leaving `/home/sasha/.local/bin/revmux` on the old `3dd938e` binary: would immediately break every normal `revmux` call once `hybrid.md` references `agy`;
- using a prose wrapper or an effort-suffixed model with a second effort: loses protocol guarantees or produces an invalid model selection;
- increasing parallelism or changing unrelated models: adds cost and makes the first integration run harder to evaluate.

## Technical Details

The new `/home/sasha/.config/revmux/prompts/profiles/hybrid.md` preserves the four existing Pi agents and stages from `pi.md` and appends one new roster entry:

```yaml
  - {name: adversarial-agy, lenses: [adversarial], model: "agy/gemini-4-neon:high", color: bright-yellow}
```

Keep the existing `adversarial` name for the GPT reviewer so prior source names remain stable. The agy process is an independent source: findings from `adversarial` and `adversarial-agy` can corroborate one another, while two lenses emitted by either single process cannot self-corroborate.

`/home/sasha/.config/revmux/config` switches its default profile line from `profile = pi` to `profile = hybrid` (with its comment updated accordingly).

The existing executor supplies stream-json input/output, native stage schemas, narration, raw `.jsonl` archives and a permissions overlay denying `write_file(*)`. Its known limitations remain as documented in `.claude/rules/executor.md`; this plan does not claim a stronger read-only sandbox or agy-specific rate-limit detection.

## Implementation Steps

### Task 1: Verify the agy baseline, run repo gates, and install the updated binary

**Files:**
- Read/verify: `app/executor/agy.go`
- Read/verify: `app/executor/agy_test.go`
- Read/verify: `app/prompt/defaults/prompts/profiles/comprehensive-plus.md`
- Read/verify: `app/revmux/main.go`
- Create outside the repository: a timestamped backup of the previous `/home/sasha/.local/bin/revmux` under `/tmp`
- Modify outside the repository: `/home/sasha/.local/bin/revmux` (replaced with symlink to `.bin/revmux` via `make install`)

- [x] Record `git status --short --branch`; preserve all unrelated work and expect no source change from this plan.
- [x] Run outside the sandbox (with `mise` and local proxy `127.0.0.1:2080` reachable): confirm `command -v agy` succeeds and `agy --version` reports an installed CLI.
- [x] Run `make test` and `go vet ./...` and require both to pass before touching any user config or installed binary.
- [x] Run `make build` and require `.bin/revmux` to build successfully.
- [x] Back up the existing `/home/sasha/.local/bin/revmux` to a timestamped file under `/tmp`, then run `BINDIR="$HOME/.local/bin" make install` so `/home/sasha/.local/bin/revmux` symlinks to `$PWD/.bin/revmux`.
- [x] Run `revmux config | jq -e '.vocabulary.executors == ["claude", "codex", "agy", "pi"]'` (using `revmux` directly from `PATH`, not a `PATH` override) and require a match.
- [x] Run `revmux config | jq -e '.profiles[] | select(.name == "comprehensive-plus") | .roster[] | select(.name == "adversarial-agy" and .executor == "agy" and .model == "gemini-4-neon" and .effort == "high")'` and require a match.

### Task 2: Create the `hybrid` user profile and set it as default

**Files:**
- Read/preserve unchanged: `/home/sasha/.config/revmux/prompts/profiles/pi.md`
- Create outside the repository: `/home/sasha/.config/revmux/prompts/profiles/hybrid.md`
- Modify outside the repository: `/home/sasha/.config/revmux/config`
- Create outside the repository: a timestamped backup of the previous `/home/sasha/.config/revmux/config` under `/tmp`

- [x] Back up `/home/sasha/.config/revmux/config` to a newly created timestamped file under `/tmp` before editing it.
- [x] Create `/home/sasha/.config/revmux/prompts/profiles/hybrid.md` from `/home/sasha/.config/revmux/prompts/profiles/pi.md`: update `description:` to describe the multi-runner hybrid panel (four Pi agents plus an agy adversarial peer) and add the `adversarial-agy` entry immediately after `adversarial`; leave every existing model, effort, lens, stage, color and body unchanged.
- [x] Update `/home/sasha/.config/revmux/config` to set `profile = hybrid` and adjust its header comment; do not set `max-parallel` or any other knob.
- [x] Run `revmux --config-dir /home/sasha/.config/revmux config | jq -e '(.knobs[] | select(.name == "profile" and .value == "hybrid" and .source == "user")) and (.profiles[] | select(.name == "hybrid") | .roster as $r | ($r | length == 5) and any($r[]; .name == "adversarial-agy" and .executor == "agy" and .model == "gemini-4-neon" and .effort == "high" and .lenses == ["adversarial"])) and (.profiles[] | select(.name == "pi") | .roster | length == 4)'` and require it to return `true`.
- [x] Diff `hybrid.md` against `pi.md` to confirm that the only differences are the `description:` line and the single added `adversarial-agy` entry, and confirm `pi.md` itself was not modified.

### Task 3: Final automated acceptance

**Files:**
- Read/verify: `/home/sasha/.config/revmux/prompts/profiles/hybrid.md`
- Read/verify: `/home/sasha/.config/revmux/config`
- Create through revmux: the smoke archive under a fresh timestamped directory `/tmp/revmux-agy-smoke-<timestamp>` (so a retry after a partial failure never collides with `task.CheckReclaim`)
- Read/verify: `/tmp/revmux-agy-smoke-<timestamp>-report.json`
- Read/verify: `/tmp/revmux-agy-smoke-<timestamp>.log`

- [x] Run outside the sandbox (with `mise` and proxy `127.0.0.1:2080` reachable): `bash .claude-plugin/skills/revmux/scripts/preflight.sh hybrid --config-dir /home/sasha/.config/revmux`; verify it resolves the newly installed `/home/sasha/.local/bin/revmux` and that every needed binary (`pi`, `agy`) passes.
- [x] Also run `bash .claude-plugin/skills/revmux/scripts/preflight.sh --config-dir /home/sasha/.config/revmux` (without a profile argument) to verify the resolved default profile is `hybrid`.
- [x] Run `revmux --config-dir /home/sasha/.config/revmux --max-parallel=4 config | jq -e '.knobs[] | select(.name == "max-parallel" and .value == 4 and .source == "flag")'` and require the flag-resolved knob record.
- [x] Pick a fresh timestamped tasks dir `SMOKE_DIR="/tmp/revmux-agy-smoke-$(date +%Y%m%d-%H%M%S)"` and create task `hybrid-agy-smoke`, round `01-initial`, with `revmux --tasks-dir "$SMOKE_DIR" --task hybrid-agy-smoke --run 01-initial --config-dir /home/sasha/.config/revmux new`; write the scope and goal only to paths printed by `new`. (If a rerun is needed after a partial failure, use a new `--run` name or fresh `SMOKE_DIR`.)
- [x] Scope the smoke review to `git diff 54378e3..b04e45a`, name the profile rename and its five affected files, and state that finding nothing is valid.
- [x] Run the round outside the sandbox with explicit flags: `revmux --tasks-dir "$SMOKE_DIR" --task hybrid-agy-smoke --run 01-initial --config-dir /home/sasha/.config/revmux --profile hybrid --no-tui --max-parallel=4 >"${SMOKE_DIR}-report.json" 2>"${SMOKE_DIR}.log"`; accept exit `0` or `1` as completion and treat `2` as a tool failure.
- [x] Require `jq -e '.sources.expected == 5 and .sources.reported == 5 and (.sources.degraded | length == 0)' "${SMOKE_DIR}-report.json"` to return `true`.
- [x] Require `jq -e '.sources.agents[] | select(.name == "adversarial-agy" and .executor == "agy" and .requested_model == "gemini-4-neon" and .effort == "high" and (.actual_model == "gemini-4-neon" or .actual_model == "gemini-4-neon-high") and .tokens > 0 and .degraded == false)' "${SMOKE_DIR}-report.json"` to return a record.
- [x] Require `prompts/agents/adversarial-agy.md` to be non-empty and contain the narration contract. Select `agents/adversarial-agy.retry.jsonl` when it exists and `agents/adversarial-agy.jsonl` otherwise; require that successful-attempt stream to be non-empty and contain a terminal `result` with `structured_output`. Do not reject a healthy retried source because its first-attempt stream is empty or truncated.
- [x] Re-run the resolved-profile assertion from Task 2 and confirm that no smoke setup changed the persistent config, `hybrid.md` or `pi.md`.
- [x] Rollback rule: if any live acceptance gate fails and is not resolved within this run, restore `/home/sasha/.config/revmux/config` from its `/tmp` backup, remove `/home/sasha/.config/revmux/prompts/profiles/hybrid.md`, and confirm `revmux config` resolves the restored four-agent `pi` default cleanly.

## Post-Completion

- Human acceptance remains pending until the user inspects the agy activity and the final report for usefulness and independence; process success alone does not prove review quality.
- Keep the smoke archive and log for diagnosis. Do not commit the user profile, smoke artifacts or this plan unless separately authorized.
- After human acceptance, update the local skill description/instructions in `~/.gemini/config/skills/revmux/SKILL.md` (and `~/.gemini/config/skills/planning-exec/SKILL.md` if it pins `--profile pi`) to reflect the new default `hybrid` profile alongside the `pi` fallback.
- Separate repository follow-ups (out of scope for this plan): fix the pre-existing `nestif` lint issue in `app/executor/pi.go:259`, fix the value-copied `strings.Builder` (`a := acc.line`) in `app/executor/pi.go:182` that drops `pi` `text_delta` activity lines, and install `shellcheck` + put `~/go/bin` on `PATH` so `make lint` passes cleanly.
- If live acceptance exposes an executor defect, record the evidence and open a new scoped plan rather than broadening this configuration-only change.
