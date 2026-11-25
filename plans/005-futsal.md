# Plan 005: A club can score futsal: two 20-minute halves, fouls per half, one timeout each

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club/club.go web/overlay web/marcar web/painel/painel.js web/i18n docs/CONTRACT.md internal/web/overlay_test.go`
> Plans 002, 003 and 004 are expected to have changed most of these files.
> Read `internal/sport/sport.go` whole before starting and compare it with
> "What plans 002 to 004 leave behind" below. A shape you can't map
> one-to-one is a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (touches the scoring path that runs on air)
- **Depends on**: plans/002-sport-package-basketball.md (the `sport.Profile`), plans/003 (sport per escalão; the game's sport comes from its escalão), plans/004 (volleyball: introduces `web/overlay/fixtures/<sport>/` and the Chrome test glob that loads it, and any per-sport period word)
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

ROADMAP Phase C lists futsal: "the clock counts down from 20:00, 5 accumulated fouls". Futsal is one of the biggest indoor sports in Portugal, and every club that plays it alongside basketball can stream it with the same PC once this lands. Futsal is close to basketball (a stopped clock counting down, team fouls, timeouts), so this plan is mostly a new `sport.Profile` plus the few rules basketball doesn't have: halves instead of quarters, fouls that carry into extra time, a shorter extra time, and no shot clock.

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`): the sport is set per escalão, defaulting to the club's; the phone scorer supports each sport from day one; console support for a sport is listed as untested until a recorded real game passes a decoder test; each new scoreboard variant goes through `build-ui` inside the R8 design system.

## Rules (verify before building)

From the FIFA Futsal Laws of the Game (Law 7, duration; Law 12 and 13, accumulated fouls; Law 7 on timeouts). Read the current edition at fifa.com (use the `read-the-damn-docs` skill) and STOP if any of these differ:

- Two halves of 20:00, stopped clock (it stops whenever the ball is out of play), counting down.
- Half-time after the first half.
- One timeout per team per half; none in extra time.
- Accumulated fouls are counted per half. From the **6th** accumulated foul of a team in a half, the other side gets a direct free kick without a wall. So the 5th foul is the warning, the 6th the penalty.
- Accumulated fouls of the second half **carry into extra time**; they do not reset between extra-time periods.
- Extra time, where a competition needs a winner: two periods of 5:00.

## Current state

### What plans 002 to 004 leave behind (expected, verify)

From plan 002, `internal/sport/sport.go`:

```go
type Profile struct {
	ID          string `json:"id"`
	Periods     int    `json:"periods"`      // regulation periods (basketball 4)
	Half        int    `json:"half"`         // the period after which the half-time break falls (2)
	Length      int    `json:"length"`       // tenths in a regulation period (6000)
	ClockDown   bool   `json:"clock_down"`
	FoulsReset  bool   `json:"fouls_reset"`  // team fouls go back to 0 each period
	FoulWarn    int    `json:"foul_warn"`    // fouls at which the pips turn hot (4); 0 = none
	FoulBonus   int    `json:"foul_bonus"`   // fouls from which the other side shoots (5); 0 = none
	ShotClock   bool   `json:"shot_clock"`
	FinalHold   int    `json:"final_hold"`   // tenths at 0:00 in the last period before the game is over (600)
	TimeoutsBy  [3]int `json:"timeouts"`     // first half, second half, each overtime: (2, 3, 1)
}
func (p Profile) Timeouts(period int) int
func (p Profile) ResetsTimeouts(period int) bool
func Get(id string) Profile
```

Plan 002 defines **no** `Overtime` method; Step 1 adds it. Plan 002 keeps the overtime at the regulation length (its line "The overtime length (5:00) is not a field").

`State.Game.Rules` carries the profile to the pages as `game.rules`; `overlay.js` and `marcar.js` read `periods`, `half`, `foul_warn`, `foul_bonus` and `timeouts` from it with basketball fallbacks.

From plan 004: `Profile` gains `Scoring`, `SetPoints`, `LastSetPoints`, `SetsToWin`, `WinBy` and the methods `Sets()`, `SetTarget(n)`, `SetWon(n, home, away)`; `Volleyball` has `ClockDown: false` (no clock); fixtures live under `web/overlay/fixtures/volleyball/`; `internal/web/overlay_test.go` globs `overlay/fixtures/*.json` **and** `overlay/fixtures/*/*.json`. Plan 004 also replaced `if *s == prev` in `internal/source/manual.go` (line 232 at 4093d0c) with `same(a, b Score) bool`, which compares every field: **every field this plan adds to `Score` goes into `same()`**. Plan 004 adds no period word; Step 1 adds `PeriodWord`.

### Where the basketball-only assumptions still sit (at 4093d0c)

- `internal/source/manual.go:210-216`, `period_next` (after plan 002 it reads `r.FoulsReset` and `r.ResetsTimeouts`). It sets `s.Clock = s.Length` (line 212), and `new_game` uses `newScore(s.Length)` (line 228), so the volunteer's `period_length` survives a new game. Futsal needs 20:00 when an escalão switches to futsal, and 5:00 in extra time.
- `web/marcar/marcar.js:284` and `288`: "Repor" sets the clock to `score.length`, which in extra time would be 20:00.
- `web/overlay/index.html:70-71` and `148, 153`: the foul pips are four fixed elements, `<span class="pips" data-pips="home"><i></i><i></i><i></i><i></i></span>`. Futsal warns at 5, so the pip count must follow `rules.foul_warn`.
- `web/marcar/marcar.js:177` draws the fouls with `marks(<fouls element>, f, 5)`, a fixed 5 marks (`marks(el, n, max)` at line 124 draws `max(max, n)` marks). `web/marcar/index.html:25` and `65` carry a `data-max="5"` that nothing reads.
- `web/overlay/overlay.js:484`: `v.shot = live.shot ?? ''`. The manual scorer already sends `"shot_clock": ""` (`internal/source/manual.go:393`), so futsal shows no shot clock without change. Check the scoreboard hides an empty shot cell cleanly in each style.
- `internal/club/club.go:30-36`:
  ```go
  var Sports = []Sport{
  	{"basketball", "BASQUETEBOL", "BASKET", true},
  	{"futsal", "FUTSAL", "FUTSAL", false},
  	...
  ```
- `internal/web/profile.go:38-40` refuses an inactive sport with `api.clube.so_basquetebol`. After plan 003 this check may sit on the escalão as well.
- `web/painel/painel.js:2151-2152`: `var SPORTS = [['basketball', …, true], ['futsal', t('painel.desporto.futsal')], …]`; the third element `true` enables the option.

### Conventions

- Go, standard library only; plain `testing`. Model scorer tests on `internal/source/manual_test.go`, state tests on `internal/state/state_test.go`.
- Every user-facing string in `web/i18n/pt.json` (European Portuguese) then `en.json` (British English), same keys, sorted. `go test ./internal/i18n` checks it.
- `web/` ships as is. Every overlay screen or state has a fixture the Chrome test loads.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Never read or edit `dados/` or `dist/`.
- Commit style: `feat: futsal scoring from the phone`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| One package | `go test ./internal/sport/ ./internal/source/ ./internal/state/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel `http://127.0.0.1:8765/painel` |
| Preview a fixture | open `http://127.0.0.1:8765/overlay?state=/overlay/fixtures/futsal/<name>.json&preview=1` | draws |

## Suggested executor toolkit

- `read-the-damn-docs` for the FIFA Futsal Laws of the Game.
- `build-ui`, extension branch, for the scoreboard and scorer-page changes, inside `docs/design/SYSTEM-R8.md` (which amends `SYSTEM-R4.md` and `SYSTEM-R3.md`; the later file wins). Stop after its quality gate; the code review comes later.

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go` (new tests only)
- `internal/state/state_test.go` (new tests only)
- `internal/club/club.go` (`Active: true` for futsal)
- `web/overlay/overlay.js`, `web/overlay/index.html`, `web/overlay/overlay.css` (pip count only)
- `web/overlay/fixtures/futsal/*.json` (create)
- `web/marcar/marcar.js`, `web/marcar/index.html`
- `web/painel/painel.js` (enable futsal in `SPORTS`)
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- `internal/ble/decode.go` and the console path. The decoder has no futsal code (`internal/ble/decode.go:13-22` lists hockey, basketball, net, timer, tennis, volleyball, handball). Futsal on a console is "untested" (Step 6).
- Any change to basketball's numbers. Every existing test passes unchanged.
- The website support table (website repo, separate PR).

## Git workflow

- Branch: `feat/futsal`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The futsal profile

In `internal/sport/sport.go`, add fields only where 002's `Profile` lacks them:

```go
	// OvertimeLength is tenths in an overtime period; 0 = the regulation Length.
	OvertimeLength int `json:"overtime_length"`
	// FoulsCarry keeps the team fouls from the last regulation period through
	// every overtime (futsal); false resets them each period as FoulsReset says.
	FoulsCarry bool `json:"fouls_carry"`
```

Also add:

```go
	// PeriodWord picks the period labels: "quarter" (basketball), "half", "set".
	PeriodWord string `json:"period_word"`
	// FoulMarks is how many foul marks the scorer page draws per team (5); 0 = no fouls row.
	FoulMarks int `json:"foul_marks"`
```

Basketball: `PeriodWord: "quarter"`, `FoulMarks: 5` (what `marcar.js:177` draws today). Volleyball: `PeriodWord: "set"`, `FoulMarks: 0`.

And the method plan 002 doesn't have:

```go
// Overtime reports whether period p is an overtime (p > Periods).
func (p Profile) Overtime(period int) bool { return period > p.Periods }
```

Add a method so callers stop reading `FoulsReset` directly:

```go
// ResetsFouls reports whether starting period p puts team fouls back to 0.
func (p Profile) ResetsFouls(period int) bool
```

Basketball: true for every period (as `FoulsReset` today). Futsal: true for period 2 only (the second half), false for 3 and 4 (extra time carries the second half's fouls).

```go
var Futsal = Profile{ID: "futsal", Periods: 2, Half: 1, Length: 12000, ClockDown: true,
	FoulsReset: true, FoulsCarry: true, FoulWarn: 5, FoulBonus: 6, ShotClock: false,
	FinalHold: 600, TimeoutsBy: [3]int{1, 1, 0}, OvertimeLength: 3000, PeriodWord: "half", FoulMarks: 5}
```

Register it in `Get`. Basketball keeps `OvertimeLength: 0` so its overtime stays at the period length exactly as today (changing basketball's overtime to 5:00 is a separate decision; note it in the PR, don't do it here).

**Verify**: `go test ./internal/sport/` → pass, with new tests: `Get("futsal")` returns `Futsal`; `Overtime(2)` false and `Overtime(3)` true for futsal, `Overtime(4)` false and `Overtime(5)` true for basketball; `Futsal.Timeouts` for periods 1..4 is `1,1,0,0`; `ResetsTimeouts` true for 2; `ResetsFouls` true for 2, false for 3 and 4; basketball `ResetsFouls` true for 2..6; basketball's existing tests unchanged.

### Step 2: The phone scorer

In `internal/source/manual.go` (as plans 002 and 004 left it), with `r := m.Rules()`:

- **Which sport the score is for.** Add `Sport string \`json:"sport"\`` and `PeriodLength int \`json:"period_length"\`` to `Score`, and add both to `same()`. At the start of `Do` (after `m.s = m.current()`), if `s.Sport != r.ID`, set `s.Length = r.Length` and `s.Sport = r.ID`: a new sport starts from its own period length. Otherwise `s.Length` is the volunteer's (`period_length`) and stays, as today. Do the same in `Restore` for a saved score with an empty `Sport` only if its `Length` equals `DefaultLength` (an old basketball file keeps its custom length and gets `Sport: "basketball"`). If plan 006 landed first and added these fields, reuse them.
- **Regulation periods keep using `s.Length`** (`period_next` at line 212, `new_game` at line 228): no change for basketball.
- **Extra time.** In `period_next`, when `r.Overtime(s.Period) && r.OvertimeLength > 0`, set `s.Clock = r.OvertimeLength`; otherwise `s.Length`.
- **The current period's length.** After every action and in `Restore`, set `s.PeriodLength` to `r.OvertimeLength` when `r.Overtime(s.Period) && r.OvertimeLength > 0`, else `s.Length`. The `/api/marcar` reply already returns the whole `Score`, so the page gets it.
- **Fouls.** `if r.ResetsFouls(s.Period) { s.HomeFouls, s.AwayFouls = 0, 0 }` in place of the `FoulsReset` check.
- Update the `Action` doc comment for `period_next`: fouls and timeouts follow the sport's profile, and an overtime runs `OvertimeLength` when the sport sets one.
- `web/marcar/marcar.js:284` and `288`: "Repor" shows and sets `score.period_length || score.length`, so futsal extra time resets to 5:00.

**Verify**: `go test ./internal/source/` → pass, with new tests (a `Manual` whose `Rules` returns `sport.Futsal`): the first action on a score saved for basketball sets `Length` to 12000 and `Sport` to `"futsal"`; a `period_length` of 15:00 survives `new_game` while the sport stays futsal; fouls 4–3 in period 1 go to 0–0 on `period_next` into period 2; from period 2 with fouls 5–2, `period_next` into period 3 keeps 5–2; period 3 starts at 3000 tenths and `PeriodLength` is 3000; timeouts reset to 0 at period 2; basketball tests unchanged.

### Step 3: The state machine

Plan 002 made `setLive` read `r.Half`, `r.Periods` and `r.FinalHold`. With futsal (`Half: 1`, `Periods: 2`):

- `intervalo` fires when period 1 ends.
- The final hold starts at 0:00 in period 2 or later.

No code change is expected. Add tests only.

**Verify**: `go test ./internal/state/` → pass, with new tests modelled on `TestPeriodsAndScreens` (`internal/state/state_test.go:48`), using a game whose `Sport` is `"futsal"`: period 1 → 2 records period 1 and switches to `intervalo`; clock running in period 2 switches back to `marcador`; 0:00 held for `FinalHold` in period 2 switches to `final` and records period 2.

### Step 4: Overlay and scorer page

Go through `build-ui` (extension branch, R8). The scoreboard keeps its layout; only these change:

- `web/overlay/index.html` lines 70-71, 148, 153: render the pips from `v.foulWarn` (basketball 4, futsal 5) instead of four fixed `<i>`: in `renderPips` (`overlay.js:556-564`), make the span hold exactly `foulWarn` children (create or remove `<i>` elements, as `marcar.js:126` does with `replaceChildren`). Basketball stays at 4.
- Warning and bonus, stated once: **the scorer page warns from `foul_warn` (5th foul); the overlay's bonus state starts at `foul_bonus` (6th foul).** `overlay.js:1163-1164` already uses `v.foulBonus` after plan 002. The label stays `overlay.bonus` ("BÓNUS"/"BONUS"); no new key.
- Period labels: with `rules.period_word === "half"`, use one key per word, in the overlay's uppercase style (`pt.json:459-464`): `overlay.periodo.parte_curto` (pt "{n}ª P.", en "H{n}") and `overlay.periodo.parte_longo` (pt "{n}ª PARTE", en "HALF {n}"); for the scorer page, `marcar.parte` (pt "{n}ª parte", en "Half {n}"). Extra time keeps "PROL." with its number counted from `periods`.
- Period table on `intervalo` and `final` (`overlay.js:587-591`): headers follow the same words.
- `web/marcar/marcar.js:177`: `marks(…, f, rules.foul_marks ?? 5)`; hide the fouls row when `foul_marks` is 0. Remove the unused `data-max="5"` at `index.html:25` and `65`. `marcar.js:135-136` `periodName` uses `marcar.parte` for `period_word === "half"`. The `marcar.bonus` warning shows from `foul_warn` fouls.

**Verify**: with the demo running, preview the futsal fixtures (Step 5) and basketball's `marcador-bonus.json`: basketball still shows 4 pips and "Bónus" at 5; futsal shows 5 pips, hot at 5, free-kick warning at 6.

### Step 5: Fixtures

Create `web/overlay/fixtures/futsal/` with five files, copying the shape of `web/overlay/fixtures/marcador.json` and adding `"game": { …, "sport": "futsal", "rules": <the Futsal profile as JSON> }`:

- `marcador.json`: 2.ª parte, 12:34, 3–2, fouls 2–4.
- `marcador-faltas.json`: fouls 6–5 (one side past the 6th, one at the warning).
- `intervalo.json`: after the first half, period table with one row.
- `prolongamento-faltas.json`: 1.º prolongamento, 3:12, 4–4, fouls 6–3 carried from the second half.
- `final-prolongamento.json`: after extra time, four periods.

**Verify**: `go test ./internal/web/ -run Overlay` → pass with Chrome installed (the glob from plan 004 picks them up).

### Step 6: Turn it on

- `internal/club/club.go`: futsal `Active: true`.
- `web/painel/painel.js:2151`: `['futsal', t('painel.desporto.futsal'), true]`.
- `docs/CONTRACT.md`: futsal's rules under the manual scorer section (line 137 onward) and in `game.rules`; the console note: "futsal on a console is untested: the decoder has no futsal code, and no recorded game has been checked".
- `docs/ROADMAP.md` Phase C: tick futsal ("Done: scored from the phone; console untested").

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0. In the demo, Definições lists Futsal as selectable, and an escalão set to futsal scores on `/marcar` with halves, 1 timeout and 5 foul marks.

## Test plan

- `internal/sport/sport_test.go`: Step 1 cases.
- `internal/source/manual_test.go`: Step 2 cases, new test functions only.
- `internal/state/state_test.go`: Step 3 cases, new test functions only.
- Overlay: five futsal fixtures loaded by the Chrome test.
- Basketball: every existing test passes unchanged.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `ls web/overlay/fixtures/futsal/*.json` lists five files
- [ ] `grep -n '"futsal", "FUTSAL", "FUTSAL", true' internal/club/club.go` matches
- [ ] `grep -c "<i></i><i></i><i></i><i></i>" web/overlay/index.html` → 0
- [ ] `git diff --stat 4093d0c..HEAD -- internal/ble` shows no change from this branch
- [ ] No existing test function edited (`git diff` on `*_test.go` shows additions only)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 004's fixtures folder, its Chrome-test glob, or the `Profile` shape differs from "What plans 002 to 004 leave behind" in a way you can't map one-to-one.
- The FIFA Laws you read differ from "Rules" above (foul count, timeouts, extra time).
- The game's sport does not reach `state.Game.Sport` from the escalão (plan 003 not done).
- An existing basketball test fails.
- The scoreboard can't fit the free-kick warning in one of the styles without a layout change beyond the pip count.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- `OvertimeLength` exists now; basketball's overtime still uses the regulation length because the scorer always did. FIBA overtime is 5:00. Changing basketball is a behaviour change for the owner to decide.
- Handball (plan 006) and soccer (plan 007) count up; they add their own clock fields. Keep `ResetsFouls` as the single place that decides fouls.
- When a futsal console session is recorded (Phase D capture), add a decoder test and change the CONTRACT note from untested.
- Reviewers: watch a full demo futsal game to `final`; check the 5th and 6th foul states on every scoreboard style.

## Plan review

One cold review, approve-with-fixes; the fixes are in. Contested: none. Choices settled in review: the overlay keeps `overlay.bonus` for futsal (no free-kick label), one key per period word, and the custom period length survives a new game unless the sport changes.
