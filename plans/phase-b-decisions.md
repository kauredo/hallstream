# Phase B decisions: multiple cameras (input for its plans)

Repo: `app/` at `b28fb61` (v0.1.2). Roadmap text (docs/ROADMAP.md, Phase B):

1. "Configurar OBS" builds one scene per camera (`Câmara 1` to `Câmara 3`), each nesting one shared `Grafismo` scene that holds the overlay.
2. The panel gets camera buttons and shows which camera is live.
3. At a timeout, a chosen camera shows picture-in-picture.
4. Onboarding adds cameras: DroidCam, a video device, or VDO.Ninja through a QR code on the panel.
5. Each camera gets a health line in the panel (no frames, low fps).
6. A guide covers two and three cameras on wifi and on cable, with the bandwidth each needs.

Roadmap decision row: "One OBS scene per camera, with the overlay in a shared nested scene, switched from the panel. Sources: DroidCam, any OBS video device, and VDO.Ninja (a phone browser, no app). Up to three cameras to start."

The owner asked for Phase B on 2026-11-26 ("/ship phase b", stacked PRs). These answers were settled unattended; each row says why, so the owner can overturn one before its PR merges.

## Facts

- Today "Configurar OBS" (`POST /api/obs/setup`, `internal/obsws/setup.go`) builds one scene, `Hallstream`, holding the camera and the overlay browser source above it, and makes it the program scene. The health check runs the same steps without changes every 30 s.
- One camera is saved: `settings.obs.camera_source`, `camera_fit`, `camera_full` (`internal/store/store.go:23`).
- At a time-out (`desconto`) the server tweens the camera item inside the program scene into the window (`internal/web/camera.go`, contract "Time-out squeeze"), and the overlay shrinks its hole in step. A refit loop puts the camera back at full screen every 4 s on `marcador` and `nada`.
- `POST /api/obs/cameras` lists camera choices: OBS video sources, unused `dshow_input` devices, and the DroidCam OBS plugin.
- obs-websocket has `SetCurrentProgramScene`, `CreateSceneItem` with a scene as the source (nesting), `GetSourceScreenshot`, `GetSourceActive` and `GetStats`. It reports no frame rate per source.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | ✅ | Settings shape | `settings.obs.cameras`: up to 3 `{ "source", "scene", "label" }` in order. `camera_source` stays and mirrors camera 1, so older code paths and a downgrade keep working. `camera_fit` stays one setting for all cameras; `camera_full` becomes per camera (keyed by source). | Additive. A downgrade to 0.1.2 reads `camera_source` and still finds its camera. |
| 2 | ✅ | Scene names | Created in the panel's language at setup ("Câmara 1" / "Camera 1", "Grafismo" / "Graphics") and saved in `cameras[i].scene` and `obs.graphics_scene`. Found by the saved name after that, so changing the language never orphans a scene. | Clubs work in their own language; scene names are labels the volunteer sees in OBS. |
| 3 | 🔥 | The old `Hallstream` scene | Configurar OBS moves to the new layout: it builds `Grafismo` with the overlay and `Câmara 1` with camera 1 and `Grafismo` nested above it, makes `Câmara 1` the program scene (not while streaming or recording: `later`), and leaves `Hallstream` in OBS untouched. The health check counts the old layout as ready while only one camera is set, so updating Hallstream never turns the chip to "Precisa de reparação" on game day. | Never deleting a user's scene is an existing rule. A club with one camera changes nothing until it runs Configurar OBS or adds a camera. |
| 4 | ✅ | Switching cameras | A camera button calls `POST /api/obs/camera/live` `{ "index" }`, which sets the program scene to that camera's scene with OBS's own transition. The live camera is read back from `CurrentProgramSceneChanged`, so a switch made in OBS shows in the panel too. The live state carries `obs.live_camera`. | OBS stays the mixer (roadmap). Reading the event keeps the panel honest when someone clicks in OBS. |
| 5 | ✅ | Squeeze with several cameras | The squeeze moves the camera item of whichever camera scene is on program. Switching cameras during `desconto` places the new camera at the window first, then switches, so the hole and the picture still line up. Each camera's full placement is saved per camera. | Keeps the time-out look identical with any camera live. |
| 6 | 🔥 | "Picture-in-picture at a timeout" | `settings.obs.timeout_camera` (`0` = none, default; `1`–`3`). With it set, entering `desconto` switches to that camera with its picture in the time-out window, and leaving switches back to the camera that was live. No second small inset. | The time-out screen already is a picture inside the graphics (window, stats column, promo card), so a second inset would collide with them. The bench or a close camera in the window is what a club wants at a time-out. A true inset can come later if a club asks. |
| 7 | ✅ | Adding cameras (onboarding) | Definições > Câmaras lists up to three cameras, each picked from the existing choices (DroidCam, a video device, an OBS source) or "Telemóvel (VDO.Ninja)". The first-run wizard's OBS step gains "Adicionar outra câmara" after the first; skipping keeps one camera. | Reuses the camera choice list that exists. |
| 8 | ✅ | VDO.Ninja | A random stream id per camera, saved in `cameras[i].vdo`. The program adds a browser source pointing at VDO.Ninja's view link for that id and shows a QR code of the push link on the panel; the phone opens it in its browser. The panel says it needs internet to connect. Exact URL parameters come from VDO.Ninja's docs at plan time. | Roadmap names it. The rest of Hallstream stays offline-first; only this source needs the internet, and the panel says so. |
| 9 | 🔥 | Camera health | One line per camera on the Ecrãs camera buttons: "Sem imagem" (the source reports 0x0 or is not active), "Imagem parada" (two small screenshots 10 s apart are identical), else OK. OBS's skipped-frames count (`GetStats`) shows once for the whole program, not per camera. | obs-websocket gives no per-source frame rate, so "low fps" per camera can't be measured honestly. A frozen phone picture is the failure clubs actually hit. |
| 10 | ✅ | The guide | `docs/cameras/GUIDE.md` and `GUIDE.en.md`, shipped in the zip next to the YouTube guide as `CAMARAS.txt` / `CAMERAS.txt`, and linked from Definições > Câmaras. Bandwidth numbers come from DroidCam's and VDO.Ninja's docs at plan time. | Same pattern as the YouTube guide. The website page can follow in the website repo. |
| 11 | ✅ | Words | Every new string in `pt.json` first, then `en.json`, as the contract's "Words" rule says. | Existing rule. |
| 12 | ✅ | Design | The camera buttons and the Câmaras section go through `build-ui` (extension branch inside the panel's design system) with an outside critique. | The panel is what volunteers touch mid-game. |

## Build order (one stacked PR each)

1. Plans (this file and plans 008 to 013).
2. Settings for several cameras, and Configurar OBS builds `Grafismo` plus one scene per camera, with the health check accepting the old layout (roadmap 1; decisions 1–3).
3. Camera buttons on Ecrãs, switching and the live camera in the state, the squeeze following the live camera (roadmap 2; decisions 4–5).
4. The time-out camera (roadmap 3; decision 6).
5. Adding cameras in Definições and the wizard, VDO.Ninja with its QR code (roadmap 4; decisions 7–8).
6. The health line per camera (roadmap 5; decision 9).
7. The guide in both languages, shipped in the zip (roadmap 6; decision 10).

Each PR: `go vet ./...`, `go test ./...`, `GOOS=windows GOARCH=386 go build ./...`, the Chrome panel tests with new mock states, pt.json then en.json, and `docs/CONTRACT.md` in the same commit as the behaviour.

## Done when

A club with three cameras runs Configurar OBS and gets `Grafismo` and three camera scenes; the panel switches between them and shows which one is live; a time-out shows the chosen camera in the window and returns to the live one; a phone joins through VDO.Ninja from a QR code; each camera shows its health; the zip carries the guide. A club with one camera that never runs Configurar OBS again sees no change.

## Corrections from planning (2026-11-26)

The plans found these against the table above. Each plan carries the detail; this list wins over the table where they differ.

- Decision 1: per-camera full placements live in `obs.camera_fulls` (a map keyed by source); `camera_full` stays as camera 1's copy. `cameras` is server-owned on `PUT /api/settings` in 008 (an old panel page can't wipe it); 011 edits the list through `POST /api/obs/setup`, and the server keeps `vdo` and `camera_fulls` on every save.
- Decision 2: a scene name already used in OBS is never adopted; the program takes the next free name ("Grafismo 2").
- Decision 3: the program step accepts any camera scene on program. A club that migrates, then deletes `Grafismo` while `Hallstream` remains, stays "ready" with one camera.
- Decision 4: the OBS client subscribes to no events today, so 009 adds a second connection that listens to scene events only. The live state gets a top-level `"obs": { "live_camera": 0 }`.
- Decision 5: the 4 s refit does not bring a camera back from the window, so 009 places the camera before every switch and after a switch made in OBS. Studio Mode is a STOP check in 009's manual OBS test. The old two-scene mode (`timeout_scene`) refuses a switch during `desconto`.
- Decision 6: entering, 010 switches with the camera at full screen and lets the squeeze shrink it; leaving, it puts the window first, then switches. A switch made by hand during the time-out wins (nothing switches back). `timeout_camera` is a select in the Definições OBS card in 010; 011 may move it to Câmaras, and removing a camera moves or resets it.
- Decision 7: a second DroidCam phone needs the plugin offered again with a USB device choice. "Telemóvel (VDO.Ninja)" is added by the panel, not by `choices()`.
- Decision 8: the QR route is `GET /api/obs/cameras/{n}/qr.svg` behind the PIN, `no-store`. No VDO.Ninja password: the QR carries the whole link. The phone's browser source keeps `shutdown: false`.
- Decision 9: "not active" means "not on Program", so it is dropped. A one-colour screenshot also counts as "Sem imagem" (a phone source never reports 0x0). Health rounds every 10 s with 128x72 screenshots, skipped during a squeeze, a setup or a lost connection. Whether OBS renders an off-air scene's source for a screenshot is the first STOP test in 012.
- Decision 10: the guide goes at the zip root beside `LEIA-ME.txt`; the panel names the file and links to GitHub. The guide says DroidCam's free version is 480p.
