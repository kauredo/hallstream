# Plan 010: A chosen camera goes on air in the time-out window

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- internal/obsws/camera.go internal/store/store.go internal/web web/painel web/i18n docs/CONTRACT.md LEIA-ME.txt READ-ME.txt`
> Plans 008 and 009 are expected to have changed all of these. First
> confirm they landed (see "Depends on"), then compare every "Current state"
> excerpt with the live code. Line numbers marked `b28fb61` will have moved;
> names introduced by plans 008 and 009 are given by name. A change you can
> map one-to-one is fine, one you can't is a STOP condition.

## Status

- **Priority**: P2 (ROADMAP Phase B item 3)
- **Effort**: M
- **Risk**: MED (it switches scenes on air at every time-out; off by default)
- **Depends on**: `plans/008-camera-scenes.md` and `plans/009-camera-switching.md`. Check: `grep -n "func (c \*Camera) Place" internal/obsws/camera.go`, `grep -n "switchMu" internal/web/server.go` and `grep -n "LiveCamera" internal/state/state.go` all match. All three must hold or STOP.
- **Category**: direction
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

ROADMAP Phase B item 3: "At a timeout, a chosen camera shows picture-in-picture." The owner settled what that means (Decision 6): the time-out screen already is a picture inside the graphics (the window, the stats column, the promo card), so the chosen camera fills that window. A club films the bench, or a close camera, during the time-out, then the game camera comes back.

After this plan, with `settings.obs.timeout_camera` set to a camera, entering `desconto` switches OBS to that camera with its picture shrinking into the time-out window, and leaving `desconto` switches back to the camera that was live, opening out to full screen. With it at `0` (the default) nothing changes.

Owner decisions this plan must honour (`plans/phase-b-decisions.md`):

- **Decision 6.** `settings.obs.timeout_camera` (`0` = none, default; `1` to `3`). With it set, entering `desconto` switches to that camera with its picture in the time-out window, and leaving switches back to the camera that was live. No second small inset.
- **Decision 5.** Switching cameras during `desconto` places the new camera at the window first, then switches, so the hole and the picture line up.
- **Decision 11.** Strings in `pt.json` first, then `en.json`.

## Decisions made in this plan (the executor does not re-decide them)

1. **Entering: full, switch, then squeeze.** On entering `desconto` the program puts the time-out camera at its full-screen placement at once, switches OBS to its scene, and then the normal squeeze tweens it into the window, in step with the overlay's hole. During OBS's transition both pictures are full screen and the hole shrinks over both, so nothing shows outside the hole. Placing the time-out camera straight in the window would show the scene's background between the picture and the hole for the length of the squeeze.
2. **Leaving: window, switch, then squeeze out.** On leaving, the program puts the camera to return to in the window at once, switches back to it, and the normal squeeze tweens it out to full screen. This is Decision 5's rule ("place the new camera at the window first, then switch").
3. **What "the camera that was live" is.** The live camera when `desconto` began (`obs.live_camera`). It is forgotten, so nothing switches back, when: the volunteer switches camera from the panel during the time-out (their pick wins), someone switches scene in OBS during it (an unexpected `CurrentProgramSceneChanged`), or the return camera no longer has a scene.
4. **When the time-out camera is skipped.** `timeout_camera` is `0`; it is higher than the number of cameras (`CameraList()`); its camera has no scene; it already is the live camera; the live camera is not known (`live_camera` 0, so OBS shows something that is no camera); or the old two-scene mode applies. In every case the squeeze runs as today on the live camera.
5. **A failed switch never stops the time-out.** If OBS refuses the switch in, the squeeze runs on the live camera and `POST /api/screen` returns the reason in `obs_error`, as camera errors do today. If the switch back fails, the error is returned and logged; the time-out camera stays on air at full screen after the squeeze out, and the volunteer can press a camera button.
6. **A promo during the time-out** (`desconto` → `promo` → `desconto`) leaves and re-enters the time-out as the squeeze already does today: the program switches back for the promo and to the time-out camera again after it. This keeps one rule for every screen change.
7. **The setting lives in the Definições OBS card for now.** A select "Câmara no desconto" next to "Câmara", with "Nenhuma (fica a câmara que está no ar)" and one option per camera. It shows only with two or more cameras. Plan 011 builds Definições > Câmaras and may move it there.
8. **`timeout_camera` is panel-owned.** `PUT /api/settings` saves it, clamped: anything outside `0` to `3` is saved as `0`. A value above the number of cameras is kept (the volunteer may add the camera later) and skipped at the time-out (Decision 4).

## Current state

### Files and their roles

At `b28fb61`:

- `internal/store/store.go:23-41`: `type OBS struct`. Plan 008 added `Cameras []Camera`, `GraphicsScene`, `CameraFulls`, `CameraList()`, `FullFor()`.
- `internal/web/camera.go`:
  - `followCamera()` (89-94) at `b28fb61`: `return s.Camera.Follow(s.State.Snapshot().Screen.Current)`. Both `RunCamera` (on every state change, lines 98-119) and `POST /api/screen` (`internal/web/server.go:642-646`) call it, so it runs from two goroutines.
  - After plan 009: the `Settings` closure takes `Scene`/`Source` from the live camera (`s.liveCamera()`), `programScene(name)` maps a program scene to a camera and places it when the scene was not expected, and `POST /api/obs/camera/live` does Place → `expect` → `SetScene` → `SetLiveCamera` under `s.switchMu`. Plan 009's maintenance note asks for that sequence as one method, `s.switchTo(n int, window bool) (warning string, err error)`. If it exists, use it; if plan 009 inlined it in the handler, extract it first (Step 2) with no change in behaviour.
- `internal/obsws/camera.go`:
  - `Follow(screen)` (172-182 at `b28fb61`): under `c.follow`, returns early when the side (`in := screen == "desconto"`) is unchanged; otherwise `c.squeeze(in)`.
  - `squeeze(in)` (184-213): the legacy two-scene switch, else resolve the camera (after plan 009: in `set.Scene`) and tween with `move`. `move` starts from the item's real transform (`it.place`) when no tween runs (lines 450-455), so a camera put in the window just before a squeeze out tweens from the window.
  - After plan 009: `Place(scene, source, window)`, `Squeezed()`, `Legacy()`.
- `internal/web/server.go`: `screen` (618-648 at `b28fb61`) calls `followCamera` when OBS is enabled and returns `obs_error`; `PUT /api/settings` OBS lines (537-551 at `b28fb61`, with plan 008's additions after them).
- `web/painel/painel.js`:
  - Definições OBS card (2122-2133 at `b28fb61`): `#dObsCam` (Câmara) and `#dObsFit` (Enquadramento) in a `.cols` row.
  - `readSettingsForm()` (2537-2547): starts from `clone(S.settings)` and sets the OBS fields from the form.
- `web/painel/mock.js`: `db.settings.obs` (line 26-27, with plan 008's and 009's additions: `cameras`, `?cams=`, `?live=`).
- `internal/web/cameras_test.go` (created by plan 009): a fake OBS with camera scenes, a recorded list of `SetSceneItemTransform` (with `sceneName`) and `SetCurrentProgramScene`. Reuse it.
- `docs/CONTRACT.md` "Time-out squeeze" (303-313 at `b28fb61`, extended by plan 009).

### obs-websocket 5.x facts this plan relies on

Only what plan 009 already uses (protocol doc, https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md): `SetCurrentProgramScene` (`sceneName`), `SetSceneItemTransform`, and `CurrentProgramSceneChanged` (`sceneName`) on the events connection.

### Conventions

- As in plans 008 and 009: English code and comments, pt then en strings, no keys built at run time, `docs/CONTRACT.md` in the same commit, never `dados/` or `dist/`, conventional commits.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| Time-out camera tests | `go test ./internal/web/ -run TimeoutCamera -v` | pass |
| Panel test | `go test ./internal/web -run TestPanelTimeoutCamera` | pass (needs Chrome) |
| i18n | `go test ./internal/i18n` | pass |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Mock | `http://127.0.0.1:8765/painel?mock=1&obs=ready&cams=3&tcam=2#definicoes` | "Câmara no desconto" shows camera 2 |

## Suggested executor toolkit

- `tdd` for Steps 2 and 3.
- The select in Step 4 sits in an existing form row; reuse its classes. No `build-ui` pass is needed for one select in an existing card, but run `/impeccable critique` on the Definições OBS card afterwards, as the owner's global rule asks for small user-facing tweaks.

## Scope

**In scope**:

- `internal/store/store.go` (`TimeoutCamera`)
- `internal/web/camera.go` (`followCamera`, the time-out switch, `programScene` and the live route forget the return camera), `internal/web/server.go` (the `PUT /api/settings` clamp; one field)
- `internal/web/cameras_test.go` (new tests), `internal/web/camera_test.go` (new settings test)
- `web/painel/painel.js` (the select and `readSettingsForm`), `web/painel/mock.js` (`?tcam=`)
- `internal/web/panel_cameras_test.go` (new test)
- `web/i18n/pt.json`, `web/i18n/en.json`, `docs/CONTRACT.md`, `LEIA-ME.txt`, `READ-ME.txt`

**Out of scope**:

- `internal/obsws/camera.go`: no change is needed; the squeeze already works on the live camera after plan 009. If you find you must change it, that is a STOP condition.
- The overlay and its fixtures: the time-out screen is unchanged.
- A second, small inset (Decision 6 rules it out).
- Definições > Câmaras and the wizard (plan 011).

## Git workflow

- Branch: `04-feat/timeout-camera`, stacked on `03-feat/camera-switching`.
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The setting

In `internal/store/store.go`, add to `OBS` after `CameraFulls`:

```go
	// TimeoutCamera is the camera that goes on air in the time-out window,
	// 1 to MaxCameras; 0 keeps the live camera (the default).
	TimeoutCamera int `json:"timeout_camera"`
```

In `internal/web/server.go` `PUT /api/settings`, next to the `CameraFit` clamp:

```go
if req.OBS.TimeoutCamera < 0 || req.OBS.TimeoutCamera > store.MaxCameras {
	req.OBS.TimeoutCamera = 0
}
```

New test in `internal/web/camera_test.go`, `TestSettingsTimeoutCamera`: `PUT` with `timeout_camera` 2 saves 2; with 7 or -1 saves 0; a `PUT` without the key (a 0.1.2 panel) saves 0; a `settings.json` without it loads 0.

**Verify**: `go test ./internal/web/ -run Settings` → pass.

### Step 2: One switch method

If plan 009 left the switch inline in `POST /api/obs/camera/live`, extract it with no change in behaviour:

```go
// switchTo puts camera n (1-based) in place (in the window when window is
// true, else at full screen), then switches OBS to its scene and marks it
// live. The caller holds switchMu. warning is a placing failure, which
// never stops the switch.
func (s *Server) switchTo(n int, window bool) (warning string, err error)
```

The route calls it with `window = s.Camera.Squeezed()`.

**Verify**: `go test ./internal/web/ -run 'Camera|Program'` → plan 009's tests pass unchanged.

### Step 3: Switch at the time-out

Add to `Server`, under `liveMu`: `toIn bool` (the time-out switch saw `desconto`) and `returnTo int` (the camera to go back to; 0 = none).

Change `followCamera()`:

```go
func (s *Server) followCamera() error {
	if s.camErr != nil {
		return s.camErr
	}
	screen := s.State.Snapshot().Screen.Current
	terr := s.timeoutSwitch(screen == "desconto")
	err := s.Camera.Follow(screen)
	return errors.Join(terr, err)
}
```

`timeoutSwitch(in bool) error`, under `s.switchMu` for its whole run:

- If `in == toIn`, return nil. Else set `toIn = in`.
- **Entering** (`in` true): read `o := settings.obs`, `cams := o.CameraList()`, `live := s.liveCamera()`, `t := o.TimeoutCamera`. Skip (return nil, `returnTo = 0`) for every case in Decision 4 (legacy through `s.Camera.Legacy()`). Else `returnTo = live`, then `s.switchTo(t, false)` (full screen: Decision 1). On error set `returnTo = 0` and return it. A `warning` is logged with `log.camara.nao_pus`.
- **Leaving** (`in` false): `r := returnTo`, `returnTo = 0`. Skip when `r == 0`, `r` is now out of range or has no scene, or `r` is already live. Else `s.switchTo(r, true)` (window: Decision 2) and return its error.

Order matters: `timeoutSwitch` runs before `Follow`, so `Follow`'s squeeze resolves the camera that is live after the switch (plan 009's `Settings` reads the live camera). Entering, the squeeze tweens the time-out camera from full to the window. Leaving, it tweens the return camera from the window (where `switchTo` put it; `move` starts from the item's real transform) to full.

Forget the return camera (Decision 3):

- `POST /api/obs/camera/live`: after a successful switch, `returnTo = 0`.
- `programScene(name)`: when the scene was not the expected one, `returnTo = 0`.

New log and error keys (pt, then en):

| Key | pt | en |
|---|---|---|
| `log.camara.desconto_troca` | `Desconto: mudei para a câmara {n}.` | `Time-out: switched to camera {n}.` |
| `log.camara.desconto_volta` | `Fim do desconto: voltei à câmara {n}.` | `End of time-out: back to camera {n}.` |
| `log.camara.desconto_falhou` | `Não consegui mudar de câmara no desconto: {err}` | `I couldn't switch camera for the time-out: {err}` |

Log the two success lines (volunteers read `dados/marcador.log`), and wrap a switch error with `log.camara.desconto_falhou` before returning it, so `obs_error` reads as a sentence.

New tests in `internal/web/cameras_test.go` (three cameras A, B, C in `Câmara 1..3`; live camera 1; `timeout_camera` 2):

- `TestTimeoutCameraInAndOut`: `POST /api/screen {screen: desconto}`. The fake's records, in order: a transform on `Câmara 2` at B's full placement, `SetCurrentProgramScene Câmara 2`, then the tween's transforms on `Câmara 2` ending at B's window placement. `LiveCamera` is 2. `s.Camera.Stop()`, then `POST /api/screen {screen: marcador}`: a transform on `Câmara 1` at A's window placement, `SetCurrentProgramScene Câmara 1`, then transforms on `Câmara 1` ending at A's full placement. `LiveCamera` is 1.
- `TestTimeoutCameraOff`: `timeout_camera` 0 → no `SetCurrentProgramScene` in or out; the squeeze runs on `Câmara 1` as before.
- `TestTimeoutCameraSkipped`: one case per Decision 4 row (out of range, no scene, already live, `live_camera` 0) → no `SetCurrentProgramScene`.
- `TestTimeoutCameraManualSwitchWins`: enter `desconto`, then `POST /api/obs/camera/live {index: 3}`, then leave: no switch back to 1; camera 3 opens out to full.
- `TestTimeoutCameraOBSSwitchForgets`: enter, then `programScene("Câmara 3")` (unexpected), then leave: no switch back.
- `TestTimeoutCameraRefused`: the fake refuses `SetCurrentProgramScene` → `POST /api/screen` answers 200 with an `obs_error` naming the failure; the screen is `desconto`; the squeeze ran on `Câmara 1`; leaving sends no switch.
- `TestTimeoutCameraPromo`: `desconto` → `promo` → `desconto`: switches 2, 1, 2 (Decision 6).

Use `s.Camera.Stop()` between steps so each tween has ended before the next screen change, as plan 009's tests do.

**Verify**: `go test ./internal/web/ -run TimeoutCamera -v` → pass; `go test ./...` → pass.

### Step 4: The select in Definições

In `web/painel/painel.js`, in the Definições OBS card's `.cols` row after `#dObsFit`:

```html
<label class="field" id="dObsTcamField"><span>Câmara no desconto</span>
  <select id="dObsTcam">
    <option value="0">Nenhuma (fica a câmara que está no ar)</option>
    <option value="1">Câmara 1</option> …
  </select></label>
```

- Options: `0` with `t('painel.definicoes.desconto_nenhuma')`, then one per camera in `S.settings.obs.cameras` with its label, else `t('painel.camaras.camara', { n })` (plan 009's key). Select `o.timeout_camera`. A saved value above the number of cameras shows as an extra selected option `t('painel.definicoes.desconto_sem_camara', { n })` so a save never changes it silently.
- Hide `#dObsTcamField` with fewer than two cameras.
- Under the row, a hint when the field shows: `t('painel.definicoes.desconto_dica')`.
- `readSettingsForm()`: `s.obs.timeout_camera = $('#dObsTcam') ? parseInt($('#dObsTcam').value, 10) || 0 : (s.obs.timeout_camera || 0)`.

New pt keys (then en):

| Key | pt | en |
|---|---|---|
| `painel.definicoes.desconto_camara` | `Câmara no desconto` | `Camera during time-outs` |
| `painel.definicoes.desconto_nenhuma` | `Nenhuma (fica a câmara que está no ar)` | `None (keep the camera on air)` |
| `painel.definicoes.desconto_sem_camara` | `Câmara {n} (já não existe)` | `Camera {n} (no longer there)` |
| `painel.definicoes.desconto_dica` | `No desconto, o OBS muda para esta câmara e a imagem encolhe para a janela do grafismo. No fim, volta à câmara que estava no ar.` | `During a time-out, OBS switches to this camera and its picture shrinks into the graphics window. At the end it goes back to the camera that was on air.` |

In `web/painel/mock.js`: `?tcam=0|1|2|3` sets `db.settings.obs.timeout_camera` (default 0).

Mock states for screenshots (all `?mock=1&obs=ready`, then `#definicoes`):

| State | URL suffix | What shows |
|---|---|---|
| One camera | `&cams=1` | no "Câmara no desconto" field |
| Three cameras, none chosen | `&cams=3&tcam=0` | the field on "Nenhuma", the hint |
| Camera 2 chosen | `&cams=3&tcam=2` | "Bancada" selected (camera 2's label in the mock) |
| A camera that is gone | `&cams=2&tcam=3` | "Câmara 3 (já não existe)" selected |

New test in `internal/web/panel_cameras_test.go`, `TestPanelTimeoutCamera`: `/painel?mock=1&obs=ready&cams=3&tcam=2#definicoes` → `#dObsTcam` value is `"2"`; set it to `"3"`, save the settings form the way the existing tests do (find the save button in `panel_sport_test.go` or the Definições form's submit), and check the mock's saved `timeout_camera` is 3 (read it back with `fetch('/api/settings')` inside `chromedp.Evaluate`). `&cams=1` → `#dObsTcamField` is hidden.

**Verify**: `go test ./internal/web -run TestPanelTimeoutCamera -v` → pass (with Chrome); `go test ./internal/i18n` → pass.

### Step 5: Contract and the read-me

- `docs/CONTRACT.md`:
  - "Panel API", `GET /api/settings`: add `"timeout_camera": 0` to the `obs` example, with a sentence on its values and the clamp.
  - "Time-out squeeze": a bullet for the time-out camera: Decisions 1 to 6 in short (enter: full, switch, squeeze in; leave: window, switch back, squeeze out; what is forgotten; when it is skipped; errors in `obs_error`; promo).
  - The panel's mock states list: add `&tcam=0|1|2|3`.
- `LEIA-ME.txt` / `READ-ME.txt`, section "DESCONTO DE TEMPO COM O OBS": two lines on "Câmara no desconto" (where it is, what it does, "Nenhuma" keeps the camera on air).

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- `internal/web/camera_test.go`: `TestSettingsTimeoutCamera` (Step 1).
- `internal/web/cameras_test.go`: seven time-out camera tests (Step 3).
- `internal/web/panel_cameras_test.go`: `TestPanelTimeoutCamera` (Step 4).
- Manual with a real OBS and two cameras on a scratch build: set camera 2 as the time-out camera, start a time-out from the panel, watch camera 2 shrink into the window in step with the hole, end it and watch camera 1 open out. Repeat with OBS's transition set to Cut and to Fade.

## Done criteria

- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, new tests present
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `grep -n '"timeout_camera"' internal/store/store.go docs/CONTRACT.md` matches in both
- [ ] `git diff <009's last commit>..HEAD -- internal/obsws/camera.go` is empty
- [ ] No existing test case modified (`git diff <009's last commit>..HEAD -- '*_test.go'` shows only additions)
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plan 008 or 009 has not landed (the "Depends on" checks fail).
- The squeeze needs a change in `internal/obsws/camera.go` to line up after a switch (for example, `move` does not start from the window after `Place`).
- On a real OBS, the switch in shows the time-out camera outside the hole during the transition (Decision 1 assumes a full-screen picture under a shrinking hole hides that).
- `followCamera` running from `RunCamera` and from `POST /api/screen` at the same moment switches twice. `switchMu` plus the `toIn` check should prevent it; if a test shows a double switch, stop and report.
- An existing test fails and only editing it would make it pass.
- A verification fails twice after a reasonable fix.

## Open questions / corrections

1. **"With its picture in the time-out window" (Decision 6)** is the state at the end of the entrance. This plan switches with the picture at full screen and lets the squeeze shrink it, rather than switching to a picture already in the window (Decision 1 here, and why). Decision 5's "window first, then switch" is kept for leaving and for a hand switch during the time-out. Recommended: accept; the hole and the picture line up through the whole move either way only with this order.
2. **Where the volunteer sets `timeout_camera`** is not in the decisions file. Build order puts this plan before Definições > Câmaras (plan 011), so the select goes in the Definições OBS card. Plan 011 may move it into Câmaras; the key stays `timeout_camera`.
3. **A hand switch during the time-out** is not covered by Decision 6 ("leaving switches back to the camera that was live"). This plan treats the volunteer's later pick as the one to keep (Decision 3). Recommended: accept; switching back over a deliberate choice would surprise the volunteer.
4. **The old two-scene mode** and an unknown live camera skip the time-out camera. Neither is in the decisions file.

## Maintenance notes

- Plan 011's Câmaras section, if it moves the select, keeps `readSettingsForm`'s `timeout_camera` line and the hide-under-two-cameras rule.
- Plan 012's health line could warn when the time-out camera shows "Sem imagem" before a time-out; not in scope here.
- If a club later asks for a true inset (Decision 6 leaves it for later), it is a second source in the camera scene and an overlay change, separate from this switch.
- Reviewers: run a real time-out with the time-out camera on a phone through DroidCam, and one with OBS streaming, and check `dados/marcador.log` (in the scratch build folder) has the two lines.
