# Plan 007: A club can score football: 2 × 45, added time, and cards and substitutions per player

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0d7570c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club internal/web/marcar.go cmd/hallstream/main.go internal/web/overlay_test.go web/overlay web/marcar web/painel docs/CONTRACT.md`
> Plans 005 and 006 (the two layers below in the stack) change most of these
> files. Read `internal/sport/sport.go`, `internal/source/manual.go` and
> `internal/state/state.go` whole before starting and check that what
> "After plans 005 and 006" lists exists. A shape you can't map one-to-one is
> a STOP condition.

## Status

- **Priority**: P3 (last sport in Phase C)
- **Effort**: L
- **Risk**: MED
- **Depends on**: plan 005 (futsal) and plan 006 (handball: the count-up clock), layers 01 and 02 of this stack. Plans 002-004 and #30 are on `main`.
- **Category**: direction
- **Planned at**: commit `0d7570c`, 2026-11-25 (reconciled; first written against `4093d0c`)
- **Stack**: layer 03 (base: `02-feat/handball`), the top.

## Why this matters

ROADMAP Phase C lists soccer: "the clock counts up, 2×45 plus stoppage time, cards and substitutions. Scored by hand." Football is the biggest club sport in Portugal, and a club that streams basketball can stream its football teams too, now that the sport is set per escalão. No Stramatel console decodes football, so it is scored from the phone only.

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`, and review):

- **Cards and substitutions are tracked per player**, tied to shirt numbers from the escalão's squad, entered on the scorer page. The scoreboard shows per-team card counts, and each card and substitution gets a lower third.
- **The game ends with an explicit "Fim do jogo"** (`game_end`, behind a confirm), never a hold: the clock runs on past 90:00 and nothing can tell the end of added time from a pause.
- **Counts:** yellows count every yellow shown, the first of a pair included (`yellow` and `second_yellow`); reds count `red` plus `second_yellow`.
- **Added time shows as "45+2'"** (the half's end minute plus the announced minutes); a card in added time as "45+3'".
- **Extra time and penalty shoot-outs are out of scope.**
- The sport is set per escalão; new scoreboard variants go through `build-ui` inside R8.

## Rules (verify before building)

From the IFAB Laws of the Game (Law 7, duration; Law 3, substitutes; Law 12, cautions and sending-offs). Read the current edition (use `read-the-damn-docs`) and STOP if these differ:

- Two halves of 45:00; the clock counts up and runs on for added time; the second half shows 45:00 to 90:00. Half-time after the first half.
- Added time is announced as whole minutes (+N).
- A second yellow card to the same player is a red card.
- Youth and amateur games often play shorter halves; the scorer's existing `period_length` covers that. `MaxClock` (99:59, `internal/source/manual.go:56`) is enough without extra time.

## Current state

### After plans 005 and 006 (layers 01 and 02)

- `sport.Profile` has `OvertimeLength`, `FoulsCarry`, `PeriodWord`, `FoulMarks` (0 hides the scorer's fouls row), `StopAtEnd`, `Suspensions`, and the methods `Overtime`, `ResetsFouls`, `Start(period, length)` and `End(period, length)`, where `length` is the scorer's saved `Score.Length`.
- `Score` has `HomeSusp`/`AwaySusp [3]Suspension` and `NextSusp`, and no period fields: starts and ends derive from `s.Length` and the profile, in Go and on the page ("Repor" computes the period's start from `game.rules` and `score.length`). The switch of period length when the game's sport changes lives in `SyncGame` (`Server.lastSport`; to or from a count-up sport it also resets the clock to the period's start, or runs `new_game` on a 0–0 period-1 score). The scorer's `current()` counts up when the profile does, and stops at `End` only when `StopAtEnd`. `emit()` sends `"clock"` via `m.clockText` (`state.ClockTextUp` for a count-up) and `"period_end"`.
- `state.Live` has `PeriodEnd` and `Suspensions`; `setLive` treats a rising clock as "started" for a count-up, and **a count-up without `StopAtEnd` never starts the final hold**. `state.ClockTextUp` exists. `Machine.RestoreSeen(susp int)` exists, and `restoreManual(st, mn, m)` calls it after `mn.Restore`.
- The scorer page hides `+2`/`+3` for a count-up sport and knows the `half` period word.

### `Score` and `==` (at `0d7570c`, unchanged by 005 and 006)

`internal/source/manual.go:329` is `if *s == prev { return nil }`. **There is no `same()` helper.** Go refuses to compile `==` on a struct that has a slice field, even when the slice is nil, so adding `Cards []Card` to `Score` breaks every `==` on it. These are all of them (at `0d7570c`):

- `internal/source/manual.go:329`: `if *s == prev`.
- `internal/source/manual_test.go:87`: `s != newScore(DefaultLength)`; `:267`: `got != want`; `:449`: `s != want`.
- `internal/source/source_test.go:113`: `s != newScore(DefaultLength)`.
- `cmd/hallstream/sets_test.go:100`: `got != want` (two `Score` values).

The `Score{…}` composite literals (`manual.go:63`, `:185`, `:202`; `manual_test.go:266`, `:448`; `source_test.go:121`) keep compiling unchanged once `Score` has slice fields; only the comparisons above break. Plans 005 and 006 may add comparisons; find them with `grep -rn "Score() [!=]=\|== prev\|!= newScore" internal/source cmd/hallstream`.

### Squad and lower third (at `0d7570c`)

- `internal/state/state.go:96`: `type SquadPlayer struct { Number, Name, PhotoURL string }`. `State.Game.Squad` is the **home** players for the game, filled in `internal/web/game.go:238` (`out.Squad = append(out.Squad, state.SquadPlayer{Number: n, Name: p.DisplayName, PhotoURL: p.PhotoURL})`). The opponent's squad is not known anywhere.
- `internal/state/state.go:185-194`:
  ```go
  type LowerThird struct {
  	Visible   bool       `json:"visible"`
  	Type      string     `json:"type"`
  	Number    string     `json:"number"`
  	FirstName string     `json:"first_name"`
  	LastName  string     `json:"last_name"`
  	Label     string     `json:"label"`
  	PhotoURL  string     `json:"photo_url"`
  	Until     *time.Time `json:"until"`
  }
  ```
  `Machine.ShowLowerThird(lt, d)` is at line 647; `Machine.Snapshot()` at line 423.
- `internal/web/server.go:646` (`lowerThird`) builds a `"jogador"` lower third; it splits the name at line 676: `first, last, _ := strings.Cut(strings.TrimSpace(p.DisplayName), " ")`.
- `web/overlay/overlay.js`: `ltContent` (line 760) and `ltFill` (line 775) know `comentador` and treat everything else as `jogador`.
- `internal/web/marcar.go:55`, `marcarDo`: decodes a `source.Action`, refuses with 409 unless the input is `manual`, calls `s.Manual.Do(a)`, replies with the score.
- `store.Overlay.LowerThird` (`internal/store/store.go:146`) is only the side the lower third shows on; there is no on/off setting.
- Timeline: `seen` (`state.go:362`), `seenNow` (371), `emit` (396) fire events by diffing counts and mute the demo; plan 006 adds `susp`.
- `checkFinal` (`state.go:936`) fires `final` after `r.FinalHold` at the hold; `finalDone` keeps it from firing twice. `matchOver` (line 301) is the volleyball path's own flag and stays untouched.
- `web/marcar/index.html:124`: the `#confirmar` dialog used for period changes.
- `internal/club/club.go`: `{"soccer", "FUTEBOL", "FUTEBOL", false}`; `internal/club/club_test.go:143` lists soccer as inactive. `web/painel/painel.js:2162` and `web/painel/mock.js:356` list the selectable sports.

### Guards that must stay green

- **Basketball, futsal and handball stay pixel-identical.** `TestBasketballScreensDrawTheSameEveryTime` must pass. Also compare screenshots against the layer below: build `02-feat/handball` and this branch to two scratch exes; run each with `-demo -nobrowser` and shoot `/overlay?state=/overlay/fixtures/<f>.json&preview=1&anim=0&style=<s>` for `f` in `marcador`, `intervalo`, `final`, `rodape-jogador`, `rodape-comentador`, `rodape-fotos`, `volleyball/marcador`, `volleyball/final`, `futsal/marcador`, `handball/marcador-exclusoes`, `handball/final` and `s` in `classico`, `lombada`, `cor`, `placa`, `pavilhao`, with headless Chrome (`--headless=new --window-size=1920,1080 --virtual-time-budget=5000 --screenshot=<file>`). Every pair must be byte-identical (`cmp`).
- **Tabular figures.** `overlay.css:91` gives tabular figures to binds ending in `Score`, `Big`, `Sets`, `Fouls`, and to `clock`, `shot`, `startTime`. Name the card-count binds `homeYellows`/`homeReds`… only if you also add their own tabular rule, or give the generated count elements `font-variant-numeric: tabular-nums` directly.

### Conventions

- Go, standard library only; plain `testing`; model on `internal/source/manual_test.go`, `internal/state/state_test.go`, and `internal/web/server_test.go` (`newServer`, `call`) for the handler.
- Every user-facing string in `web/i18n/pt.json` then `en.json`.
- Privacy: squad data stays shirt number, display name and an optional photo; nothing new about players is stored beyond what the scorer enters for this game.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Commit style: `feat: football scoring with cards and substitutions`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test -count=1 ./...` | all pass |
| Scorer, state, web | `go test ./internal/sport/ ./internal/source/ ./internal/state/ ./internal/web/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |

## Suggested executor toolkit

- `read-the-damn-docs` for the IFAB Laws.
- `build-ui`, extension branch, inside `docs/design/SYSTEM-R8.md`, for the card counts, the "45+2'", the two new lower thirds and the scorer page's card and substitution sheets. Stop after its quality gate.

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go`
- `internal/source/source_test.go`, `cmd/hallstream/sets_test.go` (the `sameScore` comparisons named in Step 2 only)
- `internal/state/state.go`, `internal/state/state_test.go`
- `internal/web/marcar.go`, `internal/web/marcar_test.go`
- `cmd/hallstream/main.go` (the `RestoreSeen` call in `restoreManual` only)
- `internal/club/club.go`, `internal/club/club_test.go` (soccer active)
- `web/marcar/index.html`, `web/marcar/marcar.js`, `web/marcar/marcar.css`
- `web/overlay/index.html`, `web/overlay/overlay.js`, `web/overlay/overlay.css`, `web/overlay/fixtures/soccer/*.json` (create)
- `internal/web/overlay_test.go` (add soccer fixtures to the `TestOverlayStyles` list only)
- `web/painel/painel.js`, `web/painel/mock.js`
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- The console path and `internal/ble`. Football is scored by hand only.
- The opponent's squad: away cards and substitutions take a typed shirt number, no name.
- Goal scorers and statistics. Goals are `+1`, as in handball.
- Extra time and penalties.
- Handball's, futsal's, basketball's and volleyball's behaviour.

## Git workflow

- Branch: `03-feat/soccer`, based on `02-feat/handball`.
- One commit per step, conventional commits. Do not push or open a PR unless told to.

## Steps

### Step 1: The soccer profile

Add `Cards bool \`json:"cards"\`` to `Profile` (true for soccer), then:

```go
var Soccer = Profile{ID: "soccer", Periods: 2, Half: 1, Length: 27000, ClockDown: false, CountsUp: true,
	StopAtEnd: false, FinalHold: 0, TimeoutsBy: [3]int{0, 0, 0}, Scoring: "points",
	PeriodWord: "half", Cards: true}
```

No fouls (`FoulMarks` 0), no timeouts, no suspensions, no shot clock, no extra time; `FinalHold` 0 because the game ends on `game_end`. Register it in `profiles`.

**Verify**: `go test ./internal/sport/` → pass: `Soccer.Start(1, 27000)` 0, `Start(2, 27000)` 27000, `End(2, 27000)` 54000, `Start(2, 21000)` 21000; other profiles' tests unchanged.

### Step 2: Cards, substitutions, added time and full time in the scorer

In `internal/source/manual.go`:

**First, give `Score` its lists and an explicit comparison** (a mechanical step; commit it alone before the football changes). `Score` keeps every flat field it has and gains two lists:

```go
	Cards []Card `json:"cards"` // nil until the first card; never set to an empty slice
	Subs  []Sub  `json:"subs"`  // nil until the first substitution

// sameScore reports whether two scores are the same; == can't compare Score
// because of its lists.
func sameScore(a, b Score) bool { return reflect.DeepEqual(a, b) }
```

`reflect.DeepEqual` tells a nil slice from an empty one, so the lists stay nil until the first card or substitution: never initialise them to `[]Card{}` (not in `newScore`, `new_game`, `Restore` or `Seed`), and decode them as they come. `slices.Clone` keeps nil as nil.

Then:

- Replace `if *s == prev` (`manual.go:329`) with `if sameScore(*s, prev)`.
- Every `==` / `!=` on `Score` in a test becomes `sameScore` (or `!sameScore`), one site at a time: `manual_test.go:87` (`s != newScore(DefaultLength)`), `manual_test.go:267` (`got != want`), `manual_test.go:449` (`s != want`), `source_test.go:113` (`s != newScore(DefaultLength)`), `cmd/hallstream/sets_test.go:100` (`got != want`; that test is in package `main` and can't see the unexported helper, so use `reflect.DeepEqual(got, want)` there). The `Score{…}` literals stay as they are. These, plus `club_test.go`, are the only edits to existing tests this plan allows; name each in the PR.
- **Undo snapshots copy the lists.** `prev, clock := m.s, false` in `Do`, and every place that stores a `Score` for later (`undone.s`, `m.sent`), holds its own lists: `prev.Cards, prev.Subs = slices.Clone(m.s.Cards), slices.Clone(m.s.Subs)`. `Score()` returns a copy the same way. An append to the live score must never write into a snapshot's backing array.
- New test for this sub-step: a score with no cards and no subs is `sameScore` with `newScore(DefaultLength)`. (The card-and-undo test comes after the `card` action exists, below.)

**Verify** (before the football changes): `go vet ./... && go test -count=1 ./...` → pass with only the mechanical edits above.

```go
// Card is one card shown.
type Card struct {
	Team   string `json:"team"`   // "home" or "away"
	Number string `json:"number"` // shirt number
	Kind   string `json:"kind"`   // "yellow", "second_yellow" or "red"
	Clock  int    `json:"clock"`  // the game clock when it was entered, tenths
}

// Sub is one substitution.
type Sub struct {
	Team  string `json:"team"`
	Out   string `json:"out"` // shirt numbers
	In    string `json:"in"`
	Clock int    `json:"clock"`
}
```

Add to `Score`: `Stoppage int \`json:"stoppage"\`` (added minutes announced for this period, a single number, not a list) and `Over bool \`json:"over"\`` (the scorer ended the game). Cards and substitutions append to `s.Cards` / `s.Subs` on the live score; the snapshot taken before the action already holds its own copy.

Add `Kind`, `Out` and `In` (strings) to `Action` (`Number` exists from plan 006), and these actions, documented in the `Action` comment:

- `"card"` with `Team`, `Number` (1 to 3 digits) and `Kind` `"yellow"` or `"red"`. A yellow for a number that already has a yellow on that team is stored as `"second_yellow"`. A card for a number already sent off (`red` or `second_yellow`): `manual.ja_expulso` (pt "Esse jogador já foi expulso.", en "That player has already been sent off."). Bad kind or number: `manual.cartao` (pt "Escolha o cartão e o número.", en "Pick the card and the number.").
- `"sub"` with `Team`, `Out` and `In`: both required, different, and each a shirt number from 1 to 99; otherwise `manual.substituicao` (pt "Indique quem sai e quem entra.", en "Say who goes off and who comes on.").
- `"stoppage"` with `N` from 0 to 15: sets `Stoppage`; out of range: `manual.descontos_tempo` (pt "O tempo de compensação vai de 0 a 15 minutos.", en "Added time is 0 to 15 minutes.").
- `"game_end"`: only when `s.Period >= r.Periods`, else `manual.fim_cedo` (pt "O jogo só pode acabar na última parte.", en "The game can only end in the last half."); sets `Over = true` and stops the clock. `clock_start` after it sets `Over = false`. Undo takes it back like any action.
- `card`, `sub`, `stoppage` and `game_end` only when `r.Cards`; otherwise `manual.sem_cartoes` (pt "Esta modalidade não tem cartões.", en "This sport has no cards."), so basketball and volleyball can't send them.
- `period_next` past `r.Periods` when `r.Cards` (soccer has no extra time): `manual.sem_prolongamento` (pt "O futebol aqui não tem prolongamento.", en "Football here has no extra time.").
- `period_back` after `game_end` clears `Over`.
- `period_next` resets `Stoppage` to 0. `new_game` sets `Cards` and `Subs` to nil and clears `Over` and `Stoppage` (so `SyncGame`'s switch to soccer on a 0–0 period-1 score starts clean too).

`emit()`: only when `r.Cards` (basketball and the other sports' frames carry none of these keys), send `"over"` (`"1"` or `""`), `"stoppage"`, and `"cards"` / `"subs"` as JSON strings. A nil list marshals as `null` in Go, so write `"[]"` explicitly when a list is empty. Each entry carries a `minute` string in place of `clock`, with `E = r.End(s.Period, s.Length)`: up to and including the period's end (`clock <= E`), `min(clock/600 + 1, E/600)` ("23'" at 22:10; "45'" at exactly 45:00.0); past it, `fmt.Sprintf("%d+%d'", E/600, (clock-E+599)/600)` ("45+1'" from 45:00.1, "45+3'" at 47:12).

**Verify**: `go test ./internal/source/` → pass, with new tests (`Rules` returning `sport.Soccer`): two yellows to home 7 store `yellow` then `second_yellow`; a card after a red errors; a sub with `Out == In` errors; `stoppage` 4 then `period_next` gives 0; undo of a card removes it, and an append after an undo leaves every older snapshot's list unchanged (no shared backing array); a card-only change is not dropped by `sameScore`; `sameScore` is false when only `Cards` or only `Subs` differ; `game_end` in period 1 errors, in period 2 sets `Over` and stops the clock; `period_back` after it clears `Over`; `period_next` in period 2 errors with `manual.sem_prolongamento`; `card`, `stoppage` and `game_end` under `sport.Basketball` error with `manual.sem_cartoes`; a sub with `Out` "0" or "100" errors; `minute` is "23'" at 22:10, "45'" at 27000, "45+1'" at 27001 and "45+3'" at 47:12 in period 1; a basketball frame has no `cards` key; an empty list is sent as `"[]"`; after one card and an undo, `Score().Cards` is `nil` (not empty) and `sameScore` with the score before the card holds.

### Step 3: The clock past 45:00 and the end of the game

Plan 006's count-up with `StopAtEnd: false` already lets the clock run past `End` and never starts the hold. Add tests that the soccer clock passes 27000 and keeps running, and that `period_next` from 47:12 starts period 2 at `Start(2, s.Length)` (45:00, or 35:00 after `period_length` 35:00).

In `internal/state/state.go`:

- Add `Over bool \`json:"over"\`` to `Live`, `v["over"] == "1"` in `ApplyConsole`.
- In `setLive(l Live)`, for a points sport: when `l.Over` turns true and `m.finalDone != p`, set `m.finalDone = p`, `m.auto("final")` and record period `p`, as `checkFinal` does. When `l.Over` turns false while `m.finalDone == p` (undo of `game_end`, or `clock_start` after it), clear `finalDone`, drop the period `p` recorded by the `Over`, and `m.auto("marcador")`, so finishing again records it once. Dropping the period is a change to `Periods`: `setLive` must report it (return true) so `recordedSaver` runs `SavePeriods` and `OnPeriods` fires, or a restart would bring the dropped period back from `periods.json`.
- `checkFinal`: return false when `r.FinalHold == 0`, so a hold can never end a soccer game.

`web/marcar`: verify (built in 006) that the next-period button appears at or past the period's end without stopping the clock; for soccer it is hidden in the last half (`period >= rules.periods && rules.cards`), where "Fim do jogo" takes its place. A "Fim do jogo" button in the last half, behind `#confirmar` (`index.html:124`), sends `game_end`. Keys `marcar.fim_jogo` ("Fim do jogo" / "Full time") and `marcar.confirmar.fim` ("Acabar o jogo?" / "End the game?").

**Verify**: `go test ./internal/source/ ./internal/state/` → pass: a frame with `over: "1"` in period 2 shows `final` and records period 2 once; a soccer clock stopped at 92:30 for 5 minutes without `over` does not show `final`; `over` back to `""` returns to `marcador`, the `SavePeriods` hook sees the list without period 2, and `over: "1"` again shows `final` with period 2 recorded once; a machine restored with `RestorePeriods` from what the hook saved after the drop has no period 2.

### Step 4: State, lower thirds and the game log

- `state.Live`: add `Stoppage int \`json:"stoppage"\``, `Cards []Card` and `Subs []Sub` (state-side types with the same JSON shape, `Minute string \`json:"minute"\`` in place of `clock`). `ApplyConsole` decodes `v["cards"]` and `v["subs"]`; a bad value leaves them empty and never fails the frame.
- Timeline, through the `seen` diff: add `cards int` and `subs int` to `seen` (`len(m.s.Live.Cards)`, `len(m.s.Live.Subs)` in `seenNow`). In `emit()`, fire `m.OnEvent("card", {"team","number","kind","minute"})` for each index from `old.cards` to `cur.cards`, and `"sub"` (`{"team","out","in","minute"}`) the same way. An undo shrinks the list: nothing fires.
- **Restart guard.** Extend plan 006's floor to `restored struct{ susp, cards, subs int; pending bool }` and `RestoreSeen(susp, cards, subs int)`; while `pending`, `seenNow()` takes the max of the live counts and the floor for all three, and the first applied frame (or `ResetPeriods`) clears it. In `main.go`, right after `restoreManual(st, manual)`, pass `len(manual.Score().Cards)` and `len(manual.Score().Subs)` too.
- `snapshot()` copies `Live` shallowly, so `Live.Cards`, `Live.Subs` and `Live.Suspensions` share their arrays with the machine. That is safe: `ApplyConsole` builds new slices on every frame and nothing appends to `Live`'s lists in place. Keep it that way.
- `state.LowerThird`: add `Card string \`json:"card"\`` and `OutNumber`, `OutName string` (`json:"out_number"`, `json:"out_name"`).
- `internal/web/marcar.go`, `marcarDo`: after a successful `card` or `sub`, show a lower third for 8 s with `s.State.ShowLowerThird`:
  - names: for `Team == "home"`, look the number up in `s.State.Snapshot().Game.Squad` (match `SquadPlayer.Number`; read `Name` and `PhotoURL`) and split the name as `server.go:676` does. Away: number only.
  - card: `LowerThird{Type: "cartao", Card: <kind of the last card in the reply's `Cards`>, Number, FirstName, LastName, PhotoURL}`.
  - substitution: `LowerThird{Type: "substituicao", Number: in, FirstName/LastName: in's name, OutNumber: out, OutName: out's name}`.
- `web/overlay/overlay.js`, `ltContent` / `ltFill` (lines 760-790): `cartao` draws a card swatch in the card's colour beside the number and name, labelled from `overlay.cartao.amarelo` ("CARTÃO AMARELO" / "YELLOW CARD"), `overlay.cartao.segundo` ("SEGUNDO AMARELO" / "SECOND YELLOW") or `overlay.cartao.vermelho` ("CARTÃO VERMELHO" / "RED CARD"); `substituicao` draws two lines (on, with an "in" mark; off, with an "out" mark), labelled `overlay.substituicao` ("SUBSTITUIÇÃO" / "SUBSTITUTION"). Every other type draws as today. `ltContent`'s `c.who` (the key that decides whether the lower third redraws) adds the card kind and, for a substitution, the off player's number and name, so a second card for the same player, or a new sub with the same player coming on, redraws.

**Verify**: `go test ./internal/state/ ./internal/web/ ./cmd/hallstream/` → pass: a frame with one card fills `Live.Cards`; bad JSON leaves it empty; a new card fires one `card` event; `RestoreSeen(0, 2, 1)`, then a `SetGame` call, then a frame with two cards and one sub, fires nothing; `POST /api/marcar` with a home card for a number in `game.squad` shows a `cartao` lower third with that name; an away card shows the number only.

### Step 5: Scorer page and scoreboard

Go through `build-ui` (extension branch, R8):

- `web/marcar`, when `rules.cards`: `+1` only and "Repor" to the period's start (45:00 in the second half, computed from `game.rules` and `score.length`) are **verify (built in 006)**; a cards row per team (yellow and red open a number picker: home lists `st.game.squad` numbers as large buttons plus "Outro número", away a number pad); a substitution button per team (pick off, then on); an added-time stepper (0 to 15) beside the clock sending `stoppage`; each team's cards listed small under its name; "Fim do jogo" in the last half.
- `web/overlay`: on the scoreboard, per team, a yellow count and a red count as small card swatches, each hidden at 0, shown the way volleyball swaps cells (an `html` attribute and an `only-…` class pair) so other sports' markup doesn't change; next to the clock, when `live.stoppage > 0`, `overlay.compensacao` ("{fim}+{n}'") with `fim = live.period_end / 600`. Cards stay off `intervalo` and `final`.
- New keys (pt, en): `marcar.cartao_amarelo` ("Amarelo" / "Yellow"), `marcar.cartao_vermelho` ("Vermelho" / "Red"), `marcar.substituicao` ("Substituição" / "Substitution"), `marcar.sai` ("Sai" / "Off"), `marcar.entra` ("Entra" / "On"), `marcar.outro_numero` ("Outro número" / "Other number"), `marcar.compensacao` ("Compensação" / "Added time"), `overlay.compensacao` ("{fim}+{n}'").

**Verify**: `go test -count=1 ./internal/web/` → pass; the screenshot comparison in "Guards" → all identical.

### Step 6: Fixtures and turning it on

- `web/overlay/fixtures/soccer/` (shape of `marcador.json`, `game.sport: "soccer"`, `game.rules` = Soccer as JSON, a squad of invented players): `marcador.json` (2.ª parte, 67:41, 2–1), `marcador-cartoes.json` (home `yellow` #4, `yellow` #9, `second_yellow` #9, `red` #2, so 3 yellows and 2 reds; away one `yellow`; stoppage 4 shown "90+4'"; clock 91:12; `period_end` 54000), `rodape-cartao.json` (a `cartao` lower third, second yellow, a home name), `rodape-substituicao.json`, `final.json`. Add `soccer/marcador-cartoes` and `soccer/rodape-substituicao` to `TestOverlayStyles`.
- `internal/club/club.go`: soccer `Active: true`; `club_test.go` asserts it. `web/painel/painel.js:2162`: enable soccer. `web/painel/mock.js:356`: accept `'soccer'`.
- `docs/CONTRACT.md`: soccer's rules, the new actions, `live.over`, `live.stoppage`, `live.cards`, `live.subs` (present on every state, empty or `false`/0 for other sports), the scorer frame keys sent only when `rules.cards`, the two lower-third types, the events, "football is scored by hand only". `docs/ROADMAP.md`: tick soccer.

**Verify**: `go vet ./... && go test -count=1 ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/sport`: soccer `Start`/`End`.
- `internal/source`: cards, second yellow, after a red, subs, added time, `game_end`, undo with cloned lists, `sameScore`, minutes.
- `internal/state`: past 45:00, no hold at `FinalHold` 0, `over` both ways, decoding, events, `RestoreSeen`.
- `internal/web`: the lower third from `marcarDo`, home and away.
- Overlay: five soccer fixtures; two in `TestOverlayStyles`.
- Basketball, futsal, handball: `TestBasketballScreensDrawTheSameEveryTime` and the screenshot comparison.

## Done criteria

- [ ] `go vet ./...`, `go test -count=1 ./...`, `GOOS=windows GOARCH=386 go build ./...` exit 0
- [ ] `ls web/overlay/fixtures/soccer/*.json` lists five files
- [ ] `grep -n '"soccer", "FUTEBOL", "FUTEBOL", true' internal/club/club.go` matches
- [ ] `grep -n "second_yellow\|substituicao" docs/CONTRACT.md` matches
- [ ] `grep -rn "Score() [!=]=\|== prev" internal/source cmd/hallstream` → no match
- [ ] The screenshot comparison against `02-feat/handball` is byte-identical
- [ ] Existing test edits limited to `club_test.go` (soccer active) and the `sameScore` comparisons named in Step 2 (`manual_test.go:87`, `:267`, `:449`, `source_test.go:113`, `cmd/hallstream/sets_test.go:100`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 006's count-up pieces (`Start`, `End`, `StopAtEnd`, `ClockTextUp`, the no-hold rule, `RestoreSeen`) are missing or shaped differently.
- The IFAB Laws differ from "Rules".
- The home squad does not reach `state.Game.Squad` (then the lower third can't name players: stop rather than read the store from the scorer).
- Any other sport's test fails, or a guard screenshot differs.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- A new list on `Score` stays nil until first used and goes into the `slices.Clone` lines that take undo snapshots; `sameScore` (`reflect.DeepEqual`) covers it without a change.
- If a Phase E provider brings the opponent's squad, away cards and subs can show names; the actions already carry numbers only.
- Reviewers: two yellows to one home player (shows "second yellow", counts as red), a substitution, 3 minutes added ("90+3'"), and "Fim do jogo" at 93:00.

## Plan review

Two reviews against `4093d0c` (revise, then approve-with-fixes). Reconciled at `0d7570c`:

- **Design change (coordinator decisions).** The draft extended plan 004's `same()`, which never shipped; `Score` is compared with `==`, which Go won't compile once `Score` holds a slice. `Cards` and `Subs` go straight on `Score` beside its flat fields, and `sameScore` is `reflect.DeepEqual`. The lists stay nil until first used so nil and empty never differ by accident, and undo snapshots use `slices.Clone`, which keeps nil as nil. A copy-on-write `*Sheet` and an embedded comparable `Board` were both considered and dropped: the first breaks undo silently on an in-place append, the second rewrites every `Score{…}` literal. Only the `==`/`!=` sites change, named in Step 2.
- `setLive` takes one argument; the draft assumed two.
- `checkFinal` gains the `FinalHold == 0` check; plan 006 already keeps the hold from starting for a count-up without `StopAtEnd`.
- Line references moved to `0d7570c` (LowerThird 185, ShowLowerThird 647, Snapshot 423, SquadPlayer 96, squad fill `game.go:238`, name split `server.go:676`, `ltContent` 760, confirm dialog 124).
- `club_test.go`, `mock.js` and the `TestOverlayStyles` list were missing from scope.
- The goldfish review of this reconciled version is recorded in the stack's PR.
