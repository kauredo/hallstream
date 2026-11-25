# Plan 003: Each escalão has its own sport, defaulting to the club's

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/store/store.go internal/store/store_test.go internal/web/club.go internal/web/game.go internal/web/groups.go internal/web/profile.go internal/web/provider.go internal/web/server.go internal/web/sync.go internal/provider/provider.go cmd/hallstream/main.go web/painel/painel.js web/i18n docs/CONTRACT.md`
> Plan 002 will have changed `internal/web/game.go`, `internal/state/state.go`,
> `internal/source/manual.go` and `cmd/hallstream/main.go`. Compare each
> "Current state" excerpt with the live code; a mismatch you can't map
> one-to-one is a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED (a data migration on `groups.json`, and the FPB gate)
- **Depends on**: `plans/002-sport-package-basketball.md` (DONE). Uses `sport.Get(id) sport.Profile`, `Manual.Rules func() sport.Profile` and `state.Game.Rules` exactly as plan 002 defines them.
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

Many clubs run more than one sport: a Portuguese club can have a senior basketball team and a senior football team. Today the sport is one value for the whole club (`settings.club.sport`), so a volunteer who streams both would switch it in Definições before every game. The owner decided on 2026-11-25 (recorded in `plans/phase-c-decisions.md`) that each escalão carries its own sport:

- `settings.club.sport` stays as the club's main sport and the default for new escalões. Existing escalões take it on load.
- Each escalão can change its sport in the panel.
- Escalão names can repeat across sports. Where two escalões share a name, show "Seniores · Futebol".
- Never match escalões by name alone.
- The game's sport comes from its escalão. The FPB offer checks the escalão's sport wherever an escalão is in play.

After this plan, picking the game's escalão picks the rules: the scorer, the state machine and the overlay all follow that escalão's sport.

## Current state

### Files and roles

- `internal/store/store.go`
  - `Club` (lines 110-123): `Sport string \`json:"sport"\` // an id from club.Sports`.
  - `Group` (lines 281-303): no sport field today. The last field is `CalendarURL string \`json:"calendar_url"\``.
  - `Open` (lines 366-396) reads `settings.json`, `groups.json`, `game.json`, `promo.json` and runs `oldHome(b, &s.settings)` on `settings.json` only (lines 383-385). `oldHome` (lines 400-415) is the migration pattern: read old keys, set new fields, and the next save drops the old shape.
  - `AddGroup` (lines 564-585) creates `Group{ID: id, Name: name, Players: []Player{}, Sponsors: []Sponsor{}}` and writes `groups.json`.
  - `UpdateGroup` (line 588) applies a function to one group under the lock.
- `internal/store/store_test.go:288-` `TestOldSettingsMoveHomeNameToClub`: the migration test to model on (writes an old file into `t.TempDir()`, calls `Open`, checks the result and the saved file).
- `internal/club/club.go:25-45`: `Sport{ID, Name, Short, Active}`, the list `Sports` (basketball active; futsal, handball, volleyball, soccer inactive) and `SportOf(id) (Sport, bool)`.
- `internal/web/profile.go:38-40`, club validation:
  ```go
  if sp, found := club.SportOf(c.Sport); !found || !sp.Active {
  	return userError(i18n.T("api.clube.so_basquetebol"))
  }
  ```
  `api.clube.so_basquetebol` (pt.json line 26): "Por agora só há basquetebol. As outras modalidades chegam em breve."
- `internal/web/game.go`
  - lines 21-23: the game's escalão is `s.Store.Game().GroupID`, read with `s.Store.Group(g.GroupID)`.
  - lines 184-188 (before plan 002): `sport, _ := club.SportOf(cl.Sport)` then `Sport: sport.ID, SportLabel: sport.Short`. Plan 002 renames the variable so it doesn't shadow the `sport` package: `sp, _ := club.SportOf(cl.Sport)` with `Sport: sp.ID, SportLabel: sp.Short, Rules: sport.Get(sp.ID)`.
- `internal/web/club.go:243-252`, `setupTeam`, the name-only match:
  ```go
  for _, x := range groups {
  	if x.ID != store.Shared && x.FPBTeamID == 0 && store.Plain(x.Name) == store.Plain(t.name) {
  		g = x
  		break
  	}
  }
  ```
  An FPB team (always basketball) could be attached to a futsal escalão called "Seniores".
- The FPB gate:
  - `internal/provider/provider.go:47-51`: `FPBOffered(c store.Club)` is true for `Country` "" or "PT" and `Sport` "" or "basketball".
  - `internal/web/provider.go:68`: `func (s *Server) fpbOffered() bool { return provider.FPBOffered(s.Store.Settings().Club) }`.
  - Callers: `internal/web/server.go:313` (`fpbGate`, 404 on `/api/fpb/*`, `/api/club/setup`, and `/api/groups/{id}/players/import|players/from-fpb|link`), `internal/web/sync.go:64` (`SyncAll`), `internal/web/club.go:157` (`FirstRun`), and `provider.For` in `internal/web/provider.go:61` (the next game's providers).
- `web/painel/painel.js`
  - lines 2151-2152: `SPORTS`, `[id, label, active]`; line 2156: `fpbFor(c)` mirrors `FPBOffered`; line 2157: `fpbOffered()` calls `fpbFor(S.settings.club)`.
  - lines 46-50: `groupName(id)` returns `g.name`.
  - line 1468: the escalão `<select>` lists `esc(g.name)`.
  - lines 1489-1494: the escalão card (name, coach, "Guardar" sends `name` and `coach`).
  - line 436: the Jogo tab's escalão select prints `esc(x.name)`.
  - line 1480: `var fpb = fpbOffered();` decides whether the FPB card shows on the escalão page.
- `cmd/hallstream/main.go:191`: `manual := source.NewManual(&source.Manual{Save: …, Log: say})`; the store `st` exists there, the server is built at line 216. Plan 002 adds the `Rules` field with a basketball default.
- `docs/CONTRACT.md`: groups are documented at lines 171, 187, 207 (`PUT /api/groups/:id` accepts `name`, `coach`, `fpb_team_id`, `fpb_team_name`, `fpb_team`, `calendar_url`).

### Conventions

- Strings: `web/i18n/pt.json` first (European Portuguese; the program speaks in the first person, instructions in the imperative), then `en.json` with the same keys, keys sorted. Read with `i18n.T("literal.key")` in Go, `t('key')` in JS. A key built at run time fails `go test ./internal/i18n`.
- `web/` ships as is: no build step.
- Change `docs/CONTRACT.md` in the same commit as the behaviour.
- Commit style: conventional commits, e.g. `feat: each escalão has its own sport`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel at `http://127.0.0.1:8765/painel` |

## Scope

**In scope**:

- `internal/store/store.go`, `internal/store/store_test.go`
- `internal/provider/provider.go`, `internal/provider/provider_test.go`
- `internal/web/provider.go`, `internal/web/next.go` (the one `providers(grp)` call at line 106), `internal/web/server.go` (`fpbGate` only), `internal/web/sync.go` (`SyncAll`'s gate and its per-escalão loop condition only), `internal/web/club.go` (`FirstRun` gate and `setupTeam` match only), `internal/web/groups.go`, `internal/web/game.go`, `internal/web/profile.go`
- `internal/web/*_test.go` for new tests (new test functions; existing ones must pass unchanged)
- `internal/provider/provider_test.go`: the one existing test that calls `For` changes with its signature (Step 3)
- `cmd/hallstream/main.go` (the `Rules` field in the `NewManual` call at line 191 only)
- `web/painel/painel.js`, `web/painel/mock.js`, `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`

**Out of scope**:

- `internal/sport` (plan 002 owns it; no new profiles here).
- Making any other sport active in `club.Sports`. Plans 004 to 007 do that one sport at a time. Until then, only basketball can be picked anywhere.
- The overlay (`web/overlay/`): it already reads `game.sport_label` and, after plan 002, `game.rules`.
- FPB sync internals in `sync.go` beyond the gate and the loop condition (season rollover, roster import).
- The wizard's club step: it still sets the club's main sport.

## Git workflow

- Branch: `feat/sport-per-escalao`
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: `Group.Sport` and the migration

In `internal/store/store.go`:

1. Add to `Group`, after `CalendarURL`: `Sport string \`json:"sport"\` // an id from club.Sports; never "" after Open`.
2. In `Open`, in exactly one place, **after** the `geral` append (lines 392-394), fill the sport of every group that has none:
   ```go
   // Escalões saved before each had its own sport take the club's.
   for i := range s.groups {
   	if s.groups[i].Sport == "" {
   		s.groups[i].Sport = s.settings.Club.Sport
   	}
   }
   ```
   No helper is needed for an empty club sport: `DefaultSettings` already sets `Sport: "basketball"` (`store.go:233-234`) and `settings.json` is unmarshalled over it, so `s.settings.Club.Sport` is only "" if a file explicitly saved `"sport":""`. In that case the escalão stays "" and every check below treats "" as basketball. `geral` (`store.Shared`) gets the club's sport here too, but it is not a real escalão: every sport check in this plan skips it (Step 3).
   Do not write `groups.json` in `Open`; the next save writes the new field.
3. In `AddGroup` (line 578), set `Sport: s.settings.Club.Sport` on the new group.

No other store function is added.

**Verify**: `go test ./internal/store/` → pass, with new tests in `store_test.go` modelled on `TestOldSettingsMoveHomeNameToClub` (line 288):
- an old `groups.json` (`[{"id":"sub16","name":"Sub 16","players":[],"sponsors":[]}]`) with `settings.json` `{"club":{"name":"VALE","short":"VAL","sport":"basketball"}}` opens with `Sport == "basketball"`;
- a group saved with `"sport":"futsal"` keeps it;
- `AddGroup` gives the club's sport.
- Inversion: delete the migration loop, run the test, watch the first case fail, put the loop back. Say so in the commit message body.

### Step 2: Validate an escalão's sport

In `internal/web/groups.go`, `putGroup`'s request struct (lines 49-57) gets `Sport *string \`json:"sport"\``. Inside the existing `UpdateGroup` callback, in this order:

1. Apply the FPB link fields as today: the callback at `groups.go:70-94` clears `FPBTeamKey`/`NeedsTeam` on a new `fpb_team_id` (71-73), sets `FPBTeamKey` from `fpb_team` for a club team (75-77), then sets `name`, `calendar_url`, `coach`, `fpb_team_id` and `fpb_team_name` (78-92).
2. Apply `Sport` when present: it must be a sport from `club.SportOf` with `Active` true; otherwise return 400 with a new key `api.escalao.desporto` (pt "Escolha uma modalidade disponível para este escalão.", en "Pick a sport that's available for this age group.").
3. Then, only when the PUT carries `sport` or `fpb_team_id`, check: if `g.FPBTeamID != 0` and `g.Sport` is neither "" nor "basketball", return 400 `api.escalao.desporto_fpb` (pt "Este escalão está ligado a uma equipa da FPB, que é de basquetebol. Desligue-o primeiro.", en "This age group is linked to an FPB team, which plays basketball. Unlink it first."). Because the link is applied first, one PUT that unlinks (`fpb_team_id: 0`) and changes the sport succeeds, and one that links a team on a non-basketball escalão fails.

Return `userError(i18n.T("api.escalao.desporto"))` (and the same for `desporto_fpb`) from the callback, as `profile.go` does: `UpdateGroup` then leaves the group untouched, and `s.save` already answers a `userError` with 400 (`server.go:709-710`).

A sport change on the game's escalão also calls `s.refreshNextInBackground()` (extend the condition at `groups.go:97` with `req.Sport != nil`). Nothing else is needed for the overlay: `ServeHTTP` already calls `s.SyncGame()` after every non-GET `/api/` request (`server.go:300-303`), which rebuilds `game.sport` and `game.rules` from Step 4.

The club check at `profile.go:38-40` stays as it is.

**Verify**: `go test ./internal/web/ -run Sport` → pass, with new tests whose names contain `Sport` (e.g. `TestPutGroupSport`, `TestPutGroupSportFPBLink`): an unknown sport is 400; an inactive sport (futsal) is 400 `api.escalao.desporto`; basketball is stored; a PUT with a non-zero `fpb_team_id` on an escalão whose sport is futsal is 400 `api.escalao.desporto_fpb` (set the sport through `s.Store.UpdateGroup` directly in the test, since futsal can't be picked yet).

### Step 3: The FPB checks the escalão

**Rule.** The FPB covers a club when its country is PT (or "") **and** its main sport is basketball **or** any escalão other than `store.Shared` is basketball. It covers one escalão when the club's country is PT (or "") and that escalão's sport is basketball (or "").

In `internal/provider/provider.go`:

```go
// FPBOffered reports whether the FPB covers the club: Portuguese, and
// basketball as its main sport or in any escalão (geral aside).
func FPBOffered(c store.Club, groups []store.Group) bool

// FPBOfferedFor reports whether the FPB covers this escalão: a Portuguese
// club and a basketball escalão.
func FPBOfferedFor(c store.Club, g store.Group) bool

// For lists the providers for an escalão, the club's own games last; the
// caller passes whether the FPB covers it (FPBOfferedFor).
func For(fpb Provider, fpbOffered bool) []Provider
```

"" counts as basketball and "" as PT, as today. `For` no longer re-checks anything itself: `if fpb != nil && fpbOffered { return []Provider{fpb, Manual{}} }; return []Provider{Manual{}}`. Update `internal/provider/provider_test.go` (its `TestForOffersTheFPBOnlyToPortugueseBasketball` calls `For(club, fake{})`): this signature change is the one allowed edit to an existing test; keep its cases, moving the club check into `FPBOffered`/`FPBOfferedFor` calls.

In `internal/web`:

- `provider.go:68`: `fpbOffered()` becomes `provider.FPBOffered(s.Store.Settings().Club, s.Store.Groups())`. Add `func (s *Server) fpbFor(g store.Group) bool { return provider.FPBOfferedFor(s.Store.Settings().Club, g) }`.
- `providers()` (`provider.go:56-62`) becomes `providers(g store.Group)`; its last line (61, today `return append(provider.For(s.Store.Settings().Club, f), s.calendar())`) becomes exactly `return append(provider.For(f, s.fpbFor(g)), s.calendar())`. Its only caller is `next.go:106` (`for _, p := range s.providers() {`); change that line to `s.providers(grp)`. This one-line edit to `next.go` is allowed.
- `fpbGate` (`server.go:309-318`) runs in `ServeHTTP` before the mux (`server.go:296`), so `r.PathValue` is empty there. For the three escalão routes, take the id from the path with strings: `rest := strings.TrimPrefix(p, "/api/groups/"); id, _, _ := strings.Cut(rest, "/")`. Load it with `s.Store.Group(id)`. If the group is not found, return true and let the handler answer its own 404. If found, refuse (404 `noFPB`) when `!s.fpbFor(g)`. `/api/fpb/*` and `/api/club/setup` keep `s.fpbOffered()`.
- `SyncAll` (`sync.go:64`) keeps the club-level gate; in its per-escalão loop (line 86, `if g.FPBTeamID == 0 || g.NeedsTeam {`) also skip escalões where `!s.fpbFor(g)`.
- `FirstRun` (`club.go:157`) keeps `s.fpbOffered()`.
- `setupTeam` (`club.go:249`): the name match becomes `x.ID != store.Shared && x.FPBTeamID == 0 && s.fpbFor(x) && store.Plain(x.Name) == store.Plain(t.name)`. In the existing `UpdateGroup` call at lines 261-263, set `g.Sport = "basketball"` next to `linkTeam(g, t.TeamInfo)`, so an escalão made from an FPB team is basketball even when the club's main sport is not.

`TestFPBOnlyForPortugueseBasketball` (`internal/web/fixtures_test.go:51-75`) must pass **unchanged**. It sets the club to futsal/PT, then POSTs "Sénior", which now takes the club's sport (futsal); `geral` is excluded, so no basketball escalão exists and `fpbOffered()` is false as the test expects. This is why `geral` must be skipped.

**Verify**: `go test ./internal/provider/ ./internal/web/` → pass, with new tests whose names contain `Sport` (e.g. `TestFPBOfferedSportPerEscalao`, `TestFPBGateSportPerEscalao`, `TestSetupTeamSportMatch`): `FPBOffered` table (PT basketball club with no escalões = true; PT futsal club with only `geral` = false; PT futsal club with a basketball escalão = true; ES basketball = false); `FPBOfferedFor` table; `fpbGate` answers 404 for `/api/groups/<futsal-id>/players/import`, passes a basketball escalão, and lets `/api/groups/nonexistent/players/import` reach the handler's 404; `setupTeam` does not reuse a futsal escalão with the same name and the escalão it creates is basketball.

### Step 4: The game's sport comes from its escalão

In `internal/web/game.go`, add a free function (not a method, so `main.go` can call it before the server exists):

```go
// GameSport is the sport of the game's escalão, else the club's main sport.
func GameSport(st *store.Store) string
```

It reads `st.Game().GroupID`, loads the group with `st.Group(id)`, and returns its `Sport` when the group exists, isn't `store.Shared` and has a sport; otherwise `st.Settings().Club.Sport`.

After plan 002, lines 184-188 read `sp, _ := club.SportOf(cl.Sport)` and set `Sport: sp.ID, SportLabel: sp.Short, Rules: sport.Get(sp.ID)`. Change that line to `sp, _ := club.SportOf(GameSport(s.Store))`. It runs under `s.syncMu`, not the store's lock; each store method locks itself.

In `cmd/hallstream/main.go:191`, `st` already exists (the server is built later, at line 216). Add the rules to the existing `NewManual` call: `Rules: func() sport.Profile { return sport.Get(web.GameSport(st)) }`. `Manual.Do` calls `Rules` while holding `Manual.mu`, and `GameSport` takes the store's lock. That order (Manual's lock, then the store's) is the one `manual.save` already uses when it calls `st.SaveManual`, and no store method calls into `Manual`, so it can't deadlock.

**Verify**: `go test ./internal/web/` → pass, with a new test `TestGameSportFollowsEscalao`: create the escalão first (it takes basketball, the default club sport), pick it as the game's escalão, then `setClub(t, s, "futsal", "PT")`; the state has `game.sport == "basketball"` and `game.rules.id == "basketball"`. With no escalão picked, `GameSport` returns the club's sport.

### Step 5: The panel

In `web/painel/painel.js`:

- Club-level FPB: `fpbFor(c)` (line 2156) becomes `(c.country || 'PT') === 'PT' && ((c.sport || 'basketball') === 'basketball' || anyBasketballGroup())`, with `anyBasketballGroup()` true when some loaded group other than `geral` has `(g.sport || 'basketball') === 'basketball'`. `fpbOffered()` (line 2157) keeps calling it. The wizard (lines 2759, 2905), the escalão page's setup card (line 1480) and Definições (lines 2197-2198, 2598) stay on these club-level functions.
- Escalão-level FPB: add `fpbForGroup(g)` = `g && (S.settings.club.country || 'PT') === 'PT' && (g.sport || 'basketball') === 'basketball'`. Use it for the Jogo card's FPB button and hint (lines 447 and 450, with `g` the selected escalão), the games card hint (line 517), and the FPB card on the escalão page (the block starting at line 1496, `if (fpb) {`, becomes `if (fpb && fpbForGroup(g))`).
- Escalão card (lines 1489-1494): add a sport `<select id="pGSport">` built from `SPORTS`, inactive ones disabled with `painel.clube.em_breve` as at line 2187, labelled with the existing key `painel.clube.modalidade` ("Modalidade"). "Guardar" sends `sport` with `name` and `coach`.
- Labels: write `groupLabel(g)` returning `g.name`, or `g.name + ' · ' + sportLabel` when another loaded group has the same trimmed, case-insensitive name; `sportLabel` is the `SPORTS` label for `g.sport`. Use it in: `groupName(id)` (lines 46-50); the Jogo escalão select at line 436 (`esc(x.name)` → `esc(groupLabel(x))`); the escalão `<select>` at line 1468; and the delete prompt at line 1694 (`{ nome: g.name }` → `{ nome: groupLabel(g) }`). Out of scope, and left as `name`: the log lines in `internal/web/sync.go` (102, 222, 225, 227), the on-air `state.Group.Name` (`internal/web/game.go:217`) and `internal/web/feedback.go:41`.
- `web/painel/mock.js`: line 163 (`sport: db.settings.club.sport`) takes the sport from the mock game's escalão (`grp.sport || db.settings.club.sport`); the `POST /api/groups` handler (starting at line 336) returns `sport: db.settings.club.sport`; the mock `PUT /api/groups/:id` stores `sport`. Give the mock groups a `sport`, with two escalões named "Seniores" with explicit ids `seniores-basquetebol` (basketball) and `seniores-futsal` (futsal), display only.

No new panel string: the label reuses `painel.clube.modalidade`.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass; demo run, `http://127.0.0.1:8765/painel?mock=1`, the escalões show "Seniores · Basquetebol" and "Seniores · Futsal", the futsal one shows no FPB card, and the sport select shows futsal disabled.

### Step 6: Contract

`docs/CONTRACT.md`:

- Line 99 (`game.sport` is `settings.club.sport`…): `game.sport`, `game.sport_label` and `game.rules` come from the game's escalão, else the club's main sport.
- Line 170 (the FPB offer): the FPB is offered while `club.country` is `PT` and the club's main sport or any escalão (not `geral`) is basketball; the per-escalão FPB routes, the FPB card and "Escolher jogo da FPB" need a basketball escalão; the daily update skips other escalões; club setup only reuses basketball escalões by name.
- Line 171 and the group lines (187, 207): `sport` on every group (an id from `club.Sports`; filled from `settings.club.sport` on load and for new escalões); `PUT /api/groups/:id` accepts `sport` (active sports only, 400 `api.escalao.desporto`; 400 `api.escalao.desporto_fpb` when an FPB team stays linked to a non-basketball escalão, checked after the link change in the same request).

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0; `grep -n '"sport"' docs/CONTRACT.md` shows the group field.

## Test plan

- `internal/store/store_test.go`: the four migration and default cases in step 1, plus the inversion.
- `internal/web`: sport validation (step 2), FPB gate per escalão and `setupTeam` match (step 3), game sport from the escalão (step 4). Model on `internal/web/server_test.go`'s `newServer` and `call` helpers.
- `internal/provider/provider_test.go`: `FPBOffered` and `FPBOfferedFor` tables; the existing `For` test adapted to the new signature.
- `TestFPBOnlyForPortugueseBasketball` passes unchanged.
- Existing tests pass unchanged; if one needs editing, read STOP conditions.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0 with the new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n 'store.Plain(x.Name) == store.Plain(t.name)' internal/web/club.go` shows the match now also checks the sport
- [ ] `grep -n "cl.Sport" internal/web/game.go` finds no match
- [ ] `grep -n "fpbForGroup\|groupLabel" web/painel/painel.js` finds both helpers in use
- [ ] `git diff --stat 4093d0c..HEAD -- internal/web/fixtures_test.go` shows no change
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 002 is not merged, or `sport.Get`, `Manual.Rules` or `state.Game.Rules` differ from plan 002's text.
- An existing test fails and only editing it would make it pass (except a test that asserts the old club-level FPB gate; list it and stop).
- `providers()` has callers beyond `next.go:106` (plan 001 may add one; update it the same way, one line).
- `TestFPBOnlyForPortugueseBasketball` fails.
- The migration would need to write `groups.json` during `Open`.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Plans 004 to 007 make sports active one at a time; nothing here changes when they do, except the `api.clube.so_basquetebol` message, which plan 004 rewords.
- Plan 001 (Nevobo) checks coverage with `Caps.Covers`; when it runs after this plan it must check the escalão's sport (its drift note says so).
- The FPB stays basketball-only. A future plan moving the FPB onto provider links keeps the "basketball escalão" rule.
- The scorer reads `Rules` on every action, so changing the game's escalão sport mid-game changes the rules for the next tap (for example a period change). Harmless while only basketball is active; once plan 004 makes volleyball active, consider blocking a sport change on the game's escalão while a game is running.
- Reviewers: open an old `dados/` copy (a test fixture, not a real one) and check every escalão comes back basketball; check a futsal escalão never shows FPB buttons; check the game's sport follows the escalão.

## Plan review

Two cold reviews: the first said revise, the second approve-with-fixes. Every point from both is written into the steps above. Contested: none. Facts checked against the code:

- `DefaultSettings` sets `Club.Sport: "basketball"` (`internal/store/store.go:233-234`), so a helper for an empty club sport is not needed.
- `ServeHTTP` already runs `s.SyncGame()` after every non-GET `/api/` request (`internal/web/server.go:300-303`), so a sport change reaches the overlay with no extra call.
- `s.save` answers a `userError` with 400 (`internal/web/server.go:709-710`).
- `st` exists at `cmd/hallstream/main.go:191` and the server is built at line 216, so `GameSport` takes the store, not the server.
