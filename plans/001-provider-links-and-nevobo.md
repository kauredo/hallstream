# Plan 001: An escalão can link to a network data source, and Nevobo is the first one

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 4093d0c..HEAD -- internal/provider internal/store/store.go internal/store/store_test.go internal/web/provider.go internal/web/groups.go internal/web/server.go web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md docs/ROADMAP.md docs/research/team-data-sources.md`
> Phase C will add a sport per escalão (owner decision 2026-11-25). If `store.Group`
> has a sport field when you start, `Caps.Covers` must check the escalão's sport
> (falling back to the club's) instead of only `store.Club.Sport`; add the
> group to `Covers`' signature and to `Linked`, and test both cases.
> Phase C (sport profiles) is expected to have changed some of these files
> before this plan runs. Compare every "Current state" excerpt below with the
> live code. A changed excerpt you can still map one-to-one is fine; one you
> can't is a STOP condition.

## Status

- **Priority**: P2
- **Effort**: L
- **Risk**: MED
- **Depends on**: ROADMAP Phase C shipping volleyball (a club can pick `volleyball` as its sport and score a volleyball game). Today volleyball is listed but inactive in `internal/club/club.go:34` and disabled in `web/painel/painel.js:2152`. No plan file exists for Phase C yet.
- **Category**: direction
- **Planned at**: commit `4093d0c`, 2026-11-25

## Why this matters

Hallstream's `Provider` interface (`internal/provider`) has three sources today: the Portuguese basketball federation (FPB), games the club typed or imported, and an escalão's calendar link (`.ics`). All of them are either Portuguese or entered by hand. ROADMAP Phase E item 3 says to "grow the interface as the first network provider needs it", and item 4 names Nevobo (Dutch volleyball). Nevobo is the best first consumer: it publishes an OpenAPI description, documents its rate limits, gives stable UUIDs, and needs no key to read teams and matches. After this plan, a Dutch volleyball club links each escalão to its Nevobo team from the panel, sees its games in Jogo and picks one, and the next-game card fills itself.

This plan is the first slice of Phase E, not all of it. Own-key sources, FPV, PlayHQ and Wikidata get their own plans; see `plans/README.md`. Moving the FPB onto links is deferred with no plan yet (README explains why), so ROADMAP item 3 stays partly done after this plan.

Decisions already made by the owner (2026-11-25), which this plan must honour:

- Sources are not asked for permission. A provider on public, documented endpoints ships switched on, paced politely, with a project User-Agent and a credit line in the panel. No `Enabled` flag.
- Sync state lives per link, not per escalão.
- Credits show in the panel only, never on the overlay.
- The FPB stays on its own `Group` fields for now (see Out of scope).
- The NBB database (Dutch basketball) is not built: it gives every team an iCalendar link, which the calendar-link import already reads.

## Current state

### Files and their roles

- `internal/provider/provider.go`: the `Provider` interface, `Fixture`, `Side`, `FPBOffered`, `For`, and the `Manual` provider.
- `internal/provider/ics.go`: the calendar provider (`Calendar`, struct at line 181), the model to follow.
- `internal/provider/ics_test.go`, `internal/provider/provider_test.go`: plain `testing` tests. Model new provider tests on `ics_test.go`.
- `internal/web/provider.go`: `fpbGames` (the FPB as a provider) and `Server.providers()` (lines 54-62).
- `internal/web/next.go:106-117`: `findNext`, the only caller of `Fixtures` for the next game. It maps `fpb.ErrNotFound` and `fpb.ErrBusy` to their own notes and every other error to `proximo.sem_ligacao`. **Do not change this file**; Nevobo errors fall into the last case, which is the right message.
- `internal/web/calendar.go:99-129`: `calendarFixtures`, `GET /api/groups/{id}/calendar`, which shapes calendar games as `store.Fixture` (id = the provider ref) for the panel to list and pick. The new linked-games endpoint copies it. **Do not change this file.**
- `internal/web/groups.go`: `putGroup` (`PUT /api/groups/:id`, lines 48-101); `nonNil` helper at line 120.
- `internal/web/server.go`: `Server` struct (`Version string` at line 43, set from `cmd/hallstream/main.go:223`), route registration from line 244 (`m.HandleFunc("GET /api/groups", s.listGroups)`). `ServeHTTP` runs the Host, Origin and PIN checks before every route.
- `internal/web/fixtures.go:25`: registers `GET /api/groups/{id}/calendar`.
- `internal/web/zone.go:10`: `Server.zone()` gives the club's `*time.Location`.
- `internal/store/store.go:281`: `Group`, saved in `dados/groups.json`. `Store.UpdateGroup` at line 588. `internal/store/store_test.go` holds the store tests.
- `internal/fpb/fpb.go`: the FPB HTTP client. **Do not change this file.** What `fetch` must copy from it:
  - Sentinels are `i18n.Key` values (line 27, 31): `var ErrBusy error = i18n.Key("fpb.ocupada")`.
  - Pacing (`turn`, 126-147): `apiBurst = 5` at once, then one every `Every`; a request waits up to `maxWait = 10s` for its turn, else `ErrBusy`. **`Every == 0` skips `turn` entirely**, including the 429 hold.
  - After a 429 (`fetch`, 161-179; `retryAfter`, 149+): nothing is sent for `Retry-After` (seconds or an HTTP date), 1 minute without it, 5 minutes at most.
  - `Get` (241-283): a 200 that is valid JSON is cached (`<file>.tmp` then rename) and returned; **404 returns `ErrNotFound` and ignores the cache; every other failure (network error, 429, 5xx, invalid JSON) falls back to the cached copy**, and only errors when there is none.
  - HTTP client (82-91): 10 s timeout, redirects only to the same host, at most 5.
- `internal/i18n/i18n.go:165-169`: `type Key string` with `Error()` returning the translated string. `internal/i18n/i18n_test.go:179-197` finds used keys only when written as a literal: `i18n.T("…")`, `i18n.Err("…")`, `i18n.Key("…")`. **Any `i18n.T(someVariable)` fails the test** ("builds a key at run time"). So a key held in a struct field must be an `i18n.Key` created with a literal, shown with `.Error()`.
- `web/painel/painel.js`: `COUNTRIES` (lines 2153-2154) is the only country list a club can pick from: PT, ES, FR, LU, CH, BR, AO, CV, MZ. **There is no NL today**, so no club can be Dutch until Step 6 adds it. The server does not validate countries; `internal/club/club.go:255-259` already maps `NL` to `Europe/Amsterdam`. `loadFixtures` (lines 497-509) fetches typed fixtures plus the calendar link's games (`cal: true`, read-only), sorts them, and the card `fixturesCard` (from line 510) lists them; the calendar-link form is at lines 541-544, the UI pattern to copy.
- `web/i18n/pt.json`, `web/i18n/en.json`: every user-facing string.
- `docs/CONTRACT.md`: the behaviour contract (FPB pacing at line 178, `game.next` at 257). Change it in the same commit as the behaviour.

### Excerpts

`internal/provider/provider.go:40-58`:

```go
// Provider lists an escalão's games. A provider that has nothing for the
// group (an FPB provider and a group with no FPB team) returns nil, nil.
type Provider interface {
	ID() string
	Fixtures(g store.Group) ([]Fixture, error)
}

// FPBOffered reports whether the FPB covers the club: Portuguese basketball.
func FPBOffered(c store.Club) bool {
	return (c.Country == "" || c.Country == "PT") && (c.Sport == "" || c.Sport == "basketball")
}

// For lists the providers for the club, the club's own games last.
func For(c store.Club, fpb Provider) []Provider {
	if fpb != nil && FPBOffered(c) {
		return []Provider{fpb, Manual{}}
	}
	return []Provider{Manual{}}
}
```

`internal/web/provider.go:54-62`:

```go
// providers are the sources of the club's games, FPB first where it covers the
// club, then the games typed or imported, then the escalão's calendar link.
func (s *Server) providers() []provider.Provider {
	var f provider.Provider
	if s.FPB != nil {
		f = fpbGames{s.FPB}
	}
	return append(provider.For(s.Store.Settings().Club, f), s.calendar())
}
```

`internal/web/calendar.go:102-129` (the listing shape to copy):

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

`internal/store/store.go:281-303`, the `Group` struct: fields `ID`, `Name`, `Coach`, `FPBTeamID`, `FPBTeamName`, `Players`, `Sponsors`, `FPBTeamKey`, `NeedsTeam`, `LastSync`, `LastChanges`, `SyncedTeam`, `Fixtures`, `CalendarURL` (`json:"calendar_url"`, the last field).

### What Nevobo offers (from `docs/research/team-data-sources.md`, "Netherlands: Nevobo (volleyball)")

- `api.nevobo.nl` serves an OpenAPI description at `/docs.jsonopenapi` with clubs (`/relatiebeheer/verenigingen`), teams (`/competitie/teams`) and matches (`/competitie/wedstrijden`).
- Reads of clubs, teams and matches need no key. Squads (`/competitie/teamleden`) need authentication: **out of scope**.
- "Alleen een UUID verwijst gegarandeerd steeds naar hetzelfde object." Store the UUID, never another id.
- On HTTP 429, wait at least `Retry-After`. Send a `User-Agent` with app name, version and a website: `Hallstream/<version> (+https://hallstream.com)`. Never the user's email or the club's name.
- The API is marked unstable, so every response used is recorded into `testdata/` and the parser is tested against those files.

### Conventions

- Code and comments in English; every user-facing string (log lines too) in `web/i18n/pt.json` first (European Portuguese; the program speaks in the first person, "Não consegui…", and instructs in the imperative, "Escolha…"), then the same key in `en.json` (British English). Keys sorted.
- `web/` ships embedded as is: no npm, no build step, nothing from a CDN.
- Never read or edit `dados/` or `dist/`.
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
- `internal/web/provider.go`, `internal/web/groups.go`, `internal/web/server.go` (one `Server` field and route registration only), `internal/web/links.go` and `internal/web/links_test.go` (create)
- `web/painel/painel.js`, `web/painel/mock.js`, `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/CONTRACT.md`, `docs/ROADMAP.md`, `docs/research/team-data-sources.md` (Step 7 lines only), `plans/README.md`

**Out of scope** (do NOT touch):

- `internal/fpb/` and every FPB field on `Group` (`FPBTeamID`, `FPBTeamName`, `FPBTeamKey`, `NeedsTeam`, `SyncedTeam`, `LastSync`, `LastChanges`). They drive the season rollover, roster import and daily sync (`internal/web/sync.go`, 22 uses). Moving the FPB onto `Links` is a later plan.
- `internal/web/next.go`, `internal/web/sync.go`, `internal/web/calendar.go`, `cmd/hallstream/main.go`.
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

	HTTP  *http.Client          // nil = New's default
	Sleep func(time.Duration)   // nil = time.Sleep; tests set it
	Now   func() time.Time      // nil = time.Now; tests set it

	mu   sync.Mutex
	next time.Time
}

// Get fetches url. A 200 with valid JSON is cached and returned. A 404 is
// ErrNotFound and ignores the cache. Any other failure (network error, 429,
// a turn too far off, 5xx, invalid JSON) returns the cached copy with
// cached = true, or ErrBusy (429 / turn) or ErrOffline (the rest) when
// nothing is cached.
func (c *Client) Get(url string) (body []byte, cached bool, err error)
```

Rules:

- Pacing: `Burst` requests at once, then one every `Every`, waiting at most `MaxWait`. Unlike the FPB client, pacing and the 429 hold **always** apply; tests set `Sleep` to record waits instead of `Every = 0`.
- After a 429: no request until `Retry-After` (seconds or an HTTP date), 1 minute without the header, never more than 5 minutes. A call during the hold does not reach the server; it returns the cache or `ErrBusy`.
- Default `HTTP`: 10 s timeout, redirects only to `Host`, at most 5 (copy `internal/fpb/fpb.go:82-91`, returning `ErrOffline` from `CheckRedirect` instead of the FPB's keys).
- Cache file name: first 16 hex chars of SHA-256 of the URL, plus `.json`. Write `<file>.tmp`, then rename. Read at most 10 MiB of a body.
- Add `fetch.ocupada` (pt "A fonte de dados está a receber demasiados pedidos. Tento outra vez daqui a pouco.", en "The data source is getting too many requests. I'll try again shortly."), `fetch.sem_ligacao` (pt "Não consegui falar com a fonte de dados e não tenho cópia guardada.", en "I couldn't reach the data source and have no saved copy.") and `fetch.sem_dados` (pt "A fonte de dados já não conhece esta equipa.", en "The data source no longer knows this team.") to both catalogs.

**Verify**: `go test ./internal/provider/fetch/` → pass, with tests on an `httptest.Server` (set `Host` to its host): a 200 writes the cache; with the server closed, the cached copy comes back with `cached == true`; a 503 returns the cached copy; a 429 with `Retry-After: 30` returns the cache and the next call (with `Now` advanced 10 s) does not reach the server (count requests); with `Now` advanced 31 s it does; a 404 returns `ErrNotFound` even with a cached copy; no cache and server closed gives `errors.Is(err, fetch.ErrOffline)`; a redirect to another host fails.

### Step 2: Links on an escalão

In `internal/store/store.go`, after the `Fixture` type, add:

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

and add `Links []Link \`json:"links"\`` as the last field of `Group`. No migration: a missing key loads as nil.

**Verify**: `go test ./internal/store/` → pass, with two new tests in `store_test.go` (model on the existing ones there): a `groups.json` with no `links` key loads with `Links == nil`; a group with one link survives `UpdateGroup` and a reopen unchanged.

### Step 3: Providers declare what they cover

In `internal/provider/provider.go`:

```go
// Caps is what a provider covers and needs.
type Caps struct {
	Sports    []string  // store.Club.Sport values; empty = any sport
	Countries []string  // store.Club.Country values; empty = any country
	Linked    bool      // the escalão picks a team at the source (store.Group.Links)
	Credit    i18n.Key  // the credit line the panel shows; "" = none. Show with .Error().
}

// Covers reports whether the provider is offered to this club. An empty
// club sport counts as "basketball" and an empty country as "PT", as in FPBOffered.
func (c Caps) Covers(club store.Club) bool

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

// Linked returns the providers with Caps().Linked that cover the club, in order.
func Linked(all []Provider, club store.Club) []Provider
```

Add `Caps() Caps` to the `Provider` interface. Implement it on `Manual` and on `Calendar` in `ics.go` (both `return Caps{}`), on the test `fake` in `internal/provider/provider_test.go:10` (`return Caps{}`) and on `fpbGames` in `internal/web/provider.go` (`Caps{Sports: []string{"basketball"}, Countries: []string{"PT"}}`). Leave `FPBOffered` and `For` as they are.

**Verify**: `go test ./internal/provider/` → pass, with a test table for `Covers` (empty Caps covers everything; `{Sports: volleyball, Countries: NL}` covers a Dutch volleyball club, not a Portuguese one, not a Dutch basketball club; empty club sport counts as basketball) and a test that `Linked` keeps input order and drops non-covering ones.

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

Mapping: `Ref` = `"nevobo/" + match UUID`; `Start` parsed in `Europe/Amsterdam` (Nevobo is a Dutch source; the panel shows it in the club's zone later), `Timed` false when there is no time; `AtHome` true when the linked team's UUID is the home team's; `Opponent.Name` and `Opponent.FullName` = the other team's name as written; `Venue` = sporthal name; `Competition` and `Round` where present. No crest, no colour. **No date filter in the provider**: `pickNext` in `next.go` already picks the next game, and a filter would make the recorded tests rot.

Paging: if the matches operation pages, follow pages until there are none or 10 pages have been read, whichever comes first. If it can filter by date, ask from 7 days ago to 120 days ahead (compute from `time.Now()` in the URL builder only; tests check the parser on testdata, not the URL dates). Write which one you used in a one-line comment.

Add `provider.nevobo.credito`: pt "Jogos da Nevobo (nevobo.nl)", en "Games from Nevobo (nevobo.nl)".

**Verify**: `go test ./internal/provider/nevobo/` → pass, with tests through a fake `Get` reading testdata: a home match and an away match map correctly; a match with no time is `Timed: false`; a 20:00 match on a winter date is 19:00 UTC; a group with no Nevobo link returns `nil, nil` and `Get` was never called; a cached answer gives `fresh == false`; `Teams` parses the search answer into `provider.Team`.

### Step 5: Wire it into the server

In `internal/web/server.go`, add two fields to `Server`: `Nevobo provider.TeamSearcher // tests set a fake; nil = nevobo() builds the real one` and `nevoboReal provider.TeamSearcher` with `nevoboOnce sync.Once`. Add a method in `internal/web/provider.go`:

```go
// nevobo returns the Nevobo source, built once with its own paced client.
func (s *Server) nevobo() provider.TeamSearcher
```

If `s.Nevobo != nil` it returns it unchanged (tests). Otherwise it builds once, through `nevoboOnce`, into `nevoboReal`, a `fetch.Client{Host: "api.nevobo.nl", CacheDir: filepath.Join(s.Store.Dir, "cache", "nevobo"), UserAgent: "Hallstream/" + s.Version + " (+https://hallstream.com)", Burst: 5, Every: 2 * time.Second, MaxWait: 10 * time.Second}` and a `nevobo.Source{Get: client.Get}`, wrapped as below.

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

with `ID`, `Caps`, `Teams` passing through, and `Fixtures(g)` calling `FixturesFresh`. The matching link is the one in the stored group with the same `Provider` **and** `TeamID` as the link in `g` (the snapshot the fetch used), so a fetch that finishes after a relink changes nothing. Compute the new values (`LastSync = time.Now().Format(time.RFC3339)` and `Error = ""` when fresh; `Error = err.Error()` on error, `LastSync` kept); call `s.Store.UpdateGroup(g.ID, …)` only when `Error` changes or the stored `LastSync` is more than 10 minutes old, so hourly refreshes and panel loads don't rewrite `groups.json` each time. Return the list and the error unchanged. `nevoboReal` is `linkSync{s, source}`; a fake in `s.Nevobo` is used as is, with no wrapper. To test `linkSync`, construct it directly with a fake `src`.

`providers()` appends `s.nevobo()` when `s.nevobo().Caps().Covers(club)`, after the FPB/manual list and before the calendar.

Create `internal/web/links.go` with three handlers, registered in `server.go` next to the groups routes:

- `GET /api/providers` → `[ { "id", "credit" } ]`: `provider.Linked(s.providers(), club)`, `credit` = `Caps().Credit.Error()`.
- `GET /api/providers/{id}/teams?q=` → `[ { "id", "name", "club" } ]` from the matching provider's `Teams` (it must implement `provider.TeamSearcher`). 404 for an unknown or uncovered provider; 400 (new key `api.fontes.pesquisa_curta`) when `q` has fewer than 3 characters; 502 (`api.fontes.sem_ligacao`) on a fetch error.
- `GET /api/groups/{id}/linked` → the timed games of every linked provider for the group, shaped exactly as `calendarFixtures` does (`store.Fixture` with `ID: f.Ref`). No covering provider or no links: `[]`. A provider that fails is skipped; if every provider with a link fails, 502 `api.fontes.sem_ligacao`.

In `putGroup` (`internal/web/groups.go`), accept `"links": [ { "provider", "team_id", "team_name" } ]`. When present it replaces the group's whole link list. 400 (new key `api.fontes.ligacao_invalida`) for a provider not in `provider.Linked(s.providers(), club)`, an empty `team_id`, or two links to the same provider. Keep `last_sync` and `error` of a link whose provider and team are unchanged; reset them otherwise. After a links change on the game's escalão, call `s.refreshNextInBackground()` (extend the condition at `groups.go:97`). In `listGroups` (`groups.go:21-29`), set `g.Links = nonNil(g.Links)` on each group in the loop before replying, and do the same on the group `putGroup` replies with, so the panel never sees `null`.

**Verify**: `go test ./internal/web/ -run 'Links|Providers|Linked'` → pass, with tests in `links_test.go` using the test server helpers in `internal/web/server_test.go` and `s.Nevobo` set to a fake `TeamSearcher`: the providers list is empty for a Portuguese basketball club and lists `nevobo` for a Dutch volleyball club; a PUT with an unknown provider is 400; a valid PUT stores the link and `GET /api/groups` returns it; `q=ab` is 400; teams search returns the fake's teams; `GET /api/groups/{id}/linked` returns the fake's timed games. One more test with a fake behind `linkSync`: a fresh answer sets `last_sync`, an error sets `error`.

### Step 6: The panel

In `web/painel/painel.js`:

- `loadFixtures` (lines 497-509): when `g.links` is non-empty, also fetch `GET /api/groups/<id>/linked` and add those games with `cal: true`, exactly as the calendar's are (read-only, can be picked).
- `fixturesCard`: after the calendar-link block (lines 541-544), add one `<details class="fx-add">` per provider from `GET /api/providers` (fetch it once per panel load). Inside: a search input (3 characters minimum, 400 ms debounce) calling `/api/providers/<id>/teams?q=`, a result list with "Ligar" on each, and once linked, "Ligado a {equipa}" with "Desligar", the link's last sync time or its error, and the credit line in a `<p class="hint">`. "Ligar" and "Desligar" send `PUT /api/groups/<id>` with the full new `links` list. Reuse existing classes; no new CSS.
- `web/painel/mock.js`: add a mock group with a linked Nevobo team and a mock providers list.

- Add the Netherlands to `COUNTRIES` (after `['LU', …]`): `['NL', t('painel.pais.nl')]`, with `painel.pais.nl` pt "Países Baixos", en "Netherlands". Both country selects (club step of the wizard and Definições) use this list.
- When `/linked` fails, keep the error in `S.calErr[gid]` as the calendar does (`painel.js:500`) and show it the same way.

New strings, pt then en: `painel.fontes.pesquisar` ("Procure a equipa pelo nome do clube" / "Search for the team by club name"), `painel.fontes.ligar` ("Ligar" / "Link"), `painel.fontes.desligar` ("Desligar" / "Unlink"), `painel.fontes.ligado` ("Ligado a {equipa}" / "Linked to {equipa}"), `painel.fontes.atualizado` ("Atualizado {quando}" / "Updated {quando}"), `painel.fontes.sem_resultados` ("Não encontrei equipas com esse nome." / "I found no teams with that name."). Server keys: `api.fontes.pesquisa_curta` ("Escreva pelo menos 3 letras." / "Type at least 3 letters."), `api.fontes.sem_ligacao` ("Não consegui falar com a fonte de dados." / "I couldn't reach the data source."), `api.fontes.ligacao_invalida` ("Essa ligação não é possível para este clube." / "That link isn't possible for this club.").

**Verify**: `go test ./internal/i18n ./internal/web/` → pass; run `go run ./cmd/hallstream -demo -nobrowser`, open `http://127.0.0.1:8765/painel?mock=1` and the escalão's games card: the Nevobo block shows the mock link and credit. With Chrome installed, the panel tests pass.

### Step 7: Contract, roadmap and research doc

- `docs/CONTRACT.md`: document `links` on groups (shape, PUT rules, per-link `last_sync` and `error`, a cached answer is no update), `GET /api/providers`, `GET /api/providers/{id}/teams`, `GET /api/groups/{id}/linked`, Nevobo among the providers that feed `game.next` (line 257), and its pacing (5 at once, then one every 2 s, 10 s wait, the 429 rules, offline copy) next to the FPB's at line 178.
- `docs/ROADMAP.md` Phase E:
  - line 24 ("Each source is opt-in…"): keep, and add "Sources on public, documented endpoints are used without asking first; a source that objects is removed."
  - item 3: "Partly done: links per escalão (the FPB keeps its own fields for now), capabilities, a shared paced cache, paging in the Nevobo source, credits in the panel. Keys wait for the first keyed source; labels per language wait for Wikidata."
  - item 4: "Nevobo done (teams and fixtures; squads need a key). The NBB database is not built: its teams' iCalendar links work with item 2." Remove "Ask both maintainers first."
- `docs/research/team-data-sources.md` line 189 ("Ask webmaster@nevobo.nl before shipping."): replace with "Built without asking, by the owner's decision of 2026-11-25; credit shown in the panel."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/provider/fetch/fetch_test.go`: the cases in Step 1.
- `internal/provider/provider_test.go`: `Covers` table, `Linked` order.
- `internal/provider/nevobo/nevobo_test.go`: the cases in Step 4, on recorded testdata.
- `internal/store/store_test.go`: the two cases in Step 2.
- `internal/web/links_test.go`: the cases in Step 5.
- Patterns: `internal/provider/ics_test.go` for provider tests, `internal/web/server_test.go` for handler tests.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, with the new tests above present and passing
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `git diff --stat 4093d0c..HEAD -- internal/fpb internal/web/sync.go internal/web/next.go internal/web/calendar.go web/overlay cmd` shows no changes from this branch
- [ ] `grep -n "nevobo" docs/CONTRACT.md` finds the new section
- [ ] `grep -n "'NL'" web/painel/painel.js` finds the country entry
- [ ] `grep -rn "@" internal/provider/fetch internal/provider/nevobo/nevobo.go` finds no email address
- [ ] `grep -rn "i18n.T(" internal/provider internal/web/links.go | grep -v 'i18n.T("'` finds nothing (no keys built at run time)
- [ ] `plans/README.md` status row updated

## STOP conditions

- The club cannot pick `volleyball` as its sport yet (Phase C not shipped).
- Phase C already added NL to `COUNTRIES` in a different shape (skip that bullet in Step 6 instead) or the server now validates countries against a list that lacks NL (STOP).
- Nevobo's team or match operations need a key or OAuth, or there is no way to find a club's teams by name.
- Matches or teams carry no UUID.
- Recorded testdata holds people's names you cannot remove.
- Any step needs a change in a file outside Scope (in particular `next.go`, `calendar.go` or `cmd/hallstream/main.go`).
- A "Current state" excerpt no longer matches and you cannot map it one-to-one.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Two pacers now exist: `internal/fpb` and `internal/provider/fetch`. A follow-up plan should move the FPB client onto `fetch` (keeping its contract behaviour, including the `Every == 0` test shortcut it relies on) so they can't drift apart. Same for `Server.calendarFeed`'s cache.
- The FPB still lives on its own `Group` fields. Moving it onto `Links` must carry the season rollover in `sync.go` with it.
- Keyed sources (BallDontLie, TheSportsDB, API-Sports) add a `NeedsKey` capability, request headers on `fetch.Get`, and a sealed key file reusing `youtube.Seal`/`Unseal` as `internal/web/youtube.go:116-130` does.
- Nevobo says its API is unstable. If a parser test passes but live data looks wrong, re-record testdata first.
- Reviewers: check the User-Agent, that no person's name is stored, that a 429 really holds requests back, that a 5xx still serves the saved copy, and that the FPB path is untouched.

## Plan review

Two cold reviews, both `revise`. Every finding was fixed in the plan except these:

- "Phase E is not fully delivered": contested. This plan is the first slice, and `plans/README.md` lists the later plans. Moving the FPB onto links is deferred on purpose: it drives the season rollover in `internal/web/sync.go` (22 uses).
- "`fetch.Client` duplicates `fpb.Client`": accepted as a known cost. The FPB client hard-codes its base URL and i18n keys, and is out of scope. The follow-up is in the Maintenance notes.
- Request volume (10 pages plus search against a burst of 5): left to the STOP condition and the 10-page cap. Nevobo's real paging is unknown until Step 4 reads the spec.
