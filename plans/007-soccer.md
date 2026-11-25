# Plan 007: A club can score football: 2 × 45, stoppage time, and cards and substitutions per player

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/sport internal/source/manual.go internal/state/state.go internal/club/club.go internal/web/marcar.go internal/web/server.go web/overlay web/marcar web/painel/painel.js web/i18n docs/CONTRACT.md internal/web/overlay_test.go`
> Plans 002, 003, 004 and 006 are expected to have changed most of these
> files. Read `internal/sport/sport.go`, `internal/source/manual.go` and
> `internal/state/state.go` whole before starting and compare them with
> "What earlier plans leave behind" below. A shape you can't map one-to-one
> is a STOP condition.

## Status

- **Priority**: P3 (last sport in Phase C)
- **Effort**: L
- **Risk**: MED
- **Depends on**: plans/002-sport-package-basketball.md (`sport.Profile`), plans/003 (sport per escalão), plans/004 (volleyball: the `web/overlay/fixtures/<sport>/` folder and its Chrome-test glob), **and plans/006-handball.md**, which adds the count-up clock (`Profile.Start`, `End`, `StopAtEnd`, `ClockTextUp`, count-up screens in `setLive`). Do not start before 006 has landed.
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

ROADMAP Phase C lists soccer: "the clock counts up, 2×45 plus stoppage time, cards and substitutions. Scored by hand." Football is the biggest club sport in Portugal, and a club that streams its basketball can stream its football teams too, now that the sport is set per escalão. Football is scored by hand only (no Stramatel console decodes it).

Owner decisions this plan honours (2026-11-25, `plans/phase-c-decisions.md`):

- **Cards and substitutions are tracked per player**, tied to shirt numbers from the escalão's squad and entered on the scorer page. The overlay shows per-team card counts on the scoreboard and a lower third for each card and each substitution. Added time shows next to the clock (settled in review as "45+2'").
- The sport is set per escalão, defaulting to the club's (a club can have a basketball "Seniores" and a football "Seniores").
- New scoreboard variants go through `build-ui` inside the R8 design system.

## Rules (verify before building)

From the IFAB Laws of the Game (Law 7, duration; Law 3, substitutes; Law 12, cautions and sending-offs). Read the current edition at theifab.com (use `read-the-damn-docs`) and STOP if these differ:

- Two halves of 45:00; the clock counts up and runs on past 45:00 for added time. Broadcasts show the second half from 45:00 to 90:00, which this plan copies.
- Half-time after the first half.
- Added time is announced as whole minutes (+N) near the end of each half.
- A second yellow card to the same player is a red card (sending-off).
- Extra time and penalty shoot-outs are **out of scope**: youth and amateur games rarely have them. So the clock never needs more than 2 × 45 plus added time, and `MaxClock` (99:59, `internal/source/manual.go:31`) is enough.
- Youth and amateur games often play shorter halves (2 × 30, 2 × 35, 2 × 40): the scorer already lets the volunteer change the period length (`period_length` action).

## Current state

### What earlier plans leave behind (expected, verify)

- Plan 002: `internal/sport/sport.go` with `Profile` and `Get`; `state.Game.Rules` sends it as `game.rules`.
- Plan 005: `OvertimeLength`, `PeriodWord`, `FoulMarks` (0 hides the scorer's fouls row), `Score.Sport`, `Score.PeriodLength`.
- Plan 006: `Profile.StopAtEnd`, `Suspensions`, `Score.PeriodStart` ("Repor" goes there), `Machine.RestoreSeen`; methods `Start(period, length)` and `End(period, length)`, where `length` is the scorer's saved `Score.Length` (count-up: time played before the period, and the end of the period); `Score.Sport` (a new sport resets `s.Length` to the profile's, otherwise the volunteer's `period_length` stays); `state.ClockTextUp`; `Live.PeriodEnd` sent by the scorer as `"period_end"`; the scorer's `current()` counts up when `ClockDown` is false and stops at `End` only when `StopAtEnd`; `setLive` treats a rising clock as "started" for a count-up. Suspensions are fixed arrays in `Score`, compared in `same()`.
- Plan 004: replaced `if *s == prev` (`internal/source/manual.go:232` at 4093d0c) with `same(a, b Score) bool`, which compares every field. **Every field this plan adds to `Score` goes into `same()`.**
- Plan 004: fixtures under `web/overlay/fixtures/<sport>/`, loaded by `internal/web/overlay_test.go`.

### The squad and the lower third (at 4093d0c)

- `internal/state/state.go:76-80`: `type SquadPlayer struct { Number, Name, PhotoURL string }`; `State.Game.Squad` is the **home** team's players for the game, filled in `internal/web/game.go:218-224` from `store.Game.Squad` (`store.SquadEntry{PlayerID, Number}`, `internal/store/store.go:326-329`) and the escalão's players. The opponent's squad is not known anywhere.
- `internal/state/state.go:163-172`:
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
  `Machine.ShowLowerThird(lt, d)` at `state.go:607-614` shows it for `d`. The panel's lower-third route (`internal/web/server.go:650-671`) builds a `"jogador"` one from a player (number from `game.Squad`, first/last name split from `DisplayName`, photo) or a `"comentador"` one.
- `web/overlay/overlay.js:678-700`, `ltContent`/`ltFill`: two kinds only, `comentador` and everything else as `jogador`.
- `internal/web/marcar.go:55-95`, `marcarDo`: decodes a `source.Action`, refuses it with 409 unless the input is `manual`, calls `s.Manual.Do(a)`, replies with the score. This is where a card or substitution can also show its lower third.
- Game log: `cmd/hallstream/main.go:181` sets `m.OnEvent = tl.Add`. Events go out from `emit()` (`state.go:363-384`), which diffs a `seen` struct (`state.go:330-346`) against the last one and mutes the demo (`old.demo || cur.demo`); periods fire there at lines 380-383. Cards and substitutions go through the same diff, never straight from `setLive`.

### The scorer's `Score` and `==`

`internal/source/manual.go:232` held `if *s == prev { return nil }` at 4093d0c; plan 004 made it `same(*s, prev)`. Every undo entry keeps a copy of the whole `Score` (`undone.s`, lines 88-91). Cards and substitutions are open-ended lists, so Step 2 adds them to `same()` and copies the slices in undo snapshots, so an undo never shares a backing array with the live score.

### Conventions

- Go, standard library only; plain `testing`; model on `internal/source/manual_test.go`, `internal/state/state_test.go`, and `internal/web/server_test.go` (`newServer`, `call` helpers) for the handler.
- Every user-facing string in `web/i18n/pt.json` then `en.json`; `go test ./internal/i18n` checks it.
- Privacy: squad data stays shirt number, display name and an optional photo. Nothing new about players is stored beyond what the scorer enters for this game.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Commit style: `feat: football scoring with cards and substitutions`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass |
| Scorer, state, web | `go test ./internal/sport/ ./internal/source/ ./internal/state/ ./internal/web/` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | scorer at `http://127.0.0.1:8765/marcar` |

## Suggested executor toolkit

- `read-the-damn-docs` for the IFAB Laws of the Game.
- `build-ui`, extension branch, inside `docs/design/SYSTEM-R8.md`, for the card counts on the scoreboard, the "45+2'" next to the clock, the two new lower thirds and the scorer page's card and substitution sheets. Stop after its quality gate.

## Owner decisions settled in review (2026-11-25)

- The game ends with an explicit **"Fim do jogo"** on the scorer page (`game_end` action, with a confirm), which shows `final`. There is no 60 s hold for football: the clock runs on past 90:00 and nothing can tell the end of added time from a pause.
- Scoreboard counts: **yellows count every yellow shown**, the first of a pair included (`yellow` and `second_yellow` entries); **reds count `red` plus `second_yellow`** entries.
- Added time shows as **"45+2'"** (the period's end minute, plus the announced minutes).

## Scope

**In scope**:

- `internal/sport/sport.go`, `internal/sport/sport_test.go`
- `internal/source/manual.go`, `internal/source/manual_test.go` (new tests)
- `internal/state/state.go`, `internal/state/state_test.go` (new tests)
- `internal/web/marcar.go`, `internal/web/marcar_test.go` (create or extend)
- `cmd/hallstream/main.go` (the `RestoreSeen` call in `restoreManual` only)
- `internal/club/club.go` (soccer `Active: true`)
- `web/marcar/index.html`, `web/marcar/marcar.js`, `web/marcar/marcar.css`
- `web/overlay/index.html`, `web/overlay/overlay.js`, `web/overlay/overlay.css`
- `web/overlay/fixtures/soccer/*.json` (create)
- `web/painel/painel.js` (enable soccer)
- `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- The console path and `internal/ble`. Football is scored by hand only; the CONTRACT says so.
- The opponent's squad: away cards and substitutions take a typed shirt number, no name.
- Goal scorers, assists, possession, any statistic. Goals are `+1` on the score as in handball.
- Extra time and penalty shoot-outs (see Rules).
- Handball's behaviour: every handball test passes unchanged.

## Git workflow

- Branch: `feat/soccer`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The soccer profile

```go
var Soccer = Profile{ID: "soccer", Periods: 2, Half: 1, Length: 27000, ClockDown: false,
	StopAtEnd: false, FinalHold: 0, TimeoutsBy: [3]int{0, 0, 0}, PeriodWord: "half",
	FoulMarks: 0, Cards: true}
```

No fouls (`FoulMarks: 0` hides the fouls row, plan 005's rule), no timeouts, no suspensions, no shot clock, no extra time, and `FinalHold: 0`: the game ends on `game_end` (Step 3). Add `Cards bool \`json:"cards"\`` to `Profile` (true for soccer) so the pages show the card rows from the profile. Register in `Get`.

**Verify**: `go test ./internal/sport/` → pass, with tests: `Soccer.Start(1, 27000)` is 0, `Soccer.Start(2, 27000)` is 27000, `End(2, 27000)` is 54000, `Soccer.Start(2, 21000)` is 21000 (a 35-minute youth half); handball, futsal and basketball tests unchanged.

### Step 2: Cards, substitutions and stoppage in the scorer

In `internal/source/manual.go`, add to `Score`:

```go
	Stoppage int    `json:"stoppage"` // added minutes announced for this period, 0 = none
	Cards    []Card `json:"cards"`
	Subs     []Sub  `json:"subs"`

// Card is one card shown in the game.
type Card struct {
	Team   string `json:"team"`   // "home" or "away"
	Number string `json:"number"` // shirt number
	Kind   string `json:"kind"`   // "yellow", "second_yellow" or "red"
	Clock  int    `json:"clock"`  // the game clock when it was entered, tenths
}

// Sub is one substitution.
type Sub struct {
	Team string `json:"team"`
	Out  string `json:"out"` // shirt numbers
	In   string `json:"in"`
	Clock int   `json:"clock"`
}
```

Also add `Over bool \`json:"over"\`` (the scorer ended the game). Add `Stoppage`, `Cards`, `Subs` and `Over` to `same()` (compare the slices element by element, as plan 004 does for `Sets`). In every place that stores a `Score` copy (the undo entry, `Restore`, `m.sent`, `prev, clock := m.s, false`), copy the slices with `slices.Clone` where a later append could alias.

New actions (add `Kind string \`json:"kind"\``, `Out`, `In`, `Number` to `Action` if absent; document each in the `Action` comment):

- `"card"` with `Team`, `Number` (1 to 3 digits, required) and `Kind` `"yellow"` or `"red"`. A yellow for a number that already has a yellow (same team) is stored as `"second_yellow"`. A card for a number already sent off: `manual.ja_expulso` (pt "Esse jogador já foi expulso.", en "That player has already been sent off."). Bad kind or number: `manual.cartao` (pt "Escolha o cartão e o número.", en "Pick the card and the number.").
- `"sub"` with `Team`, `Out` and `In` (both required, different): appends a `Sub`. Error `manual.substituicao` (pt "Indique quem sai e quem entra.", en "Say who goes off and who comes on.").
- `"stoppage"` with `N` from 0 to 15: sets `Stoppage`. Out of range: `manual.descontos_tempo` (pt "O tempo de compensação vai de 0 a 15 minutos.", en "Added time is 0 to 15 minutes.").
- `"game_end"`: sets `Over = true` and stops the clock. `clock_start` after it sets `Over = false` (the game wasn't over), and undo takes `game_end` back like any action. Only in the last regulation period or later (`s.Period >= r.Periods`); otherwise `manual.fim_cedo` (pt "O jogo só pode acabar na última parte.", en "The game can only end in the last half."). Works for every count-up sport without `StopAtEnd`.
- `period_next` resets `Stoppage` to 0. `new_game` empties cards and subs and clears `Over`.
- Undo takes back any of these.

Only accept `card` and `sub` when `m.Rules().Cards` is true; otherwise `manual.sem_cartoes` (pt "Esta modalidade não tem cartões.", en "This sport has no cards.").

`emit()`: send `"over"` (`"1"` or `""`), `"stoppage"` as a number string, `"cards"` and `"subs"` as JSON strings (`"[]"` when empty), with `clock` turned into the minute as football writes it, a string: inside the period `clock/600 + 1` ("23'"); past the period's end `E = r.End(s.Period, s.Length)`, the half's end minute plus the minutes into added time, rounded up: `fmt.Sprintf("%d+%d'", E/600, (clock-E+599)/600)` ("45+3'" for a card at 47:12 in the first half).

**Verify**: `go test ./internal/source/` → pass, with new tests (`Rules` returning `sport.Soccer`): a yellow then another yellow to home 7 stores `yellow` then `second_yellow`; a card after a red errors; a sub with `Out == In` errors; `stoppage` 4 then `period_next` gives 0; undo of a card removes it and the earlier snapshot's slice is unchanged; `game_end` in period 1 errors, in period 2 sets `Over` and stops the clock; `same()` reports a change when only a card changed; handball and basketball tests unchanged.

### Step 3: The clock past 45:00 and the end of the game

Plan 006's count-up with `StopAtEnd: false` lets the clock run past `End`. Check, and add tests:

- The scorer clock in period 1 goes past 27000 (45:00) and keeps running up to `MaxClock`.
- `period_next` from period 1 at 47:12 starts period 2 at `Start(2, s.Length)`: 45:00 with the default length, 35:00 when the volunteer set `period_length` to 35:00 for a youth game.
- `web/marcar/marcar.js`: the next-period button is offered once the clock is at or past the period's end, without stopping it (soccer); for handball it stays as 006 left it.

In `internal/state/state.go`: add `Over bool \`json:"over"\`` to `Live`, read from `v["over"] == "1"` in `ApplyConsole`. In `setLive` (its signature after plan 004 is `setLive(l Live, finished []SetScore)`), when `l.Over` turns true and `m.finalDone != p`, set `m.finalDone = p`, `m.auto("final")` and record the period, as `checkFinal` does (`state.go:767-775`). When `l.Over` turns false again (undo of `game_end`, or `clock_start` after it) while `m.finalDone == p`, clear `finalDone` and go back to `marcador` with `m.auto("marcador")`, as plan 004 does for an undo across the match end; drop the recorded last period if it was recorded by the `Over` (so finishing again records it once). **The state machine skips the final hold when `r.FinalHold` is 0**: put that check where the hold starts (`setLive`) and in `checkFinal`, so soccer can only end on `game_end`. `web/marcar`: a "Fim do jogo" button in the last half, behind the existing confirm dialog (`#confirmar`, `index.html:110-117`), sending `game_end`.

**Verify**: `go test ./internal/source/ ./internal/state/` → pass, with tests: the clock runs past 45:00; a frame with `over: "1"` in period 2 shows `final` and records period 2 once; a soccer clock stopped at 92:30 for 5 minutes without `over` does not show `final`; `over` going back to `""` returns to `marcador`, and `over: "1"` again shows `final` a second time with period 2 recorded once; a card emitted at 47:12 in period 1 carries minute "45+3'"; the half switches to `intervalo` on `period_next`.

### Step 4: State, lower thirds and the game log

- `state.Live`: add `Stoppage int \`json:"stoppage"\``, `Cards []Card` and `Subs []Sub` (state-side types with the same JSON shape, `minute string` in place of `clock`, e.g. "23'" or "45+3'"). `ApplyConsole` decodes `v["cards"]` and `v["subs"]` with `json.Unmarshal`; a bad value leaves them empty and never fails the frame.
- Game log, through the `seen` diff: add `cards int` and `subs int` to `seen` (`len(m.s.Live.Cards)`, `len(m.s.Live.Subs)` in `seenNow`). In `emit()`, for each index from `old.cards` to `cur.cards` call `m.OnEvent("card", {"team","number","kind","minute"})`, and the same for `"sub"` (`{"team","out","in","minute"}`). An undo shrinks the list: nothing fires, and `seen` takes the smaller count. The demo is muted there already.
- **Restart guard.** Extend plan 006's `RestoreSeen` to `RestoreSeen(susp, cards, subs int)` and, in `restoreManual` (`cmd/hallstream/main.go`), pass `len(sc.Cards)` and `len(sc.Subs)` from the restored score, so a restart doesn't log the game's cards and substitutions again. Add `cmd/hallstream/main.go` (that call only) to the files you touch.
- `state.LowerThird`: add `Card string \`json:"card"\`` (`"yellow"`, `"second_yellow"`, `"red"`) and `OutNumber`, `OutName string` (`json:"out_number"`, `json:"out_name"`) for a substitution.
- `internal/web/marcar.go`, `marcarDo`: after a successful `card` or `sub` action, build and show a lower third for 8 s:
  - names: for `Team == "home"`, look the number up in `s.State.Snapshot().Game.Squad` (`state.Machine.Snapshot`, `state.go:386`, already used as `s.State.Snapshot().Game` in `internal/web/calendar_test.go:47`; match `SquadPlayer.Number`, read `.Name` and `.PhotoURL`), split first/last as `server.go:665` does. Away: number only, no name, no photo.
  - card: `LowerThird{Type: "cartao", Card: kind, Number, FirstName, LastName, PhotoURL}`; label from i18n in the overlay's uppercase style (`pt.json:459-464`): `overlay.cartao.amarelo` ("CARTÃO AMARELO"/"YELLOW CARD"), `overlay.cartao.segundo` ("SEGUNDO AMARELO"/"SECOND YELLOW"), `overlay.cartao.vermelho` ("CARTÃO VERMELHO"/"RED CARD"), and `overlay.substituicao` ("SUBSTITUIÇÃO"/"SUBSTITUTION"). The kind stored by the scorer decides it (read the last card from the reply's score).
  - substitution: `LowerThird{Type: "substituicao", Number: in, FirstName/LastName: in's name, OutNumber: out, OutName: out's name}`.
  - There is no on/off setting for lower thirds (`store.Overlay.LowerThird`, `internal/store/store.go:143-145`, is only the side it shows on), so every card and substitution shows one.
- `web/overlay/overlay.js`, `ltContent`/`ltFill` (lines 678-700): handle `cartao` (a card swatch in the card's colour next to the number and name, label = the card text) and `substituicao` (two lines: the player coming on with an "in" mark, the player going off with an "out" mark). Every other type behaves as today.

**Verify**: `go test ./internal/state/ ./internal/web/` → pass, with tests: a frame with one card fills `Live.Cards`; a bad JSON value leaves it empty; a new card fires one `card` event; `POST /api/marcar` with a home card for a number in `game.squad` shows a `cartao` lower third with that name; an away card shows number only.

### Step 5: Scorer page and scoreboard

Go through `build-ui` (extension branch, R8):

- `web/marcar`, when `rules.cards` (the fouls row already hides by `rules.foul_marks` 0, the timeouts row by `rules.timeouts` all 0):
  - points `+1` only (goals), with `-1` on the existing long press (`marcar.js:243-254`).
  - "Repor" goes to `score.period_start` (plan 006): 00:00 in the first half, 45:00 in the second.
  - a cards row per team: yellow and red buttons that open a number picker. Home: the squad's numbers from `st.game.squad` as large buttons, plus "outro número". Away: a number pad.
  - a substitution button per team: pick out, then in (same pickers).
  - next to the clock: an added-time stepper (0 to 15) sending `stoppage`, shown as "45+2'".
  - in the last half: "Fim do jogo" behind the confirm dialog, sending `game_end` (Step 3). Key `marcar.fim_jogo` ("Fim do jogo"/"Full time") and `marcar.confirmar.fim` ("Acabar o jogo?"/"End the game?").
  - each team's cards listed small under its name (number and colour), so the scorer sees who already has a yellow.
- `web/overlay`: on the scoreboard, per team, a yellow count (`yellow` + `second_yellow` entries) and a red count (`red` + `second_yellow` entries) as small card swatches, each hidden at 0; next to the clock, when `live.stoppage > 0`, "{fim}+{n}'" where `fim` is the minute the period ends (`live.period_end / 600`: 45 or 90 by default) and `n` the stoppage. Check every style, the reduced bug, and the `intervalo` and `final` screens (cards hidden there).
- New keys (pt, en): `marcar.cartao_amarelo` ("Amarelo"/"Yellow"), `marcar.cartao_vermelho` ("Vermelho"/"Red"), `marcar.substituicao` ("Substituição"/"Substitution"), `marcar.sai` ("Sai"/"Off"), `marcar.entra` ("Entra"/"On"), `marcar.outro_numero` ("Outro número"/"Other number"), `marcar.compensacao` ("Compensação"/"Added time"), `overlay.compensacao` ("{fim}+{n}'"/"{fim}+{n}'").

**Verify**: in the demo, with an escalão set to football and a squad picked for the game, `/marcar` shows the card, substitution and added-time controls; a yellow to a home player shows the lower third with the name; the scoreboard shows one yellow; "+3" shows next to the clock.

### Step 6: Fixtures and turning it on

- `web/overlay/fixtures/soccer/` (shape of `web/overlay/fixtures/marcador.json`, `game.sport: "soccer"`, `game.rules` = the Soccer profile JSON, a squad of invented players):
  - `marcador.json`: 2.ª parte, 67:41, 2–1, no cards.
  - `marcador-cartoes.json`: home cards `yellow` #4, `yellow` #9, `second_yellow` #9 and `red` #2 (the scoreboard shows 3 yellows and 2 reds), away one `yellow`, stoppage 4 (shows "90+4'"), clock 91:12, `period_end` 54000.
  - `rodape-cartao.json`: a `cartao` lower third (second yellow) with a home player's name.
  - `rodape-substituicao.json`: a `substituicao` lower third, home.
  - `final.json`: 90:00 + 3, two periods.
- `internal/club/club.go`: soccer `Active: true`. `web/painel/painel.js:2151`: enable soccer.
- `docs/CONTRACT.md`: the soccer rules, the new actions, `live.stoppage`, `live.cards`, `live.subs`, the two lower-third types, the timeline events, and "football is scored by hand only".
- `docs/ROADMAP.md` Phase C: tick soccer.

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0; with Chrome, the five soccer fixtures load in every style.

## Test plan

- `internal/sport`: soccer `Start`/`End`.
- `internal/source`: cards (second yellow, after red), subs, stoppage, undo with slices.
- `internal/state`: running past 45:00, the final hold on a stopped clock, decoding cards and subs, timeline events.
- `internal/web`: the lower third built in `marcarDo` for home and away.
- Overlay: five soccer fixtures in the Chrome test.
- All existing tests pass unchanged.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `ls web/overlay/fixtures/soccer/*.json` lists five files
- [ ] `grep -n '"soccer", "FUTEBOL", "FUTEBOL", true' internal/club/club.go` matches
- [ ] `grep -n "second_yellow\|substituicao" docs/CONTRACT.md` matches
- [ ] `grep -n "reflect.DeepEqual" internal/source/manual.go` → no match (`same()` compares the new fields)
- [ ] No existing test function edited
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 006 has not landed, or its count-up fields (`Start`, `End`, `StopAtEnd`, `ClockTextUp`) differ from what this plan assumes.
- Plan 004's fixtures folder, its Chrome-test glob, or the `Profile` shape differs in a way you can't map one-to-one.
- The IFAB Laws you read differ from "Rules" above.
- The home squad does not reach `state.Game.Squad` for the game (then the lower third can't name players: stop and report rather than reading the store from the scorer).
- An existing handball, futsal or basketball test fails.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Any new `Score` field goes into `same()`; one that holds a slice must also be cloned in undo snapshots.
- Opponent names: if a later provider (Phase E) brings the opponent's squad, away cards and subs can show names; the action shape already carries numbers only.
- Shorter youth halves use the existing `period_length` action; `Start` and `End` take `s.Length`, so a 35-minute half runs 0–35 and 35–70, and the length survives a new game until the escalão's sport changes.
- Reviewers: play a demo football game with two yellows to the same home player (it must show "second yellow" and count as red), a substitution, 3 minutes of added time ("90+3'"), and "Fim do jogo" at 93:00.

## Plan review

Two cold reviews (revise, then approve-with-fixes); the fixes are in. Settled in review: the game ends with `game_end` and `FinalHold: 0` (the state machine skips the hold at 0), undo of `game_end` or `clock_start` after it returns to `marcador`, extra time and penalties are out of scope, the fouls row hides by `FoulMarks: 0`, card counts include second yellows on both sides, added time shows as "45+2'" and a card in it as "45+3'", "Repor" goes to plan 006's `PeriodStart`, `RestoreSeen` covers cards and subs after a restart, the period length stays the scorer's `s.Length`, and `Score` changes go through plan 004's `same()`. Contested: none.
