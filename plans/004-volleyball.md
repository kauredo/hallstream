# Plan 004: A club can score volleyball from the phone, and the overlay shows sets, points and the serve

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/store/store.go internal/club/club.go internal/web/profile.go internal/web/overlay_test.go cmd/hallstream/main.go web/overlay web/marcar web/painel/painel.js web/i18n docs/CONTRACT.md`
> Plans 002 and 003 are expected to have changed several of these files.
> First confirm they landed (see "Depends on"), then compare every "Current
> state" excerpt with the live code. Line numbers below are from `4093d0c`
> and will have moved; a change you can map one-to-one is fine, one you
> can't is a STOP condition.

## Status

- **Priority**: P1 (first new sport of ROADMAP Phase C; unblocks plan 001, Nevobo)
- **Effort**: L
- **Risk**: MED (new score model on the on-air path; basketball must not change)
- **Depends on**: `plans/002-sport-package-basketball.md` (`internal/sport` with `Profile`, `Get`, `Timeouts`, `ResetsTimeouts`, `Overtime`, and `game.rules` sent to the pages) and `plans/003-*.md` (each escalão has a sport; `game.sport` and `game.rules` come from the game's escalão). Check: `ls internal/sport/sport.go` exists, and `grep -n "Rules" internal/state/state.go` finds the `Rules` field on `Game`. Both must hold or STOP.
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

Hallstream can only score basketball. ROADMAP Phase C lists volleyball as "sets to 25 (15 in the last), the serving side", and the owner picked it as the first new sport (2026-11-25): it changes the score model the most, so it tests `internal/sport` hardest, and it unlocks the Nevobo provider (plan 001). After this plan a volleyball escalão is scored from `/marcar` with volleyball's rules, and the overlay draws its scoreboard, intervalo and final screens with set scores. Basketball behaves exactly as before.

Owner decisions this plan must honour (`plans/phase-c-decisions.md`):

- Volleyball first.
- The score is stored in **a separate sets list**, not in the period table. `Live.Home/Away` stay the points of the current set; finished sets live in their own list. Final, intervalo and the game log read the sets when the sport is volleyball.
- A sport is listed as supported on a console only after a recorded real game passes a decoder test. Volleyball on the Stramatel console stays "untested" here.

## Decisions made in this plan (the executor does not re-decide them)

1. **Set end is a scorer tap, never automatic.** The scorer page highlights "Set seguinte" once a team has reached the set target with a 2-point lead, and asks for confirmation as `period_next` does today. Auto-advancing would end a set on a mis-tap that the scorer then has to undo on air.
2. **The serve follows the point.** In volleyball the side that wins a rally serves next, so `point +1` for a team sets `Serve` to that team. A separate `serve` action switches it by hand for the first serve of each set and for corrections. `point -1` (a correction) leaves `Serve` alone.
3. **Points are +1 or −1 only.** Any other `N` is an error.
4. **The game ends when a team wins `SetsToWin` sets.** Then the state machine switches to `final` straight away (no 60 s hold: there is no clock) and records the last set.
5. **No automatic intervalo between sets.** Breaks between sets are about 3 minutes, and switching scenes every set would hide the score during the warm-up rallies. The volunteer can still show `intervalo` by hand, and it shows the set table.
6. **Timeouts: 2 per team per set**, reset at every new set.
7. **The sets list lives on `State`, next to `Periods`**, recorded by the state machine when the sets count rises. The owner asked for a separate list; it sits on `State` because the state machine records it, saves it to disk and restores it after a restart exactly as it does `Periods`, and `Live` is rebuilt from every frame and keeps no history. A set count can come from either source: the phone scorer sends every finished set's score, while the Stramatel console only sends the counts. With the console, the machine records the points that were on the board just before the count rose. A set whose score is unknown (seeded from a console) is stored with both scores at 0 and its `Winner` set.
8. **The deciding set** is the set whose number is `2*SetsToWin - 1` (the 5th in best of 5). Its target is `LastSetPoints` (15).

## Current state

### Files and their roles (line numbers at `4093d0c`)

- `internal/sport/sport.go`: created by plan 002. `Profile` has `ID, Periods, Half, Length, ClockDown, FoulsReset, FoulWarn, FoulBonus, ShotClock, FinalHold, TimeoutsBy [3]int` with JSON tags, plus `Timeouts(p)`, `ResetsTimeouts(p)`, `Overtime(p)`, `Get(id)` (unknown id → basketball) and `var Basketball`.
- `internal/source/manual.go`: the phone scorer.
  - `Score` (lines 13-24): `Home, Away, Period, Clock, Running, Length, HomeFouls, AwayFouls, HomeTimeouts, AwayTimeouts`, saved to `dados/manual.json`.
  - `Action` doc (lines 39-53) lists every `Do` value; `Do` (lines 162-243) switches on it. `count` (line 256) validates `point`/`foul`/`timeout`. After plan 002, `period_next` reads `m.Rules()`.
  - `emit` (lines 381-395) sends the score to the state machine as a `map[string]string`:
    ```go
    m.out(map[string]string{
    	"home": n(s.Home), "away": n(s.Away),
    	"clock": state.ClockText(s.Clock), "period": n(s.Period),
    	"home_fouls": n(s.HomeFouls), "away_fouls": n(s.AwayFouls),
    	"home_timeouts": n(s.HomeTimeouts), "away_timeouts": n(s.AwayTimeouts),
    	"shot_clock": "",
    })
    ```
  - `Restore` (lines 115-125) and `Seed` (lines 129-144): put back a saved score, or start from the score on air when the scorer takes over from the console.
- `internal/state/state.go`:
  - `Live` (lines 36-50); `Period` (lines 54-60); `State` (lines 198-210) with `Periods []Period`.
  - `ApplyConsole` (lines 666-689) builds `Live` from the frame map; it ignores keys it doesn't know (so `home_sets`, `away_sets`, `serve` are dropped today).
  - `setLive` (lines 727-764), `checkFinal` (767-775), `record` (780-790): period recording and the automatic `intervalo`/`marcador`/`final` screens. After plan 002 they read `sport.Get(m.s.Game.Sport)`.
  - Event hook (lines 380-383): `m.OnEvent("period", map[string]any{"period": p.Period, "home": p.Home, "away": p.Away})` for each newly recorded period; `seen` (line 345) counts `len(m.s.Periods)`.
  - `snapshot` (line 393-) copies `Periods`; `ResetPeriods` (623), `RestorePeriods` (635), `saver` (643-) call `SavePeriods`.
- `internal/ble/decode.go`: the console decoder. For volleyball (`isNet`, lines 24-26) it already sets `home_sets`, `away_sets` and `serve` (lines 89-96):
  ```go
  state["home_sets"], state["away_sets"] = text(f, 9, 1), text(f, 10, 1)
  switch f[19] {
  case '1':
  	state["serve"] = "home"
  case '2', '3':
  	state["serve"] = "away"
  default:
  	state["serve"] = ""
  ```
  **Do not change this file.**
- `internal/store/store.go:654-663`: `SavePeriods` / `LoadPeriods` write and read `dados/periods.json`. `SaveHistory` at line 687.
- `cmd/hallstream/main.go`:
  - lines 183-184: `m.OnPeriods = func(s state.State) { saveHistory(st, s) }` and `restorePeriods(st, m)`.
  - `restorePeriods` (350-363) loads `periods.json` and wires `m.SavePeriods`.
  - `saveHistory` (376-400) writes `dados/jogos/…` with `"game"`, `"periods"` and `"final"`, where `final` is the last period's score or the live score.
- `internal/club/club.go:31-37`: `Sports` with `{"volleyball", "VOLEIBOL", "VOLEIBOL", false}`. `internal/web/profile.go:38-40` refuses an inactive sport with `api.clube.so_basquetebol`.
- `web/painel/painel.js:2151-2152`: `SPORTS`, where the third element `true` marks a sport as available; volleyball has none. Lines 2187 and 2900 build the two sport selects from it. (Plan 003 may add an escalão sport select that reads the same list.)
- `web/marcar/index.html`: per team, `+1 +2 +3` point buttons (lines 18-20 and 58-60), a fouls row (22-) and a timeouts row (30-34); the centre has the period arrows (`data-ask="period_back"`/`"period_next"`, lines 41-44), the clock (45), clock nudges (46-49) and the start button `#arrancar` (50).
- `web/marcar/marcar.js`: `periodName` (135-136), `render` (around 170-200): fouls and timeouts marks, the `#arrancar` button that offers `period_next` at 0:00, the confirm dialog for period changes (268-277).
- `web/overlay/index.html`: the scoreboard `#s-marcador` (line 45-), intervalo (196-204) and final (207-219), each with a `<div class="ptab …" data-periods></div>` period table.
- `web/overlay/overlay.js`: `view(st)` (from line 467) derives what every screen shows; `renderPeriods(periods, v)` (568-) draws the period tables; the render function around line 1155-1180 draws fouls, bonus and calls `renderPeriods`.
- `internal/web/overlay_test.go:83-87`: `TestOverlayRendersEveryFixture` loads `fs.Glob(assets.FS, "overlay/fixtures/*.json")`. `web/embed.go:6` embeds `all:overlay`, so subfolders are already embedded.
- `docs/CONTRACT.md`: the behaviour contract; `game.rules` was added by plan 002.

### Conventions

- Go standard library only. Code and comments in English. Tests in plain `testing`; model scorer tests on `internal/source/manual_test.go`, state tests on `internal/state/state_test.go` (`TestPeriodsAndScreens`, `TestPeriodPoints`).
- Every user-facing string in `web/i18n/pt.json` first (European Portuguese; the program speaks in the first person, instructs in the imperative), then `en.json` (British English), same keys, sorted. `go test ./internal/i18n` enforces it, and fails on keys built at run time.
- `web/` ships as is: no build step, nothing from a CDN.
- The overlay stays on air for hours: a state it doesn't expect must still draw. Every new screen state gets a fixture.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Never read or edit `dados/` or `dist/`.
- Commits: `feat: volleyball …`, conventional commits.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| Overlay fixtures | `go test ./internal/web -run TestOverlayRendersEveryFixture` | pass (needs Chrome) |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel `http://127.0.0.1:8765/painel` |
| Preview a fixture | open `http://127.0.0.1:8765/overlay?state=/overlay/fixtures/volleyball/<name>.json&preview=1` | the screen draws |

## Suggested executor toolkit

- `build-ui` skill, **extension branch** ("a new section, state or screen inside a settled identity"), for Steps 5 and 6. The identity is `docs/design/SYSTEM-R3.md` amended by `SYSTEM-R4.md` and then `docs/design/SYSTEM-R8.md` (the later file wins). Run its lint and outside critique on the volleyball scoreboard and final screen before calling the step done.
- `interface-guidelines` for the scorer page buttons (touch targets on a phone held sideways).

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go` (new tests only; existing tests unchanged)
- `internal/state/state.go`, `internal/state/state_test.go` (new tests only)
- `internal/store/store.go` (`SaveSets`/`LoadSets` only)
- `cmd/hallstream/main.go` (`restorePeriods` → also sets; `saveHistory` → `"sets"` and the volleyball `final`)
- `internal/club/club.go` (volleyball `Active: true`)
- `internal/web/overlay_test.go` (glob subfolders)
- `web/marcar/index.html`, `web/marcar/marcar.js`, `web/marcar/*.css` (whichever stylesheet the page uses)
- `web/overlay/index.html`, `web/overlay/overlay.js`, `web/overlay/overlay.css`, `web/overlay/fixtures/volleyball/*.json` (create)
- `web/painel/painel.js` (volleyball available in `SPORTS`)
- `web/painel/resultado.js` (the result image: Step 6b)
- `web/painel/mock.js` (a volleyball mock: Step 6b)
- `internal/state/demo.go` (stash and restore `Sets` like `realPeriods`: Step 3)
- `internal/web/server.go:285` and `internal/web/game.go:42` (one `ResetSets` call each)
- `internal/web/server.go` and `internal/web/game.go` (only to call `ResetSets` next to each `ResetPeriods`, at `server.go:285` and `game.go:42` as of `4093d0c`)
- `internal/web/profile.go` (only the wording behind the "só basquetebol" error key, if the key stays)
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- `internal/ble/decode.go` and the console's Bluetooth path. Volleyball on the console stays untested.
- Basketball's behaviour, and every existing test file's existing tests.
- Best of 3 for youth levels (see Maintenance notes).
- Libero, rotations, substitutions, cards.
- The Nevobo provider (plan 001).
- The website's support table (website repo).

## Git workflow

- Branch: `feat/volleyball`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The volleyball profile

In `internal/sport/sport.go`, add to `Profile` (with JSON tags, since `game.rules` sends it to the pages):

```go
	Scoring       string `json:"scoring"`         // "points" (basketball) or "sets"
	SetPoints     int    `json:"set_points"`      // points to win a set (25); 0 when Scoring is "points"
	LastSetPoints int    `json:"last_set_points"` // points to win the deciding set (15)
	SetsToWin     int    `json:"sets_to_win"`     // sets to win the match (3 = best of 5)
	WinBy         int    `json:"win_by"`          // lead needed to win a set (2)
```

Set `Scoring: "points"` on `Basketball`. Add:

```go
// Volleyball: best of 5, sets to 25 (15 in the 5th), win by 2, no clock,
// 2 timeouts per team per set.
var Volleyball = Profile{ID: "volleyball", Scoring: "sets", SetPoints: 25, LastSetPoints: 15,
	SetsToWin: 3, WinBy: 2, TimeoutsBy: [3]int{2, 2, 2}}

// Sets reports whether the sport is scored in sets.
func (p Profile) Sets() bool { return p.Scoring == "sets" }

// SetTarget is the points that win set n (1-based).
func (p Profile) SetTarget(n int) int

// SetWon reports which side has won a set at home-away in set n: "home",
// "away" or "" while it goes on.
func (p Profile) SetWon(n, home, away int) string
```

- `SetTarget(n)`: `LastSetPoints` when `n == 2*SetsToWin-1`, else `SetPoints`.
- `SetWon`: a side wins at `>= SetTarget(n)` points with a lead of at least `WinBy`.
- `ResetsTimeouts(p)` must return true for every `p >= 2` when `Sets()`; `Timeouts(p)` returns `TimeoutsBy[0]` (2) for every set when `Sets()`. Keep basketball's results exactly as plan 002 left them.
- `Get("volleyball")` returns `Volleyball`.

**Verify**: `go test ./internal/sport/` → pass, with new tests: `SetTarget(1..4) == 25`, `SetTarget(5) == 15`; `SetWon(1, 25, 23) == "home"`, `SetWon(1, 25, 24) == ""`, `SetWon(1, 26, 28) == "away"`, `SetWon(5, 15, 13) == "home"`, `SetWon(5, 15, 14) == ""`; `ResetsTimeouts(2..5)` true for volleyball; basketball's tests from plan 002 unchanged.

### Step 2: The phone scorer keeps sets and the serve

In `internal/source/manual.go`:

- Add to `Score`:
  ```go
  	Sets  []SetScore `json:"sets"`  // finished sets, in order (volleyball)
  	Serve string     `json:"serve"` // "home", "away" or ""
  ```
  and a type `SetScore struct { Home, Away int; Winner string }` with JSON tags `home`, `away`, `winner`. Keep it in `source` and have `state` define its own identical type; don't import `source` from `state`. `Score` is no longer comparable with `==` because of the slice: replace `if *s == prev` (line 233) with a small `same(a, b Score) bool` that compares every field and the sets. Do the same anywhere else `Score` values are compared (search for `== prev`, `m.sent`).
- When `m.Rules().Sets()`:
  - `point`: only `N == 1` or `N == -1`, else `errPoints`. `+1` also sets `Serve` to that team.
  - `foul`: refused with a new error `manual.sem_faltas` ("Este desporto não conta faltas." / "This sport doesn't count fouls.").
  - New action `serve`: sets `Serve` to `Team` (`"home"` or `"away"`, else `errTeam`).
  - New action `set_next`: refused (new error `manual.set_por_acabar`, "O set ainda não acabou." / "The set isn't over yet.") unless `SetWon(len(s.Sets)+1, s.Home, s.Away) != ""`. Otherwise append `SetScore{s.Home, s.Away, winner}`, set `Home, Away = 0, 0`, `Period = len(s.Sets)+1`, timeouts to 0, `Serve` to `""`. Refused too once a team has `SetsToWin` sets (`manual.jogo_acabou`, "O jogo já acabou." / "The game is over.").
  - `period_next`, `period_back`, `clock_*`, `period_length` are refused with `manual.sem_relogio` ("Este desporto não tem relógio." / "This sport has no clock.").
  - `new_game` clears `Sets` and `Serve` too.
  - `undo` already restores the whole `Score`; make sure the saved copy in `undone.s` holds its own copy of `Sets` (copy the slice, don't share it).
- `emit`: when `Sets()`, also send `"home_sets"`, `"away_sets"` (the counts of sets won), `"serve"`, and `"sets"`: the finished sets as `"25-20,18-25"` (home-away, comma separated; "" for none). `"clock"` is `""` and `"period"` is the current set number. Basketball's map is unchanged.
- `Restore`: a restored volleyball score keeps `Sets` and `Serve` (they come from the JSON).
- `Seed` from the console: take `home_sets`/`away_sets` from the state's `Live` (Step 3 adds `HomeSets`, `AwaySets`, `Serve` to `Live`) and fill `Sets` with that many `SetScore{0, 0, winner}` entries (home's first), plus the current points and `Serve`. Set `Period = HomeSets + AwaySets + 1`.
- Update the `Action` doc comment with `serve`, `set_next` and which actions a sets sport refuses.

New keys `manual.sem_faltas`, `manual.set_por_acabar`, `manual.jogo_acabou`, `manual.sem_relogio`, pt then en, as above.

**Verify**: `go test ./internal/source/` → pass, with new tests on a `Manual` whose `Rules` returns `sport.Volleyball`: `+1` home sets `Serve` to home; `point` with `N: 2` errors; `foul` errors; `set_next` at 24-23 errors, at 25-23 appends `{25, 23, "home"}` and zeroes the points; three home sets then `set_next` errors with `manual.jogo_acabou`; `undo` after `set_next` brings back 25-23 and an empty `Sets`; `emit` sends `"sets": "25-23"`, `"home_sets": "1"`, `"clock": ""`. All existing tests pass unchanged.

### Step 3: The state machine records sets and ends the match

In `internal/state/state.go`:

- Add `HomeSets int`, `AwaySets int`, `Serve string` to `Live` (tags `home_sets`, `away_sets`, `serve`), filled in `ApplyConsole` from the frame map.
- Add `type SetScore struct { Set, Home, Away int; Winner string }` (tags `set`, `home`, `away`, `winner`) and `Sets []SetScore \`json:"sets"\`` on `State`, after `Periods`. `snapshot` copies it; the initial state has `Sets: []SetScore{}` (as `Periods` does at line 287).
- In `setLive`, when `sport.Get(m.s.Game.Sport).Sets()`, skip the period logic entirely and run a new `setSets(old, l)`:
  - When `l.HomeSets + l.AwaySets` is higher than the number of recorded sets, record the missing sets. If the frame map carried `"sets"` (the phone scorer), take each set's score from it. `ApplyConsole` parses `v["sets"]` into a local `[]SetScore` and passes it on: change `update(l Live, fromDemo bool)` to `update(l Live, fromDemo bool, finished []SetScore)` and `setLive(l Live)` to `setLive(l Live, finished []SetScore)`; the demo's calls pass `nil`. No field is added to `Live` for it: `State.Sets` holds the recorded result, and the parsed list is only an input to recording it. Otherwise (the console) record the new set with `old.Home`, `old.Away`, and the winner from which count rose.
  - When the count falls (an undo), drop recorded sets beyond it.
  - When either count reaches `SetsToWin` and `finalDone` is 0, call `m.auto("final")` and set `finalDone` to the number of recorded sets. Clear `finalDone` to 0 when the recorded sets drop below a winning count (an undo across the match end), and in `ResetSets`. So undoing the last set and finishing it again fires `final` a second time.
  - No automatic `intervalo` and no return to `marcador` (Decision 5), except: when the screen is `final` and a new set starts (an undo took the match back), go back to `marcador`.
  - Report "recorded" so the existing hooks (`recordedSaver`, `recorded`, `OnPeriods`) run, exactly as a period does.
- Event hook (near lines 380-383): count `len(m.s.Sets)` in `seen` and emit `m.OnEvent("set", map[string]any{"set": s.Set, "home": s.Home, "away": s.Away})` for each new one.
- `ResetPeriods` and `RestorePeriods` (623-640): add `ResetSets` (clears `Sets` and `finalDone`, called wherever `ResetPeriods` is called: `internal/web/server.go:285` and `internal/web/game.go:42`) and `RestoreSets(ss []SetScore)`, which sets `m.seen.sets = len(ss)` as `RestorePeriods` sets `m.seen.periods` (state.go:638), so a restart doesn't replay `"set"` events.
- `saver` (state.go:646-660): **one hook sequence saves both.** Add `SaveSets func([]SetScore)` next to `SavePeriods`. `saver` returns early only when both hooks are nil (or in the demo, as now); it takes one `seq` and snapshots both `Periods` and `Sets` under the lock, and the returned func, when `seq > m.savedSeq`, calls `SavePeriods(ps)` and `SaveSets(ss)` (each only if non-nil). One sequence keeps the two files from the same moment.
- `internal/state/demo.go`: stash `Sets` with `realPeriods` when the demo starts (lines 21-22: add `realSets`), restore them when it stops (line 27), and clear `Sets` in the demo's `newGame` (line 90) alongside `Periods`.
- `internal/store/store.go`: `SaveSets(v any) error` / `LoadSets(v any) error` for `dados/sets.json`, copied from `SavePeriods`/`LoadPeriods`.
- `cmd/hallstream/main.go`: `restorePeriods` also loads `sets.json` into `m.RestoreSets` and wires `m.SaveSets`. `saveHistory` adds `"sets": s.Sets`, and for a sets sport `"final"` is the sets won (`{"home": HomeSets, "away": AwaySets}` from `s.Live`).

**Verify**: `go test ./internal/state/ ./internal/store/ ./cmd/...` → pass, with new state tests (game sport `volleyball`): a manual frame with `home_sets: "1"`, `sets: "25-20"` records `{1, 25, 20, "home"}`; a console frame sequence at 24-20 with `home_sets` 0, then 0-0 with `home_sets` 1, records `{1, 24, 20, "home"}`... (the console's last frame before the count rose; use whatever the previous `Live` held); a count that falls drops the set; reaching 3 sets switches to `final` once; after that, a frame whose count drops to 2 (undo) then rises to 3 again switches to `final` a second time; `RestoreSets` with two sets then a frame recording the third emits exactly one `"set"` event; no `intervalo` between sets; a basketball game is untouched (all existing tests pass unchanged).

### Step 4: Volleyball is available

- `internal/club/club.go`: volleyball `Active: true`.
- `web/painel/painel.js` `SPORTS`: add `true` as volleyball's third element. If plan 003 added another sport list (per escalão), it reads the same array; check.
- `internal/web/profile.go`: the inactive-sport message key is `api.clube.so_basquetebol` ("only basketball"). Leave the check as is, and change the pt/en text of that key to say the sport isn't available yet, without naming basketball. Do not rename the key.

**Verify**: `go test ./internal/web/ ./internal/club/` → pass; in the demo, Definições lets you pick Voleibol.

### Step 5: The scorer page for volleyball

In `web/marcar/marcar.js` and `index.html`, when `rules.scoring === 'sets'` (from `game.rules` in the state, as plan 002 reads it):

- Hide the `+2` and `+3` buttons, the fouls row, the clock, the clock nudges and the period arrows. No new correction button: a long press on the `+1` button (`data-long`, `index.html:18`) already sends `n: -1` (`marcar.js:243-254`).
- Show per team the sets won and a serve marker; tapping the marker sends `serve` for that team.
- The centre shows "Set n" (`marcar.set`, "Set {n}" in both languages) and the finished sets as small scores (e.g. `25-20 · 18-25`).
- `#arrancar` becomes "Set seguinte" (`marcar.set_seguinte`, "Set seguinte" / "Next set"), enabled only when the current points win the set (compute it the same way as `SetWon`: target 25, or 15 in set `2*sets_to_win-1`, lead of 2), and asks for confirmation with the existing dialog (`marcar.confirmar.set`, "Acabar o set {n} com {casa}-{fora}?" / "End set {n} at {casa}-{fora}?").
- Timeouts marks use `rules.timeouts[0]` (2) for every set.
- `undoLabel` gets labels for `serve` and `set_next` (`marcar.ultimo.servico`, `marcar.ultimo.set`).

Work through `build-ui` (extension branch); reuse the page's existing classes and tokens.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass. In the demo with a volleyball escalão picked and input manual, open `http://127.0.0.1:8765/marcar` at phone width (landscape): `+1` moves the serve, "Set seguinte" stays disabled at 24-23 and works at 25-23, and the set appears in the finished list.

### Step 6: The overlay for volleyball

In `web/overlay/overlay.js`, `index.html` and `overlay.css`, when `game.rules.scoring === 'sets'`:

- Scoreboard (`#s-marcador`): per team the current set's points, the sets won next to them, and a serve marker on the serving side. No clock, no fouls pips, no bonus, no shot clock. The period label shows "Set n" (`overlay.set.curto`, "Set {n}" / "Set {n}"; long form `overlay.set.longo`, "{n}.º set" / "Set {n}").
- Intervalo and final: the `data-periods` tables show one column per finished set (header "1", "2", …, plus a total of sets won), from `st.sets` instead of `st.periods`. In `final`, the big score is the sets won, not the points.
- `voltamos` (overlay.js:522): `v.inPlay` is true once any set is recorded (`(st.sets || []).length > 0`), as well as on points, when the sport plays in sets.
- Intervalo's `halfLine` (overlay.js:524-526, "a seguir: n.º período"): for sets, "Set {n}" with n = recorded sets + 1 (new key `overlay.intervalo.set_seguinte`, "A seguir: {n}.º set" / "Next: set {n}"), hidden once a team has won.
- A state with `scoring: 'sets'` and no `sets` key must still draw (treat it as `[]`).
- Basketball fixtures and screens are unchanged.

Fixtures in `web/overlay/fixtures/volleyball/` (copy the shape of `marcador.json`, `intervalo.json`, `final.json`, and set `game.sport: "volleyball"`, `game.sport_label: "VOLEIBOL"`, `game.rules` = the volleyball profile as JSON):

- `marcador.json`: set 3, 18-21, sets 1-1, serve away.
- `marcador-set5.json`: deciding set, 13-14, sets 2-2, serve home.
- `intervalo.json`: after set 2, sets `[{25,20,home},{18,25,away}]`.
- `final.json`: 3-1 in sets, four sets listed.
- `final-sem-pontos.json`: a final whose sets were seeded from the console (all scores 0, winners set).

In `internal/web/overlay_test.go:87`, glob both `overlay/fixtures/*.json` and `overlay/fixtures/*/*.json`, and name sub-tests with the folder (`volleyball/marcador`).

Work through `build-ui` (extension branch) inside `SYSTEM-R8.md`, with its lint and outside critique on the scoreboard and the final screen.

**Verify**: `go test ./internal/web -run TestOverlayRendersEveryFixture` → pass with the five new sub-tests (Chrome installed); preview each fixture with `?preview=1` and check no clock or fouls show.

### Step 6b: The result image and the panel mock

- `web/painel/resultado.js`, `heading` (lines 30-34) says "Final" when a 4th period is recorded. For a sets sport (`(st.game.rules || {}).scoring === 'sets'`), say "Final" when either side's recorded sets won reaches `rules.sets_to_win`, else intervalo/em curso as now. The big scores (`side(...)`, lines 177-178) are the sets won. `periodsLine` (line 180) draws the set scores (`25-20 · 18-25 · …`) from `st.sets`; a set seeded with 0-0 shows only its winner's side marked (draw "–").
- `web/painel/mock.js`, near `resultadoShot` (lines 524-536): add a `case=voleibol` that sets `game.sport` `volleyball`, `game.rules` to the volleyball profile, `live` with sets won, and `sets` with four finished sets, so the result image can be previewed.

**Verify**: open `http://127.0.0.1:8765/painel?mock=1&case=voleibol` where `resultadoShot` is triggered (find how the existing `case` values are reached in `mock.js`) and check "Final" and the set scores; `go test ./internal/web/` → pass.

### Step 7: Contract and roadmap

- `docs/CONTRACT.md`: the volleyball profile fields in `game.rules`; `live.home_sets`, `live.away_sets`, `live.serve`; `state.sets`; the scorer actions `serve` and `set_next` and what a sets sport refuses; `sets.json`; the set and final rules (Decisions 1-8); the `"set"` timeline event; the game log's `"sets"` and volleyball `"final"`.
- `docs/ROADMAP.md` Phase C: under volleyball, "Done from the phone scorer. The console decodes volleyball frames but is untested until a recorded game passes."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/sport/sport_test.go`: Step 1 cases.
- `internal/source/manual_test.go`: Step 2 cases, new tests only.
- `internal/state/state_test.go`: Step 3 cases, new tests only, modelled on `TestPeriodsAndScreens` and `TestPeriodPoints`.
- `internal/web/overlay_test.go`: the five volleyball fixtures through the existing Chrome test.
- Existing tests: all pass with no change to them.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `ls web/overlay/fixtures/volleyball/*.json | wc -l` → 5
- [ ] `git diff 4093d0c..HEAD -- internal/ble/decode.go` is empty
- [ ] `grep -n '"volleyball", "VOLEIBOL", "VOLEIBOL", true' internal/club/club.go` matches
- [ ] `grep -n "set_next" docs/CONTRACT.md` matches
- [ ] No existing test case modified (review `git diff 4093d0c..HEAD -- '*_test.go'`: only additions, plus the glob change in `overlay_test.go`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 002 or 003 has not landed (the checks under "Depends on" fail).
- `game.sport` still comes from the club, not the escalão (plan 003 not done the way this plan assumes).
- An existing test fails and only editing it would make it pass.
- `Score` comparison or copying is used somewhere this plan doesn't list and the slice breaks it in a way you can't fix locally.
- The overlay draws volleyball and basketball from markup so tangled that a sets variant would rewrite the scoreboard. Report the shape and stop; that's a design decision.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- **Best of 3** (some youth and regional levels): add a per-escalão override of `SetsToWin` (and maybe `SetPoints`) later. Every rule here reads those fields, so nothing else changes.
- **Known limit:** a set ended by mistake can only be reopened with undo, and undo keeps 50 actions and is lost on a restart (`Restore` clears it). After that the scorer can't reopen the set; they start a new game or correct points by hand in the next set.
- **The console path is untested.** When a recorded volleyball session exists (ROADMAP Phase D capture mode), add a decoder test and a state test fed from it, then list volleyball as tested on the console. The machine records console sets from the board's last points before the count rose; a recording will show whether that's reliable.
- Futsal, handball and soccer (plans 005-007) are `Scoring: "points"` and reuse the period logic; only volleyball takes the `setSets` branch.
- Reviewers: run a full best-of-5 game from `/marcar` in the demo, including an undo across a set end and a match end; check `dados/jogos` gets `"sets"` (in a scratch build folder, not your real `dados/`); check a basketball game still goes through intervalo and final as before.

## Plan review

One cold review, verdict approve-with-fixes. All eleven points were checked against the code and applied. One changed shape: the reviewer asked to drop `Live.Finished` if `State.Sets` could carry it. It can't on its own, because the phone's set scores arrive in the frame before the machine records them, so the plan now passes the parsed list as an argument to `update`/`setLive` instead of adding a field to `Live`.
