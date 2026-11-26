# Plan 008: Configurar OBS builds a Grafismo scene and one scene per camera

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- internal/obsws internal/store/store.go internal/web/obs.go internal/web/camera.go internal/web/server.go web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md`
> Only plan 008's own commits should show. Compare every "Current state"
> excerpt below with the live code. Line numbers are from `b28fb61`; a change
> you can map one-to-one is fine, one you can't is a STOP condition.

## Status

- **Priority**: P1 (first build step of ROADMAP Phase B; plans 009 to 013 build on it)
- **Effort**: L
- **Risk**: HIGH (Configurar OBS and the health check run on every club's game day; the old one-scene layout must keep working)
- **Depends on**: nothing. Check: `grep -n 'SceneName   = "Hallstream"' internal/obsws/setup.go` matches. If it does not, STOP.
- **Category**: direction
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

Hallstream drives one camera today. "Configurar OBS" builds one OBS scene, `Hallstream`, with the camera and the overlay browser source above it. ROADMAP Phase B item 1 asks for one scene per camera (`Câmara 1` to `Câmara 3`), each nesting one shared `Grafismo` scene that holds the overlay. Switching cameras (plan 009), the time-out camera (plan 010) and adding cameras (plan 011) all need this layout and a settings shape that holds up to three cameras.

After this plan:

- `settings.obs.cameras` holds up to three cameras, `settings.obs.graphics_scene` names the overlay scene, and each camera has its own saved full-screen placement.
- Configurar OBS builds `Grafismo` with the overlay, then one scene per saved camera with the camera and `Grafismo` nested above it, and puts `Câmara 1` on program.
- A club that never runs Configurar OBS again keeps its `Hallstream` scene and sees no change: the health check still counts the old layout as ready while only one camera is set.

Owner decisions this plan must honour (`plans/phase-b-decisions.md`):

- **Decision 1 (settings shape).** `settings.obs.cameras`: up to 3 `{ "source", "scene", "label" }` in order. `camera_source` stays and mirrors camera 1. `camera_fit` stays one setting for all cameras. `camera_full` becomes per camera, keyed by source.
- **Decision 2 (scene names).** Created in the panel's language at setup ("Câmara 1" / "Camera 1", "Grafismo" / "Graphics"), saved in `cameras[i].scene` and `obs.graphics_scene`, and found by the saved name after that.
- **Decision 3 (the old `Hallstream` scene).** Configurar OBS builds the new layout, makes `Câmara 1` the program scene (not while streaming or recording: `later`), and leaves `Hallstream` untouched. The health check counts the old layout as ready while only one camera is set.
- **Decision 11 (words).** Every new string in `web/i18n/pt.json` first, then `en.json`.

## Decisions made in this plan (the executor does not re-decide them)

1. **`camera_fulls` is the per-camera placement.** A new map `settings.obs.camera_fulls` (`{ "<source name>": Full }`). `camera_full` stays and keeps camera 1's placement, so a downgrade to 0.1.2 still finds it. The decisions file gives no field name; this one is new here and plans 009 and 010 use it. See "Open questions / corrections".
2. **An empty `cameras` list means one camera, `camera_source`.** Settings saved by 0.1.2 have no `cameras`. Nothing rewrites them at load. One helper, `store.OBS.CameraList()`, returns `cameras` when it has entries, else `[{ "source": camera_source }]`. Every reader goes through it.
3. **In this plan the server owns `cameras`, `graphics_scene` and `camera_fulls`.** `PUT /api/settings` keeps all three as they are, with one exception: when `camera_source` changes, `cameras[0].source` follows it. Plan 011 opens `cameras` to the panel.
4. **Scene names that are taken.** The first time Configurar OBS creates a scene, it takes the language's name, or that name with " 2", " 3"… when a scene or an input in OBS already has it (OBS names are unique across scenes and inputs). It never adopts a scene the user made. After that the saved name is used as it is, and a saved scene that is missing is created again under the saved name.
5. **Step ids.** Camera 1 keeps the ids `scene`, `camera`, `old-camera`, `order` and `fit`, so the contract's list and the existing panel stay valid. Cameras 2 and 3 use the same ids with `-2` and `-3` (`scene-2`, `camera-2`, `old-camera-2`, `order-2`, `fit-2`). The `Grafismo` scene is a new step, `graphics`. `overlay` now means the overlay inside `Grafismo`. `order` now means "Grafismo is in the camera scene, shown, above the camera".
6. **Program scene.** The `program` step is `ok` when any of the cameras' scenes is on program (a club on camera 2 must not be switched back, and the health check must not fail). Otherwise Configurar OBS sets `Câmara 1` (`later` while streaming or recording).
7. **The old-layout check is a fallback of the health check only.** With `apply` false, when the new layout is not ready and `CameraList()` has one entry, the check runs the old checks on `Hallstream` (overlay in it and shown, the camera in it and shown, overlay above camera, the canvas, and the program scene being `Hallstream` or any camera scene). When those pass, the health report is the old layout's report and `ready` is true. Configurar OBS itself always builds the new layout.
8. **Cameras 2 and 3 come only from settings.** In this plan the panel still picks camera 1 only. Configurar OBS builds a scene for every camera saved in `cameras` (`"source:<name>"` choices). A saved camera 2 or 3 whose source is gone fails its `camera-N` step and is not replaced by an automatic pick. Plan 011 adds the panel and the other kinds of camera.
9. **Labels.** `cameras[i].label` is saved as `""` by this plan. The panel shows "Câmara N" (`painel.camaras.camara`) when it is empty. Plan 011 lets the volunteer type one.

## Current state

### Files and their roles (line numbers at `b28fb61`)

- `internal/store/store.go:23-41`: `type OBS struct` with `TimeoutScene`, `GameScene`, `CameraSource string json:"camera_source"`, `CameraFit`, `CameraFull *obsws.Full json:"camera_full"`, `SetupAt`, `Launch`, `CloseOnExit`. Defaults at line 231-232: `OBS{Host: "127.0.0.1", Port: 4455, GameScene: "Jogo", CameraFit: "contain"}`.
- `internal/obsws/setup.go`: the whole of Configurar OBS and the health check.
  - Constants (lines 15-22): `SceneName = "Hallstream"`, `OverlayName = "Hallstream overlay"`, `PluginKind`, `dshowKind`, `probeName`.
  - `Want` (lines 45-52): `OverlayURL`, `OverlayPorts [2]int`, `Camera string` (a choice value or ""), `Old string`.
  - `Report` (lines 55-61): `Steps`, `Camera`, `Choices`, `Ready`.
  - `Setup(cfg, w, apply, fit func(camera string) error)` (lines 80-93) runs `s.run(fit)` and sets `Ready` unless a step other than `fit` is `failed` or `choose`.
  - `run` (lines 150-174): `busy()`, `ensureScene()`, then `s.scene = SceneName`, `inputs()`, `overlay(in)`, `camera(in)`, `hideOld`, `order`, `video(busy)`, `program(busy)`, then `fit(cam)` as the `fit` step.
  - Every helper is tied to the one scene through the constant: `ensureScene` (186-213), `items()` (238-246, `GetSceneItemList` on `SceneName`), the overlay's `CreateInput` (310), `show` (360-368), `camera`'s messages (483), `auto` (514), `addDevice` (623), `addPlugin` (636), `hideOld` (666), `order` (688), `program` (731-760). `s.scene` (line 122) exists but only says where the device probe goes.
  - `uniqueName(in []input, name)` (370-379) avoids input names only.
  - `program(busy)` (730-760) is `ok` only when the program scene is `SceneName`.
- `internal/obsws/camera.go`:
  - `CameraSettings` (91-100) has one `Source` and one `Full *Full`.
  - `Camera.Save func(Full)` (117) keeps a new full-screen placement.
  - `FitNow()` (258-270) resolves the camera in the **program** scene (`resolve` → `items` → `GetCurrentProgramScene`, lines 313-329 and 367-411) and puts it at full screen.
  - `full(set, it)` (415-429) reuses `set.Full` when the source size, canvas and fit match, else makes and saves a new one.
  - `sceneItem` (148-162) decodes `inputKind` as `Kind`; a nested scene's item has no input kind, so `Kind` is `""` there.
- `internal/web/obs.go`:
  - `want(o, camera, old)` (46-56) builds `obsws.Want` from `o.CameraSource`.
  - `obsSetup` (211-254) runs `s.OBS.Setup(cfg, s.want(o, req.Camera, req.Old), true, s.fitCamera)` under `setupOBSMu`, then on `rep.Ready` saves `SetupAt`, `Launch`, `Enabled`.
  - `fitCamera(cam)` (258-278) saves `CameraSource = cam` (clearing `CameraFull` when it changed) and calls `s.Camera.FitNow()` up to 6 times, 1.5 s apart.
  - `checkOBS` (416-427) runs `s.OBS.Setup(obsConfig(o), s.want(o, "", ""), false, nil)` as the health check; `RunOBS` calls it every 30 s (line 473-475).
- `internal/web/camera.go:53-86`: `newCamera` wires `Settings` (`Source: o.CameraSource`, `Full: o.CameraFull`) and `Save` (writes `set.OBS.CameraFull = &f`).
- `internal/web/server.go:545-551` (inside `PUT /api/settings`): keeps `CameraFull`, clears it when `CameraSource` changed, saves any fit other than `"fill"` as `"contain"`. Everything else in `req.OBS` replaces the saved settings (`*cur = req`, line 552), so a field the panel does not send would be wiped.
- `web/painel/painel.js`: `setupHtml` (around line 1291) renders `r.steps` generically and `r.choices`; nothing reads step ids. `readSettingsForm` (2537-2547) starts from `clone(S.settings)`, so `obs.cameras` and the other new keys pass through a save unchanged.
- `web/painel/mock.js`: `db.settings.obs` (line 26-27); `setupReport(b)` (254-277) returns the old one-scene steps.
- `web/i18n/pt.json`: `obs.setup.*` keys (lines 452-506), for example `"obs.setup.cena": "Cena {cena}"`, `"obs.setup.ordem": "Grafismo por cima da câmara"`, `"obs.setup.no_ar_ok": "O OBS mostra a cena {cena}."`; `"painel.definicoes.obs_dica"` (line 924) says Configurar OBS "cria a cena <b>Hallstream</b> com o grafismo por cima da câmara".
- Tests:
  - `internal/obsws/setup_test.go`: `model` (lines 12-217) is a small OBS; `answer` handles each request. Its `GetSceneItemList` (136-145) calls `m.input(it.source).kind`, which panics for a nested scene (no input). `run.setup/health` (260-277) build `Want{…, Camera: camera, Old: old}`. `again` (307-325) asserts a second run changes nothing and the health check passes. Tests: `TestSetupEmptyWithVirtualWebcam`, `TestSetupExistingUserSetup`, `TestSetupPlugin`, `TestSetupNoCamera`, `TestSetupWebcamOnly`, `TestSetupTwoDevicesPick`, `TestSetupSwapCamera`, `TestSetupStreamingWrongSize`, `TestSetupWrongOverlayPort`, `TestHealthCameraGone`, `TestSetupSwapFromNDI`. Most assert `m.order(SceneName)`.
  - `internal/obsws/camera_test.go`: `rig`/`fake` for the squeeze; `TestFitNowOtherCanvas` (330).
  - `internal/web/camera_test.go:29-49` `TestSettingsKeepCamera`: `PUT /api/settings` keeps `camera_full` and clears it when `camera_source` changes.

### obs-websocket 5.x facts this plan relies on

From the official protocol doc, https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md:

- `CreateScene` takes `sceneName`. `GetSceneList` returns `scenes[].sceneName`.
- `CreateSceneItem` takes `sceneName` and `sourceName`, optional `sceneItemEnabled` (default true), and returns `sceneItemId`. The source may be a scene: that is how a scene is nested. The doc's own advice under `GetGroupSceneItemList`: "Using groups at all in OBS is discouraged … Please use nested scenes instead."
- `GetSceneItemList` takes `sceneName` and returns `sceneItems`, an array of objects. The doc does not list the object's fields. The code already decodes `sceneItemId`, `sceneItemIndex`, `sourceName`, `inputKind`, `sceneItemEnabled`, `sceneItemTransform`. For a nested scene, OBS 30 sends `inputKind: null` and `sourceType: "OBS_SOURCE_TYPE_SCENE"`. Treat a `""` `Kind` as "not a camera" (the code already does through `isCapture` and `isVideo("")` being false).
- `SetCurrentProgramScene` takes `sceneName`. `GetCurrentProgramScene` returns `sceneName` (and the deprecated `currentProgramSceneName`, which the code already falls back to).
- OBS source names are unique across inputs and scenes: `CreateScene` with a name an input already has fails (status 601, `ResourceAlreadyExists`).

### Conventions

- Go standard library plus `golang.org/x/net/websocket` (already used). Code and comments in English. Tests in plain `testing`.
- The setup never deletes or changes a user's source other than the overlay's settings (`setup.go:74-79`). Keep that true: the new scenes are the program's; `Hallstream`, `Jogo` and every user scene stay as they are.
- Strings: `i18n.T("key", "name", value)` in Go, `t('key', { name: value })` in the panel; placeholders are `{name}`. `go test ./internal/i18n` fails on a key missing in either file or a key built at run time, so never build a key from a number: use one key with an `{n}` placeholder.
- `docs/CONTRACT.md` changes in the same commit as the behaviour. Never read or edit `dados/` or `dist/`.
- Commits: conventional (`feat:`), one per step.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| Setup tests | `go test ./internal/obsws/ -run 'Setup|Health' -v` | all pass |
| Web tests | `go test ./internal/web/` | pass |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Panel mock | `go run ./cmd/hallstream -demo -nobrowser`, then `http://127.0.0.1:8765/painel?mock=1&obs=choose` | the setup checklist shows the new steps |

## Suggested executor toolkit

- `tdd` for Steps 2 to 4: write the new setup tests on the `model` first, watch them fail, then change `setup.go`.
- No UI design work in this plan: the panel only gets new copy and mock data.

## Scope

**In scope**:

- `internal/store/store.go` (the `OBS` fields, `Camera` type, `CameraList`, `FullFor`)
- `internal/obsws/setup.go`, `internal/obsws/setup_test.go`
- `internal/obsws/camera.go` (`FitIn`, `Save` by source), `internal/obsws/camera_test.go` (new tests; adjust the `Save` signature in the rig)
- `internal/web/obs.go` (`want`, `obsSetup`, `fitCamera`, `checkOBS`), `internal/web/camera.go` (`Settings`, `Save`), `internal/web/server.go` (the `PUT /api/settings` lines only)
- `internal/web/camera_test.go`, `internal/web/obs_test.go` (new tests only)
- `web/painel/mock.js` (settings and `setupReport`), `web/painel/painel.js` (only if a string key changes)
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `LEIA-ME.txt`, `READ-ME.txt` (only lines that name the `Hallstream` scene, if any)

**Out of scope**:

- Switching cameras, the panel's camera buttons, `obs.live_camera` (plan 009).
- The time-out camera (plan 010).
- Adding cameras in Definições or the wizard, VDO.Ninja, `cameras[i].vdo` (plan 011).
- Camera health lines (plan 012) and the guide (plan 013).
- The squeeze (`Camera.Follow`, `squeeze`, `move`): it keeps working on the program scene, which is now `Câmara 1`. Do not change it here.
- The old two-scene mode (`timeout_scene`, `game_scene`): unchanged.
- Deleting or renaming `Hallstream` or any user scene.

## Git workflow

- Branch: `02-feat/camera-scenes`, stacked on `01-docs/phase-b-plans` (see the `stacked-prs` skill if the owner asks for `gh stack`).
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The settings shape

In `internal/store/store.go`:

```go
// MaxCameras is how many cameras the program drives.
const MaxCameras = 3

// Camera is one camera: its OBS source, the scene the program made for it
// and the name the volunteer gave it ("" shows "Câmara N").
type Camera struct {
	Source string `json:"source"`
	Scene  string `json:"scene"`
	Label  string `json:"label"`
}
```

Add to `OBS`, after `CameraFull`:

```go
	// Cameras are up to MaxCameras cameras, camera 1 first; empty in
	// settings from before 0.2 (see CameraList). Only the server writes them.
	Cameras []Camera `json:"cameras"`
	// GraphicsScene is the scene that holds the overlay, nested in each
	// camera's scene; "" until Configurar OBS made it.
	GraphicsScene string `json:"graphics_scene"`
	// CameraFulls is each camera's last full-screen placement, by source
	// name. CameraFull keeps camera 1's too, for older versions.
	CameraFulls map[string]*obsws.Full `json:"camera_fulls"`
```

And two helpers:

```go
// CameraList is the cameras, or camera_source alone for settings from
// before there were several.
func (o OBS) CameraList() []Camera

// FullFor is source's saved full-screen placement: camera_fulls, else
// camera_full when source is camera 1.
func (o OBS) FullFor(source string) *obsws.Full
```

`CameraList` returns a copy (never the backing array), at most `MaxCameras` entries. With `Cameras` empty it returns `[]Camera{{Source: o.CameraSource}}`, even when `CameraSource` is `""` (automatic).

`DefaultSettings` needs no change (`Cameras` nil, `GraphicsScene` "", `CameraFulls` nil).

**Verify**: `go test ./internal/store/` → pass, with new tests in `internal/store/store_test.go` (create the test if the file has no OBS tests; model on the existing tests there): `CameraList` of an `OBS{CameraSource: "DroidCam"}` is `[{DroidCam "" ""}]`; of four cameras is the first three; changing the returned slice does not change `o.Cameras`; `FullFor("DroidCam")` returns `CameraFulls["DroidCam"]` when set, else `CameraFull` when `CameraSource == "DroidCam"`, else nil. A `settings.json` without the new keys loads with them empty.

### Step 2: The setup works on a named scene

Refactor `internal/obsws/setup.go` with **no change in behaviour** first, so the old tests still pass.

- Every helper that names `SceneName` takes the scene from the `setup` instead. Give `setup` a field `at string` ("the scene being checked") and use it in `items()`, `show()`, the overlay's `CreateInput`, `addDevice`, `addPlugin`, `hideOld`, `order`, and the messages that name the scene. Set `s.at = SceneName` in `run` where `s.scene = SceneName` is set today (line 155). Keep `s.scene` for the device probe as it is.
- `ensureScene()` becomes `ensureScene(id, name string) (string, bool)` with the step id and the name, returning the name it ended with. For now `run` calls `s.ensureScene("scene", SceneName)`.
- `uniqueName` gets a sibling `uniqueSourceName(in []input, scenes []string, name string)` that also avoids scene names. Leave `uniqueName` for inputs.

**Verify**: `go test ./internal/obsws/` → every existing test passes unchanged. `git diff --stat` touches only `setup.go`.

### Step 3: Configurar OBS builds Grafismo and the camera scenes

Change `Want` and `Report`:

```go
// WantCamera is one camera the setup aims for.
type WantCamera struct {
	// Choice is a Choice value, or "" to find one (camera 1 only).
	Choice string
	// Scene is the saved scene name; "" before the first setup.
	Scene string
	// Old is the camera before, hidden in the scene when another replaces it.
	Old string
}

type Want struct {
	OverlayURL   string
	OverlayPorts [2]int
	// Graphics is the saved overlay scene name; "" before the first setup.
	Graphics string
	// Cameras is 1 to 3 cameras, camera 1 first.
	Cameras []WantCamera
}

// ReportCamera is the scene and source a camera ended with.
type ReportCamera struct {
	Source string `json:"source"`
	Scene  string `json:"scene"`
}

type Report struct {
	Steps    []Step         `json:"steps"`
	Camera   string         `json:"camera"` // camera 1's source, as before
	Graphics string         `json:"graphics"`
	Cameras  []ReportCamera `json:"cameras"`
	Choices  []Choice       `json:"choices,omitempty"`
	Ready    bool           `json:"ready"`
}
```

`Setup`'s `fit` becomes `fit func(n int, scene, source string) error` (n is 1-based). `Ready` ignores every step whose id starts with `fit`.

New `run(fit)` (apply or check), in this order:

1. `busy := s.busy()`.
2. Read `GetSceneList` once into `s.scenes []string` (a failure is the `graphics` step failing, as `ensureScene` does today).
3. **`graphics` step**: name = `want.Graphics`, else `i18n.T("obs.cena_grafismo")` made unique with `uniqueSourceName` against the inputs and scenes. Label `i18n.T("obs.setup.cena_grafismo", "cena", name)`. Create it with `CreateScene` when missing (apply), else `failed` with `obs.setup.falta_cena`. Save the name in `s.rep.Graphics` whenever the scene exists at the end of the step.
4. **`overlay` step** with `s.at = graphics`: the existing `overlay(in)` logic. The overlay input may already exist elsewhere (`Hallstream`, `Jogo`): it is found by `isOverlay` as today and put into `Grafismo` with `show` (`CreateSceneItem`). A new one is created in `Grafismo`.
5. For each camera `n` (1-based) in `want.Cameras`:
   - **`scene` / `scene-N` step**: `want.Cameras[n-1].Scene`, else `i18n.T("obs.cena_camara", "n", n)` made unique. Label `i18n.T("obs.setup.cena", "cena", name)` (existing key). Created when missing.
   - **`camera` / `camera-N` step** with `s.at = that scene`: the existing `camera(in)` logic with `want.Cameras[n-1].Choice`. Camera 1 keeps the automatic pick (`auto`). `auto` looks for "a camera already in the scene" in the camera's scene first, then in `SceneName` when that scene exists, so a club moving from the old layout keeps its camera. For n ≥ 2, an empty `Choice` or a missing source fails the step (`obs.setup.camara_n_sumiu`, below); never pick automatically. A camera already used by an earlier camera in this run fails too (`obs.setup.camara_repetida`). The step label is `obs.setup.camara` for camera 1 and `i18n.T("obs.setup.camara_n", "n", n)` for the others.
   - **`old-camera` / `old-camera-N`**: `hideOld(cam, graphics)` in that scene with `want.Cameras[n-1].Old`. Skip the `Grafismo` item (`it.Name == graphics`) as it skips the overlay today.
   - **`order` / `order-N`**: `Grafismo` must be in the camera scene, shown, and above the camera. With apply, put it there (`show`, then `SetSceneItemIndex` to the top as `order` does now). New detail keys `obs.setup.grafismo_na_cena` (check fails: "A cena {cena} não tem a cena {grafismo}.") and `obs.setup.pus_grafismo` (fixed: "Pus a cena {grafismo} por cima da câmara na cena {cena}.").
   - Append `ReportCamera{Source, Scene}`; for n == 1 also set `s.rep.Camera`.
6. `video(busy)` as today.
7. **`program` step**: `ok` when the program scene is any scene in `s.rep.Cameras`. Else, busy → `later`; check → `failed`; apply → `SetCurrentProgramScene` to camera 1's scene (`fixed`). Detail keys stay; `{cena}` is camera 1's scene.
8. With apply and `fit != nil`, for each camera with a source: a `fit` / `fit-N` step that calls `fit(n, scene, source)`, as today's `fit` step does.

Never call `CreateScene` for a name in `s.scenes`. Never write to a scene that is not `Grafismo` or a camera scene, except the overlay input's own settings (as today). `Hallstream` stays as it is.

New pt keys (then en, same keys):

| Key | pt | en |
|---|---|---|
| `obs.cena_grafismo` | `Grafismo` | `Graphics` |
| `obs.cena_camara` | `Câmara {n}` | `Camera {n}` |
| `obs.setup.cena_grafismo` | `Cena do grafismo ({cena})` | `Graphics scene ({cena})` |
| `obs.setup.camara_n` | `Câmara {n}` | `Camera {n}` |
| `obs.setup.camara_n_sumiu` | `A câmara {n} ({camara}) já não existe no OBS. Escolha outra em Definições.` | `Camera {n} ({camara}) is no longer in OBS. Pick another in Settings.` |
| `obs.setup.camara_repetida` | `A câmara {n} é a mesma fonte da câmara {outra}.` | `Camera {n} is the same source as camera {outra}.` |
| `obs.setup.grafismo_na_cena` | `A cena {cena} não tem a cena {grafismo} por cima da câmara.` | `Scene {cena} doesn't have scene {grafismo} above the camera.` |
| `obs.setup.pus_grafismo` | `Pus a cena {grafismo} por cima da câmara na cena {cena}.` | `I put scene {grafismo} above the camera in scene {cena}.` |

Change `painel.definicoes.obs_dica` to say it creates the scene **Grafismo** with the overlay and one scene per camera with the graphics above it (pt first, then en). Keep the rest of the sentence.

Update `internal/obsws/setup_test.go`:

- `model.answer` `GetSceneItemList`: when `m.input(it.source)` is nil and `it.source` is a scene, send `"inputKind": nil, "sourceType": "OBS_SOURCE_TYPE_SCENE"`. `CreateSceneItem` with a `sourceName` that is neither an input nor a scene answers `false, NotFound`. `CreateScene` also refuses a name an input has (601).
- `run.setup(camera, old)` and `run.health(camera)` build `Want{…, Cameras: []WantCamera{{Choice: camera, Old: old, Scene: r.scene}}, Graphics: r.graphics}` and keep `r.scene`, `r.graphics` from the last report (`rep.Cameras[0].Scene`, `rep.Graphics`), as the server saves them.
- Existing tests: replace every `m.order(SceneName)` with `m.order(r.scene)` (the camera scene) and check `m.order(r.graphics)` is `[OverlayName]` (or the user's overlay name). `wantResults` maps keep their keys; add `"graphics": "created"` where `"scene": "created"` is expected. `m.program` becomes the camera scene. Change nothing else in their assertions. These are the only edits allowed to existing tests in this plan.

New tests in `setup_test.go`:

- `TestSetupBuildsGraphicsAndCameraScene`: empty OBS with one device. Scenes `Grafismo` and `Câmara 1` exist; `Grafismo` holds only the overlay; `Câmara 1` is `[camera, "Grafismo"]` bottom first; program is `Câmara 1`; `rep.Graphics == "Grafismo"`, `rep.Cameras == [{camera, "Câmara 1"}]`; `r.again(...)` passes.
- `TestSetupFromOldLayout`: model with `Hallstream` = `[Tablet (dshow), Hallstream overlay]` on program. Setup with `Choice: "source:Tablet"`. `Hallstream` is unchanged (same order, same enabled flags, no write names it: check `m.writes` holds no request whose `sceneName` was `Hallstream`; record sceneName per write in the model if needed); `Câmara 1` = `[Tablet, Grafismo]`; `Grafismo` = `[Hallstream overlay]`; program `Câmara 1`.
- `TestSetupThreeCameras`: three dshow inputs in a user scene; `Cameras` choices `source:A`, `source:B`, `source:C`. Three camera scenes, each `[X, Grafismo]`; `fit` called for 1, 2, 3 with the right scene and source; `again` passes.
- `TestSetupCameraTwoGone`: camera 2's source missing → `camera-2` `failed` with `obs.setup.camara_n_sumiu` text, `Ready` false, camera 1 fine.
- `TestSetupSameSourceTwice`: cameras 1 and 2 both `source:A` → `camera-2` `failed`.
- `TestSetupNameTaken`: the user has an input called `Grafismo` (as in `TestSetupExistingUserSetup`) and a scene called `Câmara 1`. The new scenes are `Grafismo 2` and `Câmara 1 2`; the user's `Câmara 1` is unchanged.
- `TestSetupSavedNamesWin`: `Want.Graphics = "Graphics"`, scene `"Camera 1"` (an English setup), panel now in Portuguese. Setup finds them and creates nothing.
- `TestSetupKeepsCameraTwoLive`: after a two-camera setup, set `m.program` to camera 2's scene. `again` passes and no `SetCurrentProgramScene` is written.
- `TestSetupStreamingKeepsProgram`: streaming, program `Hallstream` (old layout): the new scenes are built, `program` is `later`, `SetCurrentProgramScene` is never written.

**Verify**: `go test ./internal/obsws/ -run 'Setup|Health' -v` → all pass, new ones included. `go test ./internal/i18n` → pass.

### Step 4: The health check accepts the old layout

In `Setup`, when `apply` is false and the report is not ready and `len(w.Cameras) <= 1`, run `s.runLegacy()`: a second `setup` value with `at = SceneName`, using the same helpers in check mode (`ensureScene("scene", SceneName)`, `overlay`, `camera`, `order` on the overlay input, `video`, and a `program` check that is `ok` for `SceneName` or any camera scene). If its report is ready, return it (with `Ready` true and its own steps). Else return the new layout's report: its problems name what Configurar OBS would build, which is what "Reparar" fixes.

New tests:

- `TestHealthOldLayoutReady`: model as a 0.1.2 club left it (`Hallstream` with camera and overlay, on program, 1920x1080). `health` with `Want{Cameras: [{Choice: "source:Tablet"}]}` and no `Graphics` → `Ready` true, no writes.
- `TestHealthOldLayoutTwoCameras`: same model, two cameras in `Want` → `Ready` false.
- `TestHealthOldLayoutBroken`: `Hallstream` without the overlay → `Ready` false, and the problems are the new layout's (they mention `Grafismo`).
- `TestHealthAfterStreamingSetup`: run `TestSetupStreamingKeepsProgram`'s setup, then `health` → `Ready` true (new layout built, program still `Hallstream`, fallback passes).

**Verify**: `go test ./internal/obsws/` → pass.

### Step 5: Fitting a camera that is not on program

In `internal/obsws/camera.go`:

- `Save func(Full)` becomes `Save func(source string, f Full)`.
- `full(set, it)` calls `c.Save(it.name, f)`.
- `resolve(set)` takes the scene: `resolve(set CameraSettings, scene string)`; `scene == ""` keeps today's `GetCurrentProgramScene`. Split `items(cfg)` the same way (`items(cfg, scene)`).
- New `func (c *Camera) FitIn(scene, source string) (FitReport, error)`: as `FitNow`, on that scene and source, with `set.Full = nil`. `FitNow()` stays and calls the same code with `""` and `set.Source`.
- Do not change `Follow`, `squeeze`, `Check`, `move` or `run`.

In `internal/obsws/camera_test.go`, change the rig's `Save` to the new signature; add `TestFitInOtherScene`: the fake's program is `Jogo` but `FitIn("Câmara 2", "DroidCam")` sends `SetSceneItemTransform` with `sceneName` `Câmara 2`. The `fake` in `obsws_test.go` answers `GetSceneItemList` with one list whatever the scene: give it an optional `byScene map[string][]map[string]any` used when set.

**Verify**: `go test ./internal/obsws/` → pass.

### Step 6: The server saves the layout

In `internal/web/camera.go` `newCamera`:

- `Settings`: `Source: o.CameraSource`, `Full: o.FullFor(o.CameraSource)`.
- `Save(source, f)`: in one `UpdateSettings`, `set.OBS.CameraFulls[source] = &f` (make the map when nil), and when `source == set.OBS.CameraSource` also `set.OBS.CameraFull = &f`.

In `internal/web/obs.go`:

- `want(o, camera, old)` builds `Want.Graphics = o.GraphicsScene` and one `WantCamera` per `o.CameraList()` entry: camera 1's `Choice` is `camera`, else `"source:"+source` when its source is set, else `""`; its `Old` is `old`, else `o.CameraSource`. Cameras 2 and 3: `Choice: "source:"+source`, `Scene: scene`.
- `fitCamera(n int, scene, source string) error`: save first (below), then `s.Camera.FitIn(scene, source)` with the same retry as today.
- After `s.OBS.Setup` in `obsSetup` (whether or not `rep.Ready`, as long as `err == nil`), one `UpdateSettings`:
  - `GraphicsScene = rep.Graphics` when it is not `""`.
  - `Cameras`: start from `o.CameraList()`; for each `rep.Cameras[i]`, set `Scene` and, when not `""`, `Source`. Save it as `Cameras` (this is where a 0.1.2 club's settings first get a `cameras` list).
  - `CameraSource = Cameras[0].Source`; when it changed, `CameraFull = nil` as `fitCamera` does today.
  - Make the save happen before the `fit` steps run: do it inside `fitCamera` for `n == 1` too, so `Camera.Settings` sees camera 1's new source while fitting. Keep one source of truth: write a small `s.saveCameras(rep obsws.Report)` used by both.
- `checkOBS` builds `want(o, "", "")` as now; it now carries the saved scenes.

In `internal/web/server.go` `PUT /api/settings` (lines 545-551), next to the `CameraFull` lines:

```go
req.OBS.Cameras, req.OBS.GraphicsScene, req.OBS.CameraFulls = cur.OBS.Cameras, cur.OBS.GraphicsScene, cur.OBS.CameraFulls
if req.OBS.CameraSource != cur.OBS.CameraSource && len(req.OBS.Cameras) > 0 {
	req.OBS.Cameras = slices.Clone(req.OBS.Cameras)
	req.OBS.Cameras[0].Source = req.OBS.CameraSource
}
```

New tests in `internal/web/camera_test.go` (model on `TestSettingsKeepCamera`):

- `TestSettingsKeepCameras`: a `PUT` without `cameras`, `graphics_scene` and `camera_fulls` keeps them; a `PUT` with other values for them keeps the saved ones; a `PUT` with a new `camera_source` changes `cameras[0].source` and keeps `cameras[0].scene`.
- `TestSaveFullByCamera`: calling `s.Camera.Save("B", f)` fills `camera_fulls["B"]` and leaves `camera_full` alone when B is not camera 1; `Save("A", f)` with A camera 1 fills both.

A web test for `POST /api/obs/setup` against a fake OBS is optional: `internal/web/obs_test.go`'s `fakeOBSData` answers one request at a time and the `model` lives in another package. If you write one, check only that the saved settings get `graphics_scene`, `cameras[0].scene` and `camera_source` from a canned report; do not rebuild the model.

**Verify**: `go test ./internal/web/ ./internal/store/` → pass.

### Step 7: The panel mock and the contract

- `web/painel/mock.js`: add `cameras: [{ source: 'DroidCam Video', scene: 'Câmara 1', label: '' }], graphics_scene: 'Grafismo', camera_fulls: {}` to `db.settings.obs` (keep `camera_source` equal to camera 1). `setupReport`: replace the `scene` step with `graphics` ("Cena do grafismo (Grafismo)", "Criei a cena Grafismo.") and `scene` ("Cena Câmara 1", "Criei a cena Câmara 1."), the `order` detail with "Pus a cena Grafismo por cima da câmara na cena Câmara 1.", the `program` detail with "O OBS mostra a cena Câmara 1."; add `graphics: 'Grafismo', cameras: [{ source: 'DroidCam Video', scene: 'Câmara 1' }]` to both replies, and set `db.settings.obs.graphics_scene` and `cameras[0].scene` in the ready branch. The `obs=choose` problem text becomes "A cena Câmara 1 não tem câmara."
- `docs/CONTRACT.md`:
  - "Panel API", `GET /api/settings`: add `"cameras": [], "graphics_scene": "", "camera_fulls": null` to the `obs` example, and a paragraph: what each field is, `CameraList` (empty means camera 1 is `camera_source`), at most 3, server-owned in `PUT` (kept), `cameras[0].source` follows `camera_source`.
  - "Time-out squeeze", Settings bullet: `camera_fulls` is each camera's placement by source; `camera_full` stays camera 1's.
  - "OBS set up by the program": rewrite the second and third bullets for the new layout (`Grafismo` with the overlay; one scene per camera with the camera and `Grafismo` above it; names in the panel's language at the first setup, unique against existing names, saved and found by the saved name after; `Hallstream` and every user scene untouched; program `Câmara 1` unless a camera scene is already on program; `later` while streaming or recording). Reply: add `graphics` and `cameras`. Step ids: add `graphics` and the `-2`/`-3` ids; say what `order` means now. Health check: add the old-layout fallback (Decision 7).
- `LEIA-ME.txt` / `READ-ME.txt`: no line names the `Hallstream` scene today (checked at `b28fb61`); change nothing unless a grep finds one: `grep -n "Hallstream\"" LEIA-ME.txt READ-ME.txt`.

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0. Open `http://127.0.0.1:8765/painel?mock=1&obs=choose`, Definições, Configurar OBS, pick a camera: the checklist shows "Cena do grafismo (Grafismo)" and "Cena Câmara 1".

## Test plan

- `internal/store`: `CameraList`, `FullFor` (Step 1).
- `internal/obsws/setup_test.go`: the nine new setup tests (Step 3) and four health tests (Step 4); existing tests adapted only as Step 3 lists.
- `internal/obsws/camera_test.go`: `TestFitInOtherScene` (Step 5).
- `internal/web/camera_test.go`: `TestSettingsKeepCameras`, `TestSaveFullByCamera` (Step 6).
- Manual, with a real OBS 30 or 31 on a scratch build (`go build -o <scratch>/hallstream ./cmd/hallstream`, never your real `dados/`): a fresh OBS gets `Grafismo` and `Câmara 1`; the overlay draws over the camera in `Câmara 1`; a time-out squeezes the camera in `Câmara 1`; an OBS with the old `Hallstream` scene keeps it untouched.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n '"graphics_scene"' internal/store/store.go docs/CONTRACT.md` matches in both
- [ ] `grep -n '"obs.cena_camara"' web/i18n/pt.json web/i18n/en.json` matches in both
- [ ] `git diff b28fb61..HEAD -- internal/obsws/camera.go | grep -c 'func (c \*Camera) Follow'` is 0 (the squeeze is untouched)
- [ ] Existing tests changed only as Step 3 lists (review `git diff b28fb61..HEAD -- '*_test.go'`)
- [ ] `plans/README.md` status row updated

## STOP conditions

- An existing test other than the `setup_test.go` ones listed in Step 3 fails, and only editing it would make it pass.
- A real OBS answers `CreateSceneItem` with a scene as `sourceName` with an error (nesting refused). Report the OBS version and the error.
- A real OBS sends a nested scene's item with an `inputKind` that `isCapture` or `isVideo` counts as a camera.
- The refactor in Step 2 can't keep the old tests passing without changing them.
- The settings save in Step 6 races the fit (the fit reads an old `camera_source`) in a way one `UpdateSettings` before `FitIn` doesn't fix.
- A verification fails twice after a reasonable fix.

## Open questions / corrections

1. **`camera_full` per camera has no field name in the decisions file.** This plan adds `camera_fulls` (map by source) and keeps `camera_full` as camera 1's mirror, for the same downgrade reason Decision 1 gives for `camera_source`. Plans 009 to 013 should use `camera_fulls`. Recommended: accept.
2. **Who writes `cameras`.** The decisions file doesn't say whether the panel or the server owns `cameras` in `PUT /api/settings`. This plan makes it server-owned (kept on every `PUT`, only `cameras[0].source` follows `camera_source`), because no panel edits it yet and a panel page from 0.1.2 would otherwise wipe it. Plan 011 must change this rule when Definições > Câmaras starts editing the list; it should still keep each camera's `scene` from the saved settings, matched by index.
3. **A scene name that is already taken** is not covered by Decision 2. This plan never adopts a user's scene: it takes the next free name ("Grafismo 2"). Recommended: accept; the alternative (adopting) would add the overlay to a user's own scene, which breaks the "never change a user's scene" rule.
4. **The decisions file says the health check accepts the old layout "while only one camera is set".** That holds here through `CameraList()`, which is one entry for every 0.1.2 club. A club that runs the new setup and later loses `Grafismo` (deleted in OBS) is still counted ready while `Hallstream` is intact and only one camera is set. That matches the decision's aim (no red chip on game day); flagging it so the owner knows.
5. **Facts line "The health check runs the same steps without changes every 30 s"** holds: `RunOBS` calls `refreshOBS` every 10th 3 s tick (`internal/web/obs.go:473-475`).

## Maintenance notes

- Plan 009 reads `cameras[i].scene` to switch scenes and to know the live camera; it must treat `Hallstream` on program as camera 1 while `CameraList()` has one entry (same rule as Decision 7).
- Plan 011 adds `cameras[i].vdo` and other choice kinds for cameras 2 and 3. `WantCamera.Choice` takes any choice value, so `device:` and `plugin` for cameras 2 and 3 need only the `n ≥ 2` branch in Step 3 relaxed.
- A user who renames `Câmara 1` in OBS orphans the saved name; the next Configurar OBS creates `Câmara 1` again under the saved name. OBS sends a `SceneNameChanged` event (protocol doc, "Scenes" events) that a later plan could follow.
- Reviewers: run Configurar OBS twice on a real OBS (the second run must change nothing), once with the old `Hallstream` scene present, and once while streaming. Check the chip stays "Pronto" on a club that never re-runs the setup.
