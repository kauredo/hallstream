# Implementation Plans

Started 2026-11-25. Run them in this order unless the dependencies say otherwise. Executors: read the whole plan before you start, honour its STOP conditions, and update your row when you're done.

## Execution order & status

| Plan | Title | Priority | Effort | Depends on | Status |
|------|-------|----------|--------|------------|--------|
| 001 | An escalão can link to a network data source, and Nevobo is the first one | P2 | L | Phase C volleyball | TODO |

Status values: TODO | IN PROGRESS | DONE | BLOCKED (with one-line reason) | REJECTED (with one-line rationale)

## Decision records

- `phase-c-decisions.md`: the owner's answers for ROADMAP Phase C (sport profiles), 2026-11-25. Volleyball first, sport per escalão, a separate sets list, per-player soccer cards and substitutions. Write the Phase C plan from it.

## Dependency notes

- 001 needs ROADMAP Phase C to ship volleyball first, because Nevobo is a volleyball source. The owner chose Phase C before Phase E on 2026-11-25.
- Later Phase E plans, not written yet: own-key sources (BallDontLie, TheSportsDB, API-Sports; API-Sports only if its terms allow caching on a desktop app), FPV through Data Project (after Phase C volleyball), PlayHQ (only if a project key is free), Wikidata colours and labels.

## Findings considered and rejected

- Moving the FPB fields on `Group` into `Links` now (deferred, not rejected forever; ROADMAP item 3 stays partly done until it lands): they drive the season rollover, the roster import and the daily sync (`internal/web/sync.go`, 22 uses). That's a lot of risk and no gain until a second squad source exists.
- An NBB database provider: the NBB already gives every team an iCalendar link, so the calendar-link import (#24) covers Dutch basketball fixtures. Revisit only if clubs need its team search.
- Asking sources for permission before building: the owner decided against it on 2026-11-25.
