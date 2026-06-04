---
name: task-router
description: Router for advancing the ssh-idlefarmer plan stored in .claude/plans, .claude/qa, and .claude/completed. Routes to the build workflow when the user asks to build, implement, work on, or do the next task, and to the QA workflow when the user mentions QA, quality assurance, review, audit, or hardening. Use whenever the user wants to advance the plan, build the next task, or QA a built task.
---

# Task Router

This skill is **only a router** for the `ssh-idlefarmer` plan. It contains no build steps and no project code — it reads the overview, then points you at the correct playbook.

## Step 0 — Always read the overview first

Before doing ANYTHING else, read `.claude/plans/00-project-overview.md`. It defines the architecture, the cross-cutting invariants, the shared vocabulary, the locked decisions, the hard constraints, and the build order that every workflow depends on. Never skip it, in either workflow.

## The folders (state lives in the file's location)

| Folder | Meaning |
| --- | --- |
| `.claude/plans/` | Tasks **up for grabs** to build (plus `00-project-overview.md`). |
| `.claude/qa/` | Tasks that have been built and are **awaiting QA**. |
| `.claude/completed/` | Tasks that passed QA and are **done**. |

A task file moves `plans/ → qa/ → completed/` as it advances. A file's folder is the single source of truth for its state.

## Routing

Pick the workflow from the user's request:

- **QA workflow** → if the user says "QA", "quality assurance", "review", "audit", "harden", or anything indicating the built code should be scrutinized. Read and follow [qa.md](qa.md).
- **Build workflow** → every other case: the user says "build", "build it", "implement", "next task", gives additional build instructions, or gives no QA signal at all. Read and follow [build.md](build.md).

When the request is ambiguous and there is no QA signal, default to the build workflow.

After reading the overview, open the routed file and follow it exactly.
