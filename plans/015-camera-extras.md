# 015: Camera extras (four cameras, screen cameras, clean cameras, an inset)

Repo `app/` at `dc2552e` (main, v0.1.4). Independent of the Mac/Linux stack (#56–#60); docs layers may conflict with #60 and get rebased after it lands.
The owner asked on 2026-11-26 for "a camera for each screen, more than three cameras, an inset with two cameras, overlays per camera, automatic switching that follows the play", and to settle the questions unattended. Each 🔥 row is a pick the owner can overturn before its PR merges.


## Facts found

- `store.MaxCameras = 3` (`internal/store/store.go:56`); validation in `internal/web/obs.go:284`, `server.go:578`.
- Time-out camera: `settings.obs.timeout_camera`, run by `timeoutSwitch` in `internal/web/camera.go:142` under `switchMu`, with `returnTo` remembering the camera to go back to. A hand switch during the time-out wins.
- The overlay's backdrop screens cover the camera completely: `BG_SCREENS = abertura, plantel, apoiantes, intervalo, final, proximo, voltamos, promo` (`web/overlay/overlay.js:29`). Only `marcador`, `nada` and `desconto` show the camera.
- Every camera scene nests the one `Grafismo` scene; the overlay is one page and today ignores `obs.live_camera`.
- obs-websocket gives no motion or ball data; OBS has no auto-director.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | 🔥 | What does "a camera for each screen" mean when 8 of 11 screens cover the camera? | Generalise the time-out camera into `obs.screen_cameras`, a map from screen to camera, offered for `nada` and `desconto` only. `timeout_camera` stays and mirrors `screen_cameras.desconto` (downgrade safe). | Picking a camera for a screen nobody sees is a setting that does nothing. Risk: a club wants a camera on `marcador` too; that is the live camera by definition. |
| 2 | ✅ | Switching rules between screens with cameras | One rule for all: entering a screen with a camera remembers the live camera (only if nothing is remembered yet) and switches; entering a screen without one switches back to the remembered camera. A hand switch while on such a screen clears the memory (hand wins, as today). | Same machinery as `timeoutSwitch`, so `nada → desconto → marcador` returns to the camera that was live before `nada`. |
| 3 | 🔥 | How many cameras? | `MaxCameras = 4`. | Phones on hall wifi run out of bandwidth before cameras run out; four covers two baskets, bench and a wide shot. Risk: a club with capture cards wants six; the constant is one line to raise later. |
| 4 | ✅ | What else changes with 4 | Setup steps get `-4` ids, scene `Câmara 4`, the panel's camera card and Definições rows wrap, the guide gets a four-camera row. | Mechanical. |
| 5 | 🔥 | "Overlays per camera" | Per camera "Mostrar marcador" (`cameras[i].clean`, default false = show). While a clean camera is live on `marcador`, the overlay hides the corner scoreboard, the sponsor corner and the lower third. Full screens and the time-out still show on every camera. Go puts `obs.clean` in the live state; the overlay reads only that. | Hiding `Grafismo` in a scene would also hide intervalo or a time-out on that camera, so the operator picks a screen and nothing appears. Risk: someone wanted different graphics per camera, not just less. |
| 6 | 🔥 | Inset: what and when | One small inset, 480x270, of a chosen second camera, toggled from Ecrãs ("Inset: Câmara N" / off). Shown only on `marcador` and `nada`; hidden on every other screen and when the inset camera is the live one. | The time-out window and full screens already place the picture; an inset there collides. |
| 7 | ✅ | Inset in OBS | Setup adds, in every camera scene, a hidden item of each other camera's source between the camera and `Grafismo` (a source can sit in many scenes). Showing the inset sets that item's transform and shows it in the live scene only. | Uses what OBS does natively; no new sources, no extra bandwidth (the source is already decoded). |
| 8 | 🔥 | Inset position | The bottom corner the lower third doesn't use, at the overlay's corner margin. Go puts `obs.inset` (rect or null) in the state; the overlay draws a thin frame and the camera's label around it, and moves the lower third aside if needed. | Builder confirms the free corner in `overlay.css`; if no corner is free on some layout, top corner opposite the bug. |
| 9 | 🔥 | Automatic switching that follows the play | Not built. Following the ball needs computer vision on the PC, a model, and tuning per hall: its own research spike, not a PR. The ROADMAP gets a "Later" row with this reason. | The owner asked to ship it all; this is the one item that can't honestly ship in a PR. Risk: owner wanted at least a rule-based rotation. |
| 10 | ✅ | Settings ownership | `screen_cameras` and `clean` are saved through Definições > Câmaras ("Guardar e configurar o OBS" path, `POST /api/obs/setup`), with reorder/remove moving or resetting them like `timeout_camera` today. The inset toggle is live state, not a setting (`POST /api/obs/inset`), and resets on restart. | Matches how cameras are owned now. |
| 11 | ✅ | Words and contract | pt.json first then en.json; `docs/CONTRACT.md` in the same commit as the behaviour. | Repo rules. |
| 12 | ✅ | Stack | 1 plan · 2 four cameras · 3 screen cameras · 4 clean cameras · 5 inset · 6 docs (guide, readmes, ROADMAP). Branch off main in worktree `app-cams`. | Safest first; the inset touches OBS setup and the overlay, so it goes last. |

## Layers

### 1. `01-docs/plan-015` — plans/015-camera-extras.md (this file, edited), plans/README row.

### 2. `02-feat/four-cameras`
- `store.MaxCameras = 4`; every "1 to 3" in code, contract, pt/en strings and the panel.
- Setup step ids `-4`, scene names, `phones` etc. already loop; check `setup.go` for 3-specific code.
- Panel: camera card and Definições rows lay out four (wrap at phone width). Mock states with 4 cameras; Chrome panel tests.
- Tests: setup with four cameras, validation rejects five.

### 3. `03-feat/screen-cameras`
- `store.OBS.ScreenCameras map[string]int` (`screen_cameras`), keys `nada`, `desconto`; `TimeoutCamera` mirrors `desconto` both ways on load and save.
- Generalise `timeoutSwitch(in bool)` into a switch on the current screen using decision 2. Keep log strings per screen.
- `PUT /api/settings` / setup save path: validate keys and range; reorder/remove remaps like `timeout_camera` in `obs.go:359`.
- Panel Definições > Câmaras: "Câmara no desconto" stays; add "Câmara no ecrã Nada" select with the same states.
- Tests: nada→marcador returns; nada→desconto→marcador returns to the first live camera; hand switch during nada wins; mirror with `timeout_camera`.

### 4. `04-feat/clean-cameras`
- `store.Camera.Clean bool` (`clean`); kept across saves like `label`.
- Live state `obs.clean` = live camera's `clean`.
- Overlay: `html[data-clean]` hides bug, sponsor corner and lower third on `marcador` and `nada`, with the usual fade.
- Panel: a "Mostrar marcador" switch per camera row in Definições > Câmaras (existing switch component).
- Tests: Go state, overlay Chrome test with a clean state, panel row.

### 5. `05-feat/camera-inset`
- Setup: in each camera scene, one hidden item per other camera's source, between camera and `Grafismo`; step id `inset[-n]`; idempotent; never touches user items.
- `POST /api/obs/inset` `{ "camera": n }` (0 = off): validates, stores in memory, applies: transform + show in the live scene, hide elsewhere. Reapply on camera switch, on screen change (shown only on marcador/nada), when the inset camera goes live (hide).
- Live state `obs.inset`: `{ "camera", "x","y","w","h" }` in overlay px or null.
- Overlay draws a frame and label at `obs.inset`; moves the lower third if it collides.
- Panel Ecrãs camera card: an "Inset" row of buttons (off + each other camera).
- Tests: setup items, apply/hide logic with a fake OBS, overlay frame, panel buttons.

### 6. `06-docs/camera-extras`
- docs/cameras/GUIDE(.en).md: four cameras, inset, clean cameras, screen cameras.
- LEIA-ME/READ-ME camera lines, README, ROADMAP (Phase B extras done, auto-follow in Later with reason), plans/README row → DONE.

Each layer: `go vet ./...`, `GOOS=linux CGO_ENABLED=0 go vet ./...`, `go test ./...`, `GOOS=windows GOARCH=386 go build ./...`, gofmt, panel and overlay Chrome tests.

## Done when

A club can save four cameras; pick a camera for `nada` and for `desconto` and get switched and switched back; mark a camera as clean and see the corner graphics hide only while it is live on the scoreboard; turn on an inset of another camera on the scoreboard from Ecrãs; and the guide and ROADMAP say all of this, with auto-follow recorded as not built and why.
