# Phase C decisions: sport profiles (input for its plan)

Repo: `app/` at `4093d0c`. Roadmap text (docs/ROADMAP.md, Phase C):

1. Add `internal/sport`. Basketball moves into it unchanged, and the existing tests prove it.
2. Futsal (countdown from 20:00, 5 accumulated fouls), handball (counts up, 2×30, 2-minute suspensions), volleyball (sets to 25, 15 in the last, serving side), soccer (counts up, 2×45 plus stoppage, cards and substitutions, scored by hand).
3. The Stramatel decoder already reads handball, volleyball and hockey codes. A sport is listed as supported only after testing on a recorded game.
4. Overlay fixtures per sport in `web/overlay/fixtures/<sport>/`, loaded by the Chrome test.

## Facts

- `store.Club.Sport` exists; the panel's sport list (`web/painel/painel.js:2151`) has basketball enabled and the rest disabled with "em breve".
- `state.Live` (state.go:36) has `Home, Away, Clock, Period, HomeFouls, AwayFouls, Shot, Sport, HomeTimeouts, AwayTimeouts`. `state.Period` holds per-period points. `state.Game.Sport/SportLabel` feed the on-air word ("BASKET").
- The manual scorer (`internal/source/manual.go`, 402 lines) is basketball-shaped: `Period` 1..4 then 5 = first overtime, `point|foul|timeout|undo|clock_*|period_next` actions, a `Length` per period.
- The decoder (`internal/ble/decode.go`) maps first-byte codes to hockey, basketball, net, timer, tennis, volleyball, handball, and already sets `sport` in the frame; `isNet` covers volleyball's set display. No futsal or soccer code: those consoles show as the generic timer/handball modes.
- Overlay: `web/overlay/overlay.js` (1270 lines), fixtures flat in `web/overlay/fixtures/*.json`, 18 basketball-specific references (shot clock, quarters, bonus).
- Screens: marcador, abertura, plantel, desconto, intervalo, final, apoiantes, proximo, voltamos, nada. `final` and `intervalo` show the per-period table.

## Owner answers (2026-11-25), these override the table below

- **Volleyball first** (decision 1 confirmed).
- **Sport per escalão now** (overrides 5 and 6). `settings.club.sport` stays as the club's main sport and the default for new escalões; existing escalões take it on load. Each escalão can change it. A club can have "Seniores" in basketball and "Seniores" in football, so escalão names can repeat across sports: show them as "Seniores · Futebol" wherever two share a name, and never match escalões by name alone (today `internal/web/club.go:249` matches by `store.Plain(name)` for FPB teams; that match must also check the sport is basketball). The game's sport comes from its escalão. The FPB offer checks the escalão's sport, and plan 001's `Caps.Covers` must check the escalão's sport too.
- **Handball suspensions: one tap starts a 2:00 timer; the shirt number is optional** and can be added afterwards. The overlay shows the number only when entered. Up to 3 timers per team.
  - 2026-11-25: on the overlay scoreboard each team gets one compact cell, not a chip per suspension: how many players are out ("−2", U+2212) and the MM:SS of the soonest return, then that player's shirt number ("#7") when entered; nothing when no one is out. The scorer page keeps one chip per suspension, where the volunteer edits numbers and ends them early.
- **Volleyball score: a separate sets list** (overrides 4). `Live` gets `Sets []SetScore{Home, Away}` for finished sets, `Home/Away` stay the current set's points, plus `Serve`. `Periods` is not reused for volleyball; final, intervalo and the game log read `Sets` when the escalão's sport is volleyball.
- **Soccer: per player.** Cards (yellow, second yellow, red) and substitutions are tied to shirt numbers from the escalão's squad, entered on the scorer page; the overlay shows per-team card counts on the scoreboard and a lower third for each card and substitution; stoppage time as +N.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | 🔥 Burned | Which sport first? | Volleyball, then futsal, handball, soccer | Volleyball changes the score model the most (sets), so doing it first shakes out `internal/sport`. It also unlocks Nevobo and FPV (Phase E plan 001). Futsal is the bigger sport in Portugal and the clock model is closest to basketball, so it would be the quicker first win. |
| 2 | ✅ Grilled | Ship all four at once or one at a time? | One PR (and one release note) per sport, after a first PR that only moves basketball into `internal/sport` | The roadmap says basketball moves "unchanged, and the existing tests prove it": that is its own PR with zero behaviour change. |
| 3 | ✅ Grilled | `internal/sport` shape | A `Profile` data struct per sport: clock direction, period length and count, overtime rule, foul limit and reset, timeouts per period/half, score model (`points` or `sets`), set targets, which extras show (shot clock, serve, suspensions, cards), on-air label key. A few functions where data isn't enough: `NextPeriod`, `SetWon`, `GameOver`. | Data covers almost everything; behaviour only where volleyball's sets need it. |
| 4 | 🔥 Burned | Volleyball state shape | Keep `Live.Home/Away` as points in the current set; add `HomeSets`, `AwaySets`, `Serve` ("home"/"away"/""); a finished set goes into `Periods` like a quarter does. | Reuses the period table for set scores on `final`. A separate `Sets` array would be clearer to read but changes every consumer. |
| 5 | ✅ Grilled | Where is the sport picked? | `settings.club.sport`, as today. The game's sport comes from it. | Multi-sport clubs can change it in Definições between games. |
| 6 | 🔥 Burned | Per-escalão or per-game sport for multi-sport clubs? | Not now. Club-level only. | Many Portuguese clubs run basketball and futsal; a volunteer streaming both switches the setting each time. A per-escalão sport is a clean follow-up. |
| 7 | ✅ Grilled | Console or manual first for a new sport? | Manual scorer (`/marcar`) supports each sport on day one. Console support for that sport is listed "untested" until a recorded session of a real game passes a decoder test. | Roadmap item 3. Phase D adds the capture mode that makes recordings easy. |
| 8 | ✅ Grilled | One overlay or one per sport? | One overlay; the scoreboard and final/intervalo screens read the profile's extras (`shows: shot, serve, suspensions, cards`). | Keeps one file on air; fixtures cover each variant. |
| 9 | ✅ Grilled | Fixtures layout | `web/overlay/fixtures/<sport>/*.json` for new sports; basketball's stay flat. The Chrome test globs both. | Roadmap item 4; moving 40 basketball fixtures is churn with no gain. |
| 10 | 🔥 Burned | Handball suspensions on air | Up to 3 running 2-minute timers per team, entered by the scorer with one tap (no player number). | Showing the shirt number is nicer, but a volunteer won't type it fast enough mid-game. |
| 11 | 🔥 Burned | Soccer cards and substitutions | Cards as a count per team (yellow, red) entered by hand; substitutions only as a lower third, not tracked in state. Added stoppage time shown as "+N". | The roadmap lists cards and substitutions; per-player tracking is a lot of UI for amateur football where Hallstream is rarely the official record. |
| 12 | ✅ Grilled | Futsal rules | Countdown 2×20 stopped clock; 5 accumulated fouls per half shows a warning on the 5th, fouls reset at half; one timeout per team per half. | Roadmap and Laws of the Game. |
| 13 | ✅ Grilled | Words on air and in the panel | Period word per sport in i18n (`Período`/`Quarter`, `Parte`/`Half`, `Set`), plus the on-air sport label key in the profile. | Existing i18n rules. |
| 14 | ✅ Grilled | The wizard's sport list | Each sport is enabled in the club step and Definições in the PR that ships it. | `painel.js:2151` already has the switch per sport. |
| 15 | ✅ Grilled | Overlay design work | Each new scoreboard variant goes through `build-ui` (extension branch, inside the R8 system) with an outside critique before merge. | The overlay is the product people see on stream. |

## Build order (one PR each)

1. `internal/sport` with the basketball profile; manual scorer, state and overlay read it; no behaviour change; all existing tests pass unchanged.
2. Volleyball: sets, serve, set/match end rules, scorer page buttons, overlay scoreboard variant, final screen with set scores, fixtures in `fixtures/volleyball/`, enabled in the panel.
3. Futsal: countdown halves, fouls per half, overlay variant (no shot clock), fixtures, enabled.
4. Handball: count-up halves, suspension timers, fixtures, enabled.
5. Soccer: count-up halves with stoppage, cards, fixtures, enabled.
6. Docs: CONTRACT.md per PR; ROADMAP Phase C ticked per sport; website support table line per sport (website repo).

Each PR: `go vet ./...`, `go test ./...`, `GOOS=windows GOARCH=386 go build ./...`, Chrome overlay and panel tests with the new fixtures, pt.json then en.json.

## Done when

Each escalão can have its own sport (volleyball, futsal, handball, soccer or basketball, defaulting to the club's), and a club can score a game from `/marcar` with that sport's rules, and the overlay draws its scoreboard, intervalo and final screens from fixtures the Chrome test loads; basketball behaves exactly as before.
