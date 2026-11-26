# Plan 009: The panel switches cameras and shows which one is live

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- internal/obsws internal/state/state.go internal/store/store.go internal/web cmd/hallstream/main.go web/painel web/i18n docs/CONTRACT.md LEIA-ME.txt READ-ME.txt`
> Plan 008 is expected to have changed `internal/obsws`, `internal/store`,
> `internal/web/obs.go`, `internal/web/camera.go`, `internal/web/server.go`,
> `web/painel/mock.js`, `web/i18n` and the contract. First confirm it landed
> (see "Depends on"), then compare every "Current state" excerpt with the
> live code. Line numbers are from `b28fb61` and will have moved; a change you
> can map one-to-one is fine, one you can't is a STOP condition.

## Status

- **Priority**: P1 (ROADMAP Phase B item 2)
- **Effort**: L
- **Risk**: HIGH (a scene switch is on air; the time-out squeeze must still line up with the overlay's hole)
- **Depends on**: `plans/008-camera-scenes.md`. Check: `grep -n "func (o OBS) CameraList" internal/store/store.go` and `grep -n "func (c \*Camera) FitIn" internal/obsws/camera.go` both match. Both must hold or STOP.
- **Category**: direction
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

After plan 008, Configurar OBS builds one scene per camera, but nothing switches between them except a click in OBS on the laptop. ROADMAP Phase B item 2: "The panel gets camera buttons and shows which camera is live." The volunteer runs the game from the panel, often from a phone, and can't reach OBS mid-game.

After this plan:

- Ecrãs has a camera button per camera (two or three). The live one is marked.
- A button calls `POST /api/obs/camera/live` `{ "index" }`, and OBS switches with its own transition.
- The program follows `CurrentProgramSceneChanged`, so a switch made in OBS shows in the panel too. The live state carries `obs.live_camera`.
- During a time-out (`desconto`), the camera being switched to is put in the time-out window first, so the overlay's hole and the picture still line up. The squeeze and the refit work on whichever camera is live.

Owner decisions this plan must honour (`plans/phase-b-decisions.md`):

- **Decision 4.** A camera button calls `POST /api/obs/camera/live` `{ "index" }`, which sets the program scene to that camera's scene with OBS's own transition. The live camera is read back from `CurrentProgramSceneChanged`. The live state carries `obs.live_camera`.
- **Decision 5.** The squeeze moves the camera item of whichever camera scene is on program. Switching cameras during `desconto` places the new camera at the window first, then switches. Each camera's full placement is saved per camera (plan 008's `camera_fulls`).
- **Decision 11.** Every new string in `pt.json` first, then `en.json`.
- **Decision 12.** The camera buttons go through `build-ui` (extension branch inside the panel's design system) with an outside critique.

## Decisions made in this plan (the executor does not re-decide them)

1. **The program listens on a second, events-only connection.** Today's client (`internal/obsws/obsws.go`) identifies with `eventSubscriptions: 0` (line 75) and its `receive` drops every message that is not the reply it waits for (lines 99-113). There is no reader between requests, so events can't arrive on it. A new `obsws.Watch` opens its own connection subscribed to the `Scenes` category only (`1 << 2`), asks `GetCurrentProgramScene` once on it, then reads `CurrentProgramSceneChanged` events until the connection drops. The request client stays as it is. See "Open questions / corrections", item 1.
2. **`obs.live_camera` is a top-level `obs` object in the live state.** The live state has no `obs` key today (`GET /api/status` has one, the live state has `camera`). This plan adds `"obs": { "live_camera": 0 }` to `state.State`. `0` means "not a camera scene, or not known" (OBS off, not connected, another scene on program); `1` to `3` is the camera. The overlay ignores it.
3. **How a program scene maps to a camera.** `cameras[i].scene` (non-empty) equal to the program scene gives `i+1`. While `CameraList()` has one entry, `Hallstream` (the old layout, `obsws.SceneName`) on program also gives `1`. Anything else gives `0`.
4. **One switch at a time.** A `switchMu` in the server serialises `POST /api/obs/camera/live`, and plan 010's time-out switch takes the same lock.
5. **Put the camera in place, then switch.** Before every switch the program puts the target camera where the screen needs it, at once (no tween): in the window when the camera is squeezed (`desconto`), at its full-screen placement otherwise. A camera left in the window in its own scene (switched away from during a time-out) is put back at full screen the next time it goes on air. A failure to place (no picture yet, source missing) is logged and returned as `warning`; it never blocks the switch.
6. **A switch made in OBS** (an event for a scene the program did not just ask for) places the new live camera the same way, after the fact, so a camera that sat in the window in its scene doesn't stay small on `marcador`. The program remembers the scene it asked for last (`expect`) and skips placing when the event is for that scene, so the event never cuts short a tween the program started.
7. **The panel shows camera buttons only with two or more cameras.** With one camera there is nothing to switch and Ecrãs looks as it does today. A camera with no scene yet (Configurar OBS not run since it was added) shows its button disabled with the hint to run Configurar OBS.
8. **The answer is sent before the event comes back.** After OBS accepts `SetCurrentProgramScene`, the server sets `obs.live_camera` itself; the event that follows confirms it. A refused switch leaves it as it was.
9. **The old two-scene mode wins during a time-out.** With `timeout_scene` set and present in OBS (`Camera.legacy`), the program scene during `desconto` is the user's time-out scene. A camera switch then would leave it, so `POST /api/obs/camera/live` answers 409 while the screen is `desconto` in that mode. Outside `desconto` it switches as usual.

## Current state

### Files and their roles (line numbers at `b28fb61`; plan 008 moves some)

- `internal/obsws/obsws.go`:
  - `dial(cfg)` (lines 51-88): dials, reads `Hello` (op 0), sends `Identify` (op 1) with `{"rpcVersion": 1, "eventSubscriptions": 0}` (line 75), reads `Identified` (op 2). Sets a 5 s deadline on the connection (line 63).
  - `send` (90-96), `receive(conn, op, into)` (98-113): reads until a message with `op` arrives, dropping the rest.
  - `Request` (149-167) and `request` (176-230): one connection under `c.mu`; a reply is matched by `requestId`.
  - `SetScene(cfg, scene)` (124-131) wraps `SetCurrentProgramScene` and turns a refusal into `obs.nao_mudou_cena` ("o OBS não mudou para a cena {cena}: {erro}").
- `internal/obsws/camera.go`:
  - `CameraSettings` (91-100): `Config`, `Enabled`, `Source`, `Fit`, `TimeoutScene`, `GameScene`, `Full`.
  - `Camera` (114-136): `follow` mutex and `in` (the last screen followed was `desconto`), `mu` with the tween state.
  - `Follow(screen)` (172-182) → `squeeze(in)` (184-213): legacy two-scene switch, else `resolve(set)` (the **program** scene) and tween with `move`.
  - `Check()` (218-247): refit while not moving and not `in`.
  - `resolve`, `items`, `full`, `window`, `show`, `move`, `run` (313-514). After plan 008, `resolve(set, scene)` and `items(cfg, scene)` take a scene (`""` = program), `Save` is `Save(source, f)`, and `FitIn(scene, source)` exists.
- `internal/web/camera.go`:
  - `newCamera` (53-86): `Settings` closure (after plan 008: `Source: o.CameraSource`, `Full: o.FullFor(o.CameraSource)`); `Show` writes `state.Camera` with `s.State.SetCamera`.
  - `followCamera()` (89-94): `s.Camera.Follow(s.State.Snapshot().Screen.Current)`.
  - `RunCamera()` (98-119): on every state change calls `followCamera`; every 4 s refits on `marcador`/`nada`.
  - `cameraRoutes()` (121-146): `GET /api/obs/sources`, `POST /api/obs/camera/fit`.
- `internal/web/obs.go`: `obsConfig(o)` (33-35); `RunOBS` (450-478) polls `GetStreamStatus` every 3 s and the health check every 30 s.
- `internal/web/server.go`: `s.OBS.OnConnect = s.refreshOBS` (line 126); `s.cameraRoutes()` (131); `POST /api/screen` → `s.screen` (618-648), which calls `followCamera` when OBS is enabled and returns `obs_error`.
- `cmd/hallstream/main.go:265-266`: `safely(i18n.T("log.tarefa.camara"), srv.RunCamera)` and `safely("OBS", srv.RunOBS)`. Every background loop starts through `safely` (line 75).
- `internal/state/state.go`:
  - `State` (269-282): `Box, Live, Periods, Sets, Game, Screen, LowerThird, Camera *Camera, Promo, Demo, Input`.
  - `SetCamera(c *Camera)` (631-639): set under `m.mu`, `m.changed()` only when it differs. Copy this shape.
- `web/painel/painel.js`:
  - `renderEcras()` (839-893): builds the Ecrãs grid once (`ecrasBuilt`), left column starts with the "No ecrã" card (`#screens`), then `renderLtPlayers()`, `updateScreenButtons()`, `updateLowerThird()`.
  - `onLive()` (366-) runs on every state from `/api/events` and calls `updateScreenButtons()` (line 395).
  - `ecrasEvents()` (1007-1063): one click listener on `#ecras`; the screen buttons disable all `.scr` while the request runs (`S.screenBusy`), show errors in `#scrMsg`.
  - `obsOn()` (1111): `S.settings.obs.enabled`.
  - `api(method, path, body)` returns a promise; errors carry `err.message`.
- `web/painel/mock.js`:
  - `state()` (165-199) builds the mock live state (no `obs` key).
  - `obsMode` from `?obs=off|ready|problem|live|nokey|choose` (237-248).
  - Routes: `if (p === '/api/screen') …` (442); OBS routes (506-542).
- `internal/web/panel_sport_test.go:17-40`: the model for a Chrome panel test on `/painel?mock=1` (`chrome(t)`, `until(tab, js)`, `chromedp.Evaluate`).
- `internal/web/obs_test.go:35-78`: `fakeOBSData` answers requests; it never sends events.
- `internal/obsws/obsws_test.go:15-` and `:81-`: the obsws `fake` and `fakeOBS(t, password, f)`; plan 008 gave `fake` a `byScene` map.
- `docs/CONTRACT.md`: "Live state" JSON (lines 39-88); "Time-out squeeze" (303-313); "OBS set up by the program", the panel bullet with the mock states (327): `?mock=1&obs=off|ready|problem|live|nokey|choose`, `&act=setup|start|stop`.

### obs-websocket 5.x facts this plan relies on

From https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md:

- `Identify` (op 1) carries `eventSubscriptions`, "a bitmask of `EventSubscriptions` items"; by default all non-high-volume categories. The client today sends `0` (none).
- `EventSubscription::Scenes` is `(1 << 2)`, "Subscription value to receive events in the `Scenes` category".
- An `Event` (op 5) message's data is `{ "eventType": string, "eventIntent": number, "eventData": object(optional) }`, sent "to all subscribed and identified clients".
- `CurrentProgramSceneChanged` (Scenes category, since v5.0.0): data fields `sceneName` ("Name of the scene that was switched to") and `sceneUuid`.
- `SetCurrentProgramScene` takes `sceneName` (or `sceneUuid`). The doc says nothing about the transition; OBS uses the current scene transition (what a click in OBS does).
- `Reidentify` (op 3) can change `eventSubscriptions` on a live session. Not used here: the second connection identifies once with the mask it needs.

### Conventions

- See plan 008's conventions: standard library plus `golang.org/x/net/websocket`; English code and comments; pt then en strings with `{name}` placeholders and no keys built at run time; `docs/CONTRACT.md` in the same commit; never touch `dados/` or `dist/`.
- New routes go through `ServeHTTP` (they do, by being on `s.mux`).
- A new background loop starts through `safely` in `cmd/hallstream/main.go`.
- The panel's design context and tokens: `docs/design-system.md`. Panel UI goes through `build-ui`, extension branch.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| obsws tests | `go test ./internal/obsws/ -v -run 'Watch|Place|Squeeze'` | pass |
| Web tests | `go test ./internal/web/ -run 'Camera|Live'` | pass |
| Panel test | `go test ./internal/web -run TestPanelCameras` | pass (needs Chrome) |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel at `http://127.0.0.1:8765/painel` |
| Mock | `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=3&live=2#ecras` | three camera buttons, the second live |

## Suggested executor toolkit

- `build-ui` skill, **extension branch** ("a new section, state or screen inside a settled identity"), for Step 6. The identity is the panel's, `docs/design-system.md`. Run its lint and outside critique on the camera card in every mock state below before calling the step done.
- `interface-guidelines` for the buttons (touch targets on a phone, `aria-pressed`, a busy state).
- `tdd` for Steps 1 to 4.

## Scope

**In scope**:

- `internal/obsws/obsws.go` (`dial` takes the event mask; `EventScenes`), `internal/obsws/events.go` (create: `Watch`), `internal/obsws/obsws_test.go` (the fake can send events), `internal/obsws/events_test.go` (create)
- `internal/obsws/camera.go` (`CameraSettings.Scene`, `Fulls`; `Place`; `Squeezed`), `internal/obsws/camera_test.go` (new tests)
- `internal/state/state.go` (`OBS` type, `State.OBS`, `SetLiveCamera`), `internal/state/state_test.go` (new test)
- `internal/web/camera.go` (`Settings` from the live camera; `RunScenes`; `programScene`; `POST /api/obs/camera/live`), `internal/web/server.go` (fields only), `internal/web/cameras_test.go` (create)
- `cmd/hallstream/main.go` (one `safely` line)
- `web/painel/painel.js`, the panel's stylesheet (`web/painel/*.css`, whichever holds `.scr`), `web/painel/mock.js`
- `internal/web/panel_cameras_test.go` (create)
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `LEIA-ME.txt`, `READ-ME.txt`

**Out of scope**:

- The time-out camera, `settings.obs.timeout_camera` (plan 010).
- Adding or editing cameras, labels, VDO.Ninja (plan 011).
- Health lines on the camera buttons (plan 012). Leave room for one line per button (see Step 6) but draw none.
- Configurar OBS (plan 008's code): unchanged here.
- The overlay: it reads nothing new. No overlay fixture changes.
- The old two-scene squeeze (`Camera.legacy`): unchanged.

## Git workflow

- Branch: `03-feat/camera-switching`, stacked on `02-feat/camera-scenes`.
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: An events-only connection

In `internal/obsws/obsws.go`:

- `dial(cfg Config)` becomes `dial(cfg Config, events int)`; the `Identify` map sends `"eventSubscriptions": events`. Every existing caller (`Test`, `request`) passes `0`.
- `const EventScenes = 1 << 2 // EventSubscription::Scenes`.

Create `internal/obsws/events.go`:

```go
// Watch opens its own connection to OBS, subscribed to scene events only,
// and calls scene with the program scene's name: once when connected and
// again on every CurrentProgramSceneChanged. It returns when the
// connection fails or stop closes; the caller reconnects.
func Watch(cfg Config, stop <-chan struct{}, scene func(name string)) error
```

- `dial(cfg, EventScenes)`, then clear the deadline (`conn.SetDeadline(time.Time{})`).
- Close the connection when `stop` closes (a goroutine that waits on `stop` and calls `conn.Close()`; end it when `Watch` returns).
- Send one `GetCurrentProgramScene` request (op 6, any `requestId`, for example `"watch"`) and read messages until its op 7 reply; call `scene` with `sceneName`, else the deprecated `currentProgramSceneName`.
- Then loop: read one message with `websocket.JSON.Receive`. For op 5 with `eventType == "CurrentProgramSceneChanged"`, decode `eventData.sceneName` and call `scene`. Ignore every other op and event type.
- A read error is the return value. A `stop` close returns `nil`.

In `internal/obsws/obsws_test.go`, let the fake send events: add to `fake` (or to `fakeOBS`) a way to push an op 5 message to every open connection, for example `f.emit(eventType string, data map[string]any)` with the connections kept in a slice under the fake's mutex. Record each connection's `Identify` `eventSubscriptions` so a test can check it.

Create `internal/obsws/events_test.go`:

- `TestWatchSeesProgramScene`: `Watch` calls `scene("Jogo")` first (the fake's program), then `scene("Câmara 2")` after `f.emit("CurrentProgramSceneChanged", {"sceneName": "Câmara 2"})`; other events (`"SceneCreated"`) call nothing. The identify carried `eventSubscriptions` 4.
- `TestWatchStops`: closing `stop` makes `Watch` return `nil` within a second.
- `TestRequestsStillSubscribeToNothing`: a plain `Request` connection identifies with `eventSubscriptions` 0.

**Verify**: `go test ./internal/obsws/ -run Watch -v` → pass; `go test ./internal/obsws/` → pass.

### Step 2: `obs.live_camera` in the live state

In `internal/state/state.go`:

```go
// OBS is what the program knows of OBS that the panel shows live.
type OBS struct {
	// LiveCamera is the camera on program, 1 to 3; 0 when OBS shows
	// another scene or is not known.
	LiveCamera int `json:"live_camera"`
}
```

Add `OBS OBS \`json:"obs"\`` to `State` after `Camera`. Add `SetLiveCamera(n int)`, shaped like `SetCamera` (set and `m.changed()` only when it differs). `snapshot` copies it with the struct (no slice, nothing extra).

**Verify**: `go test ./internal/state/` → pass, with a new test: `SetLiveCamera(2)` changes `Snapshot().OBS.LiveCamera` and sends one change to a subscriber; `SetLiveCamera(2)` again sends none.

### Step 3: The squeeze and the refit follow the live camera

In `internal/obsws/camera.go`:

- Add to `CameraSettings`:
  ```go
  	// Scene is the live camera's scene; "" is the program scene (the old
  	// layout, or the live camera not known).
  	Scene string
  	// Fulls are the saved full-screen placements by source (camera_fulls).
  	Fulls map[string]*Full
  ```
- `squeeze`, `Check` and `FitNow` call `resolve(set, set.Scene)`.
- `full(set, it)` uses `set.Fulls[it.name]` when present, else `set.Full`.
- New:
  ```go
  // Squeezed reports whether the camera is in the time-out window (the last
  // screen followed was desconto).
  func (c *Camera) Squeezed() bool

  // Place puts source in scene at its full-screen placement, or in the
  // time-out window, at once, and shows those boxes to the overlay. The
  // server calls it before it switches to that scene.
  func (c *Camera) Place(scene, source string, window bool) error
  ```
  `Place` resolves `scene`/`source` (`set.Source = source`), computes `full` and `window` as `squeeze` does, calls `c.show(it, full, win)` and `c.move(it, target, target, false)`. It does nothing (returns nil) when `!set.Enabled` or the legacy mode applies.

New tests in `internal/obsws/camera_test.go` (use the fake's `byScene` from plan 008):

- `TestSqueezeFollowsLiveScene`: program `Câmara 1`, `set.Scene = "Câmara 2"`, `set.Source = "B"`. `Follow("desconto")` sends `SetSceneItemTransform` with `sceneName` `Câmara 2` only.
- `TestPlaceWindowAtOnce`: `Place("Câmara 2", "B", true)` sends one transform equal to the window placement (no tween: one request), and `Show` got the window box for B's aspect.
- `TestPlaceUsesItsOwnFull`: `set.Fulls["B"]` holds a placement matching B's source size; `Place(…, false)` sends that placement and does not call `Save`.
- Existing squeeze tests pass unchanged (`set.Scene` is `""` there).

**Verify**: `go test ./internal/obsws/` → pass.

### Step 4: The server knows the live camera and switches

In `internal/web/server.go`, add to `Server` (next to `camErr`): `switchMu sync.Mutex`, and under one small mutex `liveMu`: `program string` (the last program scene seen), `live int`, `expect string`.

In `internal/web/camera.go`:

- `Settings` closure: read `s.liveCamera()` (index under `liveMu`). When it is ≥ 1 and `CameraList()[n-1].Scene != ""`: `Scene` and `Source` from that camera. Else `Scene: ""`, `Source: o.CameraSource` (today's behaviour). `Full: o.FullFor(Source)`, `Fulls: o.CameraFulls`.
- `liveIndex(o store.OBS, scene string) int`: the mapping in Decision 3.
- `programScene(name string)`: called by the watcher. Under `liveMu`: set `program`, compute `n := liveIndex(…)`, read and clear `expect` (was it `name`?). Then `s.State.SetLiveCamera(n)`. When `n ≥ 1`, the scene was not expected, OBS is enabled and `s.camErr == nil`: `s.Camera.Place(scene, source, s.Camera.Squeezed())`, logging a failure with `log.camara.nao_pus` (below). Run the placing through `s.once` (it talks to OBS; the watcher must keep reading).
- `RunScenes()` (never returns): loop forever. When `settings.obs.enabled` is false, `SetLiveCamera(0)` and sleep 5 s. Else make a `stop` channel, start a goroutine that closes it when `obsConfig(settings.obs)` changes or OBS is disabled (check every 5 s), and call `obsws.Watch(cfg, stop, s.programScene)`. On return, `SetLiveCamera(0)`, wait (1 s, doubling to 10 s, reset after a watch that lasted over a minute) and go round. Log a dropped watch once per outage with `log.obs.cenas_perdi` (below), not on every retry.
- `POST /api/obs/camera/live` in `cameraRoutes`:
  ```go
  var req struct{ Index int `json:"index"` }
  ```
  1. `decode`. OBS not enabled → 409 `api.camara.obs_desligado`.
  2. `cams := o.CameraList()`; `req.Index` outside `1..len(cams)` → 400 `api.camara.nao_existe` (`{n}`); `cams[i].Scene == ""` → 409 `api.camara.sem_cena` (`{n}`).
  3. Legacy two-scene mode and screen `desconto` → 409 `api.camara.modo_antigo` (Decision 9). Read the legacy flag the way `Camera.legacy` does; export a `func (c *Camera) Legacy() (bool, error)` for it.
  4. `s.switchMu.Lock()`; defer unlock.
  5. When `s.camErr == nil`: `warn := s.Camera.Place(scene, source, s.Camera.Squeezed())`; keep its message, do not stop.
  6. Under `liveMu` set `expect = scene`. `s.OBS.SetScene(obsConfig(o), scene)`; on error clear `expect`, answer 502 with the error.
  7. Under `liveMu` set `live = req.Index`, `program = scene`; `s.State.SetLiveCamera(req.Index)`.
  8. Reply `{ "ok": true, "live_camera": n, "warning": "<place error or empty>" }`.
- `cmd/hallstream/main.go`: `safely(i18n.T("log.tarefa.cenas"), srv.RunScenes)` next to `RunCamera`.

New pt keys (then en):

| Key | pt | en |
|---|---|---|
| `api.camara.obs_desligado` | `Ligue o controlo do OBS em Definições primeiro.` | `Turn on OBS control in Settings first.` |
| `api.camara.nao_existe` | `Não há câmara {n}.` | `There is no camera {n}.` |
| `api.camara.sem_cena` | `A câmara {n} ainda não tem cena no OBS. Carregue em Configurar OBS.` | `Camera {n} has no scene in OBS yet. Press Set up OBS.` |
| `api.camara.modo_antigo` | `No modo antigo de duas cenas não troco de câmara durante o desconto.` | `In the old two-scene mode I don't switch cameras during a time-out.` |
| `log.camara.nao_pus` | `Não consegui pôr a câmara {camara} no sítio: {err}` | `I couldn't put camera {camara} in place: {err}` |
| `log.obs.cenas_perdi` | `Perdi a ligação que segue a cena do OBS: {err}. Volto a tentar.` | `I lost the connection that follows OBS's scene: {err}. Trying again.` |
| `log.tarefa.cenas` | `cenas do OBS` | `OBS scenes` |

Check the existing English for "Configurar OBS" in `en.json` (`painel.configurar_obs`) and use the same words in `api.camara.sem_cena`.

Create `internal/web/cameras_test.go` with a fake OBS that keeps a program scene, two or three camera scenes with one camera item each (source size 1280x720), `GetVideoSettings` 1920x1080, `GetSceneList`, `GetSceneItemList` per scene, `SetSceneItemTransform` (recorded with its `sceneName`) and `SetCurrentProgramScene` (recorded, sets the program). Use `fakeOBSData`. Point the server's settings at it with `obs.enabled` true and `cameras` `[{A, "Câmara 1"}, {B, "Câmara 2"}]`. Tests:

- `TestCameraLiveSwitches`: `POST /api/obs/camera/live {index: 2}` → 200, `live_camera` 2; the fake's program is `Câmara 2`; one `SetSceneItemTransform` on `Câmara 2` came **before** the `SetCurrentProgramScene`, at the full placement; `s.State.Snapshot().OBS.LiveCamera == 2`.
- `TestCameraLiveDuringTimeout`: `POST /api/screen {screen: desconto}`, wait for the squeeze to end (`s.Camera.Stop()`), then switch to 2: the transform sent on `Câmara 2` before the switch is the window placement. Then `POST /api/screen {screen: marcador}`: the tween runs on `Câmara 2`, ending at its full placement.
- `TestCameraLiveRefused`: index 0, index 3 (two cameras), a camera with `scene: ""`, OBS disabled → 400/409 as listed, and no `SetCurrentProgramScene` sent.
- `TestCameraLiveOBSRefuses`: the fake answers `SetCurrentProgramScene` with `false, 600` → 502, `live_camera` unchanged.
- `TestProgramSceneMapping`: `programScene("Câmara 2")` → `LiveCamera` 2; `programScene("Outra")` → 0; with one camera in `CameraList()`, `programScene("Hallstream")` → 1; with two, `programScene("Hallstream")` → 0.
- `TestProgramSceneFromOBSPlaces`: screen `desconto`, camera squeezed, `programScene("Câmara 2")` (not expected) → a window transform on `Câmara 2`. Right after a panel switch to 2, `programScene("Câmara 2")` sends no transform.

**Verify**: `go test ./internal/web/ -run 'Camera|Program' -v` → pass; `go test ./...` → pass.

### Step 5: The panel mock

In `web/painel/mock.js`:

- `?cams=1|2|3` (default `1`): `db.settings.obs.cameras` holds that many cameras: `{ source: 'DroidCam Video', scene: 'Câmara 1', label: '' }`, `{ source: 'Câmara HDMI', scene: 'Câmara 2', label: 'Bancada' }`, `{ source: 'HD Webcam', scene: 'Câmara 3', label: '' }`. `?cams=noscene` makes three cameras with camera 3's `scene: ''`.
- `?live=0|1|2|3` (default `1`): `db.obsLive`. `state()` adds `obs: { live_camera: obsMode === 'off' ? 0 : db.obsLive }`.
- Route `if (p === '/api/obs/camera/live')`: with `?camerr=1` answer `err(502, 'o OBS não mudou para a cena Câmara ' + b.index + ': fake')`; else set `db.obsLive = b.index`, `emit()`, `reply(200, { ok: true, live_camera: b.index, warning: '' })`.

**Verify**: the mock fakes `EventSource`, so check it through the page. Open `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=3&live=2` and run `fetch('/api/settings').then(r => r.json()).then(s => s.obs.cameras.length)` in the browser console: it gives 3. Step 6's panel test checks `obs.live_camera` end to end.

### Step 6: Camera buttons on Ecrãs

Work through `build-ui` (extension branch) inside `docs/design-system.md`, with its lint and outside critique, in every mock state listed below.

In `web/painel/painel.js`:

- In `renderEcras()`, put a new card at the top of the left column, before "No ecrã": `<div class="card" id="camCard" hidden><h2>` + `t('painel.camaras.titulo')` + `</h2><div class="cams" id="cams"></div><p class="msg" id="camMsg" role="status"></p></div>`.
- `updateCameraButtons()`: `cams = obs cameras` from `S.settings.obs.cameras` (empty list → one camera, the same rule as `CameraList`). Hide `#camCard` when `!obsOn()` or fewer than 2 cameras. Else draw one button per camera, re-drawing only when the list changes (keep focus):
  ```html
  <button class="cam" data-cam="2" aria-pressed="true|false" [disabled]>
    <span class="cam-n">2</span>
    <span class="cam-nome">Bancada</span>
    <small class="cam-estado">No ar</small>
    <small class="cam-saude"></small>   <!-- plan 012 fills this; empty here -->
  </button>
  ```
  `cam-nome` is the label, else `t('painel.camaras.camara', { n })`. `aria-pressed` is `true` for `S.state.obs.live_camera`. `cam-estado` is `t('painel.camaras.no_ar')` on the live one, `t('painel.camaras.sem_cena')` on a camera with no scene (and the button is disabled), else empty. When OBS is connected (`S.status.obs.connected`) and `live_camera` is 0, show `t('painel.camaras.outra_cena')` in `#camMsg` as a hint (class `msg`), so the volunteer knows OBS is on a scene that is no camera.
- Call `updateCameraButtons()` from `renderEcras()` and from `onLive()` next to `updateScreenButtons()`, and after settings load.
- In `ecrasEvents()`, a click on `[data-cam]`: disable every `.cam`, clear `#camMsg`, `api('POST', '/api/obs/camera/live', { index: +b.dataset.cam })`. On success set `S.state.obs = { live_camera: r.live_camera }` and redraw; show `r.warning` in `#camMsg` (class `msg err`) when not empty. On error show `err.message` (class `msg err`). Then re-enable.
- Styles: reuse the screen buttons' tokens (`.scr`) for size, focus ring and pressed state. The live button must be told apart without colour alone (the "No ar" text does that). Touch target at least 44 px.

New pt keys (then en):

| Key | pt | en |
|---|---|---|
| `painel.camaras.titulo` | `Câmaras` | `Cameras` |
| `painel.camaras.camara` | `Câmara {n}` | `Camera {n}` |
| `painel.camaras.no_ar` | `No ar` | `On air` |
| `painel.camaras.sem_cena` | `Carregue em Configurar OBS` | `Press Set up OBS` |
| `painel.camaras.outra_cena` | `O OBS mostra uma cena que não é de nenhuma câmara.` | `OBS is showing a scene that isn't any camera's.` |

Mock states for screenshots (all with `?mock=1&obs=ready`, then `#ecras`):

| State | URL suffix | What shows |
|---|---|---|
| One camera | `&cams=1` | no camera card (Ecrãs as today) |
| Two cameras, camera 1 live | `&cams=2&live=1` | two buttons, the first "No ar" |
| Three cameras, camera 2 live | `&cams=3&live=2` | "Bancada" pressed and "No ar" |
| Another scene on program | `&cams=3&live=0` | no button pressed, the hint under them |
| A camera without a scene | `&cams=noscene&live=1` | camera 3 disabled, "Carregue em Configurar OBS" |
| Switch refused | `&cams=3&live=1&camerr=1`, click camera 2 | the error under the buttons, camera 1 still live |
| OBS off | `&cams=3` with `obs=off` | no camera card |

Create `internal/web/panel_cameras_test.go`, modelled on `panel_sport_test.go`:

- `TestPanelCameras`: navigate `/painel?mock=1&obs=ready&cams=3&live=2#ecras`; `until` there are 3 `#cams .cam`; the second has `aria-pressed="true"`; click the third; `until` the third has `aria-pressed="true"` and the second `"false"`.
- `TestPanelOneCameraNoCard`: `/painel?mock=1&obs=ready&cams=1#ecras` → `#camCard` is hidden.
- `TestPanelCameraRefused`: `&cams=3&live=1&camerr=1`, click camera 2 → `#camMsg` has class `err` and camera 1 stays pressed.

**Verify**: `go test ./internal/web -run TestPanelCamera -v` → pass (with Chrome); `go test ./internal/i18n` → pass; every mock state above looks right at phone width (390 px) and desktop width.

### Step 7: Contract and the read-me

- `docs/CONTRACT.md`:
  - "Live state" JSON: add `"obs": { "live_camera": 1 }` after `"camera"`, and a rule bullet: what it is, Decision 3's mapping, `0` cases, read from `CurrentProgramSceneChanged` on a second connection subscribed to `Scenes` (`1 << 2`) with `GetCurrentProgramScene` at each connect, set at once by a panel switch, back to 0 while OBS is off or the watch is down.
  - "Time-out squeeze": the squeeze and the refit work on the live camera's scene (`cameras[n-1].scene`), or the program scene when no camera is known; before any switch the program places the target camera (window while squeezed, else full) at once; a switch made in OBS is placed after the fact.
  - "OBS set up by the program" (or a new short section "Camera switching (added by Go and the panel)"): `POST /api/obs/camera/live` `{ "index" }` with its answers (200 `{ ok, live_camera, warning }`, 400, 409 cases, 502), one switch at a time.
  - The panel's mock states list: add `&cams=1|2|3|noscene`, `&live=0|1|2|3`, `&camerr=1`.
- `LEIA-ME.txt` and `READ-ME.txt`, section "DESCONTO DE TEMPO COM O OBS" / its English twin: two lines saying that with two or three cameras, Ecrãs has a button per camera; the live one says "No ar"; a camera can be changed during a time-out and the picture stays in the window. Same tone as the lines around it.

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/obsws/events_test.go`: `Watch` (Step 1).
- `internal/obsws/camera_test.go`: live scene, `Place` (Step 3).
- `internal/state/state_test.go`: `SetLiveCamera` (Step 2).
- `internal/web/cameras_test.go`: the switch, the time-out case, refusals, the mapping, a switch made in OBS (Step 4).
- `internal/web/panel_cameras_test.go`: the camera card in the mock (Step 6).
- Manual with a real OBS 30 or 31 and two cameras, on a scratch build: click camera 2 in the panel, then click camera 1's scene in OBS and watch the panel follow; switch during a time-out and check the picture and the hole line up; leave the time-out and check camera 2 opens out to full screen.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n '"live_camera"' internal/state/state.go docs/CONTRACT.md` matches in both
- [ ] `grep -n 'POST /api/obs/camera/live' internal/web/camera.go docs/CONTRACT.md` matches in both
- [ ] `grep -n 'RunScenes' cmd/hallstream/main.go` matches (started through `safely`)
- [ ] No existing test case modified (`git diff <008's last commit>..HEAD -- '*_test.go'` shows only additions)
- [ ] The `build-ui` lint and outside critique ran on the camera card
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 008 has not landed (the "Depends on" checks fail).
- A real OBS does not send `CurrentProgramSceneChanged` to a client identified with `eventSubscriptions` 4, or sends it only after the transition ends in a way that makes the panel lag more than the transition's length. Report the OBS version and what arrives.
- `SetCurrentProgramScene` in Studio Mode switches the preview instead of the program on a real OBS. Report it; do not add Studio Mode handling.
- Placing the camera before the switch shows a visible jump on air in the real OBS (the outgoing scene renders the target camera too because the same input is in both). Report it.
- An existing test fails and only editing it would make it pass.
- A verification fails twice after a reasonable fix.

## Open questions / corrections

1. **Correction to the Facts and Decision 4: the client can't receive events today.** `dial` identifies with `eventSubscriptions: 0` (`internal/obsws/obsws.go:75`), and `receive` (lines 99-113) throws away every message that is not the awaited reply. Reading `CurrentProgramSceneChanged` needs new code. This plan adds a second, events-only connection (`obsws.Watch`), which keeps the request client and its locking untouched. The cheaper alternative is to poll `GetCurrentProgramScene` in `RunOBS`'s 3 s loop: no new connection, but a switch made in OBS shows up to 3 s late. Recommended: the event connection, as decided; the poll is the fallback if a real OBS misbehaves with two connections.
2. **Correction: "the live state carries `obs.live_camera`" has no `obs` object to go into.** The live state (`state.State`, contract "Live state") has no `obs` key; `obs` exists only in `GET /api/status`, which the panel polls rather than streams. This plan adds a top-level `obs` object to the live state, so the panel gets the live camera on every event. Recommended: accept.
3. **Decision 5's "the camera item of whichever camera scene is on program"** is implemented as "of the live camera's scene" (`CameraSettings.Scene`), not by asking OBS for the program scene at squeeze time. Plan 010 switches the scene and squeezes right after; asking OBS then could still return the old scene mid-transition. The two agree whenever the program is a camera scene; with another scene on program (`live_camera` 0) the squeeze falls back to the program scene, as today.
4. **The old two-scene mode** (`timeout_scene`) is not mentioned in the decisions file. This plan leaves its squeeze alone and refuses a camera switch during `desconto` in that mode (Decision 9). Recommended: accept; the mode is for clubs from before Configurar OBS.
5. **Correction to the Facts line "A refit loop puts the camera back at full screen every 4 s on `marcador` and `nada`".** `Camera.Check` (`internal/obsws/camera.go:218-247`) moves the camera only when its source size, the canvas or the fit differ from the saved placement (`full` returns `changed`). It does not put a camera that sits in the window back at full screen. That is why this plan places a camera before every switch and after a switch made in OBS (Decisions 5 and 6), instead of relying on the refit.
6. **Studio Mode** is not covered by the decisions. The protocol doc does not say what `SetCurrentProgramScene` does in Studio Mode. Flagged as a STOP condition for the manual test rather than handled.

## Maintenance notes

- Plan 010 (time-out camera) takes `switchMu`, sets `expect`, and uses `Place` and `SetLiveCamera` the same way. Keep those as small server methods (`s.switchTo(n int, window bool) (warning string, err error)` is a good shape) so plan 010 calls one function.
- Plan 012 fills `.cam-saude` on each button. Plan 011 lets the volunteer edit labels and add cameras; `updateCameraButtons` already redraws when the list changes.
- `Watch` reconnects on its own loop. If obs-websocket ever needs keep-alives on idle connections, add a `GetVersion` request every 30 s on the watch connection.
- Reviewers: run a real OBS with two cameras, switch from the panel and from OBS, switch during a time-out, and pull the OBS network cable (or close OBS) to watch the panel go to no live camera and come back.
