---
name: plan-review
description: Use when asked to review, verify, or check code in this project — including vague requests ("review mem service", "can you review this") and no-argument invocations like "/plan-review". Compares the implementation against the plan in README.md, surfaces discrepancies as questions instead of silently resolving them, and picks its own review scope (uncommitted > unpushed > ask) when none is given.
---

# Plan Review

Read-only review. Never edit, fix, or refactor anything found here — report and ask instead. This project's CLAUDE.md already forbids unrequested code changes; this skill is how review happens without violating that.

## 1. Determine scope

If the user named a scope (a file, package, function, feature — "review mem service", "check the ranking code") — use exactly that, regardless of git state. Skip straight to step 2.

Otherwise, derive the scope from git state, in this order:

1. `git status --porcelain` (covers staged, unstaged, and untracked). Non-empty → scope is everything uncommitted. Review it. Do not ask first.
2. Working tree clean. Check upstream tracking: `git rev-parse --abbrev-ref --symbolic-full-name @{u}`.
   - If it resolves, check `git log @{u}..HEAD --oneline`. Non-empty → scope is the unpushed commits, diffed with `git diff @{u}...HEAD`. Review it. Do not ask first.
   - If there's no upstream configured, or no unpushed commits, or nothing uncommitted and nothing unpushed — there's nothing to review on its own. **Ask the user what to review.** Don't guess, don't default to an arbitrary file.

## 2. Read the plan

Read README.md — it's the plan for this project, written and owned by the user. Read enough of it to understand what the code in scope is supposed to do, not just the section that looks closest; the numbered "Ground rules" near the top apply everywhere, so a change in one section can still violate a rule stated elsewhere.

## 3. Compare implementation to intent

For each piece of behavior in scope, check it against what the plan says should happen.

- **Plan says X, code does Y, and the conflict is a real behavioral or structural contradiction** (not a style nitpick): stop and ask the user which one is right. Do not assume the plan is stale and do not assume the code is wrong — name the specific plan section and the specific code (file:line) in the question. If there are several, batch them into one round of questions rather than asking one at a time.
- **Plan is silent on this detail** (implementation plumbing the plan never addresses): not a discrepancy. Don't invent an intent to hold the code to.
- **Code is visibly unfinished** (stub, TODO, placeholder return, empty function body): not a discrepancy by itself — only raise it as one if the *shape* of what exists so far already contradicts the plan. Otherwise just note it's unfinished in the eventual report.

Resolve every genuine discrepancy with the user before finalizing anything in step 4 that depends on the answer — the verdict on surrounding code may hinge on which side was "right."

## 4. Ordinary review

Once intent is settled, review the code in scope on its own merits: correctness bugs, concurrency/transaction issues, idiomatic Go, and anything that contradicts the project's standing invariants —

- pure Go, no cgo, single static binary
- no background daemon beyond `mem context` / `mem mcp` / `mem tidy`
- v1 makes no LLM calls anywhere
- hard deletes only, nothing archived or soft-deleted
- usage history (`hit_count`, `last_retrieved_at`) never affects ranking, only decay
- no hand-picked thresholds outside usage decay — everything else comes from automatic tuning
- assumptions that only hold at the stated scale (local SQLite, single user, no index) aren't being quietly broken

Report these findings with the ReportFindings tool, most severe first. Discrepancy-vs-plan questions from step 3 are not findings — by report time they're either resolved, or still-open questions blocking part of the review (say so plainly rather than reviewing around a gap silently).

## Idiom nitpicks: pin them to this project's Go version

The user wants idiomatic Go, but idioms shift across Go releases — what's idiomatic in one version can be dead weight or even the wrong call a few versions earlier or later. Before flagging anything as "not idiomatic" or "should use X instead":

1. Read the `go` directive in `go.mod` for the exact version this module targets (currently 1.27.3, but re-read it — don't cache this across runs or assume it hasn't moved).
2. Don't rely on memory for what's idiomatic at that version, especially for anything newer than roughly Go 1.23 — training data is stale relative to recent releases and this module may be pinned well past it. Use context7 (resolve the Go language/stdlib docs) or web search for that specific version's release notes / idiom changes before asserting a style claim, the same as any other library-version-specific question.
3. Only suggest a pattern if it's actually available at the pinned version (don't suggest something that shipped in a later release than `go.mod` declares). Conversely, if the pinned version has a newer idiom that supersedes what the code does (e.g. a stdlib helper that replaces a hand-rolled loop), that's fair game to flag.
4. Idiom nitpicks go through ReportFindings like any other finding, ranked by severity — they're almost always on the low end, not above actual correctness bugs.

## Constraints

- Never edit, create, or delete files. This is a review, not an implementation pass.
- Never assume which of "plan" or "code" is authoritative when they disagree — that's always the user's call, every time, even if a previous review already asked about something similar.
- If the git-derived scope turns out to be large, say so before diving in rather than silently reviewing only part of it.
