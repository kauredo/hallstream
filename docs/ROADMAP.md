# Roadmap

Hallstream started as a game-day tool built for one Portuguese basketball club. The goal is a free tool that any club can use to stream its games, in any country, at any level from youth and amateur to professional, and in the club's own language:

- the score read from the hall's console, or kept by hand on a phone
- several cameras through OBS
- several sports
- setup that a volunteer can finish without help

The order of the phases below is a suggestion. Each phase ships on its own.

## Decisions

| Area | Decision |
|---|---|
| Licence | AGPL-3.0. |
| Platform | One Go exe with the panel and overlay embedded. Windows first, macOS and Linux builds after. |
| Audience | Clubs anywhere, at any level. Most are run by volunteers, often on an old PC in the hall, so the app has to stay simple for them; it must also scale up for professional and semi-professional clubs. |
| Mixer | OBS stays the mixer and encoder. Hallstream sets it up over obs-websocket. |
| Score input | A `Source` interface: the Stramatel Stream Box over Bluetooth (works today), manual scoring from a phone, and demo. Serial (RS-232) decoders come from people who own those consoles, each tested against a recorded session. No OCR. |
| Sports | A sport profile holds the clock direction, periods, scoring units, fouls and penalties, overtime and the automatic screen rules. Graphics come in two families, timed periods and sets. Targets: basketball, futsal, handball, volleyball and soccer. |
| Cameras | One OBS scene per camera, with the overlay in a shared nested scene, switched from the panel. Sources: DroidCam, any OBS video device, and VDO.Ninja (a phone browser, no app). Up to three cameras to start. |
| Streaming | YouTube through a verified OAuth client that ships with the app, or the club's own client. A plain RTMP key for any other platform. |
| Squads and fixtures | A `Provider` interface: squads and fixtures typed or imported from CSV for everyone, then sources that cover more clubs (Phase E). Each source is opt-in, paced, cached for offline use and credited as its terms ask. Crests come from the club's own files, never from a source that does not license them; colours can come from anywhere. Minors' data is kept to shirt number, name and an optional photo. `docs/research/team-data-sources.md` has the sources and their terms. |
| Club identity | Name, short name, colours, crest, sponsors and sport, all entered in the first-run wizard. Nothing club-specific in the code. |
| Language | One catalog per language, `web/i18n/<lang>.json`: a flat JSON object of dotted keys (`painel.nav.ecras`, `overlay.final.titulo`, `ble.desligado`) to strings, sorted by key, with `{name}` placeholders. `pt.json` (European Portuguese) is the source. A plural is two keys, `….um` and `….outros`, picked in code; there is no plural engine. `settings.lang` (default `pt`) picks the file, and a missing file or key reads `pt`. A test fails on a key missing from `pt.json`, a key nothing uses, an unsorted file, another language with different keys, or Portuguese left in the code. `en.json` (British English) ships, picked in Definições. |
| Privacy | Player photos are off by default. Examples and fixtures use invented people. Nothing is sent anywhere unless the club opts in. Public releases are built without `HALLSTREAM_UPLOAD_TOKEN`, so they never upload game logs; only builds the maintainer makes for their own club carry it. hallstream.com says so. |
| Releases | Signed GitHub releases (SignPath for Windows), plus a website and an in-app feedback form that opens GitHub issues. |

## Phase A: any club, manual scoring, onboarding

1. **Club profile.** Done.
   - Store the club in `settings.club` (name, short, colours, crest, sport, country).
   - Feed the colours to the overlay and panel as CSS custom properties.
2. **Translations.** Done: Portuguese and English, picked in Definições.
   - Move every user-facing string, in the panel, the overlay and the log, into `i18n/<lang>.json`.
   - Add a test that fails on a missing key.
3. **`Source` interface** (`internal/source`). Done: the console and `manual` sit behind it, switched by `settings.input`; the demo stays in `internal/state`.
   - Move `internal/ble` behind it.
   - Add `manual` next to `demo`.
   - Document the frame shape in `docs/CONTRACT.md`.
4. **Scorer page** (`/marcar`). Done: the first tap takes the scoring over from the console; the panel's Marcação tab picks the input and shows the QR code.
   - A page for a phone held sideways, reached over the LAN with the PIN.
   - It covers points per team, undo, the clock, the period, fouls and timeouts.
   - It can also run next to a console, as a fallback.
5. **`Provider` interface.** Done: the next game reads every provider (`internal/provider`): the FPB where it covers the club, and fixtures typed or imported from a CSV for every club; squads come from a CSV too, and the server refuses the FPB to clubs it does not cover. Which open sources could be the next providers is in `docs/research/team-data-sources.md`.
   - Move `internal/fpb` behind it.
   - Add `manual`: typed squads, CSV import (`number,name,photo`) and typed fixtures.
   - Only offer FPB for Portuguese basketball clubs.
6. **First-run wizard.** The steps are language, club, sport, input, OBS, streaming destination and a rehearsal. Every step can be skipped and resumed.
7. **YouTube and RTMP.**
   - Ship the project OAuth client, with "use your own" as a fallback.
   - The RTMP path writes the server and key into OBS.
8. **Releases.**
   - Signed Windows builds and notarised macOS builds.
   - An opt-in update check.
   - A feedback form that opens a GitHub issue.

## Phase B: multiple cameras

1. "Configurar OBS" builds one scene per camera (`Câmara 1` to `Câmara 3`), each nesting one shared `Grafismo` scene that holds the overlay.
2. The panel gets camera buttons and shows which camera is live.
3. At a timeout, a chosen camera shows picture-in-picture.
4. Onboarding adds cameras: DroidCam, a video device, or VDO.Ninja through a QR code on the panel.
5. Each camera gets a health line in the panel (no frames, low fps).
6. A guide covers two and three cameras on wifi and on cable, with the bandwidth each needs.

## Phase C: sport profiles

1. Add `internal/sport`. Basketball moves into it unchanged, and the existing tests prove it.
2. Sports and their rules:
   - Futsal: the clock counts down from 20:00, 5 accumulated fouls.
   - Handball: the clock counts up, 2×30, 2-minute suspensions.
   - Volleyball: sets to 25 (15 in the last), the serving side.
   - Soccer: the clock counts up, 2×45 plus stoppage time, cards and substitutions. Scored by hand.
3. The Stramatel decoder already reads the handball, volleyball and hockey codes (`internal/ble/decode.go`). A sport is only listed as supported after it has been tested on a recorded game.
4. Overlay fixtures go in `web/overlay/fixtures/<sport>/` for each sport, and the Chrome test loads them all.

## Phase D: more consoles

1. A serial `Source` for RS-232 consoles: Daktronics AllSport, Fair-Play and Bodet.
2. A capture mode that records the raw bytes, so a contributor can send a session.
3. A support table on the website lists each console and sport as tested, community or wanted.

## Phase E: more data providers

In the order the research (`docs/research/team-data-sources.md`) recommends.

1. **Time zones.** Store fixture times in UTC with the club's time zone (from `settings.club.country`, or chosen), and read CSV and typed times in it. Today they are read as Lisbon time. This comes first, before any club outside Portugal.
2. **Calendar (`.ics`) import.** A team's calendar link (TeamSnap, SportsEngine, GameChanger, many federations, calendariofpb.pt) feeds its fixtures, refreshed daily and cached. It works for any sport and country.
3. **Grow the interface** as the first network provider needs it:
   - capabilities, declared per provider
   - ids namespaced by provider
   - a list of provider links on each escalão in place of the FPB-only fields
   - paging and a pacer per provider that honours 429 and `Retry-After`
   - keys stored in `dados/`
   - the attribution line each source asks for
   - names kept as the source writes them, plus optional labels per language
4. **Nevobo (volleyball) and the NBB database (basketball), Netherlands.** Open data from professional to under-10s, with no key for reading. Ask both maintainers first.
5. **PlayHQ** (Australia, New Zealand, UK, Basketball England). A documented API, but every key needs approval.
6. **FPV through Data Project** (Portuguese volleyball, with squads and youth teams). Ask the FPV first. The same parser may read other federations on Data Project.
7. **Bring your own key** for professional clubs: TheSportsDB first, BallDontLie for professional basketball, API-Sports once its terms are read.
8. **Wikidata** only to add colours and translated names to a club the user already picked.

Not to build: Sofascore, Flashscore, zerozero.pt, Transfermarkt, Sports-Reference, MaxPreps and ESPN's hidden API (their terms forbid it); Sportmonks (no storing); undocumented federation backends.

## Not planned

- A built-in mixer or encoder. OBS does that.
- Reading the board with OCR.
- Replays, statistics, or a hosted viewer page.
- Paid tiers.
