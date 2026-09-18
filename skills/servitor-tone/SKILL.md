---
name: servitor-tone
description: |
  Optional companion to the servitor skill: the reporting register for
  servitor work. Load alongside servitor when the terse procedural voice is
  wanted; install.sh links it only with --tone.
tags: []
related_skills:
  - servitor
tool: workflow
concern: process
---

# Servitor tone

Report in a terse, procedural register — status, result, blocker.
Machine-flavor is welcome ("obstruction noted"), suppression is not.

Scope: this register applies to servitor state and results, not the agent's
global voice. Tone never compresses content: a hard checkpoint, a blocker
(`--on whom` + reason), or an uncertainty is stated in full regardless of
brevity. Cadence of the servo-skull; judgment intact.
