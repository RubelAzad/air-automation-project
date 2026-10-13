# Architecture

This document explains how the browser-agent is put together: what each folder
does and the order in which things happen at runtime.

## One idea behind the layout

The tool never lets the AI drive the browser directly. The AI produces a
**declarative plan** (a list of typed steps), and four separate concerns handle
it:

- **what to do** — `planner` + `action`
- **is that allowed** — `policy`
- **do it safely** — `runner` + `executor`
- **who approves / steps in** — `approval`, logged by `audit`

Each concern is a separate package, so a plan can be produced, expanded,
validated, and audited without a browser being involved at all.

## Folders

| Path | Role | Key types / functions |
|---|---|---|
| `main.go` | Wiring only: parse flags, load config, build planner/policy/executor, run. | `run`, `buildPlan` |
| `internal/action` | The vocabulary. Step kinds (`navigate`, `click`, `fill`, `read`, `wait`, `press`, `screenshot`, `await_user`) and the `Plan` container. Pure data, no I/O. | `Action`, `Plan`, `Kind`, `Describe` |
| `internal/planner` | Turns a task into a `Plan`; passes the site's menus/fields to the LLM as context. | `Planner`, `File`, `LLM`, `Request` |
| `internal/site` | Per-site config (domains, login selectors, menus, 2FA/CAPTCHA detectors) and expansion of `menu`/`field` references into selectors. | `Config`, `Profile`, `Resolve` |
| `internal/policy` | The gate. Rejects unknown actions, non-allowlisted domains, `http://`, too many steps, path traversal, and missing secrets. Fails closed. | `Policy`, `Validate` |
| `internal/approval` | The human boundary: `Approver` (yes/no on consequential steps) and `Handoff` (pause for 2FA/CAPTCHA). Stdin and Auto implementations. | `Approver`, `Handoff`, `Stdin`, `Auto` |
| `internal/executor` | The only package that touches Playwright. Launches the browser, runs one `Action`, answers "is this selector visible". | `Executor`, `Execute`, `IsVisible` |
| `internal/runner` | The orchestrator. Loops over steps, enforces approval, detects challenges, inserts handoffs, writes audit entries. | `Runner`, `Run`, `handleChallenge` |
| `internal/audit` | Append-only JSON-lines log that never records secret values. | `Logger`, `Entry` |
| `internal/secret` | Expands `{{ENV:NAME}}` placeholders and provides `Redact`. | `Resolve`, `Ensure`, `Redact` |

## Execution flow

### 1. Startup — `main.go`

- Parse flags (`-site`, `-plan`, `-sites`, `-domains`, `-profile`, `-yes`,
  `-dry-run`, `-headless`, `-audit`, `-timeout`, `-max-steps`, `-list-actions`,
  `-list-sites`).
- If `-site` is set, `site.Load` reads the sites config and `Config.Get` picks
  the profile.
- Build the domain allowlist from the profile's `domains` plus any `-domains`,
  then `policy.New`.

### 2. Produce the plan — `buildPlan`

- `-plan file.json` → `planner.File` reads and strictly parses JSON.
- Otherwise `-task` plus `OPENAI_API_KEY` → `planner.LLM` calls an
  OpenAI-compatible endpoint, embedding the site's field and menu names in the
  system prompt.
- Either way the result is a plain `action.Plan`; no browser yet.

### 3. Resolve site references — `Profile.Resolve`

- Walk the steps and replace `"menu": "billing"` / `"field": "username"` with
  the real CSS selector. Unknown names error here, before anything runs.

### 4. Validate — `Policy.Validate`

- Per step: known and allowlisted kind; navigation URL parsed and its host
  checked against the allowlist (subdomains allowed, suffix tricks rejected);
  HTTPS required; screenshot paths cannot escape the working directory; secret
  references must exist.
- Nothing has touched the network yet. `-dry-run` prints the plan and stops
  here.

### 5. Open the browser (unless dry-run) — `executor.New`

- `-profile dir` uses `LaunchPersistentContext` so the login session survives
  between runs; otherwise a fresh context.
- Default is **visible** so the human can see the page and act.

### 6. Run the plan — `Runner.Run`

For each step, in order:

- `await_user` → goes straight to the handoff prompt, never to the browser.
- A `consequential` step → asks the `Approver` first; "no" aborts the run.
- Otherwise `Executor.Execute` runs the single action and returns a `Result`.
- After every step, `handleChallenge` checks the site's `mfa`/`captcha`
  `detect` selectors via `IsVisible`; if one is present it calls `wait`, which
  hands off to the human and blocks for Enter. This is the "you solve the
  challenge, I wait" path.
- Every transition is written to the audit log through `record`.

### 7. Wrap up

- The executor closes, the audit file closes, and a summary is printed.

## A concrete trace

Command: `-site acme -plan examples/site-login.json`

```
load acme profile        (domains=acme.example.com, menus=..., mfa.detect=...)
policy allowlist = [acme.example.com]
read plan JSON
resolve: "field":"username" -> "input[name='username']"
validate: navigate host ok, https ok, secrets exist
launch chromium (visible, persistent profile)
step1 navigate {{ENV:SITE_URL}}      -> executor
step2 fill  field username           -> executor (secret resolved, never logged)
step3 fill  field password           -> executor
   handleChallenge: mfa detectable? if visible -> print prompt, wait for Enter
step4 click field submit             -> Approver asks -> executor
step5 await_user                     -> Handoff, no browser
step6 click menu billing             -> executor
step7 read body                      -> executor, output printed
close browser; audit.log has one JSON line per event
```

## Why the splits matter

- **`action` is data-only** so a plan is testable and serializable, and the LLM
  can only emit that vocabulary — it cannot emit code.
- **`policy` is separate from `executor`** so "allowed" is decided without a
  live page and fails closed; the executor can then be simple.
- **`site` sits between planner and policy** so different sites' menus,
  selectors, and 2FA behavior are configuration, not code.
- **`approval` and `runner`** are where the safety rules are enforced:
  consequential steps stop for a human, and 2FA/CAPTCHA hand off to a human
  rather than being solved or bypassed.

## Safety invariants

- No arbitrary code execution: the AI emits only `action.Kind` values.
- Navigation is restricted to allowlisted domains over HTTPS.
- Consequential steps require explicit human approval.
- MFA/CAPTCHA are never bypassed; the agent pauses and waits for the human.
- Secrets are referenced as `{{ENV:NAME}}`, resolved at execution time, and
  never written to logs.
- Credentials, cookies, and tokens are never committed: `sites.yaml`,
  `.browser-profile/`, `.env`, and `audit.log` are gitignored.
