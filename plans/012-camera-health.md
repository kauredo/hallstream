# Plan 012: Each camera shows a health line on Ecrãs, and the panel says when OBS skips frames

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- internal/obsws internal/web/obs.go internal/web/camera.go internal/web/server.go internal/store/store.go cmd/hallstream/main.go web/painel/painel.js web/painel/mock.js web/i18n docs/CONTRACT.md`
> Plans 008 to 011 are expected to have changed several of these files.
> First confirm they landed (see "Depends on"), then compare every "Current
> state" excerpt with the live code. Line numbers below are from `b28fb61`
> and will have moved; a change you can map one-to-one is fine, one you
> can't is a STOP condition.

## Status

- **Priority**: P2 (ROADMAP Phase B item 5)
- **Effort**: M
- **Risk**: LOW-MED (read-only requests to OBS, but on the one connection the time-out squeeze also uses)
- **Depends on**: `plans/008-*.md` (`settings.obs.cameras` with `source` and `scene`), `plans/009-*.md` (the camera buttons on Ecrãs) and `plans/011-*.md` (phone cameras as 1920x1080 browser sources). Check: `grep -n '"graphics_scene"' internal/store/store.go` matches, `grep -n "live_camera" web/painel/painel.js` matches, and `ls internal/obsws/vdo.go` exists. Any miss: STOP.
- **Category**: direction
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

With two or three cameras, a dead one is easy to miss: the volunteer watches the camera on air, and the one they are about to switch to may have no picture, or a phone may have frozen on its last frame. ROADMAP Phase B item 5 asks for a health line per camera. After this plan each camera button on Ecrãs carries one line, "Sem imagem", "Imagem parada" or "Com imagem", and the panel says when OBS itself is skipping frames.

Owner decision this plan must honour (`plans/phase-b-decisions.md`, decision 9):

- One line per camera on the Ecrãs camera buttons: "Sem imagem" (the source reports 0x0 or is not active), "Imagem parada" (two small screenshots 10 s apart are identical), else OK.
- OBS's skipped-frames count (`GetStats`) shows once for the whole program, not per camera. obs-websocket gives no frame rate per source.

This plan changes one part of that decision. See "Open questions / corrections" 1: "is not active" can't be used.

## Facts from the obs-websocket 5.x protocol doc

Source: https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md, read 2026-11-26.

- `GetSourceActive`: "Gets the active and show state of a source." Response `videoActive`: "Whether the source is showing in Program"; `videoShowing`: "Whether the source is showing in the UI (Preview, Projector, Properties)". Complexity 2/5.
- `GetSourceScreenshot`: "Gets a Base64-encoded screenshot of a source." "Compatible with inputs and scenes." Complexity 4/5. Request: `sourceName`, `imageFormat` (required; "Use `GetVersion` to get compatible image formats"), `imageWidth` and `imageHeight` (">= 8, <= 4096"; "treated as 'scale to inner'… the aspect ratio of the original resolution is kept"), `imageCompressionQuality` (-1 to 100, default -1). Response `imageData`: "Base64-encoded screenshot". The doc names no prefix, but obs-websocket's own code may send it as a `data:image/png;base64,…` URI. Strip anything up to the first comma, when there is one, before decoding, and test both forms.
- `GetStats` response: `renderSkippedFrames` ("Number of frames skipped by OBS in the render thread"), `renderTotalFrames`, `outputSkippedFrames` ("Number of frames skipped by OBS in the output thread"), `outputTotalFrames`, plus `cpuUsage`, `activeFps`, `averageFrameRenderTime` and others. Complexity 2/5. All counts are totals since OBS started.
- No request returns a frame rate or frame count per source.
- `GetSceneItemList`'s `sceneItemTransform` has no documented fields. The program already reads `sourceWidth` and `sourceHeight` from it and treats 0 as "no picture" (`internal/obsws/camera.go:386-389`), so that field is known to exist in practice.

## Decisions made in this plan (the executor does not re-decide them)

1. **States**: `"no_image"` ("Sem imagem"), `"frozen"` ("Imagem parada"), `"ok"` ("Com imagem"), and `""` (not known yet: the first round, or OBS not connected). The panel shows nothing for `""`.
2. **"Sem imagem"** when any of these holds:
   - the camera's scene item is missing, or its `sourceWidth` or `sourceHeight` is 0;
   - the screenshot is one flat colour (every pixel within 6 of the first on each of R, G, B and A). This catches a phone's VDO.Ninja page with no phone connected, which is a 1920x1080 browser source with nothing drawn (plan 011), and a capture device sending black.
   - `GetSourceActive` is **not** used: its `videoActive` is true only for the camera on air, so every other camera would read "Sem imagem".
3. **"Imagem parada"** when the screenshot's `imageData` is byte-for-byte the one taken 10 s before for the same source. PNG is lossless, so a live camera's sensor noise changes at least one pixel.
4. **Screenshot size and format**: `imageFormat: "png"`, `imageWidth: 128`, `imageHeight: 72`. Small enough to be cheap, big enough that sensor noise survives the scaling.
5. **How often**: one round every 10 s. A round with three cameras sends 3 `GetSceneItemList`, 3 `GetSourceScreenshot` and 1 `GetStats`: 7 requests per 10 s. For scale, the squeeze sends `SetSceneItemTransform` every 20 ms, 50 a second, for 0.9 s at each time-out.
6. **When a round is skipped**:
   - OBS is not enabled, or `s.OBS.Status()` says not connected. `Client.Request` dials when it has no connection (`internal/obsws/obsws.go:181-190`, with a 3 s dial timeout), and `RunOBS` already handles reconnecting.
   - The camera is tweening. All requests share one connection under one mutex (`obsws.go:149-151`), so a 4/5 screenshot in the middle of the 20 ms tween steps would make the squeeze stutter on air.
   - A Configurar OBS or health check holds `setupOBSMu` (`TryLock` fails).
   A skipped round keeps the last states and the last screenshots, so the next round still compares 10 s or 20 s apart. "Imagem parada" needs only the two screenshots to match, whatever the gap.
7. **Skipped frames**: the sum of `renderSkippedFrames` and `outputSkippedFrames`, over the last minute: the current total minus the total from 6 rounds before (keep 7 samples). `total` is the same difference over `renderTotalFrames` plus `outputTotalFrames`. The panel shows the line only when the skipped difference is above 0. A total that went down (OBS restarted) resets the samples.
8. **Where the states live**: in `GET /api/status` under `obs`, which the panel already reads every 10 s (`web/painel/painel.js:3265`). Not in the live state: the contract says the live state is what the overlay reads, and the overlay doesn't show health.
9. **The log**: one line in `marcador.log` when a camera's state changes to `no_image` or `frozen`, and one when it comes back to `ok`. Volunteers read the log after a game; a camera that froze for a minute during the second half shows there.

## Current state

### Files and their roles (line numbers at `b28fb61`)

- `internal/obsws/obsws.go`:
  - `Client` (27-36) holds one connection under `mu`. `Request` (149-167) locks `mu` for the whole request; `request` (176-) dials when `c.conn` is nil.
  - `again` (170-172): requests starting with `Get` are retried once on a stale connection.
  - `Status` (233-) reports `connected` and the last error without a request.
- `internal/obsws/camera.go`:
  - `Camera` (114-136) has `mu` and `moving` (true while a tween runs).
  - `items` (313-329) reads the program scene and its items; `sceneItem` (148-162) has `Transform.SourceWidth`, `SourceHeight`.
  - `resolve` (367-411) fails with `obs.camera.sem_imagem` when the size is 0 (386-389):
    ```go
    tr := si.Transform
    if tr.SourceWidth <= 0 || tr.SourceHeight <= 0 || v.W <= 0 || v.H <= 0 {
    	return item{}, i18n.Err("obs.camera.sem_imagem", "camara", si.Name)
    }
    ```
- `internal/obsws/obsws_test.go`: `fake` with `answer(typ, d)` (26-), `fakeOBS(t, password, f answerer) Config` (81-) runs a fake obs-websocket server. Use it for the new obsws tests.
- `internal/web/obs.go`:
  - `obsHealth` (27-31), `checkOBS` (416-427), `refreshOBS` (430-435): the setup health check, under `setupOBSMu`.
  - `obsStatus` (437-446) builds `GET /api/status`'s `obs`:
    ```go
    return map[string]any{"connected": connected, "error": obsErr, "health": h, "stream": s.stream}
    ```
  - `RunOBS` (450-478): the stream status every 3 s and the health check every 10th loop.
- `internal/web/server.go`: `Server` fields (69-108) with `setupOBSMu`, `obsMu`, `health`.
- `cmd/hallstream/main.go:265-270`: background loops start through `safely`:
  ```go
  safely(i18n.T("log.tarefa.camara"), srv.RunCamera)
  safely("OBS", srv.RunOBS)
  ```
- `web/painel/painel.js`: `loadStatus` (325-) every 10 s (3265); `renderEcras` (839-), where plan 009 put the camera buttons.
- `web/painel/mock.js`: `obsStatus()` (241-248) answers the panel's status with `obsMode`.
- `docs/CONTRACT.md`: "OBS set up by the program" (315-327), the status shape on line 218.

### Conventions

- Go standard library (`image/png`, `encoding/base64` for this plan). Code and comments in English.
- Tests in plain `testing`. obsws tests on `fakeOBS`; pure logic tested without a server.
- Every user-facing string, log lines too, in `web/i18n/pt.json` first, then `en.json`, same keys. `go test ./internal/i18n` enforces it.
- Background loops start through `safely` in `cmd/hallstream/main.go`.
- `docs/CONTRACT.md` changes in the same commit as the behaviour.
- Never read or edit `dados/` or `dist/`.
- Commits: `feat: camera health …`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| obsws tests | `go test ./internal/obsws/ -run Health` | pass |
| Web tests | `go test ./internal/web/` | pass |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Mock panel | open `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=3&health=mixed` | Ecrãs shows one line per camera |

## Suggested executor toolkit

- `build-ui` skill, **extension branch**, for Step 4 (the lines on plan 009's buttons), with its lint and outside critique. The panel's tokens are in `docs/design-system.md`.

## Scope

**In scope**:

- `internal/obsws/health.go` (create), `internal/obsws/health_test.go` (create)
- `internal/obsws/camera.go` (only a `Moving() bool` method on `Camera`)
- `internal/web/obs.go` (`obsStatus`), `internal/web/health.go` (create: the loop and its state), `internal/web/server.go` (new fields on `Server` only)
- `cmd/hallstream/main.go` (one `safely` line)
- `internal/web/*_test.go` (new tests only)
- `web/painel/painel.js`, the panel stylesheet, `web/painel/mock.js`
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `docs/ROADMAP.md`

**Out of scope**:

- A frame rate per camera (no request gives it).
- Adding camera health to "Antes de começar" or the OBS chip. See Maintenance notes.
- Any change to what Configurar OBS builds, or to the squeeze.
- The overlay and the live state.

## Git workflow

- Branch: `feat/camera-health` (stacked on plan 011's branch)
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: Looking at one camera, and judging it

Create `internal/obsws/health.go`:

```go
// Look is one camera as one health round saw it.
type Look struct {
	Found  bool    // the scene has an item with the source's name
	Width  float64 // the source's size; 0 when it sends nothing
	Height float64
	Shot   string // the screenshot's imageData; "" when none was taken
	Flat   bool   // the screenshot is one colour
}

// LookAt reads source's size in scene, and when it has one, a 128x72 PNG
// screenshot. An empty scene means the program scene.
func (c *Client) LookAt(cfg Config, scene, source string) (Look, error)

// Judge is the camera's health from this look and the one 10 s before:
// "no_image", "frozen" or "ok".
func Judge(prev, cur Look) string

// Frames are OBS's skipped and total frames since it started, render and
// output threads together.
type Frames struct{ Skipped, Total float64 }

// Stats reads Frames from GetStats.
func (c *Client) Stats(cfg Config) (Frames, error)
```

- `LookAt`: `GetSceneItemList` on `scene` (with `""`, read `GetCurrentProgramScene` first, as `Camera.items` does); find the item named `source`. If it is missing or its size is 0, return without a screenshot. Else `GetSourceScreenshot` with `sourceName`, `imageFormat: "png"`, `imageWidth: 128`, `imageHeight: 72`. Keep `imageData` as is in `Shot`. For `Flat`, strip any `data:…,` prefix, base64-decode, `png.Decode`, and compare every pixel to the first (Decision 2). A decode failure is an error, not a state.
- `Judge`, in this order: not `Found`, or `Width` or `Height` 0, or `Flat` → `"no_image"`; `prev.Shot != "" && prev.Shot == cur.Shot` → `"frozen"`; else `"ok"`.
- `Stats`: `GetStats`, then `Skipped = renderSkippedFrames + outputSkippedFrames`, `Total = renderTotalFrames + outputTotalFrames`.

**Verify**: `go test ./internal/obsws/ -run 'Health|Judge|LookAt|Stats'` → pass, with new tests:
- `Judge` table: missing item, 0x0, flat → `no_image`; same `Shot` twice → `frozen`; different `Shot` → `ok`; first look (`prev` zero) with a picture → `ok`; flat and same `Shot` → `no_image` (flat wins).
- `LookAt` on `fakeOBS` (extend a `fake` with `GetSceneItemList` and `GetSourceScreenshot` answers): a 1280x720 item and a noisy 128x72 PNG → `Found`, size, `Shot` set, `Flat` false; an all-black PNG → `Flat` true; a 0x0 item → no `GetSourceScreenshot` sent; an `imageData` with a `data:image/png;base64,` prefix decodes.
- `Stats` sums the four fields.

### Step 2: The camera says when it is moving

In `internal/obsws/camera.go`, add:

```go
// Moving reports whether a tween is running.
func (c *Camera) Moving() bool
```

It reads `c.moving` under `c.mu`.

**Verify**: `go test ./internal/obsws/` → pass, with a new test on the existing `newRig` (`camera_test.go:106`): `Moving()` is true right after `Follow("desconto")` and false after `r.wait(t)`.

### Step 3: The health loop and the status

Create `internal/web/health.go`:

- New `Server` fields (declared in `server.go` next to `health`): `camLooks map[string]obsws.Look` (last look per source), `camStates []camState` (`{ Index int \`json:"index"\`; State string \`json:"state"\` }`), `frames []obsws.Frames` (up to 7 samples), all under `obsMu`.
- `func (s *Server) RunCameraHealth()`: forever, every 10 s, run `s.cameraRound()`. It never returns.
- `cameraRound()`:
  - Skip (Decision 6) when `!o.Enabled`, when `s.OBS.Status()` is not connected, when `s.Camera.Moving()`, or when `s.setupOBSMu.TryLock()` fails. With the lock taken, hold it to the end of the round.
  - The cameras: `settings.obs.cameras` in order, each with its `scene`. With no cameras saved (a club still on the 0.1.2 layout), one camera: `camera_source` in the program scene (`scene` `""`). With `camera_source` empty too, no cameras.
  - For each camera, `LookAt`; on an error, keep its last state. Else `Judge` against `camLooks[source]`, store the new look, and log a state change (Decision 9) with `log.camara.sem_imagem` ("A câmara {camara} ficou sem imagem." / "Camera {camara} lost its picture."), `log.camara.parada` ("A imagem da câmara {camara} parou." / "Camera {camara}'s picture froze."), `log.camara.voltou` ("A câmara {camara} voltou a ter imagem." / "Camera {camara} has a picture again."). `{camara}` is the camera's label, else its source name. No log line for the first state of a camera after start.
  - Drop looks for sources no longer in the list.
  - `Stats` once; append to `frames`, keep the last 7; clear the samples first if the new `Total` is below the last one.
- `obsStatus()` (`obs.go:437-446`) adds:
  - `"cameras"`: `camStates` (`[]` when not connected or OBS is off);
  - `"skipped"`: `{ "frames": <skipped in the last minute>, "total": <total in the last minute> }` when 7 samples exist and the skipped difference is above 0, else `null`.
- `cmd/hallstream/main.go`: `safely(i18n.T("log.tarefa.saude_camaras"), srv.RunCameraHealth)` after the `RunOBS` line, with `log.tarefa.saude_camaras` ("saúde das câmaras" / "camera health"), matching the other `log.tarefa.*` keys' style.

**Verify**: `go test ./internal/web/` → pass, with new tests that drive `cameraRound()` directly against a fake OBS (find how `obs_test.go` stands one in; reuse it):
- three cameras, camera 2 at 0x0, camera 3 with the same screenshot in two rounds → status `cameras` is `[{1,"ok"},{2,"no_image"},{3,"frozen"}]` after the second round;
- a round while `Moving()` is true sends no request to OBS;
- after 7 rounds where `renderSkippedFrames` rises by 30 in total, `skipped.frames` is 30; with no rise, `skipped` is `null`;
- a camera that goes `ok` → `no_image` writes exactly one `log.camara.sem_imagem` line;
- OBS disconnected → `cameras` is `[]`.

### Step 4: The lines on Ecrãs

In `web/painel/painel.js`, where plan 009 draws the camera buttons in `renderEcras`:

- Under each button's label, one line from `S.status.obs.cameras` by `index`: "Sem imagem" (`painel.camaras.sem_imagem`, en "No picture"), "Imagem parada" (`painel.camaras.parada`, en "Picture frozen"), "Com imagem" (`painel.camaras.com_imagem`, en "Picture OK"). Nothing for `""` or a missing index. "Sem imagem" and "Imagem parada" use the panel's warning tokens; "Com imagem" is quiet. Never colour alone: the words carry the state.
- Each problem line has a `title` with the fix: `painel.camaras.sem_imagem_dica` ("Confirme que a câmara está ligada e, num telemóvel, que a página do VDO.Ninja ou o DroidCam estão abertos." / "Check the camera is on and, on a phone, that the VDO.Ninja page or DroidCam is open.") and `painel.camaras.parada_dica` ("A imagem não mudou em 10 segundos. Num telemóvel, veja se o ecrã se desligou ou se a aplicação fechou." / "The picture hasn't changed in 10 seconds. On a phone, check whether the screen turned off or the app closed.").
- Under the row of buttons, when `S.status.obs.skipped` is set: `painel.camaras.saltou` ("O OBS saltou {n} de {total} imagens no último minuto: o PC não está a dar conta." / "OBS skipped {n} of {total} frames in the last minute: the PC can't keep up.").
- The lines redraw on each `loadStatus`, without rebuilding the buttons (a click in progress must not be lost).
- If plan 009 draws no buttons with a single camera, draw the same single line in the place 009 shows the camera. See "Open questions / corrections" 4.

In `web/painel/mock.js`: `?health=ok|noimage|frozen|mixed|skipped` (default `ok`), so `obsStatus()` returns `cameras` and `skipped` to match; `mixed` gives `ok`, `no_image`, `frozen` for cameras 1 to 3.

Work through `build-ui` (extension branch), lint and outside critique, on `?mock=1&obs=ready&cams=3&health=mixed` and `&health=skipped`.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass. The mock at PC width and 390 px shows the three lines and, with `skipped`, the program line.

### Step 5: Contract and roadmap

- `docs/CONTRACT.md`: a subsection "Camera health (added by Go and the panel)" after "OBS set up by the program": the states and how each is judged (Decisions 1 to 4), the requests and sizes, the 10 s round and when it is skipped (Decisions 5 and 6), `obs.cameras` and `obs.skipped` in `GET /api/status` (Decision 7), the log lines, the Ecrãs lines and the mock switch `&health=`. Add `cameras` and `skipped` to the status shape on line 218.
- `docs/ROADMAP.md` Phase B item 5: "Done: 'Sem imagem' and 'Imagem parada' per camera, and OBS's skipped frames once. obs-websocket has no frame rate per source, so 'low fps' per camera is not measured."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/obsws/health_test.go`: Step 1 cases.
- `internal/obsws/camera_test.go`: one new test for `Moving()`.
- `internal/web/*_test.go`: Step 3 cases, new tests only.
- Existing tests: all pass with no change to them.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n "GetSourceActive" internal/` finds nothing (Decision 2)
- [ ] `grep -n "RunCameraHealth" cmd/hallstream/main.go` matches a `safely(` line
- [ ] `grep -n '"skipped"' internal/web/obs.go` matches
- [ ] `grep -n "Camera health" docs/CONTRACT.md` matches
- [ ] No existing test case modified (review `git diff -- '*_test.go'`: only additions)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plans 008, 009 or 011 have not landed (the checks under "Depends on" fail).
- In a real OBS, a camera scene that is not on air gives a screenshot that never changes while its camera is live (OBS does not render it off air). Then every camera but the live one reads "Imagem parada". Test this before Step 4 with a webcam in camera 2 while camera 1 is on air: take two `GetSourceScreenshot`s 10 s apart and compare. Report the result; the frozen test would need another design.
- `GetSourceScreenshot` fails, rather than answering, for a 1920x1080 browser source with a blank page. Report the error text.
- An existing test fails and only editing it would make it pass.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- OBS sources have a "Deactivate when not showing" option (video capture devices, and DroidCam's docs mention it for sharing one phone between sources: https://droidcam.app/obs/usage/). A camera set that way stops sending frames while its scene is off air, and will read "Imagem parada" or "Sem imagem" there. That is true, but it is not a fault: the guide (plan 013) should tell clubs to leave that option off.
- A camera filming an empty, still hall at night through heavy noise reduction might give two identical 128x72 PNGs and read "Imagem parada". If a club reports it, raise the size to 256x144 before anything else.
- Health is advice only. It does not block the stream and is not in "Antes de começar". Adding "camera 2 has no picture" to that list is a later product call.
- Reviewers: with the demo and a real OBS, unplug a webcam camera and watch its line turn to "Sem imagem" within 20 s; close the VDO.Ninja page on the phone and watch its line turn to "Sem imagem" (flat page) or "Imagem parada" (last frame held); run a time-out and check the squeeze stays smooth.

## Open questions / corrections

1. **"Not active" would mark every off-air camera "Sem imagem".** Decision 9 lists `GetSourceActive` as one test for "Sem imagem". The protocol doc defines `videoActive` as "Whether the source is showing in Program", so only the camera on air is ever active. Recommended: drop `GetSourceActive` and judge "Sem imagem" from the size and a flat screenshot (Decision 2).
2. **A phone camera never reports 0x0.** Plan 011 makes each phone a 1920x1080 browser source, and a browser source keeps that size with no phone connected. Without a second test, a disconnected phone would read "Imagem parada" (a blank page twice) instead of "Sem imagem". Recommended: count a one-colour screenshot as "Sem imagem" (Decision 2).
3. **Screenshots share the squeeze's connection.** `obsws.Client` sends one request at a time under one mutex (`internal/obsws/obsws.go:149-151`), and the squeeze sends a transform every 20 ms. Recommended: skip the round while the camera tweens (Decision 6). Not in the decisions file; it changes nothing the volunteer sees.
4. **Where the line goes with one camera.** Decision 9 puts the lines on the Ecrãs camera buttons. If plan 009 hides the buttons for a club with one camera, that club gets no health line. Recommended: show the single line where 009 shows the one camera. Check 009 when it lands.
5. **Two screenshots are enough only with noise.** Decision 9 says two screenshots 10 s apart. At 128x72 PNG a live camera's noise changes the bytes. The risk of a false "Imagem parada" is in Maintenance notes; no change recommended now.
