---
name: build-next-task
description: Build workflow for the next ssh-idlefarmer task. Picks the lowest-numbered task file in .claude/plans, implements it in full, gives the whole codebase an honest once-over, then moves the task file to .claude/qa.
---

# Build Workflow

Follow these steps in order. Do not skip ahead.

## Checklist

```
- [ ] 1. Read .claude/plans/00-project-overview.md
- [ ] 2. Pick the next task up for grabs in .claude/plans
- [ ] 3. Build that task in full
- [ ] 4. Give the whole codebase an honest once-over
- [ ] 5. Move the task file from .claude/plans to .claude/qa
```

## 1. Read the overview

Read `.claude/plans/00-project-overview.md` before anything else. Treat its invariants as load-bearing: a task whose own acceptance criteria pass but which violates an invariant is NOT done.

## 2. Pick the next task

- List `.claude/plans/`. Ignore `00-project-overview.md`.
- The "next task up for grabs" is the **lowest-numbered** `task-N-*.md` file remaining in `plans/`.
- Tasks build in numeric order (1 → 6); a file leaves `plans/` once built, so the lowest remaining number is always the correct next task and its dependencies are already in place.
- Read that task file in full: objective, scope (in and out), requirements, acceptance criteria, and verification steps.

## 3. Build the task

- Implement every requirement and satisfy every acceptance-criteria checkbox for real.
- Respect the overview's invariants and the task's "out of scope" boundaries — build this layer's seams, not the next task's work.
- Honor the hard constraints (module path, config via `IDLEFARM_`-prefixed env vars, file frontmatter, etc.).
- If a requirement genuinely cannot be met as written, stop and follow that task's "If blocked" guidance: report the specific blocker and the options considered instead of substituting a weaker design.

## 4. Honest once-over

Give the whole codebase — not just the new files — a real review. Be honest, not generous:

- Everything compiles and builds from a clean checkout.
- Nothing is broken or half-wired; every seam connects.
- Nothing in scope was skipped, stubbed, or silently dropped.
- No value was hard-coded just to make a check pass or a test go green. If a result is faked to earn a checkmark, that is a failure — fix it so it works for real. Be real here.
- Acceptance criteria pass because the behavior is genuinely correct, not because it was gamed.

If the once-over surfaces problems, fix them before moving on.

## 5. Move the task to QA

Once the task is built and the once-over is clean, move its markdown file from `.claude/plans/` to `.claude/qa/`. This marks it as built and awaiting QA. Do not edit the task file's contents while moving it.
