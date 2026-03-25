# Task: Main branch broken: compilation errors from design implementation PRs

**Task ID:** pf-71f0cf
**Agent:** 

## Task Content

## Problem

Main branch doesn't compile. Worker build fails with:
- `undefined: pkgtemporal.ActivityBuildStageContext` — constant added but TODO'd out
- `undefined: nlCtxOutput` — variable referenced outside its scope (from context metadata fix)
- `PreClassifyInput redeclared` — type declared by both pf-5ee8e3 and pf-ccbd1b design PRs

## Root cause

6 designs implemented in parallel, all touching pipeline.go. Squash merges created divergent histories. Individual PRs passed CI but the accumulated changes on main are incompatible.

This is exactly what the cross-design review (pf-2b22a3) predicted: 'pipeline.go is touched by 5 of 6 designs.'

## Impact

Worker cannot be deployed. AI coordinator deployed successfully (separate binary).

## Fix needed

1. Remove duplicate PreClassifyInput/Output declarations
2. Fix nlCtxOutput scope (from the Langfuse metadata context fix)
3. Uncomment or properly implement ActivityBuildStageContext constant

## Lessons

- Cross-design integration test (pf-f5c919) should have caught this
- Need: build check on main after every merge (CI gate)
- Squash merge + parallel designs = broken main

## Instructions

Implement this task following the acceptance criteria above.

### On completion

1. Run tests: `make test && make vet`
2. Build: `make build`
3. **Run `cobuild complete pf-71f0cf`** -- this commits remaining changes, pushes, creates the PR, appends evidence, and marks the task needs-review. Do this as your LAST action.
