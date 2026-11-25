# Plan 001: An escalão can link to a network data source, and Nevobo is the first one

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a6ba610..HEAD -- internal/provider internal/store/store.go internal/store/store_test.go internal/web/provider.go internal/web/groups.go internal/web/server.go internal/web/sport_test.go internal/web/fixtures.go internal/web/next.go internal/web/calendar.go web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md docs/ROADMAP.md docs/research/team-data-sources.md`
> Empty output means the "Current state" below is exact. If it lists files,
> compare every "Current state" excerpt below with the live code. A changed
> excerpt you can still map one-to-one is fine; one you can't is a STOP
> condition.
>
> Phase C (sport profiles, plans 002 to 007) shipped before this plan was
> reconciled at `a6ba610`. Each escalão now has its own sport
> (`store.Group.Sport`), so a provider covers an escalão, not a club:
> `Caps.Covers` takes the club **and** the group, and so does `Linked`.

## Status

- **Priority**: P2
- **Effort**: L
- **Risk**: MED
- **Depends on**: plan 004 (volleyball), merged in #29 (61190d7). Volleyball is active in `internal/club/club.go:34` and in the panel's `SPORTS` list (`web/painel/painel.js:2162-2163`), and any escalão can be set to it (`PUT /api/groups/{id}` `{ "sport": "volleyball" }`). Nothing left to wait for.
- **Category**: direction
- **Planned at**: commit `a6ba610`, 2026-11-25 (first written at `4093d0c`; reconciled after Phase C merged)

## Why this matters

Hallstream's `Provider` interface (`internal/provider`) has three sources today: the Portuguese basketball federation (FPB), games the club typed or imported, and an escalão's calendar link (`.ics`). All of them are either Portuguese or entered by hand. ROADMAP Phase E item 3 says to "grow the interface as the first network provider needs it", and item 4 names Nevobo (Dutch volleyball). Nevobo is the best first consumer: it publishes an OpenAPI description, documents its rate limits, gives stable UUIDs, and needs no key to read teams and matches. After this plan, a Dutch club links each volleyball escalão to its Nevobo team from the panel, sees its games in Jogo and picks one, and the next-game card fills itself.

This plan is the first slice of Phase E, not all of it. Own-key sources, FPV, PlayHQ and Wikidata get their own plans; see `plans/README.md`. Moving the FPB onto links is deferred with no plan yet (README explains why), so ROADMAP item 3 stays partly done after this plan.

Decisions already made by the owner (2026-11-25), which this plan must honour:

- Sources are not asked for permission. A provider on public, documented endpoints ships switched on, paced politely, with a project User-Agent and a credit line in the panel. No `Enabled` flag.
- Sync state lives per link, not per escalão.
- Credits show in the panel only, never on the overlay.
- The FPB stays on its own `Group` fields for now (see Out of scope).
- The NBB database (Dutch basketball) is not built: it gives every team an iCalendar link, which the calendar-link import already reads.
- A sport per escalão (Phase C decision): a provider's coverage is checked against the escalão's sport, falling back to the club's.

## Current state

### Files and their roles

- `internal/provider/provider.go`: the `Provider` interface (lines 40-45), `Fixture`, `Side`, `FPBOffered(c store.Club, groups []store.Group)` (47-62), `FPBOfferedFor(c store.Club, g store.Group)` (64-68), the private `portuguese` (70), and the `Manual` provider (72-91). **There is no `For` function any more**; the server builds its provider list itself.
- `internal/provider/ics.go`: the calendar provider (`Calendar`, struct at line 181, `ID` at 187, `Fixtures` at 189), the model to follow.
- `internal/provider/ics_test.go`, `internal/provider/provider_test.go`: plain `testing` tests. `provider_test.go` holds `TestFPBOfferedSportPerEscalao` (line 9) and `TestManualFixtures` (46); it has **no** fake provider. Model new provider tests on `ics_test.go`.
- `internal/web/provider.go`: `fpbGames` (the FPB as a provider, lines 13-52), `Server.providers(g store.Group)` (54-62), `noFPB` (64), `Server.fpbOffered()` (66-70) and `Server.fpbOfferedFor(g)` (72-75). Both read `s.Store.Settings()`, which takes the store lock.
- `internal/web/sport_test.go:102`: `TestProvidersSportPerEscalao`, a table of club sport, country and escalão sport against the provider ids `s.providers(g)` returns ("fpb,manual,ics", "manual,ics"). `setSport` is at line 15; `setClub` is at `internal/web/fixtures_test.go:42`.
- `internal/web/next.go:94-123`: `findNext`, the only caller of `Fixtures` for the next game. It calls `s.providers(grp)` (line 106), maps `fpb.ErrNotFound` and `fpb.ErrBusy` to their own notes and every other error to `proximo.sem_ligacao` (107-116). **Do not change this file**; Nevobo errors fall into the last case, which is the right message.
- `internal/web/calendar.go:99-124`: `calendarFixtures`, `GET /api/groups/{id}/calendar`, which shapes calendar games as `store.Fixture` (id = the provider ref) for the panel to list and pick. The new linked-games endpoint copies it. **Do not change this file.**
- `internal/web/groups.go`: `listGroups` (lines 22-30), `addGroup` (32-47), `putGroup` (`PUT /api/groups/:id`, 49-113). Inside `putGroup`: the `ClubSearch` read at line 71 carries the comment `// not inside UpdateGroup: same lock`; the sport check (95-100) rejects an inactive sport with `api.escalao.desporto`; the FPB rule (101-104) rejects a non-basketball sport on an escalão with an FPB team (`api.escalao.desporto_fpb`); the refresh condition is at line 109. `nonNil` helper at line 132.
- `internal/web/server.go`: `Server` struct (lines 35-107; `Version string` at line 43, set from `cmd/hallstream/main.go:230`). Route registration: `s.manualRoutes(m)` at line 190, then the groups routes from line 245 (`m.HandleFunc("GET /api/groups", s.listGroups)`). `ServeHTTP` runs the Host, Origin and PIN checks before every route.
- `internal/web/fixtures.go:20-31`: `manualRoutes`, which registers `GET /api/groups/{id}/calendar` (line 25) among the fixture routes. A route group in its own file with one call from `server.go` is the pattern to copy.
- `internal/web/zone.go:10`: `Server.zone()` gives the club's `*time.Location`.
- `internal/web/profile.go:41`: the only server check on `club.country` is its shape (two letters A-Z). `NL` is accepted.
- `internal/store/store.go`: `Club` (111-124, `Sport` at 119, `Country` at 120); `Group` (282-306), saved in `dados/groups.json`, whose last field is now `Sport string \`json:"sport"\`` (305); `Fixture` (308-317); `const Shared = "geral"` (320); `Store.Dir` (351); `Open` fills an empty `Group.Sport` with the club's sport or `"basketball"` (399-404); `SportOf(groupID)` (408-417) gives the escalão's sport, else the club's, and treats the shared group as the club; `Store.Group` at 567; `Store.UpdateGroup` at 611. `internal/store/store_test.go` holds the store tests; `TestGroupSportMigration` (352) is the model for a load test.
- `internal/fpb/fpb.go`: the FPB HTTP client. **Do not change this file.** What `fetch` must copy from it:
  - Sentinels are `i18n.Key` values (lines 28, 31): `var ErrBusy error = i18n.Key("fpb.ocupada")`.
  - Pacing (`turn`, 126-147): `apiBurst = 5` at once (line 39), then one every `Every`; a request waits up to `maxWait = 10s` (line 40) for its turn, else `ErrBusy`. **`Every == 0` skips `turn` entirely**, including the 429 hold.
  - After a 429 (`fetch`, 161-179; `retryAfter`, 151-159): nothing is sent for `Retry-After` (seconds or an HTTP date), 1 minute without it, 5 minutes at most.
  - `Get` (241-284): a 200 that is valid JSON is cached (`<file>.tmp` then rename) and returned; **404 returns `ErrNotFound` and ignores the cache; every other failure (network error, 429, 5xx, invalid JSON) falls back to the cached copy**, and only errors when there is none.
  - HTTP client (82-91): 10 s timeout, redirects only to the same host, at most 5.
- `internal/i18n/i18n.go:167-169`: `type Key string` with `Error()` returning the translated string. `internal/i18n/i18n_test.go` (patterns at 174-181, `uses` at 184-222) finds used keys only when written as a literal: `i18n.T("…")`, `i18n.Err("…")`, `i18n.Key("…")` in Go, `t('…')` in the pages. **Any `i18n.T(someVariable)` or `t(someVariable)` fails the test** ("builds a key at run time"). So a key held in a struct field must be an `i18n.Key` created with a literal, shown with `.Error()`.
- `web/painel/painel.js`: `groupSport(g)` (line 51) is `g.sport || 'basketball'`. `COUNTRIES` (lines 2164-2165) is the only country list a club can pick from: PT, ES, FR, LU, CH, BR, AO, CV, MZ. **There is still no NL**, so no club can pick the Netherlands until Step 6 adds it. `clubCard` (2182) and the wizard's `#wCountry` select (2917) both use `COUNTRIES`. `loadFixtures` (505-517) fetches typed fixtures plus the calendar link's games (`cal: true`, read-only), keeps a calendar error in `S.calErr[gid]` (507-509), sorts, and renders. `fixturesCard` (518-555) lists them; the calendar-link block is at lines 548-553, the UI pattern to copy.
- `web/painel/mock.js`: the panel's mock server for `?mock=1`. The mock club is Portuguese basketball (line 29). Groups at lines 33-37 each carry a `sport`. `PUT /api/groups/:id` is handled at 351-364; `GET /api/groups/:id/(fixtures|calendar)` returns `[]` at 396.
- `web/i18n/pt.json`, `web/i18n/en.json`: every user-facing string. No key under `fetch.`, `api.fontes.`, `painel.fontes.`, `provider.` or `painel.pais.nl` exists yet.
- `docs/CONTRACT.md`: the behaviour contract. FPB pacing at line 208, `group.calendar_url` at 282, `game.next` at 287, what refreshes the next game at 291. Change it in the same commit as the behaviour.

### Excerpts

`internal/provider/provider.go:40-70`:

```go
// Provider lists an escalão's games. A provider that has nothing for the
// group (an FPB provider and a group with no FPB team) returns nil, nil.
type Provider interface {
	ID() string
	Fixtures(g store.Group) ([]Fixture, error)
}

// FPBOffered reports whether the FPB covers the club: Portuguese, and
// basketball as its main sport or in any escalão (geral aside).
func FPBOffered(c store.Club, groups []store.Group) bool {
	if !portuguese(c) {
		return false
	}
	if c.Sport == "" || c.Sport == "basketball" {
		return true
	}
	for _, g := range groups {
		if g.ID != store.Shared && (g.Sport == "" || g.Sport == "basketball") {
			return true
		}
	}
	return false
}

// FPBOfferedFor reports whether the FPB covers this escalão: a Portuguese
// club and a basketball escalão.
func FPBOfferedFor(c store.Club, g store.Group) bool {
	return portuguese(c) && (g.Sport == "" || g.Sport == "basketball")
}

func portuguese(c store.Club) bool { return c.Country == "" || c.Country == "PT" }
```

`internal/web/provider.go:54-75`:

```go
// providers are the sources of the club's games, FPB first where it covers the
// club, then the games typed or imported, then the escalão's calendar link.
func (s *Server) providers(g store.Group) []provider.Provider {
	out := []provider.Provider{}
	if s.FPB != nil && s.fpbOfferedFor(g) {
		out = append(out, fpbGames{s.FPB})
	}
	return append(out, provider.Manual{}, s.calendar())
}

var noFPB error = i18n.Key("api.fpb.nao_disponivel")

// fpbOffered reports whether the FPB covers this club (Portuguese, with a
// basketball team); every FPB route and job checks it.
func (s *Server) fpbOffered() bool {
	return provider.FPBOffered(s.Store.Settings().Club, s.Store.Groups())
}

// fpbOfferedFor reports whether the FPB covers this escalão: a basketball one.
func (s *Server) fpbOfferedFor(g store.Group) bool {
	return provider.FPBOfferedFor(s.Store.Settings().Club, g)
}
```

`internal/web/calendar.go:102-124` (the listing shape to copy):

```go
func (s *Server) calendarFixtures(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := s.calendar().Fixtures(g)
	if err != nil {
		fail(w, http.StatusBadGateway, i18n.T("api.calendario.sem_ligacao"))
		return
	}
	out := []store.Fixture{}
	for _, f := range list {
		if !f.Timed {
			continue
		}
		out = append(out, store.Fixture{
			ID: f.Ref, Start: f.Start.In(s.zone()).Format(time.RFC3339), Home: f.AtHome,
			Opponent: store.Opponent{Name: f.Opponent.Name}, Venue: f.Venue,
		})
	}
	reply(w, out)
}
```

`internal/web/groups.go:71-113` (the end of `putGroup`, abridged at the `...`):

```go
	search := s.Store.Settings().ClubSearch // not inside UpdateGroup: same lock
	g, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		...
		if req.Sport != nil {
			if sp, found := club.SportOf(*req.Sport); !found || !sp.Active {
				return userError(i18n.T("api.escalao.desporto"))
			}
			g.Sport = *req.Sport
		}
		// FPB teams play basketball.
		if (req.Sport != nil || req.FPBTeamID != nil) && g.FPBTeamID != 0 && g.Sport != "" && g.Sport != "basketball" {
			return userError(i18n.T("api.escalao.desporto_fpb"))
		}
		return nil
	})
	if s.save(w, err) {
		reply(w, g)
		if (req.FPBTeamID != nil || req.CalendarURL != nil || req.Sport != nil) && g.ID == s.Store.Game().GroupID {
			s.refreshNextInBackground()
		}
	}
}
```

`internal/store/store.go:282-306`, the `Group` struct: fields `ID`, `Name`, `Coach`, `FPBTeamID`, `FPBTeamName`, `Players`, `Sponsors`, `FPBTeamKey`, `NeedsTeam`, `LastSync`, `LastChanges`, `SyncedTeam`, `Fixtures`, `CalendarURL` (`json:"calendar_url"`), `Sport` (`json:"sport"`, the last field, commented "Open and AddGroup never leave it empty").

### What Nevobo offers (from `docs/research/team-data-sources.md`, "Netherlands: Nevobo (volleyball)", line 182)

- `api.nevobo.nl` serves an OpenAPI description at `/docs.jsonopenapi` with clubs (`/relatiebeheer/verenigingen`), teams (`/competitie/teams`) and matches (`/competitie/wedstrijden`).
- Reads of clubs, teams and matches need no key. Squads (`/competitie/teamleden`) need authentication: **out of scope**.
- "Alleen een UUID verwijst gegarandeerd steeds naar hetzelfde object." Store the UUID, never another id.
- On HTTP 429, wait at least `Retry-After`. Send a `User-Agent` with app name, version and a website: `Hallstream/<version> (+https://hallstream.com)`. Never the user's email or the club's name.
- The API is marked unstable, so every response used is recorded into `testdata/` and the parser is tested against those files.

### Conventions

- Code and comments in English; every user-facing string (log lines too) in `web/i18n/pt.json` first (European Portuguese; the program speaks in the first person, "Não consegui…", and instructs in the imperative, "Escolha…"), then the same key in `en.json` (British English). Keys sorted.
- `web/` ships embedded as is: no npm, no build step, nothing from a CDN.
- Never read or edit `dados/` or `dist/`.
- Never call anything that reads `s.Store.Settings()`, `s.Store.Groups()` or `s.providers(…)` from inside an `UpdateGroup` callback: they take the same lock and the program hangs. Read what you need before the call, as `putGroup` does with `search`.
- Commit style: conventional commits with a plain sentence, e.g. `feat: an age group's games from its calendar link (.ics) (#24)`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| One package | `go test ./internal/provider/...` | all pass |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo run | `go run ./cmd/hallstream -demo -nobrowser` | panel at `http://127.0.0.1:8765/painel` |

## Suggested executor toolkit

- `read-the-damn-docs` before writing the Nevobo client: fetch `https://api.nevobo.nl/docs.jsonopenapi` and read the operations for clubs, teams and matches, their query parameters and paging.

## Scope

**In scope** (the only files you should modify or create):

- `internal/provider/fetch/fetch.go`, `internal/provider/fetch/fetch_test.go` (create)
- `internal/provider/nevobo/nevobo.go`, `internal/provider/nevobo/nevobo_test.go`, `internal/provider/nevobo/testdata/*.json` (create)
- `internal/provider/provider.go`, `internal/provider/provider_test.go`, and in `internal/provider/ics.go` only the one `Caps()` method (Step 3)
- `internal/store/store.go` (the `Link` type and the `Group.Links` field only), `internal/store/store_test.go`
- `internal/web/provider.go`, `internal/web/groups.go`, `internal/web/server.go` (the Nevobo fields on `Server` and one `s.linkRoutes(m)` line only), `internal/web/sport_test.go` (one table row, Step 5), `internal/web/links.go` and `internal/web/links_test.go` (create)
- `web/painel/painel.js`, `web/painel/mock.js`, `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`, `docs/research/team-data-sources.md` (Step 7 lines only), `plans/README.md`

**Out of scope** (do NOT touch):

- `internal/fpb/` and every FPB field on `Group` (`FPBTeamID`, `FPBTeamName`, `FPBTeamKey`, `NeedsTeam`, `SyncedTeam`, `LastSync`, `LastChanges`). They drive the season rollover, roster import and daily sync (`internal/web/sync.go`). Moving the FPB onto `Links` is a later plan.
- `provider.FPBOffered`, `provider.FPBOfferedFor`, `Server.fpbOffered`, `Server.fpbOfferedFor`: leave them as they are.
- `internal/web/next.go`, `internal/web/sync.go`, `internal/web/calendar.go`, `internal/web/fixtures.go`, `cmd/hallstream/main.go`.
- `internal/sport/`, `internal/club/` and `Group.Sport` itself.
- Squads from Nevobo (need a key), API keys and any keyed provider, request headers on `fetch.Get`.
- Labels per language for team names (ROADMAP item 3's last bullet): deferred to the Wikidata plan, the first source that has them. Names stay as the source writes them.
- The overlay (`web/overlay/`). Credits never go on air.
- Crests and colours from Nevobo.

## Git workflow

- Branch: `feat/provider-links-nevobo`
- One commit per step below, conventional commits (`feat:`, `test:`, `docs:`).
- Do NOT push or open a PR unless the operator told you to.

## Steps

### Step 1: A shared, paced, cached HTTP getter for network providers

Create `internal/provider/fetch/fetch.go`:

```go
package fetch

// ErrBusy: the source answered 429, or a request would wait too long, and
// nothing is cached. ErrOffline: no usable answer and nothing cached.
// ErrNotFound: the source answered 404.
var (
	ErrBusy     error = i18n.Key("fetch.ocupada")
	ErrOffline  error = i18n.Key("fetch.sem_ligacao")
	ErrNotFound error = i18n.Key("fetch.sem_dados")
)

// Client fetches one source's answers: paced, cached on disk, polite on 429.
type Client struct {
	Host      string        // the only host requests and redirects may go to
	CacheDir  string        // created on first write
	UserAgent string
	Every     time.Duration // pause between requests once Burst have gone
	Burst     int
	MaxWait   time.Duration // a turn further off than this is ErrBusy (or the cache)

	HTTP  *http.Client          // nil = the default below
	Sleep func(time.Duration)   // nil = time.Sleep; tests set it
	Now   func() time.Time      // nil = time.Now; tests set it

	mu   sync.Mutex
	next time.Time
	hold time.Time // no request before this, after a 429
}

// Get fetches url. A 200 with valid JSON is cached and returned. A 404 is
// ErrNotFound and ignores the cache. Any other failure (network error, 429,
// a turn too far off, 5xx, invalid JSON) returns the cached copy with
// cached = true, or ErrBusy (429 / turn) or ErrOffline (the rest) when
// nothing is cached.
func (c *Client) Get(url string) (body []byte, cached bool, err error)
```

Rules:

- Pacing: `Burst` requests at once, then one every `Every`, waiting at most `MaxWait`. Unlike the FPB client, pacing and the 429 hold **always** apply; tests set `Sleep` to record waits instead of `Every = 0`. A `Burst` below 1 counts as 1; `Every` and `MaxWait` have no defaults (the caller always sets them).
- After a 429: no request until `Retry-After` (seconds, or an HTTP date measured against `c.Now`), 1 minute without the header, never more than 5 minutes. A call during the hold does not reach the server; it returns the cache or `ErrBusy`.
- A `url` whose host is not `Host` is refused with `ErrOffline` before anything is sent.
- Default `HTTP` (built on first use when nil): 10 s timeout, redirects only to `Host`, at most 5 (copy `internal/fpb/fpb.go:82-91`, returning `ErrOffline` from `CheckRedirect` instead of the FPB's keys).
- Cache file name: first 16 hex chars of SHA-256 of the URL, plus `.json`. Write `<file>.tmp`, then rename. Read at most 10 MiB of a body.
- Add `fetch.ocupada` (pt "A fonte de dados está a receber demasiados pedidos. Tento outra vez daqui a pouco.", en "The data source is getting too many requests. I'll try again shortly."), `fetch.sem_ligacao` (pt "Não consegui falar com a fonte de dados e não tenho cópia guardada.", en "I couldn't reach the data source and have no saved copy.") and `fetch.sem_dados` (pt "A fonte de dados já não conhece esta equipa.", en "The data source no longer knows this team.") to both catalogs.

**Verify**: `go test ./internal/provider/fetch/` → pass, with tests on an `httptest.Server` (set `Host` to its host): a 200 writes the cache; with the server closed, the cached copy comes back with `cached == true`; a 503 returns the cached copy; a 429 with `Retry-After: 30` returns the cache and the next call (with `Now` advanced 10 s) does not reach the server (count requests); with `Now` advanced 31 s it does; a 404 returns `ErrNotFound` even with a cached copy; no cache and server closed gives `errors.Is(err, fetch.ErrOffline)`; a redirect to another host fails.

### Step 2: Links on an escalão

In `internal/store/store.go`, after the `Fixture` type (line 317), add:

```go
// Link ties an escalão to a team at a network data source.
type Link struct {
	Provider string `json:"provider"`  // provider ID, e.g. "nevobo"
	TeamID   string `json:"team_id"`   // the source's stable id (Nevobo: the UUID)
	TeamName string `json:"team_name"` // as the source writes it, for "Ligado a …"
	LastSync string `json:"last_sync"` // RFC 3339 of the last fresh answer, "" = never
	Error    string `json:"error"`     // the last failure, in the user's language, "" = none
}
```

and add `Links []Link \`json:"links"\`` as the last field of `Group`, after `Sport`. No migration: a missing key loads as nil.

**Verify**: `go test ./internal/store/` → pass, with two new tests in `store_test.go` (model on `TestGroupSportMigration`, line 352): a `groups.json` with no `links` key loads with `Links == nil`; a group with one link survives `UpdateGroup` and a reopen unchanged.

### Step 3: Providers declare what they cover

In `internal/provider/provider.go`:

```go
// Caps is what a provider covers and needs.
type Caps struct {
	Sports    []string  // club.Sports ids; empty = any sport
	Countries []string  // store.Club.Country values; empty = any country
	Linked    bool      // the escalão picks a team at the source (store.Group.Links)
	Credit    i18n.Key  // the credit line the panel shows; "" = none. Show with .Error().
}

// Covers reports whether the provider is offered to this escalão of this
// club. The sport is the escalão's; for the shared group or an empty one it
// is the club's (as store.Store.SportOf picks it), and an empty club sport
// counts as "basketball" (as store.Open fills it). An empty country counts
// as "PT".
func (c Caps) Covers(club store.Club, g store.Group) bool

// Team is a team at a linked source, for the panel's picker.
type Team struct {
	ID   string `json:"id"`   // the source's stable id
	Name string `json:"name"`
	Club string `json:"club"`
}

// TeamSearcher is a linked provider whose teams can be searched by name.
type TeamSearcher interface {
	Provider
	Teams(query string) ([]Team, error)
}

// Linked returns the providers with Caps().Linked that cover this escalão, in order.
func Linked(all []Provider, club store.Club, g store.Group) []Provider
```

Add `Caps() Caps` to the `Provider` interface. Implement it on `Manual` and on `Calendar` in `ics.go` (both `return Caps{}`), and on `fpbGames` in `internal/web/provider.go` (`Caps{Sports: []string{"basketball"}, Countries: []string{"PT"}}`). There is no test fake to update. Leave `FPBOffered` and `FPBOfferedFor` as they are; `providers(g)` keeps using `fpbOfferedFor` for the FPB.

**Verify**: `go test ./internal/provider/` → pass, with a test table for `Covers` (empty Caps covers everything; `{Sports: volleyball, Countries: NL}` covers a Dutch club's volleyball escalão, not a Portuguese club's volleyball escalão, not a Dutch club's basketball escalão even when the club's main sport is volleyball; the shared group `store.Shared` takes the club's sport; an escalão with an empty sport takes the club's; an empty club sport counts as basketball; an empty country counts as PT) and a test that `Linked` keeps input order and drops non-linked and non-covering ones.

### Step 4: The Nevobo provider

Read `https://api.nevobo.nl/docs.jsonopenapi` first. Find: (a) an operation to find teams by club or team name; (b) the matches of one team, with date, time, home and away team, venue (sporthal) and competition; (c) how paging works and whether matches can be filtered by date; (d) which field is the UUID on teams and matches; (e) the time zone of match times.

Record one real answer for each operation you use into `internal/provider/nevobo/testdata/` (any Dutch club; team, club and match data only). Delete any person's name that appears (referees, contacts) from the recorded files.

Create `internal/provider/nevobo/nevobo.go`:

```go
// BaseURL is Nevobo's API root.
const BaseURL = "https://api.nevobo.nl"

type Source struct {
	Get func(url string) (body []byte, cached bool, err error) // a fetch.Client's Get
}

func (Source) ID() string { return "nevobo" }
func (Source) Caps() provider.Caps {
	return provider.Caps{Sports: []string{"volleyball"}, Countries: []string{"NL"}, Linked: true, Credit: i18n.Key("provider.nevobo.credito")}
}

// Fixtures lists the games of g's Nevobo link (at most one; see Step 5).
// No link: nil, nil without calling Get.
func (s Source) Fixtures(g store.Group) ([]provider.Fixture, error)

// FixturesFresh is Fixtures plus whether every answer came from the network.
func (s Source) FixturesFresh(g store.Group) (list []provider.Fixture, fresh bool, err error)

// Teams searches teams by club or team name.
func (s Source) Teams(query string) ([]provider.Team, error)
```

`Fixtures` calls `FixturesFresh` and drops `fresh`.

Search (`Teams`): if `/competitie/teams` takes a name filter, one request to it. Otherwise one request to `/relatiebeheer/verenigingen` filtered by name, then the teams of at most the first 3 matching clubs from `/competitie/teams` filtered by club. A search never sends more than 4 requests, so one search fits the burst of 5. `Team.Club` is the club's name as Nevobo writes it. If neither route can find a club's teams by name within 4 requests, that is the STOP condition below.

Mapping: `Ref` = `"nevobo/" + match UUID`; `Start`: a time with an offset is parsed with it; a time without one is read in `club.Zone("", "NL")` (`internal/club/club.go:264`, which gives `Europe/Amsterdam` and carries the time-zone data on Windows). Nevobo is a Dutch source; the panel shows it in the club's zone later. `Timed` false when there is no time; `AtHome` true when the linked team's UUID is the home team's; `Opponent.Name` and `Opponent.FullName` = the other team's name as written; `Venue` = sporthal name; `Competition` and `Round` where present. No crest, no colour. **No date filter in the provider**: `pickNext` in `next.go:149` already picks the next game, and a filter would make the recorded tests rot.

Paging: if the matches operation pages, follow pages until there are none or 10 pages have been read, whichever comes first. If it can filter by date, ask from 7 days ago to 120 days ahead (compute from `time.Now()` in the URL builder only; tests check the parser on testdata, not the URL dates). Write which one you used in a one-line comment.

Add `provider.nevobo.credito`: pt "Jogos da Nevobo (nevobo.nl)", en "Games from Nevobo (nevobo.nl)".

**Verify**: `go test ./internal/provider/nevobo/` → pass, with tests through a fake `Get` reading testdata: a home match and an away match map correctly; a match with no time is `Timed: false`; a 20:00 match on a winter date is 19:00 UTC; a group with no Nevobo link returns `nil, nil` and `Get` was never called; a cached answer gives `fresh == false`; `Teams` parses the search answer into `provider.Team`.

### Step 5: Wire it into the server

In `internal/web/server.go`, add three fields to `Server`: `Nevobo provider.TeamSearcher // tests set a fake; nil = nevobo() builds the real one` among the exported fields, and `nevoboReal provider.TeamSearcher` with `nevoboOnce sync.Once` among the private ones. Add a method in `internal/web/provider.go`:

```go
// nevobo returns the Nevobo source, built once with its own paced client.
func (s *Server) nevobo() provider.TeamSearcher
```

If `s.Nevobo != nil` it returns it unchanged (tests). Otherwise it builds once, through `nevoboOnce`, into `nevoboReal`, a `fetch.Client{Host: "api.nevobo.nl", CacheDir: filepath.Join(s.Store.Dir, "cache", "nevobo"), UserAgent: "Hallstream/" + s.Version + " (+https://hallstream.com)", Burst: 5, Every: 2 * time.Second, MaxWait: 10 * time.Second}` and a `nevobo.Source{Get: client.Get}`, wrapped as below. Building it sends nothing; only `Fixtures` and `Teams` reach the network.

**Per-link sync state.** In `internal/web/provider.go`, add a wrapper:

```go
// linkSync records each fetch on the group's link: LastSync on a fresh
// answer, Error on a failure. A cached answer changes neither (like the FPB,
// a disk copy is no update).
type linkSync struct {
	s   *Server
	src interface {
		provider.TeamSearcher
		FixturesFresh(store.Group) ([]provider.Fixture, bool, error)
	}
}
```

with `ID`, `Caps`, `Teams` passing through, and `Fixtures(g)` calling `FixturesFresh`. The matching link is the one in the stored group with the same `Provider` **and** `TeamID` as the link in `g` (the snapshot the fetch used), so a fetch that finishes after a relink changes nothing; when `g` has no link for this provider, `Fixtures` returns `nil, nil` and nothing is written. Compute the new values (`LastSync = time.Now().Format(time.RFC3339)` and `Error = ""` when fresh; `Error = err.Error()` on error, `LastSync` kept); call `s.Store.UpdateGroup(g.ID, …)` only when `Error` changes or the stored `LastSync` is more than 10 minutes old, so hourly refreshes and panel loads don't rewrite `groups.json` each time. Return the list and the error unchanged. `nevoboReal` is `linkSync{s, source}`; a fake in `s.Nevobo` is used as is, with no wrapper. To test `linkSync`, construct it directly with a fake `src`.

**The provider list.** Change `providers(g)` (`internal/web/provider.go:56-62`) to read the club once (`club := s.Store.Settings().Club`) and append `s.nevobo()` when `s.nevobo().Caps().Covers(club, g)`, after `provider.Manual{}` and before `s.calendar()`:

```go
	out = append(out, provider.Manual{})
	if n := s.nevobo(); n.Caps().Covers(club, g) {
		out = append(out, n)
	}
	return append(out, s.calendar())
```

Add one row to `TestProvidersSportPerEscalao` (`internal/web/sport_test.go:110-122`): `{"NL volleyball", "volleyball", "NL", "volleyball", false, "manual,nevobo,ics"}`. Set `s.Nevobo` to a fake in that test, or leave it nil (building the real source sends nothing). The existing rows must still pass unchanged.

**Routes.** Create `internal/web/links.go` with `func (s *Server) linkRoutes(m *http.ServeMux)`, called from `server.go` on the line after `s.manualRoutes(m)` (line 190), the same pattern as `manualRoutes` in `fixtures.go:20-31`. Coverage depends on the escalão's sport, so every route is under the escalão:

- `GET /api/groups/{id}/providers` → `[ { "id", "credit" } ]`: `provider.Linked(s.providers(g), club, g)`, `credit` = `Caps().Credit.Error()`. 404 (`store.ErrNotFound` text, as `calendarFixtures` does) for an unknown escalão.
- `GET /api/groups/{id}/providers/{pid}/teams?q=` → `[ { "id", "name", "club" } ]` from the matching provider's `Teams` (it must implement `provider.TeamSearcher`). 404 for an unknown escalão, or a provider not in `provider.Linked(s.providers(g), club, g)`; 400 (new key `api.fontes.pesquisa_curta`) when `q` has fewer than 3 characters after trimming; 502 (`api.fontes.sem_ligacao`) on a fetch error.
- `GET /api/groups/{id}/linked` → the timed games of every linked provider for the escalão, shaped exactly as `calendarFixtures` does (`store.Fixture` with `ID: f.Ref`). No covering provider or no links: `[]`. A provider that fails is skipped; if every provider with a link fails, 502 `api.fontes.sem_ligacao`.

**Saving links.** In `putGroup` (`internal/web/groups.go:49-113`), accept `Links *[]store.Link \`json:"links"\`` with entries `{ "provider", "team_id", "team_name" }`. When present it replaces the group's whole link list. Before `UpdateGroup`, next to `search` (line 71), read `home := s.Store.Settings().Club` and `sources := []provider.Provider{s.nevobo()}` (every linked provider the server knows, covered or not). Do not name the variable `club`: `groups.go` imports the package `club` (line 16) and calls `club.SportOf` inside the same callback (line 96), so a local `club` breaks the build. Trim `team_id` and `team_name` of each new link. Inside the callback, after the sport has been applied (after line 100), check **the group's final links** (the new list when `req.Links` is set, else the stored one) whenever `req.Links` or `req.Sport` is set: the escalão is not `store.Shared` (the shared group holds no links), each link's provider must be in `sources` with `Caps().Linked` and `Caps().Covers(home, *g)`, its `team_id` non-empty, and no provider may appear twice. Otherwise return `userError(i18n.T("api.fontes.ligacao_invalida"))`. So changing a linked volleyball escalão to another sport is refused until the link is removed, as the FPB rule at lines 101-104 does for FPB teams. Do not call `s.providers` or `s.Store.Settings()` inside the callback (see Conventions). Keep `last_sync` and `error` of a link whose provider and team are unchanged; reset them otherwise. A later change of the club's country (`profile.go`) leaves links in place; they stop being used because `Covers` fails, and they come back if the country does. The contract says so (Step 7).

After a links change on the game's escalão, refresh the next game: add `req.Links != nil` to the condition at `groups.go:109`. In `listGroups` (22-30) set `g.Links = nonNil(g.Links)` on each group before appending it, and do the same on the group `putGroup` (line 108) and `addGroup` (line 45) reply with, so the panel never sees `null`.

**Verify**: `go test ./internal/web/ -run 'Links|Providers|Linked'` → pass, with tests in `links_test.go` using `newServer` and `call` (`internal/web/server_test.go:74`, `107`), `setClub` (`fixtures_test.go:42`), `setSport` (`sport_test.go:15`) and `s.Nevobo` set to a fake `TeamSearcher`:

- `GET /api/groups/{id}/providers` is `[]` for a Portuguese basketball club's escalão, lists `nevobo` for a Dutch club's volleyball escalão, and is `[]` for a Dutch club's futsal escalão.
- A PUT with an unknown provider is 400; a PUT linking Nevobo on a futsal escalão is 400.
- A valid PUT stores the link and `GET /api/groups` returns it; a later PUT `{ "sport": "futsal" }` on that escalão is 400 and the link stays.
- `q=ab` is 400; teams search returns the fake's teams.
- `GET /api/groups/{id}/linked` returns the fake's timed games.
- One more test with a fake behind `linkSync`: a fresh answer sets `last_sync`, an error sets `error`.

### Step 6: The panel

In `web/painel/painel.js`:

- `loadFixtures` (lines 505-517): when `(g.links || []).length`, also fetch `GET /api/groups/<id>/linked` and add those games with `cal: true, linked: true`, read-only and pickable as the calendar's are. In the row's home/away cell (line 533), show `painel.fontes.da_fonte` instead of `painel.calendario.do_link` when `f.linked`, so a Nevobo game is not labelled as coming from the calendar link. When it fails, keep the error in `S.linkErr[gid]`, the same way the calendar keeps `S.calErr[gid]` (507-509), and show it in the Nevobo block below.
- `fixturesCard` (518-555): after the calendar-link block (548-553), add one `<details class="fx-add">` per provider from `GET /api/groups/<id>/providers`. Fetch it once per escalão per panel load into `S.providers[gid]`, and delete that entry after the escalão form's save succeeds (the `PUT` with `sport: $('#pGSport').value`, line 1701), since the sport may have changed. Inside: a search input (3 characters minimum, 400 ms debounce) calling `/api/groups/<id>/providers/<pid>/teams?q=`, a result list with "Ligar" on each, and once linked, "Ligado a {equipa}" with "Desligar", the link's last sync time (`painel.fontes.atualizado` with `quando: since(link.last_sync)`, the helper the squad card uses at line 1512) or its error, and the credit line in a `<p class="hint">`. "Ligar" and "Desligar" send `PUT /api/groups/<id>` with the full new `links` list; on success, put the reply in place of the group in `S.groups`, delete `S.fixtures[gid]` so the games reload, and render Jogo again. Reuse existing classes; no new CSS. Every `t()` call names its key as a literal; the credit arrives already translated from the server and goes through `esc()`.
- Add the Netherlands to `COUNTRIES` (line 2164, after `['LU', t('painel.pais.lu')]`): `['NL', t('painel.pais.nl')]`, with `painel.pais.nl` pt "Países Baixos", en "Netherlands". Both country selects (`clubCard` at 2182 and the wizard's `#wCountry` at 2917) use this list.

In `web/painel/mock.js`:

- Add a mock group `{ id: 'seniores-voleibol', name: 'Seniores', coach: '', fpb_team_id: null, fpb_team_name: '', sport: 'volleyball', links: [ { provider: 'nevobo', team_id: '<any UUID>', team_name: '<an invented Dutch club> DS 1', last_sync: '<an RFC 3339 time>', error: '' } ] }` to `groups` (33-37). Invented names only.
- `GET /api/groups/:id/providers`: `[{ id: 'nevobo', credit: 'Jogos da Nevobo (nevobo.nl)' }]` for a volleyball group, `[]` for the rest. The mock club stays Portuguese, so say in a one-line comment that the mock skips the country check.
- `GET /api/groups/:id/providers/nevobo/teams`: two invented teams. `GET /api/groups/:id/linked`: `[]` (extend the regex at line 396). `PUT /api/groups/:id` (351-364) stores `b.links` when present.

New strings, pt then en: `painel.fontes.pesquisar` ("Procure a equipa pelo nome do clube" / "Search for the team by club name"), `painel.fontes.ligar` ("Ligar" / "Link"), `painel.fontes.desligar` ("Desligar" / "Unlink"), `painel.fontes.ligado` ("Ligado a {equipa}" / "Linked to {equipa}"), `painel.fontes.atualizado` ("Atualizado {quando}" / "Updated {quando}"), `painel.fontes.da_fonte` ("da fonte de dados" / "from the data source"), `painel.fontes.sem_resultados` ("Não encontrei equipas com esse nome." / "I found no teams with that name."). Server keys: `api.fontes.pesquisa_curta` ("Escreva pelo menos 3 letras." / "Type at least 3 letters."), `api.fontes.sem_ligacao` ("Não consegui falar com a fonte de dados." / "I couldn't reach the data source."), `api.fontes.ligacao_invalida` ("Essa ligação não é possível para este escalão." / "That link isn't possible for this age group.").

**Verify**: `go test ./internal/i18n ./internal/web/` → pass; run `go run ./cmd/hallstream -demo -nobrowser`, open `http://127.0.0.1:8765/painel?mock=1`, pick the volleyball "Seniores" escalão in Jogo, and open its games card: the Nevobo block shows the mock link and credit. The basketball escalões show no Nevobo block. With Chrome installed, the panel tests pass.

### Step 7: Contract, roadmap and research doc

- `docs/CONTRACT.md`:
  - next to `group.calendar_url` (line 282): `links` on groups (shape, PUT rules including the sport-change refusal and no links on `geral`, per-link `last_sync` and `error`, a cached answer is no update, links kept but unused after a country change), `GET /api/groups/{id}/providers`, `GET /api/groups/{id}/providers/{pid}/teams`, `GET /api/groups/{id}/linked`.
  - `game.next` (line 287): Nevobo among the providers, for a linked volleyball escalão of a Dutch club.
  - line 291: add `links` to the fields whose change refreshes the next game.
  - Nevobo's pacing (5 at once, then one every 2 s, 10 s wait, the 429 rules, offline copy) next to the FPB's at line 208.
- `docs/ROADMAP.md` Phase E:
  - line 24 ("Each source is opt-in…"): keep, and add "Sources on public, documented endpoints are used without asking first; a source that objects is removed."
  - item 3 (line 91): "Partly done: links per escalão (the FPB keeps its own fields for now), capabilities checked per escalão, a shared paced cache, credits in the panel. Keys wait for the first keyed source; labels per language wait for Wikidata." If Step 4 found that the matches operation pages, add "paging in the Nevobo source" to the done list.
  - item 4 (line 99): "Nevobo done (teams and fixtures; squads need a key). The NBB database is not built: its teams' iCalendar links work with item 2." Remove "Ask both maintainers first."
- `docs/research/team-data-sources.md`:
  - line 189 ("Ask webmaster@nevobo.nl before shipping."): replace with "Built without asking, by the owner's decision of 2026-11-25; credit shown in the panel."
  - line 382: remove "Ask both maintainers first." and add "Nevobo is built (teams and fixtures)."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/provider/fetch/fetch_test.go`: the cases in Step 1.
- `internal/provider/provider_test.go`: `Covers` table (club and escalão), `Linked` order.
- `internal/provider/nevobo/nevobo_test.go`: the cases in Step 4, on recorded testdata.
- `internal/store/store_test.go`: the two cases in Step 2.
- `internal/web/sport_test.go`: the one new row in Step 5.
- `internal/web/links_test.go`: the cases in Step 5.
- Patterns: `internal/provider/ics_test.go` for provider tests, `internal/web/sport_test.go` and `internal/web/calendar_test.go` for handler tests.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, with the new tests above present and passing
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `git diff --stat a6ba610..HEAD -- internal/fpb internal/web/sync.go internal/web/next.go internal/web/calendar.go internal/web/fixtures.go internal/sport internal/club web/overlay cmd` shows no changes from this branch
- [ ] `grep -n -i "nevobo" docs/CONTRACT.md` finds the new section
- [ ] `grep -n "'NL'" web/painel/painel.js` finds the country entry
- [ ] `grep -rn "@" internal/provider/fetch internal/provider/nevobo/nevobo.go` finds no email address
- [ ] `grep -rn "i18n.T(" internal/provider internal/web/links.go | grep -v 'i18n.T("'` finds nothing (no keys built at run time)
- [ ] `plans/README.md` status row updated

## STOP conditions

- No escalão can be set to `volleyball` (`PUT /api/groups/{id}` `{ "sport": "volleyball" }` answers 400).
- `COUNTRIES` already has NL in a different shape (skip that bullet in Step 6 instead), or the server now validates countries against a list that lacks NL (STOP).
- Nevobo's team or match operations need a key or OAuth, or there is no way to find a club's teams by name.
- Matches or teams carry no UUID.
- Recorded testdata holds people's names you cannot remove.
- Any step needs a change in a file outside Scope (in particular `next.go`, `calendar.go`, `fixtures.go` or `cmd/hallstream/main.go`).
- A "Current state" excerpt no longer matches and you cannot map it one-to-one.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Two pacers now exist: `internal/fpb` and `internal/provider/fetch`. A follow-up plan should move the FPB client onto `fetch` (keeping its contract behaviour, including the `Every == 0` test shortcut it relies on) so they can't drift apart. Same for `Server.calendarFeed`'s cache.
- The FPB still lives on its own `Group` fields and its own coverage check (`FPBOfferedFor`). Moving it onto `Links` and `Caps` must carry the season rollover in `sync.go` with it.
- Keyed sources (BallDontLie, TheSportsDB, API-Sports) add a `NeedsKey` capability, request headers on `fetch.Get`, and a sealed key file reusing `youtube.Seal`/`Unseal` as `internal/web/youtube.go` does.
- A second linked source joins `sources` in `putGroup` and the `providers(g)` list; nothing else in the server should name Nevobo.
- Nevobo says its API is unstable. If a parser test passes but live data looks wrong, re-record testdata first.
- Reviewers: check the User-Agent, that no person's name is stored, that a 429 really holds requests back, that a 5xx still serves the saved copy, that no `UpdateGroup` callback reads settings, and that the FPB path is untouched.

## Plan review

Two cold reviews at `4093d0c`, both `revise`. Every finding was fixed in the plan except these:

- "Phase E is not fully delivered": contested. This plan is the first slice, and `plans/README.md` lists the later plans. Moving the FPB onto links is deferred on purpose: it drives the season rollover in `internal/web/sync.go`.
- "`fetch.Client` duplicates `fpb.Client`": accepted as a known cost. The FPB client hard-codes its base URL and i18n keys, and is out of scope. The follow-up is in the Maintenance notes.
- Request volume (10 pages plus search against a burst of 5): left to the STOP condition and the 10-page cap. Nevobo's real paging is unknown until Step 4 reads the spec.

Reconciled at `a6ba610`, after Phase C (plans 002 to 007) merged. Corrections:

- `provider.For` is gone. `Server.providers(g)` takes the escalão and builds the list inline; Nevobo now goes between `Manual` and the calendar there.
- `FPBOffered` takes the club and its groups, and `FPBOfferedFor(club, g)` is new. Neither changes.
- `Caps.Covers` and `provider.Linked` take the club and the escalão. The sport is the escalão's, falling back to the club's.
- The provider routes moved under the escalão (`/api/groups/{id}/providers…`), because coverage now depends on the escalão's sport. The panel fetches them per escalão.
- `putGroup` now changes an escalão's sport. Links are checked again on a sport change, a linked escalão can't leave volleyball until it is unlinked, and the check reads the club outside the store lock.
- `Group.Sport` is the last field, so `Links` goes after it. `provider_test.go` has no fake to update. `TestProvidersSportPerEscalao` gets a Nevobo row.
- The Netherlands is still missing from `COUNTRIES`; the server checks only the code's shape.
- Line numbers refreshed in every file. `internal/fpb/fpb.go` and `docs/ROADMAP.md` Phase E did not change. `next.go` changed only in its call to `providers(grp)`.
- The done-criteria diff now starts at `a6ba610`, because Phase C already touched `next.go`.
- Research doc line 382 also says "Ask both maintainers first"; Step 7 now covers it.

One cold review after the reconcile, `approve-with-fixes`. Fixed in the plan: the `club` variable that would hide the `club` package in `putGroup`, the Nevobo search capped at 4 requests, times read through `club.Zone`, a label for linked games apart from the calendar's, panel refresh after linking, and the Step 1 and `linkSync` edge cases. The index rows for 005 to 007 now say merged. Contested:

- "The Nevobo URLs, query parameters and JSON field paths are left to discovery": kept that way. The API says it is unstable (`docs/research/team-data-sources.md:186`) and Step 4 reads the live spec first. The plan pins the operations, the request cap and the fields it needs, and STOPs when they don't exist.
- "`fetch.Client` duplicates the FPB pacer": already accepted above as a known cost.
