# Roadmap

Hallstream started as a game-day tool built for one Portuguese basketball club. It now scores basketball, futsal, handball, volleyball and soccer from a phone, and reads a Stramatel console for basketball. The goal is a free tool that any club can use to stream its games, in any country, at any level from youth and amateur to professional, and in the club's own language:

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
| Cameras | One OBS scene per camera, with the overlay in a shared nested scene, switched from the panel. Sources: DroidCam, any OBS video device, and VDO.Ninja (a phone browser, no app). Up to four cameras. |
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
6. **First-run wizard.** Done: language, club (with sport and country), scoring, OBS, YouTube, the channel and a rehearsal; the destination step picks YouTube or RTMP. The steps are language, club, sport, input, OBS, streaming destination and a rehearsal. Every step can be skipped and resumed.
7. **YouTube and RTMP.** RTMP done: Definições and the setup pick YouTube or a custom server and key, written into OBS.
   - Ship the project OAuth client, with "use your own" as a fallback.
   - The RTMP path writes the server and key into OBS.
8. **Releases.**
   - Signed Windows builds and notarised macOS builds.
   - An opt-in update check. Done: once a day against GitHub Releases, a banner links to the release page.
   - A feedback form that opens a GitHub issue. Done: Ajuda opens a new issue with the version, system, scoring and sport filled in. A build with the feedback worker (`worker/`, adapted from BVA's) also sends a form with no account needed.

## Phase B: multiple cameras

1. "Configurar OBS" builds one scene per camera (`Câmara 1` to `Câmara 4`), each nesting one shared `Grafismo` scene that holds the overlay. Done: the old `Hallstream` scene stays and still counts as ready while one camera is set; the other cameras come from Definições > Câmaras.
2. The panel gets camera buttons and shows which camera is live. Done: a button per camera on Ecrãs with two or more, the live one marked from OBS's own scene changes; a switch during a time-out keeps the picture in the window.
3. At a timeout, a chosen camera shows picture-in-picture. Done: "Câmara no desconto" in Definições puts that camera on air for the time-out, shrinking into the window with the hole, and the camera that was live comes back after; a switch by hand during the time-out stays.
4. Onboarding adds cameras: DroidCam, a video device, or VDO.Ninja through a QR code on the panel. Done: Definições > Câmaras and the wizard add up to four cameras; a phone joins through VDO.Ninja from a QR code.
5. Each camera gets a health line in the panel (no frames, low fps). Done: "Sem imagem" and "Imagem parada" per camera on the Ecrãs buttons, and OBS's skipped frames once for the whole program. obs-websocket has no frame rate per source, so "low fps" per camera is not measured.
6. A guide covers two and three cameras on wifi and on cable, with the bandwidth each needs. Done: `CAMARAS.txt` / `CAMERAS.txt` in the zip, from `docs/cameras/`.
7. Camera extras (plan 015). Done:
   - Up to four cameras (`Câmara 4`), with the guide's bandwidth table for four.
   - A camera for the Nada screen beside the time-out camera (`obs.screen_cameras`). Leaving the screen switches back to the camera that was live; a switch by hand wins.
   - "Mostrar marcador" per camera: a camera with it off hides the corner scoreboard, the sponsor corner and the lower third while it is live on `marcador` or `nada`. Full screens and the time-out show on every camera.
   - An inset of a second camera, 480x270, picked from the Ecrãs "Inset" row. It reuses the camera's source in OBS, so it adds no bandwidth. It shows on `marcador` and `nada` only, because the time-out window and the full screens already place the picture.

Later:

- Automatic switching that follows the play. Not built: following the ball needs computer vision running on the club's PC, a model, and tuning for each hall. It needs its own spike before any plan. OBS has no auto-director and obs-websocket gives no motion data.

## Phase C: sport profiles

1. Add `internal/sport`. Basketball moves into it unchanged, and the existing tests prove it.
2. Sports and their rules:
   - Futsal: the clock counts down from 20:00, 5 accumulated fouls. Done: scored from the phone; console untested.
   - Handball: the clock counts up, 2×30, 2-minute suspensions. Done: scored from the phone, with suspensions; console untested.
   - Volleyball: sets to 25 (15 in the last), the serving side. Done from the phone scorer. The console decodes volleyball frames but is untested until a recorded game passes.
   - Soccer: the clock counts up, 2×45 plus stoppage time, cards and substitutions per player. Done: scored from the phone only, no extra time or shoot-out; the game ends on "Fim do jogo".
3. The Stramatel decoder already reads the handball, volleyball and hockey codes (`internal/ble/decode.go`). A sport is only listed as supported after it has been tested on a recorded game.
4. Overlay fixtures go in `web/overlay/fixtures/<sport>/` for each sport, and the Chrome test loads them all.

## Phase D: more consoles

1. A serial `Source` for RS-232 consoles: Daktronics AllSport, Fair-Play and Bodet.
2. A capture mode that records the raw bytes, so a contributor can send a session. Done: the panel's "Gravar a consola" switch writes `dados/captura/`, and `-replay <file>` plays a file back.
3. A support table on the website lists each console and sport as tested, community or wanted.

## Phase E: more data providers

In the order the research (`docs/research/team-data-sources.md`) recommends.

1. **Time zones.** Done: `settings.club.timezone`, or the country's zone, reads typed and imported times, and the pages draw in it. Store fixture times in UTC with the club's time zone (from `settings.club.country`, or chosen), and read CSV and typed times in it. Today they are read as Lisbon time. This comes first, before any club outside Portugal.
2. **Calendar (`.ics`) import.** Done: `group.calendar_url`, read once a day, listed and picked in Jogo. A team's calendar link (TeamSnap, SportsEngine, GameChanger, many federations, calendariofpb.pt) feeds its fixtures, refreshed daily and cached. It works for any sport and country.
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
