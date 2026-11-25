# Plan 005: A club can score futsal: two 20-minute halves, fouls per half, one timeout each

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0d7570c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club internal/web/overlay_test.go web/overlay web/marcar web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md`
> If any in-scope file changed since `0d7570c`, compare the "Current state"
> excerpts below with the live code; a mismatch you can't map one-to-one is a
> STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (touches the scoring path that runs on air)
- **Depends on**: nothing unmerged. Plans 002 (#27), 003 (#28), 004 (#29) and the court-art change (#30) are on `main`.
- **Category**: direction
- **Planned at**: commit `0d7570c`, 2026-11-25 (reconciled; first written against `4093d0c`)
- **Stack**: layer 01 of the futsal → handball → soccer stack. Plans 006 and 007 build on this one.

## Why this matters

ROADMAP Phase C lists futsal: "the clock counts down from 20:00, 5 accumulated fouls". Futsal is one of the biggest indoor sports in Portugal. It is close to basketball (a stopped clock counting down, team fouls, timeouts), so this plan is mostly a new `sport.Profile` plus the rules basketball doesn't have: halves instead of quarters, fouls that carry into extra time, a shorter extra time, and no shot clock.

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`): the sport is set per escalão, defaulting to the club's; the phone scorer supports each sport from day one; console support for a sport is listed as untested until a recorded real game passes a decoder test; each new scoreboard variant goes through `build-ui` inside the R8 design system. **Owner decision for this plan: basketball overtime becomes 5:00 (FIBA)**; today the scorer runs overtime at the full 10:00.

## Rules (verify before building)

From the FIFA Futsal Laws of the Game (Law 7, duration and timeouts; Laws 12 and 13, accumulated fouls). Read the current edition at fifa.com (use the `read-the-damn-docs` skill) and STOP if any of these differ:

- Two halves of 20:00, stopped clock, counting down. Half-time after the first half.
- One timeout per team per half; none in extra time.
- Accumulated fouls are counted per half. From the **6th** accumulated foul of a team in a half, the other side gets a direct free kick without a wall. So the 5th foul is the warning, the 6th the penalty.
- Accumulated fouls of the second half **carry into extra time**; they do not reset between extra-time periods.
- Extra time, where a competition needs a winner: two periods of 5:00.

## Current state (at `0d7570c`)

### `internal/sport/sport.go`

```go
type Profile struct {
	ID         string `json:"id"`
	Periods    int    `json:"periods"`
	Half       int    `json:"half"`
	Length     int    `json:"length"`
	ClockDown  bool   `json:"clock_down"`
	FoulsReset bool   `json:"fouls_reset"`
	FoulWarn   int    `json:"foul_warn"`
	FoulBonus  int    `json:"foul_bonus"`
	ShotClock  bool   `json:"shot_clock"`
	FinalHold  int    `json:"final_hold"`
	TimeoutsBy [3]int `json:"timeouts"`

	Scoring       string `json:"scoring"`         // "points" or "sets"
	SetPoints     int    `json:"set_points"`
	LastSetPoints int    `json:"last_set_points"`
	SetsToWin     int    `json:"sets_to_win"`
	WinBy         int    `json:"win_by"`
}

var Basketball = Profile{ID: "basketball", Periods: 4, Half: 2, Length: 6000, ClockDown: true,
	FoulsReset: true, FoulWarn: 4, FoulBonus: 5, ShotClock: true, FinalHold: 600,
	TimeoutsBy: [3]int{2, 3, 1}, Scoring: "points"}

var Volleyball = Profile{ID: "volleyball", Scoring: "sets", SetPoints: 25, LastSetPoints: 15,
	SetsToWin: 3, WinBy: 2, TimeoutsBy: [3]int{2, 2, 2}}

func Get(id string) Profile { … profiles[id] … else Basketball }
var profiles = map[string]Profile{Basketball.ID: Basketball, Volleyball.ID: Volleyball}
```

Methods: `Sets()`, `SetTarget(n)`, `SetWon(n, home, away)`, `Timeouts(period)`, `ResetsTimeouts(period)`. There is **no** `Overtime` method and no overtime length. `State.Game.Rules` carries the profile to the pages as `game.rules` (the whole struct, JSON tags as above).

### `internal/source/manual.go`

- `Score` (lines 15-31) is compared with `==` at line 329 (`if *s == prev { return nil }`). **There is no `same()` helper.** `Score` holds only comparable fields (volleyball's sets are a fixed array `[maxSets]state.SetScore` for this reason). Every field this plan adds must be comparable (ints, strings, bools, arrays).
- `newScore(length int)` (line 63); `NewManual` starts with `newScore(DefaultLength)` (line 152); `DefaultLength = 10*60*10` (line 54).
- `Restore` (lines 160-170) clamps `Length` to `DefaultLength` when it is `<= 0` or above `MaxClock`.
- `Do` (lines 232-340): reads `r := m.Rules()`; `refused(a, sets)` (line 345) rejects fouls, clock and period actions for a sets sport. `period_next` (lines 305-313):
  ```go
  case "period_next":
  	s.Period++
  	s.Clock, s.Running, clock = s.Length, false, true
  	if r.FoulsReset {
  		s.HomeFouls, s.AwayFouls = 0, 0
  	}
  	if r.ResetsTimeouts(s.Period) {
  		s.HomeTimeouts, s.AwayTimeouts = 0, 0
  	}
  ```
  `new_game` (lines 324-325) is `*s, clock = newScore(s.Length), true`, so the volunteer's `period_length` survives a new game. `period_length` (lines 316-323) moves a stopped clock sitting at the old length to the new one (`if !s.Running && s.Clock == s.Length { s.Clock = a.Tenths }`).
- `Seed` (lines 174-193) builds a fresh `Score{…}` from the console's values with `Length: m.s.Length` and no other carry-over.
- `Manual.Rules` is wired in `cmd/hallstream/main.go:194` to `sport.Get(st.GameSport())`: the game's escalão sport.

### `internal/state/state.go`

`setLive` (lines 820-865) reads `r := m.rules()`: `intervalo` when `oldP == r.Half`; back to `marcador` when the clock starts in `newP > r.Half`; the final hold starts when `newP >= r.Periods && t == 0`; `checkFinal` (line 936) waits `r.FinalHold`. A sets sport returns early into `setSets`. Futsal needs no state change.

### Pages

- `web/overlay/overlay.js`:
  - `view()` (line 486 on) sets `v.regulationPeriods = rules.periods ?? 4`, `v.foulWarn = rules.foul_warn ?? 4`, `v.foulBonus = rules.foul_bonus ?? 5`.
  - `periodShort` / `periodLong` (lines 468-481) pick "SET n" for a sets sport, else quarter labels (`overlay.periodo.curto` / `.longo`) up to `regulationPeriods`, else "PROL." (`overlay.periodo.prol`, `.prol_n`, `.prolongamento`, `.prolong_n`).
  - The period table (lines 663-667) uses the same labels (`overlay.periodos.n` for regulation).
  - `renderPips(team, n, warn)` (lines 596-604) lights `Math.min(n, 4)` of the markup's pips and sets `hot` at `warn`.
  - Bonus (lines 1243-1247): `bonus = v.foulBonus > 0 && n >= v.foulBonus`.
- `web/overlay/index.html` lines 71-72 (scoreboard) and 150, 155 (time-out screen): four fixed pips per team, `<span class="pips …" data-pips="home"><i></i><i></i><i></i><i></i></span>`.
- `web/marcar/marcar.js`:
  - `rulesOf()` (lines 136-143) returns `periods`, `half`, `timeouts`, `bonus` (`foul_bonus ?? 5`), and the sets fields.
  - `periodName(p, periods)` (lines 151-153): "Período n" / "Prolongamento" / "Prolongamento n".
  - Fouls (line 201): `marks(<fouls>, f, bonus)`, so basketball draws 5 marks; line 210 sets `data-bonus` at `f >= bonus`.
  - "Repor" (lines 349 and 353) shows and sets `score.length`.
- `web/marcar/index.html` lines 25 and 73: `data-max="5"` on the fouls marks, read by nothing.
- `internal/club/club.go:30-36`: `{"futsal", "FUTSAL", "FUTSAL", false}`; `internal/club/club_test.go:143` asserts futsal, handball and soccer are inactive.
- `web/painel/painel.js:2162`: `['futsal', t('painel.desporto.futsal')]` (no `true`, so disabled).
- `web/painel/mock.js:356`: the mock's `PUT /api/groups/:id` accepts only `['basketball', 'volleyball']`.
- `internal/web/overlay_test.go:86-91` globs `overlay/fixtures/*.json` and `overlay/fixtures/*/*.json`; `TestOverlayStyles` draws listed fixtures in every style.
- The overlay puts `game.sport` on `<html data-sport>` (#30); any sport but basketball already gets the neutral hall and Pavilhão without court marks.

### Guards that must stay green

- **Basketball stays pixel-identical.** `TestBasketballScreensDrawTheSameEveryTime` (`internal/web/basketball_stable_test.go`) must pass. Also compare screenshots: build `origin/main` and this branch to two scratch exes; run each with `-demo -nobrowser` and shoot `/overlay?state=/overlay/fixtures/<f>.json&preview=1&anim=0&style=<s>` for `f` in `marcador`, `intervalo`, `final` and `s` in `classico`, `lombada`, `cor`, `placa`, `pavilhao` with headless Chrome (`--headless=new --window-size=1920,1080 --virtual-time-budget=5000 --screenshot=<file>`). Every pair must be byte-identical (`cmp`). Without the time budget, fonts and crests race and the comparison is noise.
- **Tabular figures.** `web/overlay/overlay.css:91` gives tabular figures to `[data-b$="Score"]`, `[data-b$="Big"]`, `[data-b$="Sets"]`, `[data-b$="Fouls"]`, `clock`, `shot`, `startTime`. A new number bind must end in one of those suffixes or get its own rule. This plan adds none.

### Conventions

- Go, standard library only; plain `testing`. Model scorer tests on `internal/source/manual_test.go`, state tests on `internal/state/state_test.go`.
- Every user-facing string in `web/i18n/pt.json` (European Portuguese) then `en.json` (British English), same keys, sorted.
- `web/` ships as is. Every overlay screen or state has a fixture the Chrome test loads.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Never read or edit `dados/` or `dist/`.
- Commit style: `feat: futsal scoring from the phone`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -count=1 ./...` | all pass (Chrome tests skip without Chrome) |
| One package | `go test ./internal/sport/ ./internal/source/ ./internal/state/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Preview a fixture | with the program running, `http://127.0.0.1:8765/overlay?state=/overlay/fixtures/futsal/<name>.json&preview=1` | draws |

## Suggested executor toolkit

- `read-the-damn-docs` for the FIFA Futsal Laws of the Game.
- `build-ui`, extension branch, for the scoreboard and scorer-page changes, inside `docs/design/SYSTEM-R8.md` (amends `SYSTEM-R4.md` and `SYSTEM-R3.md`; the later file wins). Stop after its quality gate.

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go` (new tests only)
- `internal/web/game.go` (`SyncGame`'s switch), `internal/web/server.go` (the `lastSport` field only), a new `internal/web/sport_switch_test.go`
- `internal/state/state_test.go` (new tests only)
- `internal/club/club.go`, `internal/club/club_test.go` (futsal moves from the inactive list to active)
- `web/overlay/overlay.js`, `web/overlay/fixtures/futsal/*.json` (create), `web/overlay/index.html` (only `data-i18n` off the two `.bonus` spans at lines 71-72)
- `web/marcar/marcar.js`, `web/marcar/index.html`, `web/marcar/marcar.css` (the `nofouls` rule only)
- `internal/web/sport_test.go`, `internal/web/panel_sport_test.go`, `internal/web/profile_test.go`, `internal/web/overlay_test.go` (the edits listed in Step 6 and Step 5 only)
- `web/painel/painel.js`, `web/painel/mock.js`
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- `web/overlay/overlay.css`, and in `web/overlay/index.html` everything but the two bonus spans (Step 4): the four pip elements stay in the markup and `renderPips` adds a fifth for futsal at run time, so basketball's DOM is unchanged.
- `internal/ble/decode.go` and the console path. The decoder has no futsal code. Futsal on a console is "untested".
- Basketball's numbers, except overtime at 5:00 (Step 1; no existing test asserts the old 10:00).
- The website support table.

## Git workflow

- Branch: `01-feat/futsal`, the bottom of the stack (base: `main`).
- One commit per step, conventional commits. Do not push or open a PR unless told to.

## Steps

### Step 1: The futsal profile and basketball's 5:00 overtime

In `internal/sport/sport.go`, add to `Profile`:

```go
	OvertimeLength int    `json:"overtime_length"` // tenths in an overtime period; 0 = the regulation length
	FoulsCarry     bool   `json:"fouls_carry"`     // fouls of the last regulation period run on through overtime
	PeriodWord     string `json:"period_word"`     // "quarter", "half" or "set": which period labels the pages use
	FoulMarks      int    `json:"foul_marks"`      // foul marks the scorer page draws per team; 0 = no fouls row
	FoulPenalty    string `json:"foul_penalty"`    // the on-air word at FoulBonus: "bonus" (basketball) or "sixth" (futsal)
```

and the methods:

```go
// Overtime reports whether period is an overtime (after Periods).
func (p Profile) Overtime(period int) bool { return period > p.Periods }

// ResetsFouls reports whether starting period puts the team fouls back to 0.
func (p Profile) ResetsFouls(period int) bool
```

`ResetsFouls`: false when `!FoulsReset`; false for an overtime when `FoulsCarry`; else true.

Profiles:

- `Basketball`: add `OvertimeLength: 3000, PeriodWord: "quarter", FoulMarks: 5, FoulPenalty: "bonus"`.
- `Volleyball`: add `PeriodWord: "set"` (`FoulMarks` stays 0).
- New, registered in `profiles`:
  ```go
  var Futsal = Profile{ID: "futsal", Periods: 2, Half: 1, Length: 12000, ClockDown: true,
  	FoulsReset: true, FoulsCarry: true, FoulWarn: 5, FoulBonus: 6, FinalHold: 600,
  	TimeoutsBy: [3]int{1, 1, 0}, Scoring: "points", OvertimeLength: 3000, PeriodWord: "half", FoulMarks: 5,
  	FoulPenalty: "sixth"}
  ```

**Verify**: `go test ./internal/sport/` → pass, with new tests: `Get("futsal") == Futsal`; futsal `Overtime(2)` false, `Overtime(3)` true; basketball `Overtime(4)` false, `Overtime(5)` true; `Futsal.Timeouts` for periods 1..4 is `1,1,0,0`; futsal `ResetsFouls` true for 2, false for 3 and 4; basketball `ResetsFouls` true for 2..6.

### Step 2: The phone scorer and the sport switch

`Score` gets **no new fields** in this plan; it stays exactly as it is at `0d7570c`.

In `internal/source/manual.go`, with `r := m.Rules()`:

- **Overtime.** In `period_next`, after `s.Period++`: `s.Clock = r.OvertimeLength` when `r.Overtime(s.Period) && r.OvertimeLength > 0`, else `s.Length`. Basketball's period 5 now starts at 5:00.
- **Fouls.** Replace `if r.FoulsReset {` with `if r.ResetsFouls(s.Period) {`. (FIBA carries overtime fouls on from the 4th quarter; basketball keeps today's reset each period. That is unchanged on purpose: `Basketball.FoulsCarry` stays false.)
- Update the `Action` comment for `period_next`: fouls and timeouts follow the profile; an overtime runs `OvertimeLength` when the sport sets one.

**The server owns the switch of period length** (`internal/web/game.go`, `internal/web/server.go`):

- Add `lastSport string` to `Server`, next to `lastGame` (`server.go:102`), guarded by `syncMu`.
- In `SyncGame` (`game.go:187`, holds `syncMu`), after `sp` is computed (line 197): if `s.lastSport != "" && sp.ID != s.lastSport`, the new profile `p := sport.Get(sp.ID)` has `p.Length > 0`, `s.Manual != nil`, and `!s.Manual.Score().Running`, call `s.Manual.Do(source.Action{Do: "period_length", Tenths: p.Length})`. `period_length` already moves a stopped clock sitting at the old length to the new one (`manual.go:316-323`). Then set `s.lastSport = sp.ID` (always, also when nothing was switched).
- At startup `lastSport` is `""`, so the first `SyncGame` after a restart only records the sport: a saved custom length survives a restart, a console takeover (`Seed` keeps `m.s.Length`) and `new_game` (`newScore(s.Length)`). Nothing in `internal/source` changes for this.
- Known side effect: `Manual.Do` also counts as the phone calling in (`m.seen = m.Now()`), so the switch marks the scorer "connected" for up to 60 s. Accept it; if a test shows it matters on air, STOP and report rather than adding a new method.

**Repor.** `web/marcar/marcar.js` lines 349 and 353: "Repor" shows and sets the current period's length, computed from `game.rules` and the score: `period > rules.periods && (rules.overtime_length ?? 0) > 0 ? rules.overtime_length : score.length`.

**Existing tests:** none in `internal/source` change. `TestBadActions` (`manual_test.go:87`) and the volleyball tests stay as they are.

**Verify**: `go test ./internal/source/ ./internal/web/` → pass, with new tests:

- `internal/source` (fake `Now`, `Rules` returning `sport.Futsal`): fouls 4–3 in period 1 become 0–0 into period 2; from period 2 with fouls 5–2, `period_next` into period 3 keeps 5–2; period 3 starts at 3000; timeouts reset at period 2; with `sport.Basketball`, **period 5 starts at 3000**.
- `internal/web` (`newServer` from `server_test.go`, with a `source.Manual` wired as in `marcar_test.go`): a basketball game whose scorer has `period_length` 4800 keeps 4800 after a restart (a new server on the same store and a `Manual` restored from the same `manual.json`, then `SyncGame`); switching the game's escalão from basketball to futsal and calling `SyncGame` sets the scorer's length to 12000 and its stopped clock from 4800 to 12000; switching back sets 6000; a running clock is not touched by a switch.

### Step 3: The state machine

No code change: `setLive` already reads `Half`, `Periods` and `FinalHold`. Add tests only.

**Verify**: `go test ./internal/state/` → pass, with new tests modelled on `TestPeriodsAndScreens` (`internal/state/state_test.go:48`) on a machine whose game is futsal, set with `m.SetGame(state.Game{Sport: "futsal", Rules: sport.Futsal}, true)` (the real path; don't use `rulesFor`): period 1 → 2 records period 1 and shows `intervalo`; the clock running in period 2 returns to `marcador`; 0:00 held `FinalHold` in period 2 shows `final` and records period 2.

### Step 4: Overlay and scorer page

Go through `build-ui` (extension branch, R8). Only these change:

- **Pips.** In `renderPips` (`overlay.js:596-604`), make each `[data-pips]` span hold `count = warn > 0 ? warn : 4` children, appending `<i>` elements (no whitespace, as in the markup) or removing extras, and light `Math.min(n, count)`. Basketball keeps the markup's four; futsal gets five. `popPip` (line 408) is called at line 1174 with `p.children[k]` after `renderPips` has run (line 1241 renders before the animation pass reads the children), so the fifth pip exists by then; check that order holds after your change.
- **Bonus word.** The overlay's bonus state starts at `foul_bonus` (futsal's 6th foul); the pips go hot at `foul_warn` (the 5th). The word in the bonus cell follows `rules.foul_penalty`: `"bonus"` (or missing) is `overlay.bonus`; `"sixth"` is a new `overlay.sexta_falta`, pt "6.ª FALTA", en "6TH FOUL". `overlay.js` sets the two `.bonus` spans' `textContent` from that key in the render (next to the bonus class, lines 1243-1247), and the spans lose `data-i18n="overlay.bonus"` (`index.html:71-72`) so the page's i18n pass doesn't write the basketball word back over it. Basketball draws the same "BÓNUS", so its pixels don't move. "LIVRE DIRETO" / "DIRECT FREE KICK" was considered and rejected: it is about twice as long as "BÓNUS" and won't fit the cell. **STOP** if "6.ª FALTA" / "6TH FOUL" doesn't fit the cell in all five styles at the default scale (check with the fixture `futsal/marcador-faltas.json` in each style).
- **Period labels.** In `periodShort` / `periodLong` and the period table, when `rules.period_word === 'half'`, regulation periods use new keys in the overlay's uppercase style: `overlay.periodo.parte_curto` (pt "{n}ª P.", en "H{n}"), `overlay.periodo.parte_longo` (pt "{n}ª PARTE", en "HALF {n}"), `overlay.periodos.parte` (pt "{n}ª", en "H{n}"). Extra time keeps "PROL." counted from `periods`. Read the word into `v` in `view()` with `rules.period_word ?? 'quarter'`.
- **Scorer page.** Add `foulMarks: r.foul_marks ?? 5` and `word: r.period_word ?? 'quarter'` to `rulesOf()`. Line 201: `marks(<fouls>, f, foulMarks)`. Hide the fouls row with `document.body.classList.toggle('nofouls', foulMarks === 0)` (next to the existing `sets` toggle) and a `body.nofouls .faltas { display: none; }` rule in `web/marcar/marcar.css`; basketball and futsal never get the class. Line 210 keeps `data-bonus` at `foul_bonus` (basketball unchanged at 5, futsal 6). `periodName(p, periods)` becomes `periodName(p, periods, word)` and uses a new `marcar.parte` (pt "{n}ª parte", en "Half {n}") for regulation periods when `word` is `half`; update both callers (the period label in `render()` and the confirm in `ask()`). Remove the unused `data-max="5"` at `index.html:25` and `73`.

**Verify**: `go test -count=1 ./internal/web/ -run 'Overlay|Basketball|Marcar'` → pass; the basketball screenshot comparison in "Guards" → all 15 identical.

### Step 5: Fixtures

Create `web/overlay/fixtures/futsal/`, copying the shape of `web/overlay/fixtures/marcador.json` with `"game": { …, "sport": "futsal", "sport_label": "FUTSAL", "rules": <Futsal as JSON> }`:

- `marcador.json`: 2.ª parte, 12:34, 3–2, fouls 2–4.
- `marcador-faltas.json`: fouls 6–5.
- `intervalo.json`: after the first half, one period in the table.
- `prolongamento-faltas.json`: 1.º prolongamento, 3:12, 4–4, fouls 6–3 carried from the second half.
- `final-prolongamento.json`: after extra time, four periods.

Add `futsal/marcador` and `futsal/final-prolongamento` to the fixture list in `TestOverlayStyles` (`internal/web/overlay_test.go:128`).

**Verify**: `go test -count=1 ./internal/web/ -run Overlay` → pass with Chrome installed.

### Step 6: Turn it on

- `internal/club/club.go`: futsal `Active: true`; in `club_test.go:143` move futsal out of the inactive map and assert it is active.
- `internal/web/sport_test.go:26`: the loop `[]string{"curling", "futsal"}` expects 400 for both; futsal is now accepted (200, as volleyball is), so replace `"futsal"` with `"handball"`, still inactive, as the second 400 case, and add a futsal 200 case.
- `internal/web/panel_sport_test.go:91-92` asserts the futsal option is disabled: assert it is enabled now, and that the handball option is still disabled.
- `internal/web/profile_test.go:65`: the bad-profile table lists `{"sport": "futsal"}` as refused; replace it with `{"sport": "handball"}` (still inactive).
- `web/painel/painel.js:2162`: `['futsal', t('painel.desporto.futsal'), true]`.
- `web/painel/mock.js:356`: add `'futsal'` to the accepted sports.
- `docs/CONTRACT.md`: futsal's rules under the manual scorer section and in `game.rules` (the five new fields, basketball's 5:00 overtime); the console note "futsal on a console is untested: the decoder has no futsal code".
- `docs/ROADMAP.md` Phase C: tick futsal ("Done: scored from the phone; console untested").

**Verify**: `go vet ./... && go test -count=1 ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/sport`: Step 1 cases.
- `internal/source`: Step 2 cases.
- `internal/state`: Step 3 cases.
- Overlay: five futsal fixtures in `TestOverlayRendersEveryFixture`, two in `TestOverlayStyles`.
- Basketball: `TestBasketballScreensDrawTheSameEveryTime` and the screenshot comparison.

## Done criteria

- [ ] `go vet ./...`, `go test -count=1 ./...`, `GOOS=windows GOARCH=386 go build ./...` exit 0
- [ ] `ls web/overlay/fixtures/futsal/*.json` lists five files
- [ ] `grep -n '"futsal", "FUTSAL", "FUTSAL", true' internal/club/club.go` matches
- [ ] `grep -n 'data-max' web/marcar/index.html` → no match
- [ ] The 15 basketball screenshots are byte-identical to `main`
- [ ] `git diff --stat main -- internal/ble web/overlay/index.html` → no change
- [ ] Existing test edits limited to exactly: `internal/club/club_test.go:143`, `internal/web/sport_test.go:26`, `internal/web/panel_sport_test.go:91-92`, `internal/web/profile_test.go:65`, `internal/web/overlay_test.go:128` (fixture list); nothing in `internal/source`
- [ ] `plans/README.md` status row updated

## STOP conditions

- A "Current state" excerpt no longer matches the code and you can't map it one-to-one.
- The FIFA Laws differ from "Rules" above.
- A basketball screenshot differs from `main`, or an existing test outside the list in Done criteria fails.
- The scoreboard can't fit five pips in one of the styles without a layout change.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- `ResetsFouls` is the one place that decides fouls; handball and soccer set `FoulMarks: 0` and no fouls.
- Plans 006 and 007 reuse `Overtime`, `OvertimeLength`, `PeriodWord`, the `half` labels, and `SyncGame`'s sport switch (`lastSport`).
- When a futsal console session is recorded, add a decoder test and drop the "untested" note.

## Plan review

First review (approve-with-fixes) against `4093d0c`, applied. Reconciled at `0d7570c` against what #27-#30 actually shipped:

- Plan 004 shipped no `same()` helper; `Score` is still compared with `==`. The new `Score` fields are comparable, so nothing changes there.
- `Overtime`, the overtime length, `PeriodWord` and `FoulMarks` did not exist; Step 1 adds them all.
- **Design change after a second cold review.** Stamping the sport and the period length inside `Score` broke undo against `prev`, `TestBadActions`, a restart with a default score, `new_game`, and the volleyball whole-struct compare. `Score` now stays as it is; `SyncGame` owns the switch (`lastSport`, one `period_length` action when the game's sport changes and the clock is stopped), and "Repor" computes the period's length from `game.rules`. A restart starts `lastSport` empty, so it never resets a custom length.
- The scorer page keeps `data-bonus` at `foul_bonus`, so basketball's scorer page is unchanged; the earlier draft moved it to `foul_warn`, which would have changed basketball.
- The pip markup stays in `index.html`; `renderPips` adds the fifth pip for futsal, so basketball's DOM and pixels don't move.
- `club_test.go` and `mock.js` were missing from scope; both need futsal enabled.
- The goldfish review of this reconciled version is recorded in the stack's PR, not here.
