---
name: qa-next-task
description: QA workflow for the next built ssh-idlefarmer task. Picks the lowest-numbered task file in .claude/qa, audits the built code like an angry senior dev, fixes every fault, inefficiency, and edge case, then moves the task to .claude/completed after addressing the findings with the user.
---

# QA Workflow

Follow these steps in order. Do not skip ahead.

## Checklist

```
- [ ] 1. Read .claude/plans/00-project-overview.md
- [ ] 2. Pick the next task up for grabs in .claude/qa
- [ ] 3. Assess what was built against the criteria
- [ ] 4. Audit the code like an angry senior dev
- [ ] 5. Fix every fault, inefficiency, and edge case
- [ ] 6. After addressing findings with the user, move the task to .claude/completed
```

## 1. Read the overview

Read `.claude/plans/00-project-overview.md` before anything else. Its invariants are the bar the built code must clear, not just the task's local checkboxes.

## 2. Pick the next task

- List `.claude/qa/`.
- The "next task up for grabs" is the **lowest-numbered** `task-N-*.md` file in `qa/`.
- Read that task file in full so you know exactly what was promised.

## 3. Assess what was built

Map the built code against the task's requirements and acceptance criteria. Confirm it actually does what the task says — and that it respects the overview's invariants, not just the local checkboxes. Verify acceptance criteria pass because the behavior is genuinely correct.

## 4. Audit the code

Analyze that code like an angry senior software dev who was tasked with fixing an outage late at night an intern caused, fueled by pure determination to improve that code. Understand that more does not always mean better, and less is not always more. That code should be fault free, tested, and **efficient**.

Hunt specifically for:

- **Faults and bugs** — wrong logic, race conditions, unhandled errors, broken seams, violated invariants.
- **Inefficiencies** — needless work, wasted allocations, poor complexity, chatty I/O, anything that does more than it must.
- **Missing edge cases** — empty/blank input, concurrency and takeover, restarts and flush, resize, offline elapsed time, hostile/untrusted strings, integer overflow.
- **Hard-coded or gamed results** — anything faked to earn a checkmark rather than genuinely working.
- **Missing or shallow tests** — behavior that isn't actually proven.

## 5. Fix everything

Once all faults, inefficiencies, and edge cases have been found, fix them. Re-run the build and tests to prove the fixes hold. The goal is not more code and not less code — it is correct, tested, and efficient code.

## 6. Move to completed

Address the findings and fixes with the user. Only once everything is fixed AND confirmed with the user, move the task's markdown file from `.claude/qa/` to `.claude/completed/`. Do not move it while any issue remains open or unconfirmed.
