# Plan 002: Basketball's rules live in one `internal/sport` profile, with no change in behaviour

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/source/manual.go internal/state/state.go internal/club/club.go internal/web/game.go web/overlay/overlay.js web/marcar/marcar.js docs/CONTRACT.md`
> If any of these changed, compare the "Current state" excerpts with the live
> code; a mismatch you can't map one-to-one is a STOP condition.

## Status

- **Priority**: P1 (first plan of ROADMAP Phase C; plans 003 to 007 build on it)
- **Effort**: M
- **Risk**: MED (touches the scoring path that runs on air)
- **Depends on**: none
- **Category**: tech-debt (enabler for direction work)
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

Hallstream scores only basketball, and basketball's rules are written straight into four places: the phone scorer (`internal/source/manual.go`), the state machine that records periods and switches screens (`internal/state/state.go`), the overlay (`web/overlay/overlay.js`) and the scorer page (`web/marcar/marcar.js`). ROADMAP Phase C step 1 says: "Add `internal/sport`. Basketball moves into it unchanged, and the existing tests prove it." After this plan, every basketball number (four 10-minute periods, the half after period 2, overtime from period 5, FIBA timeouts, team fouls reset each period, the 5th-foul bonus, the 4-foul warning, the shot clock) comes from one `sport.Profile`, the Go side reads it, and the pages read the same values from `game.rules` in the state. Later plans add volleyball, futsal, handball and soccer as new profiles instead of new `if` branches.

**This plan changes no behaviour.** Every existing test must pass without being edited.

## Current state

### Where basketball's rules live today

- `internal/source/manual.go`:
  - line 28-29: `DefaultLength = 10 * 60 * 10` (a basketball period, 10:00, in tenths).
  - line 16: `Period int // 1 and up; 5 is the first overtime`.
  - lines 210-216, `period_next`:
    ```go
    case "period_next":
    	s.Period++
    	s.Clock, s.Running, s.HomeFouls, s.AwayFouls, clock = s.Length, false, 0, 0, true
    	// FIBA timeouts: 2 in the first half, 3 in the second, 1 in each overtime.
    	if s.Period == 3 || s.Period >= 5 {
    		s.HomeTimeouts, s.AwayTimeouts = 0, 0
    	}
    ```
  - line 100: `NewManual` sets `m.s = newScore(DefaultLength)`.
- `internal/state/state.go`:
  - lines 243-245: `const FinalHold = 60 * time.Second` ("the clock must sit at 0 in period 4+ before the game counts as over").
  - `setLive`, lines 727-764: on a period change from `oldP` to `oldP+1` it records the period, and `if oldP == 2 { m.auto("intervalo") }`; when the clock starts in `newP >= 3` while the screen is `intervalo` or `final` it goes back to `marcador`; `if newP >= 4 && t == 0` starts the final hold.
  - `State.Game` (line 104-120) carries `Sport` and `SportLabel`, filled in `internal/web/game.go:184-188` from `club.SportOf(cl.Sport)`. Note `game.go:184` names a local variable `sport` (`sport, _ := club.SportOf(cl.Sport)`), which will shadow the new package: rename it to `sp`.
  - `State.Live.Sport` (line 46) also exists, but it is the console's own sport code (`internal/ble/decode.go`) and the manual scorer never sets it. The rules follow `Game.Sport`, the club's choice.
- `web/overlay/overlay.js`:
  - lines 487-488: `pn <= 4` → quarter label, `pn === 5` → "Prol.", above → "Prol. n-4".
  - lines 588-589: the period table header, the same `<= 4` / `=== 5` rule.
  - lines 558-565, `renderPips`: `Math.min(n, 4)` lights at most 4 pips (the markup in `web/overlay/index.html:70,71,148,153` has four `<i>` per team; that 4 is the pip count and stays) and the `hot` class at `n >= 4` (the warning: this one moves to the profile).
  - line 525: `const lastDone = periods.length ? periods[periods.length - 1].period : 2;` (the half, as a fallback for the intervalo line).
  - lines 1098-1105: the shot clock pulses under 5 s. Basketball-only and harmless when `live.shot` is empty; it stays.
  - lines 1160-1165: `bonus` class at `n >= 5` ("FIBA: from the 5th team foul of the period the other side shoots free throws").
  - line 484: `v.shot = live.shot ?? ''` (the shot clock).
- `web/marcar/marcar.js`:
  - lines 135-136, `periodName`: `p <= 4` → "Período n", `p === 5` → "Prolongamento", above → "Prolongamento n".
  - line 174: `const maxTO = period <= 2 ? 2 : period <= 4 ? 3 : 1;` (timeouts allowed).
  - line 177: `marks($(\`[data-marks="${tx}Fouls"]\`), f, 5);` and line 181: `….toggleAttribute('data-bonus', f >= 5);` (the bonus).
  - `render()` starts at line 155 with `const g = st.game || {}, l = st.live || {}, …`.
- `internal/state/demo.go:105-152`: the demo plays a basketball game with its own literals (600 s periods, a 300 s overtime, fouls reset in periods ≤ 4, half at 2, a 24 s shot clock). **The demo stays basketball and keeps its literals**; it is out of scope.
- `internal/club/club.go:25-45`: `Sport{ID, Name, Short, Active}` and the list `Sports` (basketball active; futsal, handball, volleyball, soccer listed, inactive), `SportOf(id)`.

### Conventions

- Go, standard library only. Code and comments in English. Tests use plain `testing`; model new ones on `internal/source/manual_test.go`.
- Every user-facing string lives in `web/i18n/pt.json` then `en.json`; this plan adds none.
- `web/` ships as is: no build step, nothing from a CDN.
- `docs/CONTRACT.md` changes in the same commit as the behaviour it describes.
- Commit style: `refactor: basketball's rules in internal/sport`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel `http://127.0.0.1:8765/painel`, overlay `/overlay` |

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go` (create)
- `internal/source/manual.go`
- `internal/state/state.go`
- `internal/web/game.go` (filling `game.rules`)
- `cmd/hallstream/main.go` (only if `NewManual` needs the rules function passed in)
- `web/overlay/overlay.js`, `web/marcar/marcar.js`
- `docs/CONTRACT.md`

Why the pages change in a "move" plan: `plans/phase-c-decisions.md` says the overlay reads the profile, and later sports need the pages to stop hard-coding basketball. The values sent equal today's literals, so nothing on screen changes.

**Out of scope**:

- `internal/club/club.go`: `Sports` and `SportOf` stay where they are (names and on-air words). The profile is keyed by the same id.
- `internal/state/demo.go`: the demo is a basketball game by design and keeps its own numbers.
- `web/overlay/index.html`: the four foul pips per team are markup, not a rule.
- The shot-clock pulse (`overlay.js:1098-1105`): basketball-only, hidden when there is no shot clock.
- `internal/ble/decode.go` and the console path.
- Any existing test file. If a test needs editing to pass, behaviour changed: STOP.
- Any new sport. Volleyball is plan 004.

## Git workflow

- Branch: `refactor/sport-profiles`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The `sport` package with the basketball profile

Create `internal/sport/sport.go`:

```go
// Package sport holds each sport's rules: how the clock runs, how many
// periods, when the half falls, timeouts and fouls. The scorer, the state
// machine and the pages read them from here.
package sport

// Profile is one sport's rules. Times are in tenths of a second. It goes to
// the pages as game.rules, so the JSON names are part of the contract.
type Profile struct {
	ID         string `json:"id"`          // as in club.Sports
	Periods    int    `json:"periods"`     // regulation periods
	Half       int    `json:"half"`        // the period after which the half-time break falls
	Length     int    `json:"length"`      // tenths in a regulation period
	ClockDown  bool   `json:"clock_down"`  // the clock counts down (plans 006 and 007 add count-up sports)
	FoulsReset bool   `json:"fouls_reset"` // team fouls go back to 0 each period
	FoulWarn   int    `json:"foul_warn"`   // fouls at which the pips turn hot; 0 = never
	FoulBonus  int    `json:"foul_bonus"`  // fouls from which the other side shoots; 0 = never
	ShotClock  bool   `json:"shot_clock"`  // the sport has a shot clock
	FinalHold  int    `json:"final_hold"`  // tenths the clock sits at 0 in the last period before the game is over
	TimeoutsBy [3]int `json:"timeouts"`    // timeouts per team: first half, second half, each overtime
}

// Basketball is FIBA.
var Basketball = Profile{ID: "basketball", Periods: 4, Half: 2, Length: 6000, ClockDown: true,
	FoulsReset: true, FoulWarn: 4, FoulBonus: 5, ShotClock: true, FinalHold: 600,
	TimeoutsBy: [3]int{2, 3, 1}}

// Get returns the profile for id; an unknown or empty id is basketball.
func Get(id string) Profile

// Timeouts is how many timeouts each team has in period p:
// TimeoutsBy[0] for p <= Half, [1] for Half < p <= Periods, [2] after.
func (p Profile) Timeouts(period int) int

// ResetsTimeouts reports whether starting period p puts timeouts back to 0:
// p == Half+1, and every p > Periods.
func (p Profile) ResetsTimeouts(period int) bool
```

The overtime length (5:00) is not a field: the manual scorer uses the same `Length` for overtime today (`period_next` sets `Clock` to `s.Length`), and this plan keeps that. The pages get the whole `Profile` as `game.rules`; there is no separate `Rules` type.

**Verify**: `go test ./internal/sport/` → pass, with tests: `Get("")` and `Get("nonsense")` are basketball; `Timeouts` for periods 1..7 is `2,2,3,3,1,1,1`; `ResetsTimeouts` is true for 3, 5, 6 and false for 1, 2, 4.

### Step 2: The phone scorer reads the profile

In `internal/source/manual.go`:

- Add a field to `Manual`: `Rules func() sport.Profile // nil = basketball`. In `NewManual`, set it only `if m.Rules == nil`, to `func() sport.Profile { return sport.Basketball }`. `cmd/hallstream/main.go:191` keeps calling `NewManual` without it; plan 003 wires the escalão's sport in.
- `DefaultLength` stays as a constant (other code and tests use it); a new test asserts `DefaultLength == sport.Basketball.Length`.
- `period_next`: replace the literal rules with the profile:
  ```go
  r := m.Rules()
  s.Period++
  s.Clock, s.Running, clock = s.Length, false, true
  if r.FoulsReset {
  	s.HomeFouls, s.AwayFouls = 0, 0
  }
  if r.ResetsTimeouts(s.Period) {
  	s.HomeTimeouts, s.AwayTimeouts = 0, 0
  }
  ```
- Update the doc comment on `Action` (lines 46-48) to say "the timeouts go back to 0 when the sport's profile says so (basketball: the second half and each overtime)".

`Rules` is called under `m.mu`; it must not call back into `Manual`.

**Verify**: `go test ./internal/source/` → pass with no test file changed.

### Step 3: The state machine reads the profile

In `internal/state/state.go`, add `func (m *Machine) rules() sport.Profile { return sport.Get(m.s.Game.Sport) }` (call with the lock held, as `setLive` and `checkFinal` are) and use `r := m.rules()` in both. `checkFinal` also runs from `Tick` (line 814); the helper covers it.

- `if oldP == 2 { m.auto("intervalo") }` → `if oldP == r.Half`.
- `if newP >= 3 && (cur == "intervalo" || cur == "final")` → `if newP > r.Half && …`.
- `if newP >= 4 && t == 0` → `if newP >= r.Periods && t == 0`.
- `checkFinal`'s `FinalHold` → `time.Duration(r.FinalHold) * 100 * time.Millisecond`. Keep the exported `FinalHold` constant (tests use it) and add a test that it equals `sport.Basketball.FinalHold` in time.

Update the comment at line 270 (`// clock at 0 in period >= 4 since then`) to "in the last regulation period or later". `m.s.Game.Sport` is `""` in tests that never set a game; `sport.Get("")` is basketball, so they keep passing.

**Verify**: `go test ./internal/state/` → pass with no test file changed.

### Step 4: The pages get the rules in `game.rules`

- `internal/state/state.go`: add to `Game` (after `SportLabel`): `Rules sport.Profile \`json:"rules"\``.
- `internal/web/game.go:184-188`: rename the local `sport` to `sp`, then set `Sport: sp.ID, SportLabel: sp.Short, Rules: sport.Get(sp.ID)`.
- `web/overlay/overlay.js`: at the top of `view(st)`, read `const rules = game.rules || {}` and define the fallbacks once with `??` (never `||`, so a real 0 means "never"): `v.periods = rules.periods ?? 4`, `v.half = rules.half ?? 2`, `v.foulWarn = rules.foul_warn ?? 4`, `v.foulBonus = rules.foul_bonus ?? 5`. Replace the literals:
  - lines 487-488 and 588-589: `<= 4` → `<= periods`, `=== 5` → `=== periods + 1`, `pn - 4` → `pn - periods`.
  - line 525: the fallback `2` → `v.half`.
  - `renderPips` (558-565): add a third parameter `warn`; keep `Math.min(n, 4)` (the pip count in the markup); `n >= 4` (both places) → `warn > 0 && n >= warn`. Call it as `renderPips('home', hf, v.foulWarn)` at lines 1159-1160.
  - lines 1163-1164: `n >= 5` → `v.foulBonus > 0 && n >= v.foulBonus`.
  - Leave the shot clock as is (an empty `live.shot` already hides it).
- `web/marcar/marcar.js`: in `render()` (line 155), next to `g`, read `const rules = g.rules || {}` and the same fallbacks with `??`: `periods = rules.periods ?? 4`, `half = rules.half ?? 2`, `to = rules.timeouts ?? [2, 3, 1]`, `bonus = rules.foul_bonus ?? 5`. `periodName` (135-136) takes `periods` as a second argument (update its callers at 187 and 273). `maxTO` (174) → `period <= half ? to[0] : period <= periods ? to[1] : to[2]`. Line 177's `5` → `bonus`; line 181's `f >= 5` → `bonus > 0 && f >= bonus`.
- `docs/CONTRACT.md`: document `game.rules` (every field, basketball's values) where `game.sport` and `game.sport_label` are described. Where the contract states basketball rules as if generic (lines 91, 92, 111, 126, 141, 159: periods, half, overtime, fouls, final hold), add "(basketball; other sports follow `game.rules`)" or point at `game.rules`. Change no rule.

**Verify**: `go test ./...` → pass (with Chrome installed, the overlay and panel tests load every fixture; the fixtures have no `rules`, so the fallbacks are exercised). Then `go run ./cmd/hallstream -demo -nobrowser`, open `http://127.0.0.1:8765/overlay?state=/overlay/fixtures/marcador-bonus.json&preview=1` and `…/final-prolongamento.json`: bonus and "Prol." show as before.

### Step 5: Prove nothing changed

- `git diff --stat 4093d0c..HEAD -- '*_test.go'` shows only `internal/sport/sport_test.go` and any new test you added; no existing test file is modified.
- `grep -n "Period == 3\|Period >= 5" internal/source/manual.go` → no match.
- `grep -n "oldP == 2\|newP >= 4\|newP >= 3" internal/state/state.go` → no match.
- `grep -n "pn <= 4\|n >= 5\|f >= 5\|period <= 4\|period <= 2\|n >= 4" web/overlay/overlay.js web/marcar/marcar.js` → no match.

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/sport/sport_test.go`: the Step 1 cases.
- `internal/source`: a new test that a `Manual` with a custom `Rules` (`Half: 1`, `FoulsReset: false`) keeps fouls on `period_next` and resets timeouts at period 2. This proves the scorer reads the profile.
- `internal/state`: a new test that `FinalHold == time.Duration(sport.Basketball.FinalHold) * 100 * time.Millisecond`. Do not add a fake profile to `Get`; plan 004 adds the first real second profile.
- No existing test changes.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] The four greps in Step 5 return nothing
- [ ] No existing `*_test.go` file modified (`git diff --stat 4093d0c..HEAD -- '*_test.go'`)
- [ ] `grep -n "rules" docs/CONTRACT.md` finds `game.rules`
- [ ] `plans/README.md` status row updated

## STOP conditions

- An existing test fails and only editing it would make it pass: behaviour changed.
- The overlay or scorer page reads a basketball number from a place this plan doesn't list (search `4`, `5`, `10:00`, `600` near period, foul or timeout code). List them and stop.
- `m.s.Game.Sport` is not set when a real game runs (check `internal/web/game.go:184-188` still fills it).
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Plan 003 moves the sport from the club to each escalão; `internal/web/game.go` then takes it from the game's escalão instead of `cl.Sport`.
- Plans 004 to 007 add profiles (volleyball, futsal, handball, soccer) and the fields they need (sets, count-up clock, suspensions, cards). Add fields to `Profile`; don't fork the scorer per sport.
- Reviewers: run the demo and watch a full game through to "final"; check the overlay's fouls and bonus at 4 and 5; check overtime labels.

## Plan review

One cold review (`revise`). All findings fixed in the plan except: `Manual.Rules` is used by nothing but a test until plan 003. Kept, because plan 003 wires it and the test proves the scorer reads the profile.
