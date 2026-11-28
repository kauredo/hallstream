# Hallstream

Hallstream is an open-source game-day tool for clubs that score basketball, futsal, handball, volleyball or soccer. Volunteers open a Windows exe, a Mac app or a Linux binary on game day. It reads a Stramatel scoreboard over Bluetooth, a Daktronics All Sport 5000 or Bodet Scorepad on a serial cable, or the score from a phone, and serves the OBS overlay and the control panel. It has to keep working with no internet and nobody technical in the room.

## Commands

- `go vet ./...` and `go test ./...` need no network and no secrets. The `TestOverlay…` and `TestPanel…` tests drive headless Chrome and skip when the machine has none.
- CI vets and tests on Linux and macOS, and builds the program for Linux (with cgo, and the static cgo-free build the release ships), macOS (amd64 and arm64 for macOS 13) and Windows (amd64 and the 32-bit one the release ships). Before you finish, run `GOOS=windows GOARCH=386 go build ./...`.
- `go run ./cmd/hallstream -demo -nobrowser` plays an invented game with no Bluetooth. The panel is at `http://127.0.0.1:8765/painel` and the overlay at `/overlay`. A second copy only opens the first one's panel and exits.
- `go run ./cmd/hallstream -replay internal/ble/testdata/basketball-synthetic.capture -nobrowser` plays a recorded console session through the console source at its recorded pace, with no Bluetooth, so the overlay moves as it did at the game. The panel's "Gravar a consola" switch makes these files in `dados/captura/` (format in `docs/CONTRACT.md`, "Console capture").
- The program keeps its data in `dados/` beside the executable, or in the current folder under `go run`. The Mac app, run from inside `Hallstream.app`, keeps it in `~/Library/Application Support/Hallstream/dados`, since it can't write inside its signed bundle. Demo mode invents the game and still uses the OBS and YouTube logins saved there. For a clean run, `go build -o <scratch folder>/hallstream ./cmd/hallstream` and start that.
- `./build.sh <version> [windows|macos|linux]` is a zsh script. It runs the tests, then builds into `dist/`: `windows` (the default) the two exes and `Hallstream-<version>.zip`; `macos` (on a Mac only) a universal, ad-hoc-signed `Hallstream.app` and `Hallstream-<version>-macos.dmg`, or no DMG when `HALLSTREAM_SIGN` is set and the workflow signs first; `linux` a static amd64 binary in `Hallstream-<version>-linux-x64.tar.gz`. Without `HALLSTREAM_UPLOAD_TOKEN` set, the builds never upload game logs. Public releases are always built without it: the website promises they send nothing. `HALLSTREAM_YOUTUBE_CLIENT_ID` and `HALLSTREAM_YOUTUBE_CLIENT_SECRET` build in Hallstream's own YouTube client; without them the panel asks for the club's own (`docs/CONTRACT.md`, YouTube). `release.yml` passes them from the secrets of the same names only while the repo variable `HALLSTREAM_YOUTUBE_BUILTIN` is `on`, which waits for Google's verification of the client.
- To release, push a tag `vX.Y.Z`. `.github/workflows/release.yml` runs three jobs with no upload token. `windows` checks the zip holds exactly the exes, `LEIA-ME.txt` and `READ-ME.txt`, `LICENSE.txt`, `THIRD-PARTY-NOTICES.txt`, the YouTube guide in both languages and the camera guide in both languages. `macos` signs the app with the Hardened Runtime, notarises and staples it, then does the same for the DMG, and checks the DMG holds exactly the app, an `Applications` link and the same texts. `linux` checks the tarball holds exactly a `Hallstream/` folder with the binary (executable and statically linked) and the same texts. `publish` then puts the three files on the release with `SHA256SUMS` and auto-generated notes, and calls the Vercel deploy hook (secret `VERCEL_DEPLOY_HOOK`) so hallstream.com shows the new version. On a tag the `macos` job fails unless the five Apple secrets are set: `MACOS_CERTIFICATE` and `MACOS_CERTIFICATE_PWD` (the Developer ID Application `.p12`, base64, and its password), `APPLE_API_KEY`, `APPLE_API_KEY_ID` and `APPLE_API_ISSUER` (the App Store Connect API key, base64, its ID and the issuer). A pull request that touches the release files, or a manual run, builds all three as a dry run, with the Mac app unsigned.

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
