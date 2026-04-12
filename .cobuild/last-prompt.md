# Task: Switch penfold pipeline config to built-in auto review

**Task ID:** pf-a30052
**Agent:** 

## Task Content

## Scope
Update the penfold repo’s `.cobuild/pipeline.yaml` to use built-in auto review instead of depending on Gemini. Keep the repo’s existing build, test, and deploy settings intact while adding the review block needed for cross-model review and PR comment posting.

## Acceptance Criteria
- [ ] `/Users/james/github/otherjamesbrown/penfold/.cobuild/pipeline.yaml` sets review to `provider: auto` with cross-model review enabled.
- [ ] Existing penfold build, test, and deploy settings remain unchanged outside the review block.
- [ ] The final config is valid YAML.
- [ ] Tests pass: `make test && make vet`
- [ ] Build passes: `make build`

## Code Locations
- `.cobuild/pipeline.yaml` — replace the repo’s inherited broken Gemini default with built-in auto review.

## Wave
4

## Notes
Target project is `penfold`, target repo is `penfold`. Blocked by the cobuild `process-review` integration task.

## Design Context (from cb-32ed2a)

**Built-in cross-model PR review — replace broken Gemini integration**

## Problem

CoBuild's PR review step depends on Gemini Code Review, an external GitHub integration that posts review comments on PRs. As of 2026-04-12, Gemini has stopped reviewing PRs entirely — 11 consecutive PRs across cobuild and penfold received zero reviews. Every PR was merged via the CI-based fallback (CI passes → approve), which is effectively no review at all.

The external dependency is fragile: we can't debug it, can't configure it, and can't control which model reviews the code. When it breaks, the entire quality gate disappears silently.

## Goal

Replace the external Gemini dependency with a built-in LLM review step that CoBuild controls end-to-end. The review model should be different from the model that wrote the code (cross-model review catches different classes of bugs).

## Primary user

The orchestrator agent running \`cobuild process-review\`, and any developer who wants confidence that PRs are actually reviewed before merge.

## Success criteria

1. \`cobuild process-review <task-id>\` calls an LLM with the PR diff + task spec + parent design context and gets a structured approve/request-changes verdict with findings.
2. Cross-model by default: code written by Codex (gpt-5.4) is reviewed by Claude (sonnet), and vice versa. Configurable override.
3. Review findings are posted as a PR comment (visible in GitHub) and recorded in the pipeline gate.
4. If the review model is unavailable, falls back to CI-only (current behavior) with a warning.
5. Existing \`gemini-github\` provider still works as an option for repos that have a working Gemini integration.
6. Review completes in under 2 minutes for typical PRs (< 500 lines changed).

## Non-goals

- Building a full code review UI. The review is a single LLM call that produces a structured verdict, not an interactive review session.
- Replacing human review. This is an automated first pass. Human review can still be required via GitHub branch protection.
- Supporting every LLM provider. v1 supports claude (Anthropic API) and openai (for Codex-model review). Ollama/local is a future enhancement.
- Reviewing PRs outside CoBuild's pipeline. This is for \`cobuild process-review\`, not a general GitHub review bot.

## Technical approach

### New package: internal/review/

A \`Reviewer\` interface with provider implementations:

\`\`\`go
type ReviewResult struct {
    Verdict  string   // "approve" or "request-changes"
    Findings []Finding
    Summary  string
}

type Finding struct {
    File     string
    Line     int
    Severity string // "critical", "suggestion", "nit"
    Body     string
}

type Reviewer interface {
    Review(ctx context.Context, input ReviewInput) (*ReviewResult, error)
}
\`\`\`

Providers:
- \`claude.go\` — calls Anthropic API (claude-sonnet-4-6 default). Uses the Go SDK (\`github.com/anthropics/anthropic-sdk-go\`).
- \`openai.go\` — calls OpenAI API (gpt-5.4 default). Uses the Go SDK (\`github.com/openai/openai-go\`).
- \`external.go\` — waits for external review comments (current Gemini behavior, preserved for backwards compat).

### ReviewInput construction

\`process-review\` already fetches the PR diff via \`gh pr diff\`. The review input combines:
- PR diff (from \`gh pr diff <number>\`)
- Task spec (from the work item content)
- Parent design context (from the parent design shard)
- Acceptance criteria extracted from the task

### Cross-model resolution

The dispatch runtime and model are recorded in \`pipeline_sessions\`. \`process-review\` reads the session record to determine what model wrote the code, then picks the opposite family:

| Code written by | Reviewed by |
|----------------|-------------|
| codex / gpt-* | claude / sonnet |
| claude-code / claude-* | openai / gpt-5.4 |
| unknown | claude / sonnet (default) |

Override via \`review.model\` in pipeline.yaml if you want a specific reviewer.

### Config

\`\`\`yaml
review:
    provider: auto            # auto | claude | openai | external
    model: ""                 # empty = cross-model auto-select
    cross_model: true         # pick opposite model family from dispatch
    post_comments: true       # post findings as PR comments
    ci_mode: pr-only
    wait_for_ci: true
    timeout: 120s             # max time for LLM review call
\`\`\`

\`provider: auto\` (default) uses cross-model selection. \`provider: external\` preserves the current wait-for-Gemini behavior.

### Integration with process-review

In \`internal/cmd/review.go\`, the flow becomes:

1. Fetch PR diff and CI status (existing)
2. If \`provider != external\`: call the built-in reviewer with diff + task context
3. Post findings as PR comment via \`gh pr comment\`
4. Record gate verdict from reviewer result
5. If approve + CI green: merge
6. If request-changes: append findings to task, re-dispatch

### API keys

- Anthropic: \`ANTHROPIC_API_KEY\` env var (already set on dev01 for Claude Code)
- OpenAI: \`OPENAI_API_KEY\` env var (already set for Codex)

No new credential management needed.

### Review prompt

The prompt follows a structured template:

\`\`\`
You are reviewing a PR for a CoBuild pipeline task.

## Task
{task title and spec}

## Parent Design
{design context, if any}

## PR Diff
{full diff}

## Instructions
Review this PR against its task spec. For each issue found, report:
- File and line number
- Severity: critical (blocks merge), suggestion (should fix), nit (optional)
- What's wrong and how to fix it

If the code correctly implements the spec with no critical issues, approve.
Output JSON: {"verdict": "approve"|"request-changes", "findings": [...], "summary": "..."}
\`\`\`

## Test strategy

1. Unit test for cross-model resolution: verify codex-dispatched tasks get claude reviewer, claude-dispatched get openai.
2. Unit test for ReviewInput construction: verify diff, task spec, and design context are correctly assembled.
3. Integration test: mock the LLM API, verify process-review posts a comment and records the gate.
4. Regression test: \`provider: external\` still waits for GitHub review comments (current behavior).
5. End-to-end: run a task through CoBuild with \`provider: auto\`, verify the cross-model review happens and findings are posted.

## Acceptance criteria

1. \`cobuild process-review\` with \`provider: auto\` calls an LLM and gets a verdict without waiting for Gemini.
2. Cross-model selection works: codex PRs reviewed by claude, claude PRs reviewed by openai.
3. Findings posted as GitHub PR comment with file/line references.
4. Gate recorded in pipeline_gates with the LLM review verdict and findings.
5. Fallback to CI-only works when the review model API is unreachable.
6. \`provider: external\` still works for repos with Gemini.
7. All other CoBuild-using projects (penfold, context-palace, penf-cli, mycroft) updated to use \`provider: auto\`.

## Work location

- \`internal/review/review.go\` — Reviewer interface, ReviewInput, ReviewResult types
- \`internal/review/claude.go\` — Anthropic API implementation
- \`internal/review/openai.go\` — OpenAI API implementation
- \`internal/review/external.go\` — current Gemini-wait behavior (extracted from review.go)
- \`internal/review/cross_model.go\` — model family detection and cross-model selection
- \`internal/cmd/review.go\` — integrate Reviewer into process-review flow
- \`internal/config/config.go\` — review config fields
- Pipeline.yaml in cobuild, penfold, context-palace, penf-cli, mycroft repos

## Dependencies

- \`github.com/anthropics/anthropic-sdk-go\` — Anthropic API client
- \`github.com/openai/openai-go\` — OpenAI API client
- Both are standard Go SDKs, no vendoring issues expected.

## Related

- cb-a3bf71 (CoBuild v0.1)
- Gemini outage observed 2026-04-12: 11 PRs across cobuild/penfold with zero reviews

---
*Appended by agent-mycroft at 2026-04-12 10:32 UTC*

## Decomposition

Spec to target map:
- Built-in reviewer package, cross-model selection, review input assembly,  integration, and cobuild pipeline config rollout -> project CoBuild — pipeline automation for turning designs into working code.

Orchestrates agents through structured pipelines with enforced stage gates.

COMMANDS:
  setup                          Register repo for pipeline automation
  poller                         Poll for triggers, spawn M sessions
  init-skills                    Copy default skills into repo
  insights                       Analyze pipeline execution data
  improve                        Suggest pipeline improvements

  init <shard-id>                Initialize pipeline on a design
  show <shard-id>                Display pipeline state
  gate <shard-id> <gate-name>    Record a gate verdict
  review <shard-id>              Phase 1 readiness review
  decompose <shard-id>           Phase 2 decomposition
  audit <shard-id>               Show pipeline audit trail
  lock/unlock/lock-check <id>    Pipeline lock management

  dispatch <task-id>             Dispatch task to agent via tmux
  complete <task-id>             Post-agent completion bookkeeping

  work-item (wi)                 Work item operations via connector
    show <id>                    Show a work item
    list                         List work items
    links <id>                   Show relationships
    status <id> <status>         Update status
    append <id> --body "..."     Append content
    create --type <t> --title    Create a work item
    label add <id> <label>       Add a label
    links add <from> <to> <type> Create a relationship

CONFIGURATION:
  Uses ~/.cobuild/config.yaml and .cobuild.yaml for project/agent.
  Legacy ~/.cxp/ and .cxp.yaml paths are also supported.

Usage:
  cobuild [command]

Available Commands:
  admin          System health, cleanup, and maintenance
  audit          Show pipeline audit trail
  complete       Post-agent completion: push, create PR, mark needs-review
  completion     Generate the autocompletion script for the specified shell
  dashboard      Pipeline analytics dashboard
  decompose      Record Phase 2 decomposition verdict
  deploy         Deploy services affected by a design's merged changes
  dispatch       Dispatch a task to an agent via tmux
  dispatch-wave  Dispatch the next wave of ready tasks for a design
  explain        Show the full pipeline in human-readable form
  gate           Record a pipeline gate verdict
  help           Help about any command
  improve        Suggest pipeline improvements based on execution patterns
  init           Initialize pipeline metadata on a design shard
  init-skills    Copy or update default pipeline skills in the repo
  insights       Analyze pipeline execution data and produce a report
  investigate    Record bug investigation verdict
  kb-sync        Run the kb-sync phase: sync KB articles affected by a merged work item
  lock           Acquire pipeline lock
  lock-check     Check pipeline lock status
  merge          Merge an approved task PR and close the task
  merge-design   Analyse conflicts and merge all task PRs for a design
  next           Print the single next concrete command for a pipeline
  poller         Continuously poll for actionable pipeline state and dispatch agents
  process-review Process Gemini code review and merge or re-dispatch for fixes
  retro          Run a pipeline retrospective on a completed design
  review         Record Phase 1 readiness review verdict
  run            Submit a work item for autonomous processing by the poller
  scan           Generate a project anatomy file — file index with descriptions and token estimates
  setup          Register current repo for pipeline automation
  show           Display current pipeline state
  status         Show all active pipelines and their state
  unlock         Release pipeline lock
  update         Update pipeline state on a design shard
  update-agents  Generate or update AGENTS.md from current skills and config
  version        Print version information
  wait           Wait for tasks to reach a target status
  work-item      Work item operations via the connector

Flags:
      --agent string     Override agent identity
      --config string    Override config file path
      --debug            Verbose logging
  -h, --help             help for cobuild
  -o, --output string    Output format (text|json|yaml) (default "text")
      --project string   Override project from config

Use "cobuild [command] --help" for more information about a command., repo CoBuild — pipeline automation for turning designs into working code.

Orchestrates agents through structured pipelines with enforced stage gates.

COMMANDS:
  setup                          Register repo for pipeline automation
  poller                         Poll for triggers, spawn M sessions
  init-skills                    Copy default skills into repo
  insights                       Analyze pipeline execution data
  improve                        Suggest pipeline improvements

  init <shard-id>                Initialize pipeline on a design
  show <shard-id>                Display pipeline state
  gate <shard-id> <gate-name>    Record a gate verdict
  review <shard-id>              Phase 1 readiness review
  decompose <shard-id>           Phase 2 decomposition
  audit <shard-id>               Show pipeline audit trail
  lock/unlock/lock-check <id>    Pipeline lock management

  dispatch <task-id>             Dispatch task to agent via tmux
  complete <task-id>             Post-agent completion bookkeeping

  work-item (wi)                 Work item operations via connector
    show <id>                    Show a work item
    list                         List work items
    links <id>                   Show relationships
    status <id> <status>         Update status
    append <id> --body "..."     Append content
    create --type <t> --title    Create a work item
    label add <id> <label>       Add a label
    links add <from> <to> <type> Create a relationship

CONFIGURATION:
  Uses ~/.cobuild/config.yaml and .cobuild.yaml for project/agent.
  Legacy ~/.cxp/ and .cxp.yaml paths are also supported.

Usage:
  cobuild [command]

Available Commands:
  admin          System health, cleanup, and maintenance
  audit          Show pipeline audit trail
  complete       Post-agent completion: push, create PR, mark needs-review
  completion     Generate the autocompletion script for the specified shell
  dashboard      Pipeline analytics dashboard
  decompose      Record Phase 2 decomposition verdict
  deploy         Deploy services affected by a design's merged changes
  dispatch       Dispatch a task to an agent via tmux
  dispatch-wave  Dispatch the next wave of ready tasks for a design
  explain        Show the full pipeline in human-readable form
  gate           Record a pipeline gate verdict
  help           Help about any command
  improve        Suggest pipeline improvements based on execution patterns
  init           Initialize pipeline metadata on a design shard
  init-skills    Copy or update default pipeline skills in the repo
  insights       Analyze pipeline execution data and produce a report
  investigate    Record bug investigation verdict
  kb-sync        Run the kb-sync phase: sync KB articles affected by a merged work item
  lock           Acquire pipeline lock
  lock-check     Check pipeline lock status
  merge          Merge an approved task PR and close the task
  merge-design   Analyse conflicts and merge all task PRs for a design
  next           Print the single next concrete command for a pipeline
  poller         Continuously poll for actionable pipeline state and dispatch agents
  process-review Process Gemini code review and merge or re-dispatch for fixes
  retro          Run a pipeline retrospective on a completed design
  review         Record Phase 1 readiness review verdict
  run            Submit a work item for autonomous processing by the poller
  scan           Generate a project anatomy file — file index with descriptions and token estimates
  setup          Register current repo for pipeline automation
  show           Display current pipeline state
  status         Show all active pipelines and their state
  unlock         Release pipeline lock
  update         Update pipeline state on a design shard
  update-agents  Generate or update AGENTS.md from current skills and config
  version        Print version information
  wait           Wait for tasks to reach a target status
  work-item      Work item operations via the connector

Flags:
      --agent string     Override agent identity
      --config string    Override config file path
      --debug            Verbose logging
  -h, --help             help for cobuild
  -o, --output string    Output format (text|json|yaml) (default "text")
      --project string   Override project from config

Use "cobuild [command] --help" for more information about a command.
- Repo config rollout for penfold -> project , repo 
- Repo config rollout for penf-cli -> project , repo 
- Repo config rollout for context-palace -> project , repo 
- Repo config rollout for mycroft -> project , repo 

9 tasks across 4 waves:

Wave 1:
-  — Add built-in review core types, config, and cross-model resolution

Wave 2:
-  — Assemble review input and prompt from diff, task, and design context (blocked by )
-  — Implement Claude, OpenAI, and external reviewer providers (blocked by )

Wave 3:
-  — Integrate built-in reviewer flow into process-review (blocked by , )

Wave 4:
-  — Switch cobuild pipeline config to built-in auto review (blocked by )
-  — Switch penfold pipeline config to built-in auto review (blocked by )
-  — Switch penf-cli pipeline config to built-in auto review (blocked by )
-  — Switch context-palace pipeline config to built-in auto review (blocked by )
-  — Add mycroft pipeline config for built-in auto review (blocked by )

Context check:
- Added 
- Added 
- Refreshed  with Generated /Users/james/worktrees/cobuild/cb-32ed2a/.cobuild/context/always/anatomy.md (157 files, 351723 total estimated tokens)


---
*Appended by agent-mycroft at 2026-04-12 10:32 UTC*

## Decomposition Summary (corrected)

The previous decomposition append was malformed by shell command substitution. Use this section as the authoritative decomposition record.

Spec to target map:
- Built-in reviewer package, cross-model selection, review input assembly, process-review integration, and cobuild pipeline config rollout -> project cobuild, repo cobuild
- Repo config rollout for penfold -> project penfold, repo penfold
- Repo config rollout for penf-cli -> project penfold, repo penf-cli
- Repo config rollout for context-palace -> project context-palace, repo context-palace
- Repo config rollout for mycroft -> project mycroft, repo mycroft

9 tasks across 4 waves:

Wave 1:
- cb-ec76d3 — Add built-in review core types, config, and cross-model resolution

Wave 2:
- cb-d77848 — Assemble review input and prompt from diff, task, and design context (blocked by cb-ec76d3)
- cb-75ab97 — Implement Claude, OpenAI, and external reviewer providers (blocked by cb-ec76d3)

Wave 3:
- cb-5a4ce0 — Integrate built-in reviewer flow into process-review (blocked by cb-d77848, cb-75ab97)

Wave 4:
- cb-396778 — Switch cobuild pipeline config to built-in auto review (blocked by cb-5a4ce0)
- pf-a30052 — Switch penfold pipeline config to built-in auto review (blocked by cb-5a4ce0)
- pf-b43e77 — Switch penf-cli pipeline config to built-in auto review (blocked by cb-5a4ce0)
- cp-0273b8 — Switch context-palace pipeline config to built-in auto review (blocked by cb-5a4ce0)
- my-e84fbf — Add mycroft pipeline config for built-in auto review (blocked by cb-5a4ce0)

Context check:
- Added .cobuild/context/always/architecture.md
- Added .cobuild/context/implement/coding-patterns.md
- Refreshed .cobuild/context/always/anatomy.md with cobuild scan

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. **Run `cobuild complete pf-a30052`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.

**IMPORTANT RULES:**
- NEVER use raw `git merge` or `git push` to main — always use `cobuild complete` which creates a PR
- NEVER merge PRs yourself — the orchestrating agent handles merge via `cobuild merge` after review
- If a reviewer (Gemini, human) leaves a critical comment on your PR, you MUST address it before the PR can merge
- Check review comments: `gh pr view <pr-number> --comments`
