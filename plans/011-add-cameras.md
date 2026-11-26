# Plan 011: A club adds up to three cameras from Definições and the first-run wizard, a phone among them through VDO.Ninja

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- internal/obsws internal/web/obs.go internal/web/camera.go internal/web/server.go internal/web/promo.go internal/store/store.go web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md`
> Plans 008, 009 and 010 are expected to have changed most of these files.
> First confirm they landed (see "Depends on"), then compare every "Current
> state" excerpt with the live code. Line numbers below are from `b28fb61`
> and will have moved; a change you can map one-to-one is fine, one you
> can't is a STOP condition.

## Status

- **Priority**: P1 (ROADMAP Phase B item 4; the first way a club gets a second camera without touching OBS)
- **Effort**: L
- **Risk**: MED (Configurar OBS creates new sources in the club's OBS; one new route)
- **Depends on**: `plans/008-*.md` (`settings.obs.cameras` as up to 3 `{ "source", "scene", "label" }`, `settings.obs.graphics_scene`, and Configurar OBS building `Grafismo` plus one scene per camera), `plans/009-*.md` (camera buttons on Ecrãs, `obs.live_camera`) and `plans/010-*.md` (`settings.obs.timeout_camera`). Check: `grep -n '"graphics_scene"' internal/store/store.go`, `grep -n '"timeout_camera"' internal/store/store.go` and `grep -n 'live_camera' docs/CONTRACT.md` all match. Any miss: STOP.
- **Category**: direction
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

Today a club has one camera, picked in Definições > OBS > Câmara. ROADMAP Phase B item 4 says onboarding adds cameras: DroidCam, a video device, or VDO.Ninja through a QR code on the panel. Plans 008 to 010 give OBS one scene per camera and the panel buttons to switch them, but nothing in the panel adds a second or third camera yet. After this plan the volunteer opens Definições > Câmaras, adds a camera from the list OBS already offers, or picks "Telemóvel (VDO.Ninja)", and a phone that scans the QR code becomes a camera with no app installed.

Owner decisions this plan must honour (`plans/phase-b-decisions.md`, decisions 7, 8, 11 and 12):

- Definições > Câmaras lists up to three cameras, each picked from the existing choices (DroidCam, a video device, an OBS source) or "Telemóvel (VDO.Ninja)".
- The first-run wizard's OBS step gains "Adicionar outra câmara" after the first. Skipping keeps one camera.
- VDO.Ninja: a random stream id per camera, saved in `cameras[i].vdo`. The program adds a browser source on VDO.Ninja's view link and shows a QR code of the push link on the panel. The panel says it needs internet.
- Every new string goes in `pt.json` first, then `en.json`.
- The Câmaras section goes through `build-ui`, extension branch, with an outside critique.

## Decisions made in this plan (the executor does not re-decide them)

1. **The VDO.Ninja links.** Built in one Go place, `internal/obsws/vdo.go`, from the stream id. Each parameter comes from docs.vdo.ninja (read 2026-11-26):
   - Push (the phone): `https://vdo.ninja/?push=<id>&webcam&autostart&facing=rear&quality=0&audiodevice=0`
     - `push`: "1 to 64-characters long. Non-alphanumeric characters are sanitized to `_`; case sensitive." A second phone on an id in use is refused: "If the stream ID is already in active use, an error will be shown and the stream will not publish." (https://docs.vdo.ninja/advanced-settings/setup-parameters/push.md)
     - `webcam` "Automatically selects the Share your Camera option and hides the screen-share button" (https://docs.vdo.ninja/advanced-settings/setup-parameters/and-webcam.md). `autostart` "Skips the camera/audio device or screenshare selection" and "Can be used together with `&webcam`" (https://docs.vdo.ninja/advanced-settings/setup-parameters/and-autostart.md).
     - `facing=rear` picks the back camera; values are `rear`, `environment` or `back` (https://docs.vdo.ninja/advanced-settings/mobile-parameters/and-facing.md).
     - `quality=0` targets 1920x1080; "Actual capture resolution depends on the camera, browser, and hardware" (https://docs.vdo.ninja/advanced-settings/video-parameters/and-quality.md). Do not use `&width`/`&height`: "If the camera cannot support the width, it will fail" (https://docs.vdo.ninja/advanced-settings/video-parameters/and-width.md).
     - `audiodevice=0` turns the phone's microphone off, sender side (https://docs.vdo.ninja/advanced-settings/setup-parameters/audiodevice.md). The game's sound comes from the PC, as today.
   - View (the OBS browser source): `https://vdo.ninja/?view=<id>&noaudio&cleanoutput&videobitrate=4000`
     - `noaudio` is viewer side: "Delivers video only streams; audio playback is disabled" (https://docs.vdo.ninja/advanced-settings/audio-parameters/noaudio.md).
     - `cleanoutput` "Hides many of the UI elements and pop-ups" (https://docs.vdo.ninja/advanced-settings/design-parameters/cleanoutput.md).
     - `videobitrate` is "the 'desired target' bitrate in kbps", viewer side; the default "will target around 2500-kbps" (https://docs.vdo.ninja/advanced-settings/video-bitrate-parameters/bitrate.md). 4000 matches the 1080p (`q=0`) target in https://docs.vdo.ninja/guides/default-resolution-and-bitrate.md and gives plan 013's guide a fixed number per phone.
   - No room: one phone to one OBS source needs only push and view (https://docs.vdo.ninja/getting-started/vdo.ninja-basics.md).
   - No `&password`. The random id is the secret, and the QR code carries the push link whole, so a password in the same link adds nothing a person holding the QR code lacks. See "Open questions / corrections" 3.
2. **The stream id**: 20 characters from `a-z0-9`, from `crypto/rand` (about 103 bits). Lower case only, so nobody misreads it if they type it.
3. **Only the server writes `cameras[i].vdo`.** It makes the id when a phone camera is added through Configurar OBS. `PUT /api/settings` keeps each camera's `vdo` from the saved settings (matched by `source`) and drops any `vdo` the request invented, the way it keeps `camera_full` today (`internal/web/server.go:545`).
4. **The phone's source**: a `browser_source` named `Telemóvel N` (`obs.setup.telemovel_nome`, "Telemóvel {n}" / "Phone {n}", made unique with `uniqueName`), placed in that camera's scene, with settings `{ "url": <view link>, "width": 1920, "height": 1080, "is_local_file": false, "shutdown": false, "restart_when_active": false }`. VDO.Ninja's OBS page says "Make sure 'Shutdown source when not visible' is NOT checked" and "Make sure 'Refresh browser when scene becomes active' is NOT checked" (https://docs.vdo.ninja/guides/enabling-webrtc-sources-in-obs.md). A camera scene that is not on air is "not visible", so `shutdown: true` would drop the phone every time the club switched away from it.
5. **"Telemóvel (VDO.Ninja)" is added by the panel, not by `choices()`.** `auto()` uses `choices()` to decide between "the only choice" and "choose" (`internal/obsws/setup.go:517-527`). A phone entry always in that list would break both. The panel appends the phone option to what `POST /api/obs/cameras` returns, with value `vdo`.
6. **A second DroidCam phone through the plugin.** Today the plugin is offered only while no plugin source exists, and a new plugin source connects to the first phone on USB. Both change (Step 2), because DroidCam's own docs say "To use multiple phones, just add another DroidCam source to your scene" (https://droidcam.app/obs/usage/).
7. **Removing a camera** takes it out of `settings.obs.cameras` only. Its scene and its source stay in OBS (the existing rule: never delete a user's source). A removed phone camera's id is dropped, and a phone added later gets a new id. The last camera can't be removed. `timeout_camera` (plan 010) follows the list: it becomes `0` when its camera is removed and moves down by one when a camera before it is removed.
8. **The QR route is under `/api/`**: `GET /api/obs/cameras/{n}/qr.svg`, `n` from 1 to 3. The push link lets anyone who has it put a picture into that camera slot whenever the club's phone is not connected. `allowed` (`internal/web/server.go:339-359`) asks other devices for the PIN on `/api/*` only, and the pages outside `/api/` are public. The panel sends the PIN as the `pin` cookie with `path=/` (`web/painel/painel.js:201`), so an `<img>` on the panel carries it.
9. **The QR shows only for a saved phone camera**, after Configurar OBS made its source. A row just set to "Telemóvel (VDO.Ninja)" and not saved yet shows the internet note and "Guarde para ver o código".

## Current state

### Files and their roles (line numbers at `b28fb61`)

- `internal/obsws/setup.go`: Configurar OBS and the health check.
  - `Choice` (lines 34-42): `Value` is `"source:<name>"`, `"device:<id>"` or `"plugin"`; `Type` is `source`, `device` or `plugin`.
  - `Cameras` (97-113): the choice list for `POST /api/obs/cameras`.
  - `inputs` (215-235) reads settings only for `browser_source` and `dshow_input`:
    ```go
    for i := range r.Inputs {
    	if k := r.Inputs[i].Kind; k == "browser_source" || k == dshowKind {
    ```
  - `isOverlay` (258-268): a browser source is the overlay only when its URL path is `/overlay` on ports 8765-8775. A VDO.Ninja URL never matches.
  - `camera` (417-492) resolves one choice; `use` (530-538) turns a picked choice into a source; `addDevice` (621-630) and `addPlugin` (634-654) create inputs in `SceneName`.
  - `choices` (542-579) offers the plugin only without a plugin source:
    ```go
    if slices.Contains(kinds.Kinds, PluginKind) && !plugin {
    	list = append(list, Choice{"plugin", "plugin", "DroidCam OBS (plugin)", true})
    }
    ```
  - `addPlugin` connects the new source to the first USB entry of `device_list` (lines 642-651), skipping only `dev_id_wifi`.
  - Plan 008 rewrites `run`, `ensureScene`, `camera` and the scene name constants for one scene per camera. Read its version before Step 2.
- `internal/obsws/setup_test.go`: `model` (lines 12-47) is a small fake OBS with scenes, inputs, devices and DroidCam `phones`; `answer` (87-) handles each request type; `newRun`, `setup`, `health`, `wantResults` (256-306) drive it. `TestSetupPlugin` (381) is the pattern for plugin tests.
- `internal/web/obs.go`:
  - `want` (47-56) builds `obsws.Want` from the saved settings and the request.
  - `obsRoutes` (58-118): `POST /api/obs/setup` (`obsSetup`, 211-254) and `POST /api/obs/cameras` (62-75, under `setupOBSMu`).
  - `fitCamera` (258-278) saves the camera used.
- `internal/web/promo.go`: `promoQR` (434-449) and `qrSVG(code *qr.Code) []byte` (451-464), which draws an SVG with a 4-module margin. Reuse `qrSVG`; do not copy it.
- `internal/web/marcar.go:151-167`: `marcarQR`, the other QR route. Both QR routes sit outside `/api/` because their links hold no secret.
- `internal/web/server.go`:
  - `ServeHTTP` (294-309) runs `localHost`, `sameSite`, `allowed` before every route.
  - `allowed` (339-359): other devices need the PIN on `/api/*`.
  - `sameSite` (374-392): writes need `application/json` and a same-host `Origin`. GET passes.
  - `putSettings` (506-576) keeps the server-owned fields (lines 530-551).
- `internal/store/store.go:23-41`: `OBS`. Plan 008 adds `Cameras` (a slice of a camera struct with `Source`, `Scene`, `Label`) and `GraphicsScene`; plan 010 adds `TimeoutCamera`. Find the struct with `grep -n 'json:"scene"' internal/store/store.go`.
- `web/painel/painel.js`:
  - `renderDefinicoes` (2099-): the OBS card (2122-2150) with the Configurar OBS button, `#obsResult`, the `#dObsCam` select (2127) and the Enquadramento select (2128-2130). Plan 008 may have changed this card.
  - `setupHtml` (1289-1304) draws the checklist and the "choose" picker; `runSetup` (1306-1324) saves the form, then posts `POST /api/obs/setup`.
  - `camOptions` (2511-2526) and `loadObsCameras` (2527-2535) build the camera select from `POST /api/obs/cameras`.
  - The wizard: `renderWzBody` (2927-), OBS step at 2986-2990; `wzObs` (3037-3049); click handling at 3091-3097.
  - `MOCK` (line 6) is true with `?mock=1`.
- `web/painel/mock.js`: `obsMode` (238), `cameras` (249-253), `setupReport` (254-), the `/api/obs/*` answers (506-542).
- `docs/CONTRACT.md`: "OBS set up by the program" (lines 315-327, which plan 008 rewrites) and "Configuração inicial" (381-386).

### Conventions

- Go standard library plus the modules in `go.mod` (`rsc.io/qr` is already there). Code and comments in English.
- Tests in plain `testing`. Setup tests on the `model` fake in `internal/obsws/setup_test.go`; HTTP tests with `call(t, s, method, path, body, &out)` and `httptest` as in `internal/web/marcar_test.go:104-132` (`TestMarcarNeedsThePINFromThePhone` sets `req.RemoteAddr = "192.168.1.20:5000"` to act as a phone).
- Every user-facing string in `web/i18n/pt.json` first (European Portuguese; the program speaks in the first person, instructs in the imperative), then `en.json` (British English), same keys. `go test ./internal/i18n` enforces it and fails on keys built at run time.
- `web/` ships as is: no build step, nothing from a CDN.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Never read or edit `dados/` or `dist/`.
- Commits: `feat: cameras …`, conventional commits.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| OBS setup tests | `go test ./internal/obsws/` | pass |
| Web tests | `go test ./internal/web/` | pass |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel `http://127.0.0.1:8765/painel` |
| Mock panel | open `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=vdo` | Definições > Câmaras draws |

## Suggested executor toolkit

- `build-ui` skill, **extension branch** ("a new section, state or screen inside a settled identity"), for Steps 5 and 6. The panel's design context and tokens are in `docs/design-system.md`. Run its lint and outside critique on the Câmaras card (one camera, three cameras, a phone camera with its QR code) before calling the step done.
- `interface-guidelines` for the rows (labels on every select and input, the remove button's accessible name, focus after adding a row).

## Scope

**In scope**:

- `internal/obsws/vdo.go` (create), `internal/obsws/vdo_test.go` (create)
- `internal/obsws/setup.go` (the `vdo` choice, the phone source, the plugin changes), `internal/obsws/setup_test.go` (new tests and new cases in `model.answer` only)
- `internal/web/obs.go` (the camera list in the setup request, saving `vdo`), `internal/web/camera.go` or a new `internal/web/cameras.go` (the QR route)
- `internal/web/server.go` (`putSettings` keeps `vdo` and fixes `timeout_camera`)
- `internal/store/store.go` (`Vdo` on 008's camera struct only)
- `internal/web/*_test.go` (new tests only)
- `web/painel/painel.js`, the panel stylesheet the Definições cards use, `web/painel/mock.js`
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- The camera buttons on Ecrãs, switching, `live_camera` (plan 009), the time-out camera (plan 010).
- The health line per camera (plan 012) and the guide (plan 013).
- Deleting anything in OBS: scenes, sources or items, ever.
- Running VDO.Ninja offline (its own offline deployment). This plan uses `vdo.ninja`.
- The overlay. It does not change.

## Git workflow

- Branch: `feat/add-cameras` (stacked on plan 010's branch)
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The VDO.Ninja links and the stream id

Create `internal/obsws/vdo.go`:

```go
// VDOPush is the link a phone opens to send its back camera, with no sound,
// at up to 1080p, to stream id (docs.vdo.ninja, read 2026-11-26).
func VDOPush(id string) string

// VDOView is the link the OBS browser source opens to show that phone.
func VDOView(id string) string

// NewVDOID is a random stream id: 20 characters from a-z and 0-9.
func NewVDOID() (string, error)

// IsVDOView reports whether url is VDOView for some id, and which.
func IsVDOView(url string) (id string, ok bool)
```

Use the exact links in Decision 1. Build the query with `net/url` values, but keep the bare flags (`webcam`, `autostart`, `noaudio`, `cleanoutput`) with no `=`: write them into the string yourself, since `url.Values` would add `=`. Put a comment above each function that names the doc page of every parameter (the URLs in Decision 1).

**Verify**: `go test ./internal/obsws/ -run VDO` → pass, with new tests: `VDOPush("abc")` equals `https://vdo.ninja/?push=abc&webcam&autostart&facing=rear&quality=0&audiodevice=0`; `VDOView("abc")` equals `https://vdo.ninja/?view=abc&noaudio&cleanoutput&videobitrate=4000`; 100 calls of `NewVDOID` give 100 distinct ids, each matching `^[a-z0-9]{20}$`; `IsVDOView(VDOView("abc"))` gives `"abc", true`; `IsVDOView("http://127.0.0.1:8765/overlay")` gives `false`.

### Step 2: Configurar OBS adds a phone, and a second DroidCam

Read plan 008's version of `internal/obsws/setup.go` first. It decides how `Want` carries several cameras and in which scene each camera's source goes. Then, in `internal/obsws/setup.go`:

- A new choice value `vdo` (new phone) and `vdo:<id>` (a phone whose id exists). Add the type `phone` to the `Choice` doc comment. `choices()` never returns them (Decision 5).
- Where 008 resolves each camera's choice (today `camera` and `use`), handle `vdo` and `vdo:<id>` with a new `addPhone(st *Step, in []input, scene, id string) string`, modelled on `addPlugin`:
  - For `vdo`, make the id with `NewVDOID`.
  - If an input of kind `browser_source` already has `url` equal to `VDOView(id)`, use it (an existing phone camera).
  - Else, with `apply`, `CreateInput` in the camera's scene with the name and settings in Decision 4, result `created`, detail `obs.setup.juntei_telemovel` ("Juntei a fonte {fonte}, que mostra o telemóvel ligado pelo VDO.Ninja." / "I added the source {fonte}, which shows the phone connected through VDO.Ninja."). Without `apply`, the step fails with `obs.setup.falta_telemovel` ("Falta a fonte do telemóvel da câmara {n}." / "The phone source for camera {n} is missing.").
  - On a refusal, `failed` with `obs.setup.nao_juntou_telemovel` ("Não consegui juntar o telemóvel: {erro}" / "I couldn't add the phone: {erro}").
  - Report the id back. Add a field to the per-camera result that 008 returns (or to `Report`, as `Phones map[string]string` from source name to id, if 008 has no per-camera result) so the server can save it.
- An existing phone source whose `url` no longer matches its saved id: the health check fails the camera step with `obs.setup.telemovel_endereco` ("A fonte {fonte} já não aponta para o telemóvel desta câmara." / "The source {fonte} no longer points at this camera's phone."). Configurar OBS sets the `url` back with `SetInputSettings` (`overlay: true`, so other settings stay), result `fixed`, detail `obs.setup.telemovel_corrigido` ("Pus a fonte {fonte} outra vez a apontar para o telemóvel." / "I pointed the source {fonte} back at the phone.").
- `inputs()`: also read settings for `PluginKind` inputs, so the next change can see which phone each one uses.
- `choices()`: offer the plugin whenever its kind is installed, not only when no plugin source exists. Keep its label. `auto()` is unchanged: it returns a DroidCam source before it ever reaches `choices()`.
- `addPlugin`: skip `device_list` values that another `droidcam_obs` input already has in its `device_list` setting, as well as `dev_id_wifi`. With no free USB device, it keeps today's Wi-Fi detail (`obs.setup.juntei_wifi`).

The fake already answers `CreateInput`, `GetInputSettings`, `SetInputSettings` and `PressInputPropertiesButton`. Add to `model.answer` only what the new tests need.

**Verify**: `go test ./internal/obsws/` → pass, with new tests on the `model` fake:
- A setup with cameras `[device, vdo]` creates `Telemóvel 1` (kind `browser_source`) in camera 2's scene with `url` matching `IsVDOView`, `width` 1920, `height` 1080, `shutdown` false, `restart_when_active` false, and reports its id.
- The same setup run again with `vdo:<that id>` creates nothing (`m.wrote()` has no `CreateInput`).
- The health check with the source's `url` changed by hand fails with the `telemovel_endereco` detail; Configurar OBS fixes it.
- With one `droidcam_obs` source already on phone `A` and phones `A` and `B` in `m.phones`, `choices()` still lists `plugin`, and adding it connects the new source to `B`.
- All existing tests pass unchanged.

### Step 3: The server saves phone ids and keeps them

- `internal/store/store.go`: add `Vdo string \`json:"vdo,omitempty"\`` to plan 008's camera struct, with the comment "the VDO.Ninja stream id of a phone camera; only the server writes it".
- `internal/web/obs.go`: the setup request takes the camera list in the shape plan 008 gave it. The panel sends `vdo` for a new phone and `vdo:<id>` for a saved one. After a setup, save each returned phone id into that camera's `Vdo` in the same `UpdateSettings` call that 008 uses to save the cameras.
- `internal/web/server.go` `putSettings`, inside `UpdateSettings` next to line 545: for each camera in `req.OBS.Cameras`, set `Vdo` to the saved camera's `Vdo` with the same `Source`, else `""`. Then fix `req.OBS.TimeoutCamera` (Decision 7): compare the old and new lists by `Source`; if the time-out camera's source is gone, set `0`; else set it to that source's new 1-based index.

**Verify**: `go test ./internal/web/` → pass, with new tests: a `PUT /api/settings` that sends `vdo: "evil"` on a camera keeps the saved id; one that drops camera 2 of 3 with `timeout_camera: 3` saves `timeout_camera: 2`; one that drops the time-out camera saves `0`.

### Step 4: The QR route

Add `GET /api/obs/cameras/{n}/qr.svg` next to the other camera routes (`cameraRoutes` in `internal/web/camera.go`, or a new `internal/web/cameras.go` registered from `New` in `server.go`):

- `n` from 1 to the number of saved cameras, else 404. A camera with no `Vdo`, 404.
- `qr.Encode(obsws.VDOPush(vdo), qr.M)`, then `qrSVG(code)` from `promo.go`.
- Headers as `promoQR` (`promo.go:445-447`): `Content-Type: image/svg+xml`, `Content-Security-Policy: script-src 'none'`, `X-Content-Type-Options: nosniff`. Add `Cache-Control: no-store`: the link is a secret and must not sit in a cache.

It goes through `ServeHTTP` like every route: `localHost` refuses another Host, `allowed` asks other devices for the PIN, `sameSite` passes a GET.

**Verify**: `go test ./internal/web/` → pass, with a new test modelled on `TestMarcarNeedsThePINFromThePhone`: with phone access on and PIN `1234`, saved cameras `[{source: "DroidCam"}, {source: "Telemóvel 1", vdo: "abc"}]`:
- from `192.168.1.20:5000` with no PIN → 401;
- with `X-PIN: 1234` → 200, body starts with `<svg`, `Cache-Control: no-store`;
- from `127.0.0.1` with no PIN → 200;
- `/api/obs/cameras/1/qr.svg` (no `vdo`) and `/api/obs/cameras/4/qr.svg` → 404;
- Host `evil.example` → 403.

### Step 5: Definições > Câmaras

In `web/painel/painel.js`, add a card "Câmaras" (`painel.camaras.titulo`) right after the OBS card in `renderDefinicoes`. Move the camera picking out of the OBS card into it: the `#dObsCam` select becomes camera 1's row. Enquadramento stays one setting for all cameras (decision 1); keep it where 008 left it.

- One row per saved camera, in order, up to three. Each row has:
  - the camera's name, "Câmara {n}" (`painel.camaras.camara`) as its heading;
  - a select built by a new `camRowOptions(cur)`: today's `camOptions` choices (OBS sources, "Juntar ao OBS" devices, the plugin) plus "Telemóvel (VDO.Ninja)" (`painel.camaras.telemovel`, value `vdo`). A saved phone camera shows that option selected, with value `vdo:<id>`;
  - a label input, "Nome no painel" (`painel.camaras.nome`), saved as `cameras[i].label` (the name on the Ecrãs buttons);
  - "Tirar" (`painel.camaras.tirar`), hidden while only one row is left. Its accessible name includes the camera's name.
- For a row on a phone: the note `painel.camaras.internet` ("Precisa de internet: o telemóvel e o OBS encontram-se através do VDO.Ninja. O resto do Hallstream funciona sem internet." / "Needs the internet: the phone and OBS find each other through VDO.Ninja. The rest of Hallstream works offline."). For a saved phone, also the QR code `<img src="/api/obs/cameras/{n}/qr.svg?v={vdo}" width="148" height="148" alt="…">` (`painel.camaras.qr_alt`, "Código QR para ligar o telemóvel da câmara {n}" / "QR code to connect camera {n}'s phone") and `painel.camaras.qr_dica` ("No telemóvel que vai filmar, leia este código com a câmara e abra o link no navegador. Deite o telemóvel e deixe-o ligado ao carregador." / "On the phone that will film, scan this code with its camera and open the link in the browser. Turn the phone sideways and keep it on the charger."). For a phone row not saved yet: `painel.camaras.guarde_qr` ("Guarde para ver o código." / "Save to see the code."). With `MOCK`, use a small inline placeholder SVG as the `src`, so the mock screenshot shows a square.
- "Adicionar câmara" (`painel.camaras.adicionar`) adds an empty row; disabled at three.
- "Guardar e configurar o OBS" (`painel.camaras.guardar`) runs the existing `runSetup` flow with the camera list and labels: save the form, then `POST /api/obs/setup`, then reload settings, then draw the checklist in `#obsResult`. The checklist stays where it is.
- `painel.camaras.dica` under the title: "Até três câmaras. Cada uma tem a sua cena no OBS, com o grafismo por cima. Mude a lista e carregue em <b>Guardar e configurar o OBS</b>." / "Up to three cameras. Each has its own scene in OBS, with the graphics on top. Change the list, then press <b>Save and set up OBS</b>."

Remove `painel.definicoes.camara` and `painel.definicoes.camara_dica` from both language files if nothing uses them any more (`grep -n "definicoes.camara" web/painel/painel.js`). Move the useful sentence of `camara_dica` (the old camera stays hidden in OBS; "Imagem inteira" leaves margins) into `painel.camaras.dica` only if it still holds after 008.

In `web/painel/mock.js`: a `cams` switch, `?cams=1|2|3|vdo` (default `1`), that fills `db.settings.obs.cameras`. `vdo` gives three cameras with camera 2 a phone (`source: "Telemóvel 1"`, `vdo: "mockmockmockmockmock"`). `setupReport` keeps the cameras it is sent.

Work through `build-ui` (extension branch) with its lint and outside critique.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass. Open `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=vdo` on the PC width and at 390 px wide: three rows, camera 2 shows the internet note and the QR square, "Adicionar câmara" is disabled, "Tirar" removes a row and focus lands on the next row's select. `?cams=1` hides "Tirar".

### Step 6: The wizard's OBS step

In `renderWzBody`, OBS step (2986-2990), PC only like the Configurar OBS button:

- Once `S.obsSetup && S.obsSetup.ok` and fewer than three cameras are saved, show "Adicionar outra câmara" (`painel.wz.outra_camara`, "Adicionar outra câmara" / "Add another camera").
- It opens one camera row inline (the same row markup as Step 5, from one shared function), with a primary button that runs `wzObs` with the saved cameras plus the new one.
- After it, the button shows again while fewer than three are saved.
- The step's state (Feito, Falta…) does not change. Skipping keeps one camera.

The existing mock switch `&act=wz&n=<step>` opens the OBS step (check `n` against the OBS step's index in `wzIndex`).

**Verify**: `go test ./internal/web/` → pass. In the mock, `?mock=1&setup=part&obs=ready&act=wz&n=3` shows "Adicionar outra câmara" after Configurar OBS succeeds; `&pc=0` hides it.

### Step 7: Contract and roadmap

- `docs/CONTRACT.md`, "OBS set up by the program": the `vdo` and `vdo:<id>` choice values, the phone source's name, kind and settings (Decision 4), the two VDO.Ninja links with their parameters and the doc pages (Decision 1), the plugin offered again and its USB device choice, the health check on a phone source's URL, `cameras[i].vdo` (server-only; `PUT /api/settings` keeps it), `timeout_camera` following removals, and `GET /api/obs/cameras/{n}/qr.svg` (PIN for other devices, `no-store`, 404 rules).
- "Configuração inicial": "Adicionar outra câmara" in the OBS step.
- The panel's mock states: `&cams=1|2|3|vdo`.
- `docs/ROADMAP.md` Phase B item 4: "Done: Definições > Câmaras and the wizard add up to three cameras; a phone joins through VDO.Ninja from a QR code."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/obsws/vdo_test.go`: Step 1 cases.
- `internal/obsws/setup_test.go`: Step 2 cases on the `model` fake, new tests only.
- `internal/web/*_test.go`: Step 3 and Step 4 cases, new tests only, modelled on `TestSettingsKeepCamera` (`camera_test.go:29`) and `TestMarcarNeedsThePINFromThePhone` (`marcar_test.go:104`).
- If a `TestPanel…` Chrome test covers Definições after 008, add a case that loads `?mock=1&obs=ready&cams=vdo` and finds three rows and one QR image. Model it on `TestPanelSportPerEscalao` (`internal/web/panel_sport_test.go:17`).
- Existing tests: all pass with no change to them.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n "audiodevice=0" internal/obsws/vdo.go` matches
- [ ] `grep -n "qr.svg" internal/web/*.go | grep "/api/obs/cameras"` matches
- [ ] `grep -n "painel.camaras.internet" web/i18n/pt.json web/i18n/en.json` matches in both
- [ ] `grep -n "vdo" docs/CONTRACT.md` matches
- [ ] `git diff b28fb61..HEAD -- internal/obsws/setup.go | grep -c RemoveScene` is 0 (nothing deletes a scene)
- [ ] No existing test case modified (review `git diff -- '*_test.go'`: only additions)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plans 008, 009 or 010 have not landed (the checks under "Depends on" fail).
- Plan 008's `POST /api/obs/setup` has no way to receive several cameras with a choice value each (it only builds scenes from `settings.obs.cameras` and has no per-camera choice). Report what it takes; how to send a new phone is then a design question.
- Plan 008 named the camera struct's fields other than `source`, `scene`, `label`.
- OBS's browser source does not accept `shutdown` or `restart_when_active` as setting keys (check obs-browser's `browser-source.cpp`, `obs_data_get_bool` calls, before Step 2). Report the real keys.
- An existing test fails and only editing it would make it pass.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- VDO.Ninja is a third-party site. If it changes a parameter, only `internal/obsws/vdo.go` changes. A link a phone already opened keeps working until that phone reloads.
- VDO.Ninja needs the internet to load the page and connect, even with the phone and the PC on the same network: "Loading the page and completing the handshake still need internet access" (https://docs.vdo.ninja/readme/how-does-it-work.md). The picture itself "usually stays on the local network". The panel says so; the guide (plan 013) explains it.
- The phone source is always 1920x1080, whatever the phone sends, so the squeeze and the refit treat it as a 16:9 camera. A phone held upright shows with bars inside that 16:9 page. Plan 012's health line must not rely on a 0x0 size for phones; see its plan.
- Reviewers: in a real OBS, add a phone camera, scan the QR with a phone, and check the picture shows in camera 2's scene; switch to camera 1 and back and check the phone did not reconnect (`shutdown` off). Then remove camera 2 and check its scene and `Telemóvel 1` are still in OBS.

## Open questions / corrections

1. **A second DroidCam phone is not in "the existing choices" today.** `choices()` offers the plugin only while no plugin source exists (`internal/obsws/setup.go:559`), and `addPlugin` connects to the first USB phone (`setup.go:642-651`). Decision 7 assumed the list already covers a second DroidCam. Step 2 changes both; recommended, since DroidCam's docs say "To use multiple phones, just add another DroidCam source" (https://droidcam.app/obs/usage/). A phone on DroidCam's Windows client already shows as "DroidCam Source 2" and "Source 3" devices (https://www.dev47apps.com/droidcam/windows/), which the device list covers.
2. **"Telemóvel (VDO.Ninja)" can't join `choices()`.** That list decides "the only choice" and "choose" in `auto()` (`setup.go:517-527`). The panel adds the option instead (Decision 5). No change to the decision's meaning.
3. **No VDO.Ninja password.** The task asked to consider `&password`. Both links carry the stream id, and the QR code is the push link whole, so a password inside it protects nothing extra. A password the volunteer types on the phone would add a step at every game. Recommended: no password, a 20-character random id, and the QR route behind the PIN (Decision 8). Overturn if the owner wants the id kept off VDO.Ninja's server logs, where `#p=` in the fragment would help (https://docs.vdo.ninja/advanced-settings/setup-parameters/and-password.md).
4. **The QR route needs the PIN, unlike the two QR routes that exist.** `/marcar/qr.svg` and `/promo/{id}/qr.svg` are public because their links hold no secret. The push link does. Recommended: `/api/obs/cameras/{n}/qr.svg` (Decision 8).
5. **Audio.** The decisions file says nothing about the phone's sound. `&noaudio` exists only on the view side; the phone side turns the microphone off with `&audiodevice=0`. The plan uses both, so the phone never asks for the microphone and OBS gets no second sound source.
6. **Removing a camera moves `timeout_camera`.** Decision 6 indexes the time-out camera as 1 to 3. Taking camera 2 out of three would point it at the wrong camera. Step 3 moves or clears it.
