# Plan 006: A club can score handball: a clock that counts up, and 2-minute suspensions on air

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0d7570c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club cmd/hallstream/main.go internal/web/overlay_test.go web/overlay web/marcar web/painel docs/CONTRACT.md`
> Plan 005 (futsal, the layer below in the stack) changes most of these
> files. Read `internal/sport/sport.go` and `internal/source/manual.go` whole
> before starting and check that what "After plan 005" lists exists. A shape
> you can't map one-to-one is a STOP condition.

## Status

- **Priority**: P2
- **Effort**: L
- **Risk**: MED (a new clock mode in the scoring path that runs on air)
- **Depends on**: plan 005 (futsal), layer 01 of this stack. Plans 002-004 and #30 are on `main`.
- **Category**: direction
- **Planned at**: commit `0d7570c`, 2026-11-25 (reconciled; first written against `4093d0c`)
- **Stack**: layer 02 (base: `01-feat/futsal`). Plan 007 builds on this one.

## Why this matters

ROADMAP Phase C lists handball: "the clock counts up, 2×30, 2-minute suspensions". The Stramatel decoder already reads handball frames (`internal/ble/decode.go`, `'=': "handball"`), so handball clubs are the likeliest next console users. Two things assume basketball and block handball: the phone scorer's clock only counts down, and nothing on air shows a suspension. This plan adds a count-up clock mode (plan 007, soccer, reuses it) and suspension timers.

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`):

- **Suspensions: one tap starts a 2:00 timer for a team; the shirt number is optional and can be added afterwards.** The overlay shows the number only when entered. Up to 3 running timers per team.
- The sport is set per escalão, defaulting to the club's.
- The phone scorer supports the sport from day one; console support stays "untested" until a recorded real game passes a decoder test.
- New scoreboard variants go through `build-ui` inside the R8 design system.

## Rules (verify before building)

From the IHF Rules of the Game (Rule 2, playing time and team timeouts; Rule 16, suspensions). Read the current edition (use `read-the-damn-docs`) and STOP if these differ:

- Two halves of 30:00. Half-time after the first. The clock counts up; the second half runs from 30:00 to 60:00, as most scoreboards show it.
- Team timeouts: 3 per team per match, at most 2 in one half. This plan shows 2 marks per half and does not enforce the 3 per match.
- Suspensions are 2:00 of playing time: they stop whenever the game clock stops.
- Extra time, where needed: two periods of 5:00.

## Current state (at `0d7570c`, plus what plan 005 adds)

### After plan 005 (layer 01)

`sport.Profile` gains `OvertimeLength`, `FoulsCarry`, `PeriodWord`, `FoulMarks`, and the methods `Overtime(period)` and `ResetsFouls(period)`; `Basketball.OvertimeLength` is 3000. `Score` is unchanged by plan 005. The server owns the switch of period length: `Server.lastSport` (under `syncMu`), and in `SyncGame`, when the game's sport changes, the new profile has a clock and the scorer's clock is stopped, one `s.Manual.Do(Action{Do: "period_length", Tenths: p.Length})`. At startup `lastSport` is empty, so a restart never resets a custom length. "Repor" on the scorer page computes the period's length from `game.rules` and `score.length`. The overlay and scorer page know `period_word === 'half'` (keys `overlay.periodo.parte_curto`, `overlay.periodo.parte_longo`, `overlay.periodos.parte`, `marcar.parte`) and the scorer page hides its fouls row when `foul_marks` is 0.

### `internal/source/manual.go` at `0d7570c`

- `Score` (lines 15-31) is compared with `==` at line 329 (`if *s == prev`). **There is no `same()` helper**, and `Score` must stay comparable: use only ints, strings, bools and fixed arrays (volleyball's `Sets [maxSets]state.SetScore` is the model).
- Field comments: `Clock int // tenths of a second left in the period`, `Running bool // the clock is counting down`.
- `newScore(length int)` (line 63): `Score{Period: 1, Clock: length, Length: length}`.
- `Seed` (lines 174-193): when the console sent no clock, `s.Clock = s.Length`.
- `Do` (lines 232-340): `clock_start` starts only when `s.Clock > 0`; `period_next` sets `s.Clock` from the length (plan 005 adds the overtime rule); `period_length` (lines 316-323) moves the clock only when `!s.Running && s.Clock == s.Length`; `new_game` is `newScore(s.Length)`.
- `refused(a, sets)` (line 345) rejects clock, period and foul actions for a sets sport.
- `Tick` (line 456) skips a frame when `state.ClockText(m.s.Clock) == state.ClockText(was.Clock)`.
- `current()` (lines 474-482) counts down and stops at 0:
  ```go
  func (m *Manual) current() Score {
  	s := m.s
  	if s.Running {
  		s.Clock = max(m.fromClock-int(m.Now().Sub(m.from)/(100*time.Millisecond)), 0)
  		s.Running = s.Clock > 0
  	}
  	return s
  }
  ```
- `emit()` (lines 503-536) sends a `map[string]string` to the state machine; a sets sport sends its own map. New values travel in this map.

### `internal/state/state.go` at `0d7570c`

- `ApplyConsole(v map[string]string)` (line 722) builds a `Live` from the map.
- `setLive(l Live) bool` (line 820) takes **one argument**. It returns early into `setSets` for a sets sport. "Started" means the clock text went down (`t < m.lastTenths`); the final hold starts when `newP >= r.Periods && t == 0`; `checkFinal` (line 936) fires after `r.FinalHold`.
- `ClockText(t)` (line 996) rounds **up** and shows tenths under a minute: right for a countdown, wrong for a clock counting up.
- Timeline: `type seen struct` (line 362, fields `screen, lowerThird, box, periods, sets, demo`), `seenNow()` (line 371), and `emit()` (line 396), which diffs `seen`, mutes the demo, and fires `"period"` and `"set"` events in loops (lines 412-419). `RestorePeriods` (line 687) sets `m.seen.periods` so a restart doesn't replay periods (line 690).

### `cmd/hallstream/main.go` at `0d7570c`

Line 194 builds the scorer with `Rules: sport.Get(st.GameSport())`; line 195 calls `restoreManual(st, manual)`; `restoreManual(st *store.Store, mn *source.Manual)` (lines 386-393) loads `dados/manual.json` and calls `mn.Restore(sc)`. `cmd/hallstream/sets_test.go:99` and `:109` call `restoreManual(st, …)` with two arguments, so its signature stays. The machine `m` exists before line 195.

### Pages at `0d7570c`

- `web/marcar/index.html` lines 18-20 (home) and 66-68 (away): `+1`, `+2`, `+3` buttons, each `data-long` (a long press takes one away; handler at `marcar.js:301`).
- `web/marcar/marcar.js`: `clockText` (lines 130-134) mirrors `ClockText`; the "next period" button logic (lines 234-255) waits for `score.clock === 0`; "Repor" at lines 349 and 353.
- `web/overlay/index.html` lines 71-72: the scoreboard's fouls cells (`sb-f`, `data-bonus`, `data-pips`); lines 150, 155: the time-out screen's fouls. The scoreboard already swaps cells by `html[data-scoring]` for volleyball (`overlay.css`, the `only-pts` / `only-sets` rule).
- `internal/club/club.go`: `{"handball", "ANDEBOL", "ANDEBOL", false}`; `internal/club/club_test.go:143` lists handball as inactive.
- `web/painel/painel.js:2162`: handball has no `true` flag. `web/painel/mock.js:356`: the mock's escalão sports list.

### Guards that must stay green

- **Basketball and futsal stay pixel-identical.** `TestBasketballScreensDrawTheSameEveryTime` must pass. Also compare screenshots against the layer below: build `01-feat/futsal` and this branch to two scratch exes; run each with `-demo -nobrowser` and shoot `/overlay?state=/overlay/fixtures/<f>.json&preview=1&anim=0&style=<s>` for `f` in `marcador`, `intervalo`, `final`, `futsal/marcador`, `futsal/final-prolongamento` and `s` in `classico`, `lombada`, `cor`, `placa`, `pavilhao`, with headless Chrome (`--headless=new --window-size=1920,1080 --virtual-time-budget=5000 --screenshot=<file>`). Every pair must be byte-identical (`cmp`).
- **Tabular figures.** `overlay.css:91` gives tabular figures to binds ending in `Score`, `Big`, `Sets`, `Fouls`, and to `clock`, `shot`, `startTime`. The suspension chips' time is generated, not bound: give the chip's time element `font-variant-numeric: tabular-nums` in its own rule.

### Conventions

- Go, standard library only; plain `testing`; model on `internal/source/manual_test.go` and `internal/state/state_test.go`.
- Every user-facing string in `web/i18n/pt.json` then `en.json`. The program speaks in the first person.
- Every overlay screen or state has a fixture the Chrome test loads; the overlay stays on air for hours, so a state it doesn't expect must still draw.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Commit style: `feat: handball scoring with suspensions`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -count=1 ./...` | all pass |
| Scorer and state | `go test ./internal/sport/ ./internal/source/ ./internal/state/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |

## Suggested executor toolkit

- `read-the-damn-docs` for the IHF rules.
- `build-ui`, extension branch, inside `docs/design/SYSTEM-R8.md`, for the suspension chips and the scorer page's suspension row. Stop after its quality gate.

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go`
- `internal/state/state.go`, `internal/state/state_test.go`
- `internal/club/club.go`, `internal/club/club_test.go` (handball active)
- `cmd/hallstream/main.go` (one `m.RestoreSeen(…)` call right after `restoreManual(st, manual)` at line 195; `restoreManual` itself is unchanged)
- `internal/web/game.go` (the count-up part of plan 005's `SyncGame` switch) and `internal/web/sport_switch_test.go` (new cases)
- `web/marcar/index.html`, `web/marcar/marcar.js`, `web/marcar/marcar.css`
- `web/overlay/index.html`, `web/overlay/overlay.js`, `web/overlay/overlay.css`, `web/overlay/fixtures/handball/*.json` (create)
- `internal/web/overlay_test.go` (add handball fixtures to the `TestOverlayStyles` list only)
- `web/painel/painel.js`, `web/painel/mock.js`
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- `internal/ble/decode.go`: console handball stays "untested".
- Suspensions from a console: the console sends none; chips show only from the phone scorer.
- Player stats, handball warnings (yellow cards), disqualifications.
- Basketball, futsal and volleyball behaviour.

## Git workflow

- Branch: `02-feat/handball`, based on `01-feat/futsal`.
- One commit per step, conventional commits. Do not push or open a PR unless told to.

## Steps

### Step 1: The handball profile and count-up methods

In `internal/sport/sport.go` add:

```go
	StopAtEnd   bool `json:"stop_at_end"` // a count-up clock stops at the end of the period; false runs on (soccer)
	Suspensions int  `json:"suspensions"` // tenths a suspension lasts; 0 = the sport has none
```

and:

```go
// Start is the clock at the start of period, given the scorer's regulation
// length. Countdown: length, or OvertimeLength in an overtime when set.
// Count-up: the time played before period (handball's period 2 starts at 30:00).
func (p Profile) Start(period, length int) int

// End is where period ends on a count-up clock (its Start plus its length);
// 0 for a countdown.
func (p Profile) End(period, length int) int
```

An overtime's length is `OvertimeLength`, or `length` when that is 0.

Also add, and use it everywhere this plan and plan 007 would otherwise write `!ClockDown && Length > 0`:

```go
// CountUp reports whether the clock counts up (handball, soccer); false for a
// countdown and for a sport with no clock (volleyball).
func (p Profile) CountUp() bool { return !p.ClockDown && p.Length > 0 }
```

The pages get it as `game.rules.count_up`: add `CountsUp bool \`json:"count_up"\`` to `Profile`, set `true` in the `Handball` literal (and plan 007's `Soccer`), and a test that `p.CountsUp == p.CountUp()` for every profile in `profiles`, so the field and the method can't disagree.

```go
var Handball = Profile{ID: "handball", Periods: 2, Half: 1, Length: 18000, ClockDown: false, CountsUp: true,
	StopAtEnd: true, FinalHold: 600, TimeoutsBy: [3]int{2, 2, 0}, Scoring: "points",
	OvertimeLength: 3000, PeriodWord: "half", Suspensions: 1200}
```

No team fouls (`FoulWarn`, `FoulBonus`, `FoulMarks` 0, `FoulsReset` false). Register it in `profiles`.

**Verify**: `go test ./internal/sport/` → pass, with tests: `CountsUp == CountUp()` for every registered profile; (length 18000): `Start(1)=0`, `Start(2)=18000`, `Start(3)=36000`, `Start(4)=39000`; `End(1)=18000`, `End(2)=36000`, `End(3)=39000`; length 15000: `Start(2)=15000`, `End(2)=30000`; basketball (length 6000): `Start(1..4)=6000`, `Start(5)=3000`, `End(n)=0`.

### Step 2: The scorer's clock counts up

In `internal/source/manual.go`, with `r := m.Rules()`:

- `Score` gets no period fields: every start and end derives from `s.Length` and the profile, in Go (`r.Start(s.Period, s.Length)`, `r.End(…)`) and on the page (the same arithmetic from `game.rules` and `score.length`).
- `current()`: for a count-up (`r.CountUp()`), `s.Clock = fromClock + elapsed`; when `r.StopAtEnd`, cap at `r.End(s.Period, s.Length)` and stop there (`Running = false`); otherwise cap at `MaxClock`. Countdown unchanged.
- `clock_start`: a count-up starts whenever it is below its end (always, without `StopAtEnd`). Countdown unchanged.
- `period_next`: `s.Clock = r.Start(s.Period, s.Length)` after the increment (for basketball this is plan 005's rule: `s.Length`, or 3000 in overtime).
- `new_game`: keep `newScore(length)` exactly as it is (every existing caller and test uses it) and add `newScoreAt(length, start int) Score` (`Score{Period: 1, Clock: start, Length: length}`). `new_game` calls `newScoreAt(s.Length, r.Start(1, s.Length))`, which is `newScore(s.Length)` for a countdown and 0 on the clock for handball.
- **Switching to or from a count-up sport** happens in plan 005's `SyncGame` switch (`internal/web/game.go`), right after its `period_length` call and under the same conditions (sport changed, the new profile has `Length > 0`, the scorer's clock stopped). When the old or the new profile counts up: if the score is 0–0 in period 1, `s.Manual.Do(Action{Do: "new_game"})`, which clears any suspensions and puts the clock at `r.Start(1, s.Length)`; otherwise `s.Manual.Do(Action{Do: "clock_set", Tenths: p.Start(period, p.Length)})`, and running suspensions stay as they are (their ends sit on the game clock; a mid-game sport change is a correction, not a new game). The scorer's `Rules` already return the new sport at that moment, because the store changed before `SyncGame` runs. Add `internal/web/game.go` to the files you touch for this.
- `clock_set` / `clock_add` on a count-up: clamp to `[r.Start(s.Period, s.Length), r.End(s.Period, s.Length)]` (to `MaxClock` above without `StopAtEnd`), and keep a running clock running; the `s.Running = s.Running && s.Clock > 0` line (lines 292-303) applies to a countdown only.
- `period_back` on a count-up: the clock goes to the earlier period's `r.End(p, s.Length)`, stopped.
- `period_length` on a count-up: moves the clock only when it is stopped at the old `r.Start(s.Period, oldLength)`, to `r.Start(s.Period, newLength)`. The countdown rule stays.
- `Seed`: with no clock from the console, use `r.Start(p, s.Length)`.
- `Tick`: compare the text `emit()` sends. Add `func (m *Manual) clockText(t int) string` (`state.ClockTextUp` for a count-up, `state.ClockText` otherwise) and use it in both.
- `emit()`: send `"clock"` via `m.clockText` and a new `"period_end"`, `strconv.Itoa(r.End(s.Period, s.Length))`, or `""` for a countdown.
- Field comments: `Clock` is "tenths on the clock: left in the period for a countdown, played in the game for a count-up"; `Running` is "the clock is running".
- `web/marcar/marcar.js` "Repor" (lines 349, 353): the current period's start, computed like `Profile.Start` from `game.rules` (`count_up`, `periods`, `overtime_length`) and `score.length`: for a count-up, the time played before the period (handball's second half 30:00); for a countdown, plan 005's rule (`overtime_length` in an overtime when set, else `score.length`). Put the arithmetic in one helper next to `rulesOf()` and reuse it for the next-period button (Step 3).

**Verify**: `go test ./internal/source/` → pass, with new tests (fake `Now`, `Rules` returning `sport.Handball`): a new game shows 0; `clock_add` +10 s on a running clock at 0:05 keeps it running at 0:15, and `clock_set` past the period's end clamps to it; `period_back` from period 2 puts the clock at 18000, stopped; 65 s after `clock_start` the clock is 650; it stops at 18000 in period 1; `period_next` starts period 2 at 18000; with `period_length` 15000 the second half starts at 15000 and survives `new_game`; `period_length` at 0:00 in period 1 leaves the clock at 0, and at a stopped 30:00 in period 2 moves it to the new start; a running clock never moves; basketball and futsal tests unchanged. In `internal/web/sport_switch_test.go`: switching a stopped 0–0 period-1 basketball game at 10:00 to handball leaves the scorer at 0:00 with `Length` 18000 and no suspensions; switching a stopped handball game at 41:17 in period 2 to basketball sets the clock to 6000 (period 2's countdown start) and keeps its suspensions; a running clock is never touched.

### Step 3: Clock text and the state machine for a count-up

- `internal/state/state.go`: add `ClockTextUp(t int) string`: floors to the second, always `MM:SS` (`"00:12"`, `"30:00"`, `"59:59"`). `Tenths` already parses `MM:SS`.
- Add `PeriodEnd int \`json:"period_end"\`` to `Live`, read in `ApplyConsole` from `v["period_end"]` (0 when absent).
- In `setLive`, with `r := m.rules()`:
  - "started": for a count-up, `m.lastTenths >= 0 && t > m.lastTenths`. `lastTenths` resets to -1 on a period change (`state.go:841`), so the first frame of period 2 (the clock at 30:00) does not count as started; the next frame with a higher clock does. Test: period 1 → 2 shows `intervalo`, the first period-2 frame at 30:00 leaves it on `intervalo`, a frame at 30:01 returns to `marcador`.
  - the period's end: `l.PeriodEnd`, or `r.End(newP, r.Length)` when 0 (a console frame).
  - final hold: a countdown waits at 0 as today; a count-up with `StopAtEnd` waits at (or past) that end in period `>= r.Periods`; a count-up without `StopAtEnd` (soccer, plan 007) never starts the hold.
- `web/marcar/marcar.js`: `clockText` matches `ClockTextUp` when `rules.count_up`. The next-period button (lines 234-255) appears at the period's end for a count-up (`score.clock >=` the end computed by the Step 2 helper from `game.rules` and `score.length`), at 0 for a countdown. `live.period_end` stays for the state machine, which doesn't know `s.Length`.

**Verify**: `go test ./internal/state/` → pass, with new tests: `ClockTextUp(125) == "00:12"`, `ClockTextUp(18000) == "30:00"`; a handball game going period 1 → 2 records period 1 and shows `intervalo`; the clock rising in period 2 returns to `marcador`; sitting at 36000 for `FinalHold` in period 2 shows `final`; basketball, futsal and volleyball state tests unchanged.

### Step 4: Suspensions in the scorer

Add to `Score` (fixed arrays keep it comparable; three per team is built in):

```go
	HomeSusp [3]Suspension `json:"home_susp"`
	AwaySusp [3]Suspension `json:"away_susp"`
	NextSusp int           `json:"next_susp"` // the last ID given out

// Suspension is one running suspension; an empty slot has ID 0.
type Suspension struct {
	ID     int    `json:"id"`
	Number string `json:"number"` // shirt number, "" until the scorer adds it
	End    int    `json:"end"`    // the game clock (count-up, tenths) at which it ends
}
```

Add `ID int \`json:"id"\`` and `Number string \`json:"number"\`` to `Action`, and these actions (document them in the `Action` comment):

- `"suspension"` with `Team`: fills the first empty slot with `{ID: NextSusp+1, End: s.Clock + r.Suspensions}`. All three full: `i18n.Key("manual.suspensoes_cheias")` (pt "Esta equipa já tem 3 exclusões a correr.", en "This team already has 3 suspensions running."). `r.Suspensions == 0`: `manual.sem_exclusoes` (pt "Esta modalidade não tem exclusões.", en "This sport has no suspensions.").
- `"suspension_number"` with `ID` and `Number` (1 to 3 digits, or "" to clear). Unknown ID: `manual.exclusao_desconhecida` (pt "Essa exclusão já acabou.", en "That suspension has already ended.").
- `"suspension_end"` with `ID`: clears the slot early (a mistaken tap needs a way out besides undo).

Undo takes these back like any action. `refused` needs no change (these are not clock actions), but a sets sport must answer `manual.sem_exclusoes` through the `r.Suspensions == 0` check.

Timing: time left is `End - Clock`. Because `End` is a point on the game clock, a suspension only loses time while the clock runs, a tap while it runs needs no bookkeeping, and an undo restores the right time left. In `current()`, empty every slot whose `End <= s.Clock`. The count-up clock carries across the half, so a suspension taken at 29:00 ends at 31:00.

`emit()`: send `"suspensions"` as a JSON string, `[{"team":"home","id":1,"number":"7","left":"01:23"}, …]`, `left` = `state.ClockTextUp(End - Clock)`, only slots with `End > Clock` (an expired slot is never sent, even between `current()` calls), home first; `"[]"` when none.

**Verify**: `go test ./internal/source/` → pass, with new tests: a tap at clock 5000 sets `End` 6200; a fourth tap on one team errors and changes nothing; after 30 s of running clock the time left is 900, and it stays 900 while the clock is stopped; at `End` the slot empties; a tap while running, 10 s, then undo of an unrelated point leaves 1100 left; `suspension_number` sets "7"; undo of `suspension` removes it; a suspension-only change is not dropped by the `*s == prev` check.

### Step 5: Suspensions in the state

- `state.Live`: add `Suspensions []Suspension \`json:"suspensions"\`` with `type Suspension struct { Team string \`json:"team"\`; ID int \`json:"id"\`; Number string \`json:"number"\`; Left string \`json:"left"\` }`. (`Live` may hold a slice: it is not compared with `==`.)
- `ApplyConsole`: `json.Unmarshal` `v["suspensions"]`; on an error or empty value leave it empty. Never fail the frame.
- Timeline, through `emit()`'s `seen` diff (never from `setLive`): add `susp int` to `seen` (the highest ID in `Live.Suspensions`, filled in `seenNow`); in `emit()`, after the period and set loops, fire `m.OnEvent("suspension", map[string]any{"team": …, "number": …})` for each suspension whose ID is above `old.susp`. The demo is already muted there.
- **Restart guard.** `seenNow()` rebuilds `seen` from `m.s.Live` on every `changed()`, and `srv.SyncGame()` (`main.go:238`) calls `SetGame` → `changed()` before the scorer's first frame arrives, so setting `m.seen.susp` alone is undone at once. Keep a floor instead: add `restored struct{ susp int; pending bool }` to `Machine` and `func (m *Machine) RestoreSeen(susp int)`, which sets `m.restored = {susp, true}` under the lock. While `pending`, `seenNow()` uses `max(<highest live ID>, m.restored.susp)`. The first frame applied through `ApplyConsole` clears `pending` after its own `changed()` has run (so that frame's diff still respects the floor), and `ResetPeriods` clears it too (a new game starts IDs at 1). Leave `restoreManual` as it is (`cmd/hallstream/sets_test.go:99,109` call it with two arguments). Right after `restoreManual(st, manual)` at `main.go:195`, call `m.RestoreSeen(<highest ID in manual.Score().HomeSusp and AwaySusp>)`; put that computation in a small helper in `main.go` so it is testable. Plan 007 extends the floor to cards and substitutions.
- `docs/CONTRACT.md`: `live.suspensions`, the three actions, the `"suspension"` event, and, under the scorer's frame keys for every sport, `period_end` (empty for a countdown) and `suspensions` (`"[]"` when none).

**Verify**: `go test ./internal/state/ ./cmd/hallstream/` → pass, with tests: a frame with one suspension fills `Live.Suspensions`; bad JSON leaves it empty and the score still updates; a new ID fires one `suspension` event; nothing fires while the demo runs; after `RestoreSeen(2)`, then a `SetGame` call (as `SyncGame` makes at start), then a frame holding IDs 1 and 2, nothing fires; a later frame with ID 3 fires one event; after `ResetPeriods` a frame with ID 1 fires again.

### Step 6: Scorer page and overlay

Go through `build-ui` (extension branch, R8):

- `web/marcar`, driven by the profile: fouls row hidden by `foul_marks` 0 (plan 005); `+2` and `+3` hidden when `rules.count_up` (a count-up sport scores one goal at a time), decided from the profile and never from the sport's id; `+1` keeps `data-long`. A suspensions row per team when `rules.suspensions > 0`: a "+2'" button (`data-do="suspension"`), and each running suspension as a chip with its time left; tapping a chip opens a small number field (`suspension_number`) and an "Acabou" button (`suspension_end`). Timeouts: 2 marks per half from `rules.timeouts`.
- `web/overlay`: `html[data-scoring]` stays as it is. Add `html[data-penalties]`, set to `"suspensions"` when `rules.suspensions > 0` and removed otherwise; under it, the scoreboard's fouls cells show a header `overlay.exclusoes` (pt "EXCLUSÕES", en "SUSPENSIONS") and up to 3 chips per team from `live.suspensions`, text in `MM:SS` (`02:00 · 7`, the number only when set). A missing or `null` `live.suspensions` reads as empty. The cells swap with a class pair like volleyball's `only-pts` / `only-sets`, so basketball's markup and pixels don't move. The time-out screen's fouls hide under `data-penalties`.
- New keys (pt, en): `marcar.exclusao` ("+2'"), `marcar.exclusao_numero` ("Número" / "Number"), `marcar.exclusao_acabou` ("Acabou" / "Ended"), `marcar.exclusoes` ("Exclusões" / "Suspensions"), `overlay.exclusao` (aria: "Exclusão {numero} {tempo}" / "Suspension {numero} {tempo}"), `overlay.exclusoes` ("EXCLUSÕES" / "SUSPENSIONS"). The scorer page also treats a missing suspensions list as empty.

**Verify**: `go test -count=1 ./internal/web/` → pass; the screenshot comparison in "Guards" → all identical.

### Step 7: Fixtures and turning it on

- `web/overlay/fixtures/handball/` (shape of `marcador.json`, `game.sport: "handball"`, `game.rules` = Handball as JSON): `marcador.json` (2.ª parte, 41:17, 18–17), `marcador-exclusoes.json` (home 2, one with number "7"; away 3), `intervalo.json`, `final.json` (60:00, two periods). Add `handball/marcador-exclusoes` and `handball/final` to `TestOverlayStyles`.
- `internal/club/club.go`: handball `Active: true`; `club_test.go` moves handball out of the inactive map.
- `web/painel/painel.js:2162`: enable handball. `web/painel/mock.js:356`: accept `'handball'`.
- `docs/ROADMAP.md`: tick handball ("Done: scored from the phone, with suspensions; console untested"). `docs/CONTRACT.md`: handball's rules, `ClockTextUp`, `live.period_end`, "handball on a console is untested".

**Verify**: `go vet ./... && go test -count=1 ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/sport`: `Start`/`End` for handball and basketball.
- `internal/source`: count-up clock, period starts, suspensions, timing, numbers, undo.
- `internal/state`: `ClockTextUp`, count-up screens and hold, suspension decoding, the event, `RestoreSeen`.
- Overlay: four handball fixtures; two in `TestOverlayStyles`.
- Basketball and futsal: `TestBasketballScreensDrawTheSameEveryTime` and the screenshot comparison.

## Done criteria

- [ ] `go vet ./...`, `go test -count=1 ./...`, `GOOS=windows GOARCH=386 go build ./...` exit 0
- [ ] `ls web/overlay/fixtures/handball/*.json` lists four files
- [ ] `grep -n '"handball", "ANDEBOL", "ANDEBOL", true' internal/club/club.go` matches
- [ ] `grep -n "suspension" docs/CONTRACT.md` finds the actions and `live.suspensions`
- [ ] The screenshot comparison against `01-feat/futsal` is byte-identical
- [ ] `git diff --stat 01-feat/futsal -- internal/ble` → no change
- [ ] Existing test edits limited to `club_test.go` (handball active)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 005's fields or methods are missing or shaped differently than "After plan 005".
- The IHF rules differ from "Rules".
- A basketball, futsal or volleyball test fails, or a guard screenshot differs.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Plan 007 reuses the count-up clock with `StopAtEnd: false`; it must not change handball.
- Suspensions follow the scorer's clock. If a recorded console session shows the box sends suspensions, map them in the decoder.
- Reviewers: three suspensions across the half; chips stop with the clock and carry into the second half; undo restores a cleared chip.

## Plan review

Two reviews against `4093d0c` (revise, then fixes in). Reconciled at `0d7570c`:

- Plan 004 shipped no `same()`; `Score` is still compared with `==`. Suspensions are fixed arrays of comparable structs, so `==` keeps working; every "add to `same()`" instruction is gone.
- `setLive` takes one argument (the draft assumed two).
- `restoreManual` keeps its signature (`sets_test.go` calls it); `main.go` calls `RestoreSeen` right after it.
- The `+2`/`+3` buttons hide by profile, not by sport id.
- After 005's redesign, `Score` holds no `Sport`, `PeriodLength` or `PeriodStart`; the switch to or from a count-up sport is part of `SyncGame`'s switch, and the page computes starts and ends from `game.rules` and `score.length`.
- The overlay swaps the fouls cells for suspension chips the way volleyball swaps them for sets, so basketball's markup doesn't change.
- `club_test.go`, `mock.js` and the `TestOverlayStyles` list were missing from scope.
- The goldfish review of this reconciled version is recorded in the stack's PR.
