# Plan 006: A club can score handball: a clock that counts up, and 2-minute suspensions on air

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club/club.go internal/web/marcar.go web/overlay web/marcar web/painel/painel.js web/i18n docs/CONTRACT.md internal/web/overlay_test.go`
> Plans 002, 003 and 004 (and 005 if it landed first) are expected to have
> changed most of these files. Read `internal/sport/sport.go` and
> `internal/source/manual.go` whole before starting and compare them with
> "What earlier plans leave behind" below. A shape you can't map one-to-one
> is a STOP condition.

## Status

- **Priority**: P2
- **Effort**: L
- **Risk**: MED (new clock mode in the scoring path that runs on air)
- **Depends on**: plans/002-sport-package-basketball.md (`sport.Profile`), plans/003 (sport per escalão), plans/004 (volleyball: the `web/overlay/fixtures/<sport>/` folder and its Chrome-test glob). Not on 005, but if 005 landed first, reuse its `OvertimeLength` and `PeriodWord` fields instead of adding them.
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

ROADMAP Phase C lists handball: "the clock counts up, 2×30, 2-minute suspensions". The Stramatel decoder already reads handball frames (`internal/ble/decode.go:21`, `'=': "handball"`), so handball clubs are the most likely next console users. Two things in Hallstream assume basketball and block handball: the phone scorer's clock only counts down, and nothing on air shows a suspension. This plan adds a count-up clock mode (plan 007, soccer, reuses it) and suspension timers.

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`):

- **Suspensions: one tap starts a 2:00 timer for a team; the shirt number is optional and can be added afterwards.** The overlay shows the number only when entered. Up to 3 running timers per team.
- The sport is set per escalão, defaulting to the club's.
- The phone scorer supports the sport from day one; console support stays "untested" until a recorded real game passes a decoder test.
- New scoreboard variants go through `build-ui` inside the R8 design system.

## Rules (verify before building)

From the IHF Rules of the Game (Rule 2, playing time and team timeouts; Rule 16, suspensions). Read the current edition at ihf.info (use `read-the-damn-docs`) and STOP if these differ:

- Two halves of 30:00. Half-time after the first.
- The clock counts up. Most scoreboards run the second half on from 30:00 to 60:00; this plan does the same.
- Team timeouts: 3 per team per match, at most 2 in one half (and at most one in the last 5 minutes of regular time; this plan does not enforce that one).
- Suspensions are 2:00 of playing time: the suspension clock stops whenever the game clock stops.
- Extra time, where a winner is needed: two periods of 5:00, after the second half's score is level.

## Current state

### What earlier plans leave behind (expected, verify)

From plan 002, `internal/sport/sport.go` has `Profile{ID, Periods, Half, Length, ClockDown, FoulsReset, FoulWarn, FoulBonus, ShotClock, FinalHold, TimeoutsBy [3]int}` with `Timeouts`, `ResetsTimeouts` and `Get` (no `Overtime` method: plan 005 adds it; add it here if 005 hasn't landed, `period > Periods`). `ClockDown` is `true` for basketball and `false` for volleyball (plan 004: no clock at all), and no code reads it yet. Plan 004 adds `Scoring`, `SetPoints`, `LastSetPoints`, `SetsToWin`, `WinBy` and `Sets()`, and replaces `if *s == prev` in `internal/source/manual.go` (line 232 at 4093d0c) with `same(a, b Score) bool`, which compares every field: **every field this plan adds to `Score` goes into `same()`**. If plan 005 landed first, it added `Score.Sport`, `Score.PeriodLength`, `OvertimeLength`, `PeriodWord` and `FoulMarks`; reuse them. `state.Game.Rules` sends the profile to the pages as `game.rules`, and `overlay.js` and `marcar.js` read it with basketball fallbacks. From plan 004: `internal/web/overlay_test.go` also globs `overlay/fixtures/*/*.json`.

### The scorer's clock today (at 4093d0c)

`internal/source/manual.go`:

- line 17-19: `Clock int // tenths of a second left in the period`, `Running bool // the clock is counting down`, `Length int`.
- line 37: `func newScore(length int) Score { return Score{Period: 1, Clock: length, Length: length} }`.
- lines 191-194, `clock_start` only starts when there is time left:
  ```go
  case "clock_start":
  	if !s.Running && s.Clock > 0 {
  		s.Running, m.from, m.fromClock = true, m.Now(), s.Clock
  	}
  ```
- lines 351-358, `current()` counts down and stops at 0:
  ```go
  // current is the score with the running clock brought up to now; it stops at 0.
  func (m *Manual) current() Score {
  	s := m.s
  	if s.Running {
  		s.Clock = max(m.fromClock-int(m.Now().Sub(m.from)/(100*time.Millisecond)), 0)
  		s.Running = s.Clock > 0
  	}
  	return s
  }
  ```
- `period_next` (after plan 002) sets `s.Clock = s.Length`; `new_game` uses `newScore(s.Length)`; `Restore` and `Seed` (lines 115-145) assume a countdown (`Seed` sets the clock to `s.Length` when the console sent none).
- line 232: `if *s == prev { return nil }` at 4093d0c; after plan 004 it is `if same(*s, prev)`. New `Score` fields go into `same()`.
- line 212 (`period_next`) and line 228 (`new_game`) use the scorer's saved `s.Length`, so a custom `period_length` survives. This plan keeps that: count-up starts and ends derive from `s.Length`, never from `Profile.Length`.
- `emit()` (lines 381-395) sends a `map[string]string` to the state machine (`state.Machine.ApplyConsole`, `internal/state/state.go:666-689`). New values travel in that map.

`internal/state/state.go`:

- lines 827-833, `ClockText` rounds **up** to the second and shows tenths under a minute (`"12.3"`): right for a countdown, wrong for a clock counting up from 0:00.
- `setLive` (lines 727-764, after plan 002): "started" means the clock text went **down** (`t < m.lastTenths`); the final hold waits for **0:00**.

### The scorer page and overlay

- `web/marcar/index.html:14-35` (home) and `53-76` (away): each team has points buttons `+1 +2 +3`, a fouls row and a timeouts row. Handball scores one goal at a time and has no team fouls.
- `web/overlay/index.html:70-71`, `148`, `153`: the foul rows (`data-bonus`, `data-pips`).
- The lower third (`state.LowerThird`, `internal/state/state.go:163-172`) is not used for suspensions.

### Conventions

- Go, standard library only; plain `testing`; model on `internal/source/manual_test.go` and `internal/state/state_test.go`.
- Every user-facing string in `web/i18n/pt.json` then `en.json`; `go test ./internal/i18n` checks it. The program speaks in the first person ("Não consegui…").
- Every overlay screen or state has a fixture the Chrome test loads. The overlay stays on air for hours: a state it doesn't expect must still draw.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Commit style: `feat: handball scoring with suspensions`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass |
| Scorer and state | `go test ./internal/sport/ ./internal/source/ ./internal/state/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel `http://127.0.0.1:8765/painel`, scorer `/marcar` |

## Suggested executor toolkit

- `read-the-damn-docs` for the IHF rules.
- `build-ui`, extension branch, inside `docs/design/SYSTEM-R8.md`, for the suspension chips on the scoreboard and the scorer page's suspension row. Stop after its quality gate.

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go` (new tests)
- `internal/state/state.go`, `internal/state/state_test.go` (new tests)
- `internal/club/club.go` (handball `Active: true`)
- `web/marcar/index.html`, `web/marcar/marcar.js`, `web/marcar/marcar.css`
- `web/overlay/index.html`, `web/overlay/overlay.js`, `web/overlay/overlay.css`
- `web/overlay/fixtures/handball/*.json` (create)
- `web/painel/painel.js` (enable handball)
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- `internal/ble/decode.go`: it already reads handball frames, but without a recorded game nothing proves the field mapping. Console handball stays "untested".
- Suspensions from a console. The console path sends none; the scoreboard shows the chips only from the phone scorer.
- Player-level stats, warnings (yellow cards in handball), disqualifications.
- Any change to basketball or futsal numbers.

## Git workflow

- Branch: `feat/handball`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The handball profile and the count-up fields

In `internal/sport/sport.go` add, where absent:

```go
	// StopAtEnd stops a count-up clock at the end of the period (handball);
	// false lets it run on (soccer's stoppage time, plan 007). Countdowns stop at 0.
	StopAtEnd bool `json:"stop_at_end"`
	// OvertimeLength is tenths in an overtime period; 0 = the regulation Length.
	OvertimeLength int `json:"overtime_length"`
	// PeriodWord picks the period labels: "quarter", "half", "set".
	PeriodWord string `json:"period_word"`
	// Suspensions is how long a suspension runs, in tenths; 0 = the sport has none.
	Suspensions int `json:"suspensions"`
```

and these methods:

```go
// Start is the clock at the start of period p, given the scorer's regulation
// period length (Score.Length). Countdown: length, or OvertimeLength in an
// overtime when set. Count-up: the time played before p (handball period 2
// starts at 30:00, the first overtime at 60:00).
func (p Profile) Start(period, length int) int

// End is where period p ends on a count-up clock (Start of p plus its
// length), or 0 for a countdown.
func (p Profile) End(period, length int) int
```

Handball:

```go
var Handball = Profile{ID: "handball", Periods: 2, Half: 1, Length: 18000, ClockDown: false,
	StopAtEnd: true, FinalHold: 600, TimeoutsBy: [3]int{2, 2, 0}, OvertimeLength: 3000,
	PeriodWord: "half", Suspensions: 1200}
```

No team fouls: `FoulWarn: 0`, `FoulBonus: 0`, `FoulsReset: false`. Timeouts: the counter goes back to 0 at the second half and the page shows 2 marks per half; the 3-per-match total is not enforced (the volunteer at the table knows it). Register in `Get`.

An overtime's length is `OvertimeLength`, or `length` when that is 0.

**Verify**: `go test ./internal/sport/` → pass, with tests (length 18000): handball `Start(1)=0`, `Start(2)=18000`, `Start(3)=36000`, `Start(4)=39000`; `End(1)=18000`, `End(2)=36000`, `End(3)=39000`; with length 15000 (a youth game) `Start(2)=15000`, `End(2)=30000`; basketball (length 6000) `Start(n)=6000` for every n and `End(n)=0`.

### Step 2: The scorer's clock counts up when the profile says so

In `internal/source/manual.go`, with `r := m.Rules()`:

- **Which sport the score is for** (skip if plan 005 already did this). Add `Sport string \`json:"sport"\`` to `Score` and to `same()`. At the start of `Do`, if `s.Sport != r.ID`, set `s.Length = r.Length` and `s.Sport = r.ID`; otherwise keep `s.Length`, so a volunteer's `period_length` (say 25:00 for a youth game) survives a new game.
- `current()`: for a count-up, `s.Clock = fromClock + elapsed`; when `r.StopAtEnd`, cap at `r.End(s.Period, s.Length)` and stop there (`Running = false`); otherwise cap at `MaxClock`. Countdown unchanged.
- `clock_start`: a count-up may start whenever it is below its end (or always, when not `StopAtEnd`). Countdown unchanged (`s.Clock > 0`).
- `period_next`: `s.Clock = r.Start(s.Period, s.Length)` (after the increment). For basketball this equals today's `s.Length`.
- `new_game` and `newScore`: start at `r.Start(1, s.Length)` (0 for handball).
- `Seed`: when the console sent no clock, use `r.Start(p, s.Length)` instead of `s.Length`.
- `emit()`: also send `"period_end"`, the tenths where the current period ends (`r.End(s.Period, s.Length)`; `""` for a countdown), so the state machine knows the end without knowing `s.Length`.
- `Restore`: keep the clamp to `[0, MaxClock]`.
- Update the field comments: `Clock` is "tenths on the clock: left in the period for a countdown, played in the game for a count-up"; `Running` is "the clock is running".

**Verify**: `go test ./internal/source/` → pass, with new tests (fake `Now`, `Rules` returning `sport.Handball`): a new game shows 0; after `clock_start` and 65 s the clock is 650; it stops at 18000 in period 1; `period_next` starts period 2 at 18000; with `period_length` 15000 the second half starts at 15000 and survives `new_game`; basketball tests unchanged.

### Step 3: Clock text and the state machine for a count-up

- `internal/state/state.go`: add `ClockTextUp(t int) string` that floors to the second and always shows `MM:SS` (`"00:12"`, `"30:00"`, `"59:59"`). The scorer's `emit()` uses it when the profile counts up; `ClockText` stays for countdowns. `Tenths` already parses `MM:SS`.
- `setLive`, with `r := sport.Get(m.s.Game.Sport)`:
  - "started": for a count-up, the clock went **up** (`t > m.lastTenths`).
  - the period's end: add `PeriodEnd int \`json:"period_end"\`` to `Live`, read from `v["period_end"]` in `ApplyConsole` (0 when absent). Use `l.PeriodEnd`, or `r.End(newP, r.Length)` when it is 0 (a console frame).
  - final hold: for a count-up with `StopAtEnd`, the clock sits at that end (or more) in period `>= r.Periods`; for a countdown, at 0 as today. Soccer (no `StopAtEnd`) is plan 007's job; leave a countdown-or-StopAtEnd condition so soccer never triggers the hold by accident.
- `web/marcar/marcar.js` has a client-side `clockText` (used at line 284): make it match `ClockTextUp` for count-up rules. Its "at 0:00 the button offers the next period" logic (around lines 191-198) becomes "at the end of the period" for a count-up (`clock >=` the period's end, computed from `rules` and `score.length` the same way as `Profile.End`).

**Verify**: `go test ./internal/state/` → pass, with new tests: `ClockTextUp(125)` is `"00:12"`, `ClockTextUp(18000)` is `"30:00"`; a handball game going period 1 → 2 records period 1 and shows `intervalo`; the clock rising in period 2 returns to `marcador`; sitting at 36000 for `FinalHold` in period 2 shows `final`; basketball tests unchanged.

### Step 4: Suspensions in the scorer

Add to `Score` (fixed arrays, so three per team is built in; add them to `same()`):

```go
	HomeSusp [3]Suspension `json:"home_susp"`
	AwaySusp [3]Suspension `json:"away_susp"`

// Suspension is one running 2-minute suspension; an empty slot has ID 0.
type Suspension struct {
	ID     int    `json:"id"`     // 1 and up, unique in the game
	Number string `json:"number"` // shirt number, "" until the scorer adds it
	End    int    `json:"end"`    // the game clock (count-up, tenths) at which it ends
}
```

and a `NextSusp int` counter on `Score` for IDs.

New actions (document them in the `Action` comment and add `ID int \`json:"id"\`` and `Number string \`json:"number"\`` to `Action`):

- `"suspension"` with `Team`: fills the first empty slot with `{ID: next, End: s.Clock + r.Suspensions}` (the clock at the tap plus 2:00). All three full: `i18n.Key("manual.suspensoes_cheias")` (pt "Esta equipa já tem 3 exclusões a correr.", en "This team already has 3 suspensions running."). A profile with `Suspensions == 0`: `manual.sem_exclusoes` (pt "Esta modalidade não tem exclusões.", en "This sport has no suspensions.").
- `"suspension_number"` with `ID` and `Number` (1 to 3 digits, or "" to clear): sets the number. Unknown ID: `manual.exclusao_desconhecida` (pt "Essa exclusão já acabou.", en "That suspension has already ended.").
- `"suspension_end"` with `ID`: clears the slot early (a goal by the other side does not end a handball suspension, but a mistaken tap needs a way out besides undo).
- Undo takes these back like any other action.

Timing: the time left is `End - Clock`. Because `End` is a point on the game clock, a suspension only loses time while the clock runs, a tap while the clock is running needs no bookkeeping, and an undo that puts back an older `Score` puts back the right time left. In `current()` (and so in every `Do`, `Tick` and `Score`), empty every slot whose `End <= s.Clock`. The count-up clock carries across the half and into overtime (`Start(2)` is the end of period 1), so a suspension taken at 29:00 ends at 31:00 in the second half with no special case. If the scorer sets the clock back with `clock_set`, time left grows by the same amount; that is consistent with "time left follows the game clock" and needs no special case either.

`emit()`: send `"suspensions"` as a JSON string, `[{"team":"home","id":1,"number":"7","left":"01:23"}, …]`, with `left` = `ClockTextUp(End - Clock)`, `MM:SS`, only non-empty slots, home first. Empty list as `"[]"`.

**Verify**: `go test ./internal/source/` → pass, with new tests: a tap at clock 5000 starts a slot with `End` 6200; a fourth tap on the same team errors and changes nothing; after the clock ran 30 s the time left is 900, and it stays 900 while the clock is stopped; at `End` the slot empties; a tap while the clock runs, then 10 s, then undo of an unrelated point, leaves the suspension's time left at 1100; `suspension_number` sets "7"; undo of a `suspension` removes it; `same()` reports a change when only a suspension changed.

### Step 5: Suspensions in the state

- `state.Live`: add `Suspensions []Suspension \`json:"suspensions"\`` with `type Suspension struct{ Team, Number, Left string; ID int }` and JSON tags `team`, `number`, `left`, `id`.
- `ApplyConsole` (`state.go:666-689`): decode `v["suspensions"]` with `json.Unmarshal`; on a decode error or an empty value, leave the list empty. Never fail the frame.
- Game log: through the `seen` diff in `emit()` (`state.go:330-384`), never from `setLive`. Add `susp int` to `seen` (the highest suspension ID in `Live.Suspensions`, filled in `seenNow`), and in `emit()` call `m.OnEvent("suspension", map[string]any{"team": …, "number": …})` for each suspension whose ID is above `old.susp`, as periods do at lines 380-383. The demo is muted there already (`old.demo || cur.demo`).
- `docs/CONTRACT.md`: document `live.suspensions`, the three actions, and the timeline event.

**Verify**: `go test ./internal/state/` → pass, with tests: a frame with one suspension fills `Live.Suspensions`; a bad JSON value leaves it empty and the score still updates; a new ID fires one `suspension` event; nothing fires while the demo runs.

### Step 6: Scorer page and overlay

Go through `build-ui` (extension branch, R8):

- `web/marcar/index.html` and `marcar.js`, when `rules.id === "handball"` (or more generally `rules.suspensions > 0` and `rules.foul_warn === 0`):
  - points buttons: `+1` only (goals). A long press on it takes one away, as the existing handler at `marcar.js:243-254` does for every `data-long` button; keep `data-long` on it.
  - hide the fouls row; show a suspensions row per team: a "+2'" button (`data-do="suspension"`), and each running suspension as a chip with its time left and, when tapped, a small number field (`suspension_number`) and an "Acabou" button (`suspension_end`).
  - timeouts row: 2 marks per half from `rules.timeouts`.
  - period word "half" as in plan 005: keys `overlay.periodo.parte_curto` (pt "{n}ª P.", en "H{n}"), `overlay.periodo.parte_longo` (pt "{n}ª PARTE", en "HALF {n}") and `marcar.parte` (pt "{n}ª parte", en "Half {n}"); add them if 005 hasn't landed.
- `web/overlay/index.html` and `overlay.js`: under each team on the scoreboard, up to 3 suspension chips `2:00 · 7` (number only when set), drawn from `live.suspensions`. No fouls row for handball (hide when `foul_warn` is 0). Check every scoreboard style and the reduced bug.
- New keys (pt, en): `marcar.exclusao` ("+2'" / "+2'"), `marcar.exclusao_numero` ("Número" / "Number"), `marcar.exclusao_acabou` ("Acabou" / "Ended"), `marcar.exclusoes` ("Exclusões" / "Suspensions"), `overlay.exclusao` (aria label: "Exclusão {numero} {tempo}" / "Suspension {numero} {tempo}").

**Verify**: in the demo, with an escalão set to handball, `/marcar` shows +1, the suspension row and two timeout marks; three taps start three chips, a fourth shows the error; the overlay shows the chips ticking only while the clock runs.

### Step 7: Fixtures and turning it on

- `web/overlay/fixtures/handball/` (copy the shape of `web/overlay/fixtures/marcador.json`, with `game.sport: "handball"` and `game.rules` = the Handball profile JSON):
  - `marcador.json`: 2.ª parte, 41:17, 18–17, no suspensions.
  - `marcador-exclusoes.json`: home 2 suspensions (one with number "7", one without), away 3.
  - `intervalo.json`: after the first half.
  - `final.json`: 60:00, two periods.
- `internal/club/club.go`: handball `Active: true`. `web/painel/painel.js:2151`: enable handball.
- `docs/ROADMAP.md` Phase C: tick handball ("Done: scored from the phone, with suspensions; console untested").
- `docs/CONTRACT.md`: handball's rules, the count-up clock (`ClockTextUp`), and "handball on a console is untested".

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0; with Chrome, the four handball fixtures load in every style.

## Test plan

- `internal/sport`: `Start`/`End` for handball and basketball.
- `internal/source`: count-up clock, period starts, suspension slots, timing, numbers, undo.
- `internal/state`: `ClockTextUp`, count-up screens and final hold, suspension decoding and the timeline event.
- Overlay: four handball fixtures in the Chrome test.
- All existing tests pass unchanged.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `ls web/overlay/fixtures/handball/*.json` lists four files
- [ ] `grep -n '"handball", "ANDEBOL", "ANDEBOL", true' internal/club/club.go` matches
- [ ] `grep -n "suspension" docs/CONTRACT.md` finds the actions and `live.suspensions`
- [ ] `git diff --stat 4093d0c..HEAD -- internal/ble` shows no change from this branch
- [ ] No existing test function edited
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 004's fixtures folder, its Chrome-test glob, or the `Profile` shape differs from what this plan assumes in a way you can't map one-to-one.
- The IHF rules you read differ from "Rules" above.
- The game's sport does not reach `state.Game.Sport` from the escalão (plan 003 not done).
- An existing basketball or futsal test fails.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Plan 007 (soccer) reuses the count-up clock with `StopAtEnd: false` and adds stoppage time; it must not change handball's behaviour.
- The suspension timer follows the scorer's clock. A console sends no suspensions; if a recorded handball session shows the box does, map them in the decoder and drop the phone-only note.
- Reviewers: run a demo handball game with three suspensions across the half; check the chips stop when the clock stops, carry into the second half, and that undo restores a cleared chip.

## Plan review

One cold review, revise; the fixes are in. Settled in review: the period length stays the scorer's `s.Length` (so `Start`/`End` take it as an argument), suspensions store their end on the game clock instead of a running `Left`, timeline events go through `emit()`'s `seen` diff, and `+1` is the only goal button with `-1` on the existing long press. Contested: none.
