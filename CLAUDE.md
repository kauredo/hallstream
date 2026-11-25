# Hallstream

Hallstream is an open-source game-day tool for clubs that score basketball, futsal, handball, volleyball or soccer. Volunteers double-click a Windows exe on game day. It reads a Stramatel scoreboard over Bluetooth, or takes the score from a phone, and serves the OBS overlay and the control panel. It has to keep working with no internet and nobody technical in the room.

## Commands

- `go vet ./...` and `go test ./...` need no network and no secrets. The `TestOverlay…` and `TestPanel…` tests drive headless Chrome and skip when the machine has none.
- CI vets and tests on Linux and macOS, and builds the exe for Linux, macOS and Windows (amd64 and the 32-bit one the release ships). Before you finish, run `GOOS=windows GOARCH=386 go build ./...`.
- `go run ./cmd/hallstream -demo -nobrowser` plays an invented game with no Bluetooth. The panel is at `http://127.0.0.1:8765/painel` and the overlay at `/overlay`. A second copy only opens the first one's panel and exits.
- `go run ./cmd/hallstream -replay internal/ble/testdata/basketball-synthetic.capture -nobrowser` plays a recorded console session through the console source at its recorded pace, with no Bluetooth, so the overlay moves as it did at the game. The panel's "Gravar a consola" switch makes these files in `dados/captura/` (format in `docs/CONTRACT.md`, "Console capture").
- The program keeps its data in `dados/` beside the exe, or in the current folder under `go run`. Demo mode invents the game and still uses the OBS and YouTube logins saved there. For a clean run, `go build -o <scratch folder>/hallstream ./cmd/hallstream` and start that.
- `./build.sh <version>` is a zsh script. It runs the tests, then builds both Windows exes and the zip into `dist/`. Without `HALLSTREAM_UPLOAD_TOKEN` set, the exes never upload game logs. Public releases are always built without it: the website promises they send nothing.
- To release, push a tag `vX.Y.Z`. `.github/workflows/release.yml` tests, runs `build.sh` with no upload token, checks the zip holds exactly the exes, `LEIA-ME.txt` and `READ-ME.txt`, `LICENSE.txt`, `THIRD-PARTY-NOTICES.txt` and the YouTube guide in both languages, publishes the release with `SHA256SUMS` and auto-generated notes, and calls the Vercel deploy hook (secret `VERCEL_DEPLOY_HOOK`) so hallstream.com shows the new version. A pull request that touches the release files runs the same build as a dry run.

## Where the truth is

- `docs/CONTRACT.md` is the behaviour contract: the state shape, every endpoint and every rule. Change it in the same commit as the behaviour.
- Nothing club-specific lives in the code. The club's name, sigla, FPB search name (`settings.club_search`), coaches and sponsors come from the settings and the panel; `web/assets/crest.svg` is a neutral stand-in. Fixtures and tests use invented clubs and people only.
- The visual system is `docs/design/SYSTEM-R3.md`, amended by `SYSTEM-R4.md` and then `SYSTEM-R8.md`. The later file wins.
- The control panel's design context and tokens are in `docs/design-system.md`; it wears the Hallstream brand, while the overlay takes each club's colours.

## Constraints

- All copy is written first in European Portuguese for volunteers, then in British English in `web/i18n/en.json` with the same keys. That includes log lines, since volunteers read `dados/marcador.log`. Every string lives in `web/i18n/pt.json`, read with `i18n.T` in Go and `t()` or `data-i18n` in the pages ("Words" in the contract); `go test ./internal/i18n` fails on a missing key, a key `en.json` lacks, or Portuguese left in the code. The program speaks in the first person ("Não consegui…") and instructs in the imperative ("Carregue…"). Code and comments stay in English.
- The program works offline after one FPB load. Read the FPB API through `fpb.Client.Get`, which keeps a disk copy of every answer and paces requests under the calendar's limit.
- `web/` ships embedded exactly as it is in the repo, with no npm, no build step and nothing loaded from a CDN.
- The overlay stays on air in OBS for hours, so a state it does not expect must still draw. Every screen has fixtures in `web/overlay/fixtures/` and the Chrome test loads each one. Add a fixture with any new screen or state.
- The server listens on 127.0.0.1 unless phone access is on. `ServeHTTP` in `internal/web/server.go` runs the Host, Origin and PIN checks (`localHost`, `sameSite`, `allowed`) before any route. New routes go through it.
- A panic in a goroutine closes the program in the middle of a game. Background loops start through `safely` in `cmd/hallstream/main.go`, which logs the panic and starts the loop again. One-off background work in `internal/web` starts through `Server.once`, which logs it and carries on. The Bluetooth reader handles its own reconnects and stays outside both.
- Leave `dados/` and `dist/` alone, unread and unedited. `dados/` holds the program's saved data, the OBS password and the YouTube login included, and `dist/` holds the release exes.

## Preview a screen

With the program running, open `http://127.0.0.1:8765/overlay?state=/overlay/fixtures/<name>.json&preview=1`. The header of `web/overlay/overlay.js` lists the other switches (`screen`, `anim`, `now`), and the contract lists the panel's mock states (`/painel?mock=1&…`).


## Roadmap

`docs/ROADMAP.md` holds the decisions and the phases (any club, multiple cameras, sport profiles, more consoles). Read it before starting a feature.
