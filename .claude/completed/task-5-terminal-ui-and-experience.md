---
name: terminal-ui
description: Build the player-facing terminal UI for ssh-idlefarmer with Bubble Tea, Lip Gloss, and Bubbles — the farm view, market/shop, land expansion, rebirth, stats/achievements, and help screens — wired to drive the deterministic engine's live ticks and actions, with a responsive layout, new-key onboarding, a returning-player "here's what grew" summary, a cozy low-stakes feel, and graceful in-UI handling of errors and disconnects.
---

# Task 5 — Terminal UI, Navigation & Player Experience

## Objective

Make the game a pleasure to actually play. This task replaces the placeholder screen with the full terminal interface: the screens a player navigates, the keybindings, the live updating farm, onboarding for first-time keys, and the welcome-back summary that delivers the genre's core hook ("look what grew while I was gone"). The UI is a thin presentation/input layer over the deterministic engine from Tasks 3–4 — it drives ticks and sends actions, but the engine remains the authority on game rules.

## Prerequisites

- Tasks 1–4 complete: SSH server with PTY/resize handling, persistence, the deterministic engine, and the progression/rebirth systems.

## Scope

### In scope
- The full Bubble Tea program: model, update, view, and navigation between screens.
- The screens: farm view, market/shop, land expansion, rebirth, stats/achievements, help.
- Keybindings and a consistent navigation model.
- Driving the engine's live tick on an interval and rendering progress in real time.
- New-key onboarding and the returning-player summary.
- Responsive layout using the resize events from Task 1.
- Cozy, low-stakes styling and tone.
- Graceful in-UI handling of errors, the concurrency refusal from Task 2, and disconnects.

### Out of scope (deferred)
- Game rules and economy (owned by the engine; the UI only calls into it).
- Deployment — Task 6.

## Requirements

### Architecture
- Use **Bubble Tea** for the program (model → update → view), **Lip Gloss** for styling, and **Bubbles** for stock widgets (lists, inputs, viewport, help) where they fit.
- The Bubble Tea model holds a reference to the engine state and the player's identity/save; **all game mutations go through the engine's action functions** from Tasks 3–4. The UI never implements rules itself, so it cannot diverge from or bypass the engine's validation.
- Drive the engine's **live tick** via a Bubble Tea tick command on a regular interval (e.g. once per second) so crops visibly progress. Because the engine is time-driven and pure (Task 3), this is just advancing by a small span each tick.
- On entering the program, the **offline catch-up** has already been computed at load time (Task 2/3); the UI presents its result (see returning-player summary).

### Screens
At minimum:
- **Farm view (home):** the plots and their state (empty / growing with progress / ready to harvest), the wallet, and current key stats. Primary actions: plant, harvest. This is where players spend most of their time.
- **Market / shop:** buy seeds / view crop info; if a separate sell step exists, sell here. Show crop archetypes and their tradeoffs clearly.
- **Land expansion:** view and purchase additional plots, with the scaling cost surfaced.
- **Rebirth:** present the prestige preview — what will be gained (currency, the bonuses available to buy) and what will be lost — and let the player confirm. Make the "is it worth it" decision legible (Task 4's pacing goal should be visible here).
- **Stats / achievements:** lifetime numbers, prestige level, earned and pending achievements/milestones.
- **Help:** keybindings and a short explanation of the loop, reachable from anywhere.

### Navigation & input
- A consistent, discoverable navigation model (e.g. number/letter keys or tabs to switch screens, arrow keys within a screen, a clearly shown quit key).
- Show context-appropriate key hints on screen (a footer/help line) so a new player is never stuck.
- Confirm destructive/irreversible actions (notably rebirth) before applying them.
- Reject or gray out actions the engine would refuse (unaffordable purchases, harvesting immature plots) and explain why, rather than letting the player attempt them and bounce.

### New-key onboarding
- The first time a key (and default save) connects, present a brief, skippable welcome that explains the loop in a sentence or two and seeds the player into a playable starting state (a plot or two and a little starting cash, per the engine's defaults).
- Keep it short — the goal is to be planting within seconds.

### Returning-player summary
- On reconnect, show a concise "while you were away" summary derived from the offline catch-up: time elapsed, what matured, and what's ready to harvest. This is the core hook of the genre — make it satisfying and immediate, then drop them into the farm view.

### Responsiveness & resilience
- Use the window-change events from Task 1 to re-layout on resize; the UI must remain usable across a range of terminal sizes and degrade gracefully when very small.
- Handle the **concurrency refusal** from Task 2 (save already open elsewhere) with a clear message rather than a crash or a blank screen.
- Handle disconnects and the **graceful-shutdown** path cleanly: the session should not corrupt the save (Task 2 flushes it), and the player should be able to reconnect and continue.
- Surface engine/content errors as friendly messages, never raw stack traces.

### Tone & feel
- The styling and copy should reinforce *relaxing and low-stakes*: calm palette, gentle language, no timers creating pressure, no nagging. Sessions should feel complete in well under two minutes but reward lingering.

## Security considerations

- The UI is untrusted with respect to rules: it must call the engine for every action and render only what the engine confirms. Do not let the presentation layer compute economy or progression outcomes.
- Do not render sensitive data (full keys, internal paths). Showing the short fingerprint and save name is fine.
- Keep input handling robust against unusual terminal input and escape sequences so a malformed client cannot wedge the session.

## Suggested artifacts produced by this task

- A built-out `internal/tui/` replacing the placeholder: the root model, per-screen models/components, styling, the tick command, and the onboarding/summary flows.
- Wiring in `internal/server/` so the session launches the full program with the loaded engine state and identity.

## Acceptance criteria

- [ ] The full loop is playable over SSH: connect, see the farm, plant, watch crops grow live, harvest, earn, expand, and rebirth, all through the UI.
- [ ] A brand-new key sees onboarding and is planting within seconds.
- [ ] A returning player sees an accurate "while you were away" summary on reconnect.
- [ ] All screens are reachable and navigable with on-screen key hints; help is always available.
- [ ] Rebirth shows a clear gain/loss preview and requires confirmation.
- [ ] The UI re-lays-out on resize and degrades gracefully when small.
- [ ] The save-already-open refusal, disconnects, and shutdown are handled with friendly messaging and no save corruption.
- [ ] Illegal actions are prevented/explained in the UI, and the engine remains the sole authority on rules.

## Verification steps

1. Play a complete session over SSH from a fresh key: onboarding → plant → watch live growth → harvest → buy a plot → rebirth.
2. Disconnect during growth, wait, reconnect — confirm the away-summary is accurate.
3. Resize the terminal repeatedly during play; confirm the layout holds and remains usable when small.
4. Open the same save from a second terminal; confirm the refusal message renders cleanly.
5. Send SIGTERM mid-session; confirm a clean exit and that reconnecting resumes without loss.
6. Attempt unaffordable/illegal actions; confirm they are blocked with an explanation.

## If blocked

If a UI behavior would require the presentation layer to make game-rule decisions the engine doesn't expose, stop and request the needed engine action/query rather than reimplementing rules in the UI. Keeping the engine authoritative is a hard architectural line.
