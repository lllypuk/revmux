# Add an agy adversarial peer to the local pi review profile

## Overview

Current `master` (`b04e45a`) already contains the native agy executor from `#60` and the shipped `comprehensive-plus` example from `#61`. The remaining goal is to install the updated local `revmux` binary and enable an agy reviewer in the user's overriding `pi` profile without rewriting the executor or changing the existing model assignments.

Scope:

- verify the current agy implementation, run the full Go/script baseline tests, build `.bin/revmux`, and update `/home/sasha/.local/bin/revmux` so the installed binary on `PATH` supports `agy` before editing the live user profile;
- add one `adversarial-agy` roster entry to `~/.config/revmux/prompts/profiles/pi.md`;
- preserve the existing Pi runner, Kimi, GPT and GLM assignments and leave the persistent `max-parallel` configuration unchanged; pin only the smoke run to `--max-parallel=4`;
- prove the mixed five-source review end to end.

Acceptance criteria:

- the full automated baseline (`make test`, `go vet ./...`, `make build`) passes before any live user config is modified;
- `/home/sasha/.local/bin/revmux` points to the newly built binary and `revmux config` reports `agy` in the executor vocabulary;
- the resolved user `pi` profile has five agents, including `adversarial-agy` with executor `agy`, model `gemini-4-neon` and effort `high`;
- a real review accepts exit `0` or `1`, reports five expected and five reported sources, has no degraded sources, and records token usage plus a non-empty successful agy attempt, whether it succeeds initially or on retry.

Out of scope: executor rewrites, production code edits (including pre-existing `pi.go` lint/bug fixes), new embedded profiles, speculative rate-limit handling, changes to the other models, raising `max-parallel`, commits and merges.

## Context

- `app/executor/agy.go`, `app/executor/agy_test.go` and `app/executor/testdata/agy-clean.jsonl` already implement and test the native stream-json/json-schema protocol.
- `app/prompt/roster.go` already accepts `claude`, `codex`, `agy` and `pi`; `app/revmux/main.go` already dispatches `agy` and applies its longer default idle timeout.
- `app/prompt/defaults/prompts/profiles/comprehensive-plus.md` is the working example of a second adversarial peer on agy.
- `.claude/rules/executor.md` documents the agy protocol, permissions and model grammar. The correct spelling is `agy/gemini-4-neon:high`: the bare model family plus `--effort`. Do **not** use `agy/gemini-4-neon-high:high`; a suffixed model combined with an effort can be rejected.
- `/home/sasha/.local/bin/revmux` is currently a standalone binary copy from `pi-executor` (`3dd938e`) whose executor vocabulary is `["claude", "codex", "pi"]`, and `~/.config/revmux/config` sets `profile = pi`. Updating `/home/sasha/.local/bin/revmux` before adding `agy` to `~/.config/revmux/prompts/profiles/pi.md` is required so ordinary `revmux` invocations outside this repo do not break on prompt load.
- `/home/sasha/.local/bin/agy` is a wrapper script that exports `HTTP_PROXY`/`HTTPS_PROXY=http://127.0.0.1:2080` and invokes `/usr/bin/mise which agy`. Consequently, `agy --version`, `preflight.sh`, and the live smoke run must run outside a network/filesystem sandbox with `mise` and the local proxy `127.0.0.1:2080` reachable.
- `~/.config/revmux/prompts/profiles/pi.md` overrides the embedded `pi` profile. Preserve its current `bugs+impl`, `arch+quality`, `docs+tests`, `adversarial`, synthesis and verify entries unchanged.
- The current built-in default is `max-parallel=6`, and the user configuration does not override it. The smoke invocation explicitly uses `--max-parallel=4`, so the fifth agent waits for a slot during this acceptance run; persistent configuration remains unchanged.
- `make lint` is deliberately omitted as a gate for this configuration-only plan: on the current `master` baseline, `golangci-lint` (in `~/go/bin`, not on `PATH`) flags a pre-existing `nestif` issue in `app/executor/pi.go:259`, and `shellcheck` is not installed on this host. Those are tracked as separate follow-ups in `Post-Completion`.

## Development Approach

- Use configuration-first validation: run all automated repository gates (`make test`, `go vet ./...`, `make build`) and install the updated binary **before** modifying any live user profile.
- Complete tasks in order and do not advance with a failing validation gate.
- Do not modify production code unless a baseline failure is independently reproduced and the user approves a scope change.
- If live acceptance fails and cannot be immediately resolved, restore `/home/sasha/.config/revmux/prompts/profiles/pi.md` from its `/tmp` backup so the live profile does not remain in an unverified state.
- Update this plan if execution uncovers a verified gap; do not silently turn the smoke run into an executor redesign.

## Testing Strategy

- Run the full repository test suite (`make test`, covering both Go race/coverage tests and skill script tests), `go vet ./...`, and `make build` before modifying the user profile.
- Verify both the shipped `comprehensive-plus` roster and the resolved user `pi` roster through machine-readable `revmux config` output from the installed `revmux` binary.
- Run one real mixed review pinned to the small commit range `54378e3..b04e45a` outside any network/filesystem sandbox; validate its JSON report and archived agy stream.
- Human judgment of review quality is recorded separately rather than inferred from a successful process exit.

## Solution Overview

Reuse the current native agy executor, install the current `master` binary into `~/.local/bin/revmux`, and add a single independent adversarial process to the user profile. This follows the shipped `comprehensive-plus` pattern and preserves synthesis, verification, attribution and retry behavior.

Rejected alternatives:

- implementing another agy executor: redundant with `#60` and risks diverging from upstream;
- adding a new embedded profile: unnecessary because the existing user `pi` override is the intended target;
- leaving `/home/sasha/.local/bin/revmux` on the old `3dd938e` binary: would immediately break every normal `revmux` call once `pi.md` references `agy`;
- using a prose wrapper or an effort-suffixed model with a second effort: loses protocol guarantees or produces an invalid model selection;
- increasing parallelism or changing unrelated models: adds cost and makes the first integration run harder to evaluate.

## Technical Details

The new roster entry is:

```yaml
  - {name: adversarial-agy, lenses: [adversarial], model: "agy/gemini-4-neon:high", color: bright-yellow}
```

Keep the existing `adversarial` name for the GPT reviewer so prior source names remain stable. The agy process is an independent source: findings from `adversarial` and `adversarial-agy` can corroborate one another, while two lenses emitted by either single process cannot self-corroborate.

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

- [ ] Record `git status --short --branch`; preserve all unrelated work and expect no source change from this plan.
- [ ] Run outside the sandbox (with `mise` and local proxy `127.0.0.1:2080` reachable): confirm `command -v agy` succeeds and `agy --version` reports an installed CLI.
- [ ] Run `make test` and `go vet ./...` and require both to pass before touching any user config or installed binary.
- [ ] Run `make build` and require `.bin/revmux` to build successfully.
- [ ] Back up the existing `/home/sasha/.local/bin/revmux` to a timestamped file under `/tmp`, then run `BINDIR="$HOME/.local/bin" make install` so `/home/sasha/.local/bin/revmux` symlinks to `$PWD/.bin/revmux`.
- [ ] Run `revmux config | jq -e '.vocabulary.executors == ["claude", "codex", "agy", "pi"]'` (using `revmux` directly from `PATH`, not a `PATH` override) and require a match.
- [ ] Run `revmux config | jq -e '.profiles[] | select(.name == "comprehensive-plus") | .roster[] | select(.name == "adversarial-agy" and .executor == "agy" and .model == "gemini-4-neon" and .effort == "high")'` and require a match.

### Task 2: Add the agy peer to the user pi profile

**Files:**
- Modify: `/home/sasha/.config/revmux/prompts/profiles/pi.md`
- Create outside the repository: a timestamped backup of the previous profile under `/tmp`

- [ ] Back up the current profile to a newly created timestamped file under `/tmp` before editing it.
- [ ] Change the description to state that the profile has four Pi agents plus an agy adversarial peer.
- [ ] Add the agreed `adversarial-agy` entry immediately after the existing `adversarial` entry; leave every existing model, effort, lens, stage and color unchanged.
- [ ] Run `revmux --config-dir /home/sasha/.config/revmux config | jq -e '.profiles[] | select(.name == "pi") | .roster as $r | ($r | length == 5) and any($r[]; .name == "adversarial-agy" and .executor == "agy" and .model == "gemini-4-neon" and .effort == "high" and .lenses == ["adversarial"])'` and require it to return `true`.
- [ ] Compare the edited profile with its backup and verify that the only roster addition is `adversarial-agy` apart from the description change.

### Task 3: Final automated acceptance

**Files:**
- Read/verify: `/home/sasha/.config/revmux/prompts/profiles/pi.md`
- Create through revmux: the smoke archive under a fresh timestamped directory `/tmp/revmux-agy-smoke-<timestamp>` (so a retry after a partial failure never collides with `task.CheckReclaim`)
- Read/verify: `/tmp/revmux-agy-smoke-<timestamp>-report.json`
- Read/verify: `/tmp/revmux-agy-smoke-<timestamp>.log`

- [ ] Run outside the sandbox (with `mise` and proxy `127.0.0.1:2080` reachable): `bash .claude-plugin/skills/revmux/scripts/preflight.sh pi --config-dir /home/sasha/.config/revmux`; verify it resolves the newly installed `/home/sasha/.local/bin/revmux` and that every needed binary (`pi`, `agy`) passes.
- [ ] Run `revmux --config-dir /home/sasha/.config/revmux --max-parallel=4 config | jq -e '.knobs[] | select(.name == "max-parallel" and .value == 4 and .source == "flag")'` and require the flag-resolved knob record.
- [ ] Pick a fresh timestamped tasks dir `SMOKE_DIR="/tmp/revmux-agy-smoke-$(date +%Y%m%d-%H%M%S)"` and create task `pi-agy-smoke`, round `01-initial`, with `revmux --tasks-dir "$SMOKE_DIR" --task pi-agy-smoke --run 01-initial --config-dir /home/sasha/.config/revmux new`; write the scope and goal only to paths printed by `new`. (If a rerun is needed after a partial failure, use a new `--run` name or fresh `SMOKE_DIR`.)
- [ ] Scope the smoke review to `git diff 54378e3..b04e45a`, name the profile rename and its five affected files, and state that finding nothing is valid.
- [ ] Run the round outside the sandbox with explicit flags: `revmux --tasks-dir "$SMOKE_DIR" --task pi-agy-smoke --run 01-initial --config-dir /home/sasha/.config/revmux --profile pi --no-tui --max-parallel=4 >"${SMOKE_DIR}-report.json" 2>"${SMOKE_DIR}.log"`; accept exit `0` or `1` as completion and treat `2` as a tool failure.
- [ ] Require `jq -e '.sources.expected == 5 and .sources.reported == 5 and (.sources.degraded | length == 0)' "${SMOKE_DIR}-report.json"` to return `true`.
- [ ] Require `jq -e '.sources.agents[] | select(.name == "adversarial-agy" and .executor == "agy" and .requested_model == "gemini-4-neon" and .effort == "high" and (.actual_model == "gemini-4-neon" or .actual_model == "gemini-4-neon-high") and .tokens > 0 and .degraded == false)' "${SMOKE_DIR}-report.json"` to return a record.
- [ ] Require `prompts/agents/adversarial-agy.md` to be non-empty and contain the narration contract. Select `agents/adversarial-agy.retry.jsonl` when it exists and `agents/adversarial-agy.jsonl` otherwise; require that successful-attempt stream to be non-empty and contain a terminal `result` with `structured_output`. Do not reject a healthy retried source because its first-attempt stream is empty or truncated.
- [ ] Re-run the resolved-profile assertion from Task 2 and confirm that no smoke setup changed the persistent profile.
- [ ] Rollback rule: if any live acceptance gate fails and is not resolved within this run, restore `/home/sasha/.config/revmux/prompts/profiles/pi.md` from its `/tmp` backup and confirm `revmux config` resolves the restored four-agent profile cleanly.

## Post-Completion

- Human acceptance remains pending until the user inspects the agy activity and the final report for usefulness and independence; process success alone does not prove review quality.
- Keep the smoke archive and log for diagnosis. Do not commit the user profile, smoke artifacts or this plan unless separately authorized.
- After human acceptance, update the local skill description in `~/.gemini/config/skills/revmux/SKILL.md` so it no longer states that the local `pi` profile runs only four Pi agents.
- Separate repository follow-ups (out of scope for this plan): fix the pre-existing `nestif` lint issue in `app/executor/pi.go:259`, fix the value-copied `strings.Builder` (`a := acc.line`) in `app/executor/pi.go:182` that drops `pi` `text_delta` activity lines, and install `shellcheck` + put `~/go/bin` on `PATH` so `make lint` passes cleanly.
- If live acceptance exposes an executor defect, record the evidence and open a new scoped plan rather than broadening this configuration-only change.
