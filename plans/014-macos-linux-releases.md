# Plan 014: Every release ships a notarised Mac app and a Linux build next to the Windows zip

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat dc2552e..HEAD -- build.sh scripts/notices.sh .github/workflows cmd/hallstream/main.go internal/desktop internal/obsapp internal/store/store.go internal/web/server.go internal/web/obs.go internal/web/bluetooth.go web/painel/painel.js web/i18n LEIA-ME.txt READ-ME.txt`
> Compare every "Current state" excerpt with the live code. Line numbers
> are from `dc2552e` and may have moved; a change you can map one-to-one is
> fine, one you can't is a STOP condition.

## Owner checklist (only the owner can do these)

Do 1 to 4 before PR 2 of the stack merges. A tag pushed without them fails the release on purpose (Decision 9).

1. **Add five repository secrets** to `kauredo/hallstream` (Settings > Secrets and variables > Actions). The BVA repo already has values for all five under the same names, and they can be reused: one Developer ID certificate signs any number of apps.
   - `MACOS_CERTIFICATE`: the "Developer ID Application" certificate and its private key, exported from Keychain Access as a `.p12`, then base64 (`base64 -i cert.p12 | pbcopy`).
   - `MACOS_CERTIFICATE_PWD`: the password set on that `.p12` export.
   - `APPLE_API_KEY`: the App Store Connect API key file `AuthKey_XXXX.p8`, base64.
   - `APPLE_API_KEY_ID`: that key's ID (10 characters).
   - `APPLE_API_ISSUER`: the issuer ID shown above the keys list in App Store Connect (a UUID).
   With the GitHub CLI: `gh secret set MACOS_CERTIFICATE --repo kauredo/hallstream < cert.b64`, and the same for the others.
2. **Check the certificate type and date.** In Keychain Access the certificate must be named "Developer ID Application: <name> (<team id>)". "Apple Development", "Mac App Distribution" or "Developer ID Installer" are refused by the notary service. Check it has not expired.
3. **Check the API key's access.** It must be a team key (it has an issuer ID) with the Developer role or higher. BVA's key already notarises, so it qualifies.
4. **Accept any pending agreement** in the Apple Developer account (developer.apple.com > Account). The notary service refuses submissions while one is outstanding. No bundle ID registration is needed: Developer ID apps outside the App Store do not register theirs.
5. **Test once on real hardware** before announcing Mac support: on a Mac, open the DMG from a release, drag the app to Applications, start it, accept the Bluetooth prompt and connect to a Stramatel box. If the executor had no Mac, this item also covers the Mac checks marked "(Mac)" in Steps 1.2, 1.4 and 2.3, and the SIGINT measurement in Step 1.4. On a Linux PC if one is at hand, the same with the tarball.
6. **Merge the website PR** (PR 5, website repo, a follow-up outside this repo) after the first release that carries the Mac and Linux files, so the download section has real files to point at.

## Status

- **Priority**: P2 (ROADMAP "Windows first, macOS and Linux builds after", Phase A item 8)
- **Effort**: L (a stack of four PRs here and one in the website repo)
- **Risk**: MED. The Windows exe and zip must not change. The new code paths only run on macOS and Linux, and the release workflow gains jobs.
- **Depends on**: none. Plans 001 to 013 do not touch these files except `main.go` and the i18n files, which change often; rebase over them.
- **Category**: direction / DX (release machinery)
- **Planned at**: commit `dc2552e`, 2026-11-26

## Why this matters

Hallstream releases one zip with two Windows exes. Clubs with a Mac or a Linux PC in the hall cannot use it, and the ROADMAP has promised those builds since Phase A ("Windows first, macOS and Linux builds after", `docs/ROADMAP.md:17`; "Signed Windows builds and notarised macOS builds", `docs/ROADMAP.md:55`). CI already proves the code compiles on Linux and macOS (`.github/workflows/ci.yml:22` and the `macos` job at `:27-38`). What is missing is the desktop layer, which only Windows has today, and the packaging. The desktop layer is the tray icon, the message boxes, starting and closing OBS, "Abrir com o Windows" and the clipboard. Packaging means a signed and notarised `.app` for Macs, since an unsigned app from the internet will not open on a current Mac without a trip to System Settings, and a tarball for Linux.

SignPath rejected the Windows signing application, so Windows stays unsigned. Only the Mac build is signed.

## Facts measured or read for this plan

Measured on the owner's Mac (macOS 26, OBS 33) on 2026-11-26:

- obs-websocket's config is at `~/Library/Application Support/obs-studio/plugin_config/obs-websocket/config.json`. That is what `configPath()` already returns on macOS, because `os.UserConfigDir()` is `~/Library/Application Support` there (`internal/obsapp/obsapp.go:43-51`).
- OBS's bundle is `/Applications/OBS.app`, bundle ID `com.obsproject.obs-studio`, executable `OBS` (`defaults read /Applications/OBS.app/Contents/Info.plist CFBundleIdentifier` and `CFBundleExecutable`).
- **If OBS starts with `--minimize-to-tray` on its first run, its browser sources never render** (CEF starts no renderer). The overlay is a browser source, so a minimised first start leaves the stream with no score.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/hallstream` succeeds and gives a statically linked ELF. On Linux, the tray (`fyne.io/systray`) and Bluetooth (`tinygo.org/x/bluetooth`) both talk D-Bus in pure Go, so the Linux build needs no cgo and can be cross-built on the Ubuntu runner.
- On this Mac, `CGO_ENABLED=1 GOARCH=amd64` and `GOARCH=arm64` builds both succeed, and `lipo -create` joins them (`lipo -archs` prints `x86_64 arm64`, 33 MB unstripped). So one macOS runner can build both architectures.
- **Without `MACOSX_DEPLOYMENT_TARGET`, the cgo build is stamped with the build machine's macOS** (`otool -l` shows `minos 26.0`), and an older Mac refuses to start it. With `MACOSX_DEPLOYMENT_TARGET=13.0` it shows `minos 13.0`.

Read in the docs:

- Go 1.27 (this repo's `go.mod` says `go 1.27.3`) needs **macOS 13 Ventura or newer** and Linux kernel 3.2 or newer. https://go.dev/wiki/MinimumRequirements
- Notarisation needs a Developer ID Application signature, the Hardened Runtime, a secure timestamp, and no `get-task-allow` entitlement. https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution
- The notary service accepts UDIF disk images, flat installer packages and ZIP archives; an `.app` goes up as a ZIP made with `ditto -c -k --keepParent`. `--wait` makes `notarytool` return only when Apple has finished. A ZIP cannot be stapled; staple the items inside and zip again. A stapled ticket "ensures that Gatekeeper can find the ticket even when a network connection isn't available". https://developer.apple.com/documentation/security/customizing-the-notarization-workflow
- The API-key flags `--key <p8 path> --key-id <id> --issuer <uuid>` are in `xcrun notarytool submit --help`, and BVA's workflows use them: `/Users/kauredo/code/personal/basketball-video-analyzer/app/.github/workflows/release.yml:100-160` (keychain import and submit) and `.../notarize.yml` (wait and staple).
- Hardened Runtime has entitlements for the camera, microphone, location, contacts, calendars, photos and Apple events. **None for Bluetooth**: Bluetooth on macOS is governed by the privacy prompt and its usage string. https://developer.apple.com/documentation/security/hardened-runtime
- `NSBluetoothAlwaysUsageDescription`: "This key is required if your app uses the device's Bluetooth interface." macOS 11 and later. A process that touches CoreBluetooth without it is killed by the privacy system, so the Mac app must be a bundle with an `Info.plist`. https://developer.apple.com/documentation/bundleresources/information-property-list/nsbluetoothalwaysusagedescription
- `LSUIElement`: "A Boolean value indicating whether the app is an agent app that runs in the background and doesn't appear in the Dock." https://developer.apple.com/documentation/bundleresources/information-property-list/lsuielement
- Universal binaries: build each architecture, then `lipo -create -output <out> <x86> <arm>`; check with `lipo -archs`. https://developer.apple.com/documentation/apple-silicon/building-a-universal-macos-binary
- `fyne.io/systray` v1.12.2 (module cache, `README.md` and `systray.go:36-38`): it calls `runtime.LockOSThread()` in `init`, so `systray.Run` must be called from `main`, which `cmd/hallstream/main.go:362` already does. On macOS it needs an app bundle and suggests `LSUIElement`. On Linux it uses the StatusNotifierItem/AppIndicator D-Bus spec with no GTK; "older tray implementations may not load the icon", and GNOME needs an extension (Ubuntu ships one turned on; elsewhere https://extensions.gnome.org/extension/615/appindicator-support/). `SetIcon` takes `.ico/.jpg/.png` outside Windows, and the macOS side draws it at 16×16 points (`systray_darwin.m:402`). https://github.com/fyne-io/systray
- `tinygo.org/x/bluetooth` v0.17.0 (`README.md`): Linux uses BlueZ over D-Bus ("a fairly recent version of BlueZ"); macOS uses CoreBluetooth through cgo and must be compiled on macOS. On macOS `Enable` waits up to 10 s for the radio and returns an error otherwise (`adapter_darwin.go:50-73`); a refused permission shows up that way. https://github.com/tinygo-org/bluetooth#linux and #macos
- OBS launch parameters: `--minimize-to-tray`, `--multi`; on macOS they are passed with `open -a "OBS" --args <params>`. https://obsproject.com/kb/launch-parameters
- OBS on Linux comes as `obs` on the PATH (Ubuntu PPA, distro packages) or as the Flatpak `com.obsproject.Studio` (https://flathub.org/apps/com.obsproject.Studio). A Flatpak app's `XDG_CONFIG_HOME` is `~/.var/app/<app-id>/config` (https://docs.flatpak.org/en/latest/conventions.html), so the Flatpak OBS keeps obs-websocket's file at `~/.var/app/com.obsproject.Studio/config/obs-studio/plugin_config/obs-websocket/config.json`.
- OBS shutdown on macOS and Linux: OBS installs a SIGINT handler on non-Windows systems ("UI: Properly handle SIGINT on non-Windows platforms", https://git.tjdev.de/mirror/obs-studio/commit/51823680092cd3b137ea968ddf8e42cc7bb0d0aa, a mirror of upstream). Forum reports say SIGTERM is not a clean exit (https://obsproject.com/forum/threads/command-line-exit.71266/latest). **Whether SIGINT saves OBS's settings is unverified**, and Step 1.4 measures it.
- AppImages need FUSE 2, which Ubuntu 22.04 and later lack by default (`libfuse2`, `libfuse2t64` on 24.04). https://docs.appimage.org/user-guide/troubleshooting/fuse.html
- XDG autostart: a `.desktop` file in `~/.config/autostart/` starts at sign-in. https://specifications.freedesktop.org/autostart-spec/latest/ . A per-user launchd agent is a plist in `~/Library/LaunchAgents` with `RunAtLoad`. https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html
- Desktop BlueZ setups let the signed-in user use Bluetooth over D-Bus. Some distributions also want the user in the `bluetooth` group (https://wiki.archlinux.org/title/Bluetooth). READ-ME says so in one line, and the code does nothing about it.

## Decisions made in this plan (the executor does not re-decide them; the owner may overturn any before merge)

1. **One stack, four PRs here, one in the website repo.** (1) desktop code for macOS and Linux, with no packaging; (2) the Mac app, signing and notarisation in the release; (3) the Linux tarball in the release; (4) docs. (5) is the website download section, in `/Users/kauredo/code/personal/hallstream/website`. PR 1 is safe to merge alone, since it changes nothing on Windows and nothing ships. PR 2 needs the owner checklist done. The owner can land the stack in one go when it is green, per their habit.
2. **Mac format: a universal `Hallstream.app` inside a DMG.** A DMG with an `Applications` link is the drag-to-install every Mac user knows, and the website already maps `.dmg` to macOS (`website/src/utils/assets.ts:26-28`). One universal app beats two per-architecture downloads: a volunteer does not know which chip their Mac has, and the size cost (about 2×, roughly 15 MB compressed) does not matter. Minimum macOS 13, which Go 1.27 already forces.
3. **The app is notarised and stapled first, then the DMG.** Two notary submissions of a few minutes each. Stapling only the DMG would leave the copied-out app needing Apple's servers on its first start, and the hall may have no internet (CLAUDE.md: "It has to keep working with no internet"). With the app stapled, a volunteer can install at home and run offline in the hall.
4. **A Dock icon and the menu bar icon on macOS (no `LSUIElement`).** Owner decision, 2026-11-26: volunteers look for a running app in the Dock, and a notched MacBook hides menu bar icons that do not fit, so the menu bar alone can leave Hallstream invisible. Clicking the Dock icon, or opening Hallstream again from Finder or Launchpad while it runs, opens the panel in the browser: macOS sends the running app a reopen event (`applicationShouldHandleReopen:hasVisibleWindows:` on the app delegate, https://developer.apple.com/documentation/appkit/nsapplicationdelegate/applicationshouldhandlereopen(_:hasvisiblewindows:)). The Dock menu keeps the system Quit, which runs the same exit as the tray's Sair. `fyne.io/systray` owns the app delegate on macOS, so hook the reopen event without replacing it (a small cgo/Objective-C file that adds the method to the delegate's class); STOP and report if that can't be done without forking systray.
5. **Linux format: one `tar.gz`, amd64 only, static binary.** It extracts to a `Hallstream/` folder that holds the binary, the texts and, later, `dados/`. An AppImage would need FUSE 2 on recent Ubuntu and mounts read-only, which breaks "data beside the program". `.deb`/`.rpm` mean per-distro packaging work nobody has asked for. arm64 is one more line of build later if a club asks (Open question 1).
6. **Where data lives.**
   - Windows and Linux: unchanged, `dados/` beside the executable (Linux: inside the extracted `Hallstream/` folder).
   - macOS, **only when running inside a `.app`** (the executable path contains `.app/Contents/MacOS/`): `~/Library/Application Support/Hallstream/dados`. Writing inside the bundle would break its signature, and a quarantined app that was not moved to Applications runs from a read-only translocated copy.
   - macOS outside a bundle (`go run`, a scratch `go build`): unchanged, so CLAUDE.md's development commands keep working.
   - The folder stays named `dados`, so every message that says `dados/marcador.log` stays true. The panel already shows the real path (`data_dir`, `internal/web/server.go:515`, shown at `web/painel/painel.js:2292`).
7. **What each Windows-only feature does on each OS:**

   | Feature | Windows (today) | macOS | Linux |
   |---|---|---|---|
   | Tray icon and menu | `tray_windows.go` | **Port**: same code, menu bar icon from a PNG | **Port**: same code; needs an AppIndicator host (Ubuntu has one) |
   | Fatal / Warn / Ask boxes | MessageBox | **Port**: `osascript` dialogs | **Port**: `zenity` if on the PATH, else the console as today |
   | Start OBS | `obs64.exe` | **Port**: `open -a <OBS.app> --args --disable-shutdown-check`, **never** `--minimize-to-tray` | **Port**: `obs` on the PATH, else `flatpak run com.obsproject.Studio`; never `--minimize-to-tray` |
   | Is OBS running | process list | **Port**: process named `OBS` | **Port**: process named `obs` |
   | Close OBS on exit | WM_CLOSE | **Port** with SIGINT, if Step 1.4 proves it clean; otherwise `Close` stays nil and the panel hides the switch (new `status.obs_close`, Step 1.5) | same as macOS |
   | obs-websocket config path | `%APPDATA%` | already right | already right for native OBS; **add** the Flatpak path |
   | "Abrir com o Windows" | Startup-folder shortcut | **Port**: a LaunchAgent plist; refused while the app runs translocated | **Port**: `~/.config/autostart/hallstream.desktop` |
   | Desktop shortcut | `.lnk` | **Hide** (stays nil): Mac users keep apps in Applications and the Dock | **Later** (stays nil); Open question 4 |
   | Copy overlay address (tray) | Win32 clipboard | **Port**: `pbcopy` | **Port**: `wl-copy`, else `xclip`, else `xsel`; none found is a logged error |
   | Bluetooth | WinRT | works through CoreBluetooth; needs the Info.plist key; a refused permission gets its own message | works through BlueZ; READ-ME line about the `bluetooth` group |
   | "Open Bluetooth settings" | `ms-settings:` | already works (`internal/web/bluetooth.go:70-71`) | already answers with `api.ble.abra_definicoes` |
   | YouTube token sealing | DPAPI | unchanged: file mode 0600 (`internal/youtube/seal_other.go`) | unchanged |
   | Second launch opens the panel | works | **Port**: macOS does not start a second copy; it sends the running app a reopen event, which opens the panel (decision 4), as does clicking the Dock icon | works |
   | Left click on the icon | opens the panel (`SetOnTapped`) | **Shows the menu** (Mac convention: `fyne.io/systray` on darwin shows the menu only when no tap handler is set, `systray_darwin.go:172-180`); "Abrir painel" is its first item | opens the panel, as Windows |
   | A USB or capture camera in "Configurar OBS" and Câmaras | offered when OBS lists `dshow_input` (`internal/obsws/setup.go:20, 843-849, 889-906`) | **Later**: OBS names its device source differently on macOS and Linux, so the device choices do not appear; DroidCam (plugin) and the VDO.Ninja phone still work. READ-ME says so | **Later**, same |

8. **Names and versions.** One tag `vX.Y.Z` builds all three. The release holds:
   - `Hallstream-X.Y.Z.zip`: Windows, **name and contents unchanged**. The website treats a zip without "mac" or "linux" in its name as Windows (`website/src/utils/assets.ts:12-14, 37-40`), and the zip check in `release.yml:72` stays as is.
   - `Hallstream-X.Y.Z-macos.dmg`
   - `Hallstream-X.Y.Z-linux-x64.tar.gz`
   - `SHA256SUMS` listing all three.
   The Mac bundle's `CFBundleShortVersionString` and `CFBundleVersion` are `X.Y.Z`. Dry runs (`0.0.0-dev.<sha>`) put `0.0.0` there, since Apple wants numbers only.
9. **A tag release fails if the Apple secrets are missing.** It never publishes an unsigned Mac app. Pull-request dry runs and forks build the app unsigned (the linker's ad-hoc signature) and skip notarisation.
10. **Notarise with `--wait` inside the release job**, unlike BVA's separate staple-later workflow. A 15 MB Go app comes back in minutes, and the website deploy hook must not fire until every file is on the release. Timeout 30 minutes; a timeout fails the release before anything is published, and the owner re-runs the failed jobs from the Actions page (the tag stays).
11. **Signing lives in the workflow; `build.sh` stays secret-free.** `build.sh` gains an optional second argument, `windows` (the default, so `./build.sh 1.0.0` behaves exactly as today), `macos` or `linux`. Its `macos` target makes an ad-hoc-signed `Hallstream.app` and the DMG layout; the workflow signs, notarises and makes the final DMG.
12. **Settings key `start_with_windows` keeps its name** on every OS (`internal/store/store.go:123-125`). Renaming it would drop the switch from every saved `settings.json`. Only the label changes per OS.
13. **Windows behaviour does not change**, including `--minimize-to-tray` (`internal/obsapp/obsapp_windows.go:88`). Open question 2 asks whether the first-run CEF problem also hits Windows. That is a separate fix.
14. **The menu bar and Linux tray icon is `web/assets/icon/favicon-32.png`** (32 px, so 16 pt at 2×). A monochrome template icon that follows the Mac's light and dark menu bar is later polish (Open question 3). Windows keeps `hallstream.ico`.
15. **Bundle ID `com.hallstream.app`.** It is the owner's domain (hallstream.com) and is used for the LaunchAgent label too.

## Current state

### Files and their roles (line numbers at `dc2552e`)

- `internal/desktop/desktop.go`: package doc ("what the program does on the Windows desktop") and `type Tray struct` (`Icon, Tooltip, Panel, Overlay, Status, Changed, ConfirmQuit, Quit, Log`).
- `internal/desktop/desktop_windows.go`: `const Console = false`; `Fatal`, `Warn`, `Ask` (MessageBox); `var Shortcut` (line 46) and `var Autostart` (lines 50-63), both through `link` (PowerShell + WScript.Shell); `CopyText` (Win32 clipboard).
- `internal/desktop/tray_windows.go`: `func (t Tray) Run()` with `fyne.io/systray`. It is Windows-only only because of its file name; the code is portable. It calls `CopyText` (line ~40).
- `internal/desktop/desktop_other.go` (`//go:build !windows`):
  ```go
  // Console is true where the program has a console window for its messages.
  const Console = true

  // Shortcut and Autostart are nil: only Windows gets shortcuts.
  var (
  	Shortcut  func() (string, error)
  	Autostart func(on bool) error
  )

  // Ask has no one to ask here (the console closes with Ctrl+C): it says yes.
  func Ask(msg string) bool { fmt.Println(msg); return true }
  ```
  plus `Fatal` (prints, waits for Enter, lines 29-34), `Warn` (line 37) and a `Run` that waits for Ctrl+C (lines 39-45).
- `internal/desktop/desktop_other_test.go`: asserts `Shortcut` and `Autostart` are nil and `Ask` says yes. **This plan changes that test** (allowed: it tests the behaviour this plan replaces).
- `internal/obsapp/obsapp.go`: `App{ConfigPath, Running, Start, Close}` (lines 18-30; "nil funcs mean this system can't (anything but Windows)"); `configPath()` (lines 43-51) joins `os.UserConfigDir()` with `obs-studio/plugin_config/obs-websocket/config.json`.
- `internal/obsapp/obsapp_windows.go`: `System()` (21-24), `Path()` (53-77, error `obs.nao_instalado`), `start()` (79-94) with `--minimize-to-tray --disable-shutdown-check`, `closeOBS(force)` (WM_CLOSE, wait 15 s, then terminate when `force`).
- `internal/obsapp/obsapp_other.go` (`//go:build !windows`): `func System() App { return App{ConfigPath: configPath()} }`.
- `internal/obsapp/obsapp_test.go:96-104` `TestSystem`: asserts the funcs are nil off Windows. **This plan changes that assertion.**
- `internal/store/store.go:408-416`:
  ```go
  func DataDir() string {
  	dir, _ := os.Getwd()
  	if exe, err := os.Executable(); err == nil && !strings.Contains(exe, "go-build") {
  		dir = filepath.Dir(exe)
  	}
  	return filepath.Join(dir, "dados")
  }
  ```
- `cmd/hallstream/main.go`: `store.DataDir()` at 148; log setup with `desktop.Console` at 152-160; server fields `Shortcut`, `Autostart`, `OBSApp: obsapp.System()` at 245-248; autostart refresh at start (260-264, "a moved program needs it pointing at the new exe"); `icon, _ := assets.FS.ReadFile("assets/icon/hallstream.ico")` at 324; `desktop.Tray{...}.Run()` at 326-362; `openFolder` (468-) and `openBrowser` (488-) already switch on `runtime.GOOS`.
- `internal/web/server.go:46-52`: `Shortcut`, `Autostart`, `OBSApp` on `Server`. `:516-519` in the status answer: `"shortcut"`, `"autostart"` (non-nil and loopback), `"obs_app": s.OBSApp.Start != nil`.
- `web/painel/painel.js:2240-2244`: the "Abrir o OBS" and "Fechar o OBS ao sair" switches both sit under `S.status.obs_app`. A nil `Close` does **not** hide "Fechar o OBS ao sair" today; `internal/web/obs.go:788` only skips the call.
- `internal/ble/reader.go:326`: when scanning starts, the message becomes `ble.a_procura` only if it was empty or `ble.desligado`.
- `internal/web/obs.go`: `:741` starts OBS when `o.Launch && local(o.Host) && s.OBSApp.Start != nil`; `:788` closes it on exit only when `s.OBSApp.Close != nil`; `:810-830` `autostart` handler (501 `api.arranque_so_windows` when nil).
- `internal/web/bluetooth.go:65-80` `openBluetoothSettings`: Windows and darwin open the settings; Linux answers `api.ble.abra_definicoes`.
- `internal/ble/radio.go`: `Adapter()` wraps `bluetooth.DefaultAdapter`; `Connect` already handles the macOS UUID address ("a MAC on Windows, a UUID on macOS").
- `web/painel/painel.js:2301-2308`: the "Neste PC" card. **The whole card, autostart switch included, is inside `if (S.status && S.status.shortcut)`** (2301). Inside it: the shortcut button (2304), and the "Abrir com o Windows" switch (2306, label `painel.definicoes.abrir_windows`, hint `painel.definicoes.abrir_windows_dica` at 2307), shown when `S.status.autostart`. `:2900-2904` toggles the switch. With `Shortcut` nil, as on macOS and Linux, the switch never shows today.
- `web/i18n/en.json` (and the same keys in `pt.json`) that name Windows or the exe and are seen on macOS/Linux after this plan: `api.arranque_so_windows` (2), `api.nao_mudei_arranque` (69), `log.arranque_erro` (218), `log.nao_pus_arranque` (291), `painel.atualizacao.dica` (757, "put it in place of Hallstream.exe"), `painel.definicoes.abre_windows` (945), `painel.definicoes.abrir_windows` (948), `painel.definicoes.abrir_windows_dica` (949, "…when you turn on the PC"), `painel.definicoes.nao_abre_windows` (991). The Ajuda texts `painel.ajuda.dia.2` (639) and `painel.ajuda.pc.4` (667) also say Windows (Open question 5).
- `build.sh`: zsh. Lines 1-7: header, `version`, `cd`, `go test ./...`, `mkdir -p dist`. Lines 12-17: the `winres` function and its two calls. Lines 25-28: `flags` with `-H windowsgui`, both exe builds, removing the `.syso` files. Lines 30-41: texts, licence, notices and guides into `dist/`. Lines 42-43: the zip. Line 44: `ls -lh dist`.
- `scripts/notices.sh:20-22`: lists the linked modules with `GOOS=windows` only. The Mac app links `github.com/tinygo-org/cbgo` and the Linux build links `github.com/godbus/dbus/v5`, so each target needs its own notices.
- `.github/workflows/release.yml`: one `ubuntu-latest` job (line 31): version from the tag, vet, `zsh ./build.sh` (64), the exact zip check (72), upload artifact on dry runs, publish with `softprops/action-gh-release@v2` (87), Vercel hook (98). `pull_request.paths` at 11-20.
- `.github/workflows/ci.yml`: Linux job builds `./cmd/hallstream` for linux (cgo on), windows/amd64, windows/386 (22-25); `macos` job vets, tests and builds (27-38).
- `LEIA-ME.txt:14` / `READ-ME.txt:15`: "Precisa de Windows 10 ou mais recente" / "It needs Windows 10 or newer".
- `web/assets/icon/`: `hallstream.ico`, `favicon-32.png`, `icon-512.png` (the source for the `.icns`), and others. `brand/icons.sh` makes them from `brand/icon.svg`.
- Website (other repo): `website/src/utils/assets.ts` `parseAssets` maps `.dmg` to macOS, a zip with "mac"/"darwin"/"linux" to that OS, and skips anything else (so a `.tar.gz` is skipped today). `detectArchitecture` labels a Mac file with no arch in its name as `arm64`.

### Conventions

- Go standard library plus the modules already in `go.mod`. **Add no module.** Code and comments in English.
- Platform code goes in `_windows.go`, `_darwin.go`, `_linux.go` files with the matching build tags, as `desktop`, `obsapp` and `youtube` already do. Keep the OS-independent logic (file contents, argument lists, path choice) in plain functions in an untagged file so the tests run on every CI OS.
- Every user-facing string goes in `web/i18n/pt.json` first (European Portuguese; first person, imperative), then `en.json`, same keys, kept in sorted order. `go test ./internal/i18n` enforces it, and fails on keys built at run time.
- Shell scripts are zsh (`build.sh`, `scripts/notices.sh`, `brand/icons.sh`), `set -e`, `${0:A:h}` for the script's folder, `/bin/cp -f` for copies.
- Commands run as plain programs with plain arguments. In Go, `exec.Command(name, args...)`, never a shell string; pass user-controlled text as arguments or environment, never inside a script (the pattern in `desktop_windows.go:80-86`).
- Never read or edit `dados/` or `dist/`.
- Commits: conventional (`feat(desktop): …`, `ci(release): …`, `docs: …`).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Vet | `go vet ./...` | exit 0 |
| Vet for Windows | `GOOS=windows go vet ./...` | exit 0 |
| Tests | `go test ./...` | all pass (Chrome tests skip without Chrome) |
| Windows 32-bit build | `GOOS=windows GOARCH=386 go build ./...` | exit 0 |
| Linux static build | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/hs-linux ./cmd/hallstream` | exit 0 |
| Linux vet | `GOOS=linux CGO_ENABLED=0 go vet ./...` | exit 0 |
| Mac builds (on a Mac) | `MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1 GOARCH=amd64 go build -o /tmp/hs-amd64 ./cmd/hallstream`, same with `arm64` | exit 0 |
| Mac minimum OS | `otool -l /tmp/hs-arm64 \| grep -A3 LC_BUILD_VERSION` | `minos 13.0` |
| Windows release dry run | `zsh ./build.sh 0.0.0-test` | the same zip as before (see Step 2.6) |
| Demo | `go run ./cmd/hallstream -demo -nobrowser` | panel at `http://127.0.0.1:8765/painel` |

## Suggested executor toolkit

- `read-the-damn-docs` before writing the signing and notarising steps; re-read the Apple pages cited above.
- `infra-review` on the `release.yml` and `ci.yml` diffs (PRs 2 and 3).
- `stacked-prs` to cut the four PRs.

## Scope

**In scope**:

- `internal/desktop/`: `desktop.go` (doc only), rename `tray_windows.go` to `tray.go` (build tag `windows || darwin || linux`), new `signals_windows.go` and `signals_unix.go`, new `desktop_darwin.go`, `desktop_linux.go`, `autostart.go` (untagged file builders), `autostart_test.go`; `desktop_other.go` narrowed to `!windows && !darwin && !linux`; `desktop_other_test.go` moved/updated.
- `internal/obsapp/`: new `obsapp_darwin.go`, `obsapp_linux.go`, `launch.go` (untagged helpers), tests; `obsapp_other.go` narrowed; `obsapp.go` (`configPath` Flatpak fallback, doc comments); `obsapp_test.go` (`TestSystem` assertion).
- `internal/store/store.go` (`DataDir` and a testable helper), `internal/store/store_test.go` (new test).
- `cmd/hallstream/main.go` (tray icon per OS only).
- `internal/web/server.go` (add `"os"` and `"obs_close"` to the status answer), `internal/web/app_test.go` (new test only). `internal/web/obs.go` does not change.
- `internal/ble/reader.go` (only the darwin permission message and the line that clears it; Step 1.6).
- `web/painel/painel.js` (autostart label per OS; the update tip), `web/painel/mock.js` (`os` in the mock status).
- `web/i18n/pt.json`, `web/i18n/en.json`.
- `build.sh`, `scripts/notices.sh`, new `macres/Info.plist`, new `macres/pt.lproj/InfoPlist.strings`, new `macres/en.lproj/InfoPlist.strings`.
- `.github/workflows/release.yml`, `.github/workflows/ci.yml`.
- `LEIA-ME.txt`, `READ-ME.txt`, `README.md`, `CLAUDE.md`, `docs/ROADMAP.md`, `docs/CONTRACT.md`, `plans/README.md`.
- Website repo, PR 5 only: `src/utils/assets.ts`, its tests, the download section.

**Out of scope**:

- Any change to what Windows does or ships: `desktop_windows.go`, `obsapp_windows.go`, `winres/`, the zip's name and contents, `--minimize-to-tray` on Windows.
- Windows signing (SignPath rejected; stays unsigned).
- Mac App Store, App Sandbox, `SMAppService` login items, Keychain storage of the YouTube token.
- `.deb`, `.rpm`, AppImage, Snap, Flatpak packaging of Hallstream itself; Linux arm64.
- An auto-updater. The update check stays a banner linking the release page (`internal/update/update.go`).
- `dados/` and `dist/`.

## Git workflow

- Stack of branches, each based on the one below: `feat/desktop-mac-linux` (PR 1), `ci/macos-release` (PR 2), `ci/linux-release` (PR 3), `docs/mac-linux` (PR 4). Website: `feat/mac-linux-downloads` in the website repo (PR 5).
- One commit per step. Do NOT push or open PRs unless told to.

## Steps

### PR 1: the desktop layer on macOS and Linux

#### Step 1.1: Data folder inside a Mac app bundle

In `internal/store/store.go`, split `DataDir` into the OS lookup and a pure helper:

```go
// dataDir picks the dados folder. Inside a macOS app bundle the program
// can't write beside itself (the bundle is signed, and a quarantined copy
// runs read-only), so it uses Application Support instead.
func dataDir(goos, exe, wd, configDir string) string
```

- `exe` contains `go-build` or is empty → `wd/dados` (as today).
- `goos == "darwin"` and `exe` contains `.app/Contents/MacOS/` and `configDir != ""` → `configDir/Hallstream/dados`.
- Otherwise → `filepath.Dir(exe)/dados` (as today).

`DataDir()` calls it with `runtime.GOOS`, `os.Executable()`, `os.Getwd()`, `os.UserConfigDir()`. Update its doc comment.

In the same commit, add the data-folder rule to `docs/CONTRACT.md` next to where it describes `data_dir` (around line 335; CLAUDE.md: the contract changes in the same commit as the behaviour).

**Verify**: `go test ./internal/store/ -run TestDataDir` → pass, with a new table test. Build the cases with forward-slash paths so they pass on the Linux and macOS CI (where `filepath.Dir` does not split on a backslash); add the Windows case `C:\x\Hallstream.exe` → `C:\x\dados` only when `runtime.GOOS == "windows"`. Cases: `/home/a/Hallstream/Hallstream` on linux → `/home/a/Hallstream/dados`; `/Applications/Hallstream.app/Contents/MacOS/Hallstream` on darwin with configDir `/Users/a/Library/Application Support` → `/Users/a/Library/Application Support/Hallstream/dados`; the same path on linux → beside the exe; a `go-build` path → `wd/dados`.

#### Step 1.2: The tray on every desktop OS

- `git mv internal/desktop/tray_windows.go internal/desktop/tray.go` and add `//go:build windows || darwin || linux` at the top. Do not change its body except two things. First, call `systray.SetOnTapped(t.Panel)` only when `runtime.GOOS != "darwin"`, so a left click on the Mac shows the menu (Decision 7). Second, on non-Windows, also stop on SIGINT/SIGTERM (Ctrl+C in a terminal) by running the same `quit()` then `systray.Quit()`. Put that in a tiny per-OS helper `func watchSignals(stop func())`: a no-op in a new `signals_windows.go`, and in a new `signals_unix.go` (`//go:build darwin || linux`) a goroutine doing `signal.Notify(c, os.Interrupt, syscall.SIGTERM)` then `stop()`. `tray.go` calls it once inside the `systray.Run` ready func. `desktop_windows.go` does not change.
- `desktop_other.go`: change its build tag to `//go:build !windows && !darwin && !linux` (BSDs and others keep the console behaviour). Move `desktop_other_test.go` to the same tag.
- `cmd/hallstream/main.go:324`: read `assets/icon/hallstream.ico` on Windows and `assets/icon/favicon-32.png` elsewhere (`runtime.GOOS == "windows"`). Update the `Icon` field comment in `desktop.go` ("`.ico` on Windows, PNG elsewhere").
- Update the package doc in `desktop.go`: Windows, macOS and Linux desktops, console elsewhere.

**Verify**: `go vet ./... && GOOS=windows go vet ./... && GOOS=linux CGO_ENABLED=0 go vet ./...` → exit 0. (Mac) `go build -o /tmp/hs ./cmd/hallstream && /tmp/hs -demo -nobrowser` from a scratch folder (it will write `dados/` beside `/tmp/hs`) shows a menu bar icon; a left click shows the menu with "Abrir painel", "Copiar endereço do overlay", the status line and "Sair"; "Sair" exits; Ctrl+C in the terminal also exits through `Quit` (the log's last line is the `log.a_fechar` text).

#### Step 1.3: Messages, clipboard and autostart on macOS and Linux

Create `internal/desktop/autostart.go` (no build tag) with pure builders, so tests run everywhere:

```go
// launchAgent is the LaunchAgent plist that opens the app bundle at sign-in.
func launchAgent(label, bundle string) []byte
// autostartEntry is the XDG autostart .desktop file for the executable.
func autostartEntry(exe string) []byte
// translocated reports whether macOS runs a quarantined copy from a
// read-only random folder, whose path would be gone after a restart.
func translocated(exe string) bool // strings.Contains(exe, "/AppTranslocation/")
// bundlePath is the .app folder around exe, or "" outside a bundle.
func bundlePath(exe string) string
```

- `launchAgent`: XML-escape `bundle` with `encoding/xml`'s `EscapeText`. Keys: `Label` = `com.hallstream.app`, `ProgramArguments` = `/usr/bin/open`, `-a`, `<bundle>`, `RunAtLoad` true.
- `autostartEntry`: `[Desktop Entry]`, `Type=Application`, `Name=Hallstream`, `Exec=` the path quoted per the Desktop Entry spec (wrap in double quotes; escape `"`, `` ` ``, `$` and `\` with a backslash), `Path=` the folder, `X-GNOME-Autostart-enabled=true`, `Terminal=false`.

`internal/desktop/desktop_darwin.go`:

- `var Console = inConsole()`, where `inConsole` returns true when `os.Executable()` fails, else `bundlePath(exe) == ""`. Inside the bundle there is no console. `main.go:152-160` already reads `desktop.Console`; a `var` works there unchanged.
- `Fatal`, `Warn`: `osascript` with the message passed as an argument, never inside the script: `exec.Command("/usr/bin/osascript", "-e", "on run argv", "-e", "display alert \"Hallstream\" message (item 1 of argv) as critical", "-e", "end run", msg)`; `Warn` uses `as warning`. When `Console` is true also print, as today.
- `Ask`: `display dialog (item 1 of argv) with title "Hallstream" buttons {(item 2 of argv), (item 3 of argv)} default button (item 2 of argv) with icon caution`, with argv `msg, i18n.T("app.nao"), i18n.T("app.sim")`. It returns true only when stdout contains `button returned:` followed by the "Sim" label. Cancel or an error means no. New keys `app.sim` ("Sim" / "Yes") and `app.nao` ("Não" / "No").
- `CopyText`: `pbcopy` with `s` on stdin.
- `Shortcut` stays nil.
- `Autostart(on)`: off → remove `~/Library/LaunchAgents/com.hallstream.app.plist` (missing is fine). On → `bundlePath(exe)`; empty (not a bundle) → `i18n.Err("app.arranque_sem_app")` ("Só consigo abrir sozinho a partir da aplicação Hallstream." / "I can only open by myself from the Hallstream app."); `translocated` → `i18n.Err("app.arranque_mover")` ("Arraste primeiro o Hallstream para a pasta Aplicações e abra-o de lá." / "First drag Hallstream into the Applications folder and open it from there."); else `os.MkdirAll` the `LaunchAgents` folder (0o755) and `os.WriteFile` the `launchAgent(...)` bytes (0o644). The Windows autostart writes no temp file either.
- **Autostart stays nil when `Console` is true** (a development build outside a bundle), so the panel hides the switch there: `var Autostart = autostartFunc()`, where `autostartFunc` returns nil when `Console` is true and the function above otherwise.

`internal/desktop/desktop_linux.go`:

- `const Console = true` (a terminal may be attached; harmless when not).
- `Fatal`, `Warn`, `Ask`: if `exec.LookPath("zenity")` finds it, `zenity --error|--warning|--question --title=Hallstream --text=<msg>` (for `--question` add `--ok-label=<Sim> --cancel-label=<Não> --default-cancel`; exit 0 is yes). Always also print to stdout. Without zenity: today's console behaviour (`Fatal` prints and waits for Enter only when stdin is a terminal, tested with `unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS)` from `golang.org/x/sys/unix`, already in `go.mod`; `Ask` prints and says yes, as now).
- `CopyText`: first of `wl-copy`, `xclip -selection clipboard`, `xsel --clipboard --input` found by `exec.LookPath`, text on stdin; none → `i18n.Err("app.sem_area_transferencia")` ("Não encontrei como copiar: instale o wl-clipboard ou o xclip." / "I found no way to copy: install wl-clipboard or xclip.").
- `Shortcut` stays nil.
- `Autostart(on)`: `os.UserConfigDir()` + `autostart/hallstream.desktop`; on does `os.MkdirAll` of the `autostart` folder (0o755, often missing on a fresh profile) and writes `autostartEntry(exe)` (0o644); off removes it (missing is fine).

Rename the i18n *text* (not the keys) of four messages so it no longer names Windows:

| Key | pt | en |
|---|---|---|
| `api.arranque_so_windows` | "Neste computador não consigo abrir sozinho." | "I can't open by myself on this computer." |
| `api.nao_mudei_arranque` | "Não consegui mudar o arranque automático: {err}" | "I couldn't change opening at sign-in: {err}" |
| `log.arranque_erro` | "Abrir ao iniciar sessão: {err}" | "Open at sign-in: {err}" |
| `log.nao_pus_arranque` | "Não consegui pôr o Hallstream a abrir ao iniciar sessão: {err}" | "I couldn't set Hallstream to open at sign-in: {err}" | Keep the keys: renaming them touches `internal/web/obs.go` and `main.go` for no gain.

**Verify**: `go test ./internal/desktop/ ./internal/i18n` → pass, with new tests in `autostart_test.go`: `launchAgent` output contains `<string>/usr/bin/open</string>` and escapes `&` in `/Users/a/A&B/Hallstream.app`; `autostartEntry("/home/a/My \"club\"/Hallstream/Hallstream")` has `Exec="/home/a/My \"club\"/Hallstream/Hallstream"`; `translocated` is true for `/private/var/folders/x/T/AppTranslocation/ABC/d/Hallstream.app/Contents/MacOS/Hallstream`; `bundlePath` of that path ends in `Hallstream.app` and of `/usr/local/bin/hallstream` is `""`. Replace `desktop_other_test.go`'s assertions only as far as its new build tag needs (it still runs on other systems). `GOOS=windows GOARCH=386 go build ./...` → exit 0.

#### Step 1.4: Start, find and close OBS on macOS and Linux

Create `internal/obsapp/launch.go` (no build tag):

```go
// startArgs are the arguments for OBS on macOS and Linux. Never
// --minimize-to-tray: on OBS's first run that leaves its browser sources,
// the overlay among them, without a renderer.
func startArgs() []string { return []string{"--disable-shutdown-check"} }
// macCandidates are the places OBS.app is looked for, in order.
func macCandidates(home string) []string // /Applications/OBS.app, home/Applications/OBS.app
// linuxConfigPath picks native OBS's config.json when its obs-studio folder
// exists, else the Flatpak's when that folder exists, else the native path.
func linuxConfigPath(configDir, home string, exists func(string) bool) string
```

`internal/obsapp/obsapp_darwin.go`:

- `System()` returns all four funcs.
- `Path()`: first of `macCandidates` that is a directory, else `i18n.Err("obs.nao_instalado")` (its text names no OS; leave it).
- `func pids() ([]int, error)`: `exec.Command("/usr/bin/pgrep", "-x", "OBS").Output()`; exit status 1 → empty list, no error; other failures → the error; otherwise one decimal pid per line (`strconv.Atoi` on each trimmed line, skipping blanks).
- `running()`: `len(pids()) > 0`, passing the error on.
- `start()`: `exec.Command("/usr/bin/open", append([]string{"-a", path, "--args"}, startArgs()...)...)`, run with `cmd.Run()` (`open` returns at once, and OBS runs on its own, not as our child). Wrap a failure in `i18n.Err("obs.nao_abri", "err", err)`, as Windows does (`obsapp_windows.go:91`).
- `closeOBS(force)`: send SIGINT to every pid from `pgrep -x OBS`, wait up to 15 s polling `running()`; still running and `force` → SIGKILL; still running and not `force` → `i18n.Err("obs.nao_fechou")` (same contract as Windows, `obsapp.go:26-28`).
- **Measure before keeping `closeOBS`**: on a Mac with OBS 33 open and a scene collection changed (add an empty scene named `teste-sigint`), run `kill -INT $(pgrep -x OBS)`. Then start OBS again normally. Clean means OBS exits within 15 s, the `teste-sigint` scene is still there, and OBS does not show "OBS did not shut down properly" (start it **without** `--disable-shutdown-check` for this test). If any of the three fails, set `Close: nil` in `System()` for darwin and write the result into this plan's Maintenance notes. The panel then hides "Fechar o OBS ao sair" through `status.obs_close` (Step 1.5). **If you have no Mac, ship `Close: nil` on darwin too**, keep `closeOBS` in the file unused behind a comment pointing at this step, and leave the measurement to owner checklist item 5.

`internal/obsapp/obsapp_linux.go`:

- `System()` returns all four funcs. `ConfigPath`: call `os.UserConfigDir()` and `os.UserHomeDir()`; if either fails, use `configPath()` as today; else `linuxConfigPath(configDir, home, dirExists)`, with `func dirExists(p string) bool` (`os.Stat` succeeds and `IsDir()`), private to this file.
- `func launcher() (name string, args []string, err error)`: `exec.LookPath("obs")` found → `(that path, startArgs(), nil)`; else `exec.LookPath("flatpak")` found and `flatpak info com.obsproject.Studio` exits 0 → `(flatpak path, append([]string{"run", "com.obsproject.Studio"}, startArgs()...), nil)`; else `("", nil, i18n.Err("obs.nao_instalado"))`. There is no exported `Path()` on Linux: nothing outside `obsapp` calls it.
- `func pids() ([]int, error)`: `os.ReadDir("/proc")` (its error is returned); for each all-digit entry, read `/proc/<n>/comm` and skip any read error (the process may have gone); keep the pid when the trimmed content is `obs` (the Flatpak's process is also named `obs` in the host's process list). `running()` is `len(pids()) > 0`.
- `start()`: `launcher()`, then `exec.Command(name, args...)` with `SysProcAttr: &syscall.SysProcAttr{Setsid: true}` so closing a terminal doesn't close OBS; `cmd.Start()` then `go cmd.Wait()`; a failure wrapped in `obs.nao_abri`.
- `closeOBS(force)`: same as darwin, with the pids from `/proc`. Same measurement on Linux if a Linux machine is available. If it isn't, ship `Close: nil` on Linux and note it (Open question 6).

`internal/obsapp/obsapp_other.go`: build tag `!windows && !darwin && !linux`.

`obsapp_test.go` `TestSystem` (lines 101-103): change the assertion to `runtime.GOOS` not in windows/darwin/linux → funcs nil; on darwin and linux, `Running`, `Start` non-nil. That is the only edit to an existing test case here.

In the same commit, update `docs/CONTRACT.md` where it describes `obs_app` and starting and closing OBS (around lines 337 and 420): true on macOS and Linux too; OBS starts visible there (no `--minimize-to-tray`); `obs_close` (Step 1.5) says whether closing works.

**Verify**: `go test ./internal/obsapp/` → pass, with new tests: `startArgs()` does not contain `--minimize-to-tray`; `macCandidates("/Users/a")` order; `linuxConfigPath` picks the Flatpak path when only `~/.var/app/com.obsproject.Studio/config/obs-studio` exists, and the native path when both exist. (Mac) the demo with OBS closed and "Abrir o OBS" on in Definições starts OBS **with its window visible**; `GET /api/status` has `"obs_app": true`.

#### Step 1.5: The panel labels the autostart switch per OS

- `internal/web/server.go` status answer (near line 514): add `"os": runtime.GOOS` and `"obs_close": s.OBSApp.Close != nil`. The file imports only `runtime/debug` today (line 14); add `runtime`. Document both fields in `docs/CONTRACT.md` (status, around line 221) in the same commit, and that `autostart` is true on macOS (from the app) and Linux.
- **Split the "Neste PC" card** (`web/painel/painel.js:2301-2308`): show the card when `S.status.shortcut || S.status.autostart`; inside it, the shortcut button and the "atalho criado" hint only when `S.status.shortcut`, the autostart switch and its hint only when `S.status.autostart`. Keep the `fechar_dica` hint. Check `painel.js:352` (`if (S.tab === 'definicoes' && s && s.shortcut && !$('#dShortcut')) renderDefinicoes();`) still does what it means: re-render when the card should appear; extend it to `autostart` with `#dAutostart`.
- **Gate "Fechar o OBS ao sair"** (`painel.js:2242-2243`) on `S.status.obs_close`; "Abrir o OBS" stays under `obs_app`.
- Autostart words per OS, picked by `S.status.os` (`darwin` → mac, `linux` → linux, anything else → windows), for the label (2306), the hint (2307) and the two toasts (near 2900). **The i18n test only counts a key written as `t('literal')`** (`internal/i18n/i18n_test.go:174-175`: `t(` followed by anything but a quote fails as a run-time key, and a key never written as `t('…')` fails as unused). So build the words with the calls inside, e.g. `function autoWords(os) { return os === 'darwin' ? {label: t('painel.definicoes.abrir_mac'), hint: t('painel.definicoes.abrir_mac_dica'), on: t('painel.definicoes.abre_mac'), off: t('painel.definicoes.nao_abre_mac')} : os === 'linux' ? {…linux keys…} : {…today's windows keys…}; }`. In Go (Step 1.6) the same rule holds: `i18n.T("literal")` only.
- New keys (pt / en):

  | Key | pt | en |
  |---|---|---|
  | `painel.definicoes.abrir_mac` | Abrir ao iniciar sessão no Mac | Open when I sign in to the Mac |
  | `painel.definicoes.abrir_mac_dica` | O Hallstream abre sozinho quando inicia sessão no Mac. Com "Abrir o OBS com o Hallstream", o OBS abre também. | Hallstream opens by itself when you sign in to the Mac. With "Open OBS with Hallstream", OBS opens too. |
  | `painel.definicoes.abre_mac` | Abre ao iniciar sessão no Mac | Opens when you sign in to the Mac |
  | `painel.definicoes.nao_abre_mac` | Já não abre ao iniciar sessão no Mac | No longer opens when you sign in to the Mac |
  | `painel.definicoes.abrir_linux` | Abrir ao iniciar sessão | Open when I sign in |
  | `painel.definicoes.abrir_linux_dica` | O Hallstream abre sozinho quando inicia sessão. Com "Abrir o OBS com o Hallstream", o OBS abre também. | Hallstream opens by itself when you sign in. With "Open OBS with Hallstream", OBS opens too. |
  | `painel.definicoes.abre_linux` | Abre ao iniciar sessão | Opens when you sign in |
  | `painel.definicoes.nao_abre_linux` | Já não abre ao iniciar sessão | No longer opens when you sign in |

  Check the English of "Open OBS with Hallstream" against the real label key `painel.definicoes.abrir_obs` in `en.json` and use its exact words.
- Copy that becomes false outside Windows once these switches show there. Make it OS-neutral (keep the keys):
  - `painel.definicoes.abrir_obs_dica` (947, "OBS opens hidden by the clock…") → pt "O OBS abre sozinho e liga-se assim que «Configurar OBS» tiver corrido bem." / en "OBS opens by itself and connects once Set up OBS has run well." (match the existing quoting style in that key).
  - `painel.definicoes.fechar_dica` (976) and `painel.ajuda.dia.13` (638), which say to click or right-click "the icon by the clock": pt "Para fechar o Hallstream, abra o menu do seu ícone (ao pé do relógio no Windows, na barra de menus no Mac) e escolha <b>Sair</b>." / en "To close Hallstream, open its icon's menu (by the clock on Windows, in the menu bar on a Mac) and choose <b>Quit</b>." Use it for both keys.
- `painel.atualizacao.dica`: make it OS-neutral ("Descarregue-a da página da versão e ponha-a no lugar da que tem. Os seus dados ficam." / "Download it from the release page and put it in place of the one you have. Your data stays.").
- `web/painel/mock.js`: add `os: 'windows'` and `obs_close: true` to the mock status.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass, with a new web test that `GET /api/status` has `"os"` equal to `runtime.GOOS`, and `"obs_close"` false when the server's `OBSApp` has `Start` but no `Close`. In the demo with `/painel?mock=1`, Definições looks as before. (Mac) Definições shows "Abrir ao iniciar sessão no Mac" only when running from the bundle (PR 2 makes it); from `go run` the switch is hidden, because `Autostart` is nil there.

#### Step 1.6: Bluetooth permission message on macOS

In `internal/ble/reader.go`, where an `Enable` error sets `Bluetooth: "unavailable"` with `ble.desligado` (around line 226): on `runtime.GOOS == "darwin"`, use a new key `ble.sem_autorizacao_mac` ("Não consigo usar o Bluetooth. Ligue-o, e em Definições do Sistema > Privacidade e segurança > Bluetooth deixe o Hallstream usá-lo." / "I can't use Bluetooth. Turn it on, and in System Settings > Privacy & Security > Bluetooth let Hallstream use it."). At `reader.go:326`, where scanning replaces the message only if it was `""` or `ble.desligado`, also replace it when it is `ble.sem_autorizacao_mac`, so the hint clears once Bluetooth works. Change nothing else in the reader.

**Verify**: `go test ./internal/ble/ ./internal/i18n` → pass (existing tests unchanged).

#### Step 1.7: CI builds what the release will build

In `.github/workflows/ci.yml`:

- Linux job: add `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/hallstream` after line 22.
- `macos` job: add `MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1 GOARCH=amd64 go build -o /dev/null ./cmd/hallstream` and the same with `arm64`.

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./... && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/hs-linux ./cmd/hallstream` → all exit 0. `git diff dc2552e -- internal/desktop/desktop_windows.go internal/obsapp/obsapp_windows.go winres` is empty.

### PR 2: the Mac app in every release

**Build cache (found while executing PR 1, applies to Steps 2.3 and 2.4).** Go's build cache does not key on `MACOSX_DEPLOYMENT_TARGET`. On a Mac that had built the program before without it, `MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1 GOARCH=arm64 go build` linked cached cgo objects stamped `minos 26.0` into a binary stamped 13.0 (`ld: warning: object file ... was built for newer 'macOS' version (26.0) than being linked (13.0)`, about 40 lines per architecture); with `-a` the warnings were gone. So `build.sh macos` builds both architectures with `go build -a` (or with `GOCACHE` pointed at a fresh temporary folder), so no cgo object built for a newer macOS links in. The same holds on CI: `actions/setup-go` restores the build cache by default, so the `macos` release job either builds with `-a` or sets `cache: false` on setup-go. Treat any "built for newer 'macOS' version" line in the build log as a failure.

#### Step 2.1: Bundle files

Create `macres/Info.plist` (XML plist, tabs) with `@VERSION@` placeholders:

`CFBundleName` Hallstream, `CFBundleDisplayName` Hallstream, `CFBundleIdentifier` com.hallstream.app, `CFBundleExecutable` Hallstream, `CFBundleIconFile` Hallstream, `CFBundlePackageType` APPL, `CFBundleShortVersionString` @VERSION@, `CFBundleVersion` @VERSION@, `CFBundleDevelopmentRegion` en, `CFBundleLocalizations` (en, pt), `LSMinimumSystemVersion` 13.0, ``NSHighResolutionCapable` true, `NSBluetoothAlwaysUsageDescription` "Hallstream uses Bluetooth to read the hall's scoreboard.".

Create `macres/pt.lproj/InfoPlist.strings` (UTF-8): `NSBluetoothAlwaysUsageDescription = "O Hallstream usa o Bluetooth para ler o marcador do pavilhão.";` and `macres/en.lproj/InfoPlist.strings` with the English line.

**Verify**: `plutil -lint macres/Info.plist macres/*/InfoPlist.strings` (on a Mac) → each `OK`.

#### Step 2.2: `scripts/notices.sh` takes the target

Add a second argument, the target: `windows` (default: today's `amd64 386` loop), `darwin` (`amd64 arm64` with `CGO_ENABLED=1`), `linux` (`amd64` with `CGO_ENABLED=0`). Only the `GOOS=… GOARCH=… go list` line changes. Update the header comment ("for the Windows exes" → "for one target's binaries").

**Verify**: the Windows default is unchanged. `git worktree add /tmp/hs-old dc2552e`, `zsh /tmp/hs-old/scripts/notices.sh /tmp/n-old.txt`, `git worktree remove /tmp/hs-old`, then `scripts/notices.sh /tmp/n-win.txt` and `cmp /tmp/n-win.txt /tmp/n-old.txt` → identical. On a Mac, `scripts/notices.sh /tmp/n-mac.txt darwin` contains `github.com/tinygo-org/cbgo`. `scripts/notices.sh /tmp/n-linux.txt linux` contains `github.com/godbus/dbus/v5`.

#### Step 2.3: `build.sh macos`

Restructure `build.sh` with as small a diff as possible: `target=${2:-windows}` after line 4. Lines 30-41 (texts, licence, notices, guides) become a function `texts <folder> <notices target>` that writes the same files under `<folder>` (`dist` for Windows, so the paths stay identical). The Windows branch is today's lines 12-28, then `texts dist windows`, then lines 42-43, all under `if [[ $target == windows ]]` and otherwise unchanged. `go test ./...` and `mkdir -p dist` stay before the branches; `ls -lh dist` stays last.

The `macos` branch (zsh, runs on macOS only; fail with a message elsewhere):

0. `rm -rf dist/mac dist/mac-dmg` first, so a stale file never reaches the DMG listing check.
1. `export MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1`; `flags="-s -w -X main.version=$version -X main.uploadToken=$HALLSTREAM_UPLOAD_TOKEN -X main.feedbackURL=$HALLSTREAM_FEEDBACK_URL"` (no `-H windowsgui`).
2. Build `GOARCH=amd64` and `GOARCH=arm64` into a temp folder, then `lipo -create -output dist/mac/Hallstream.app/Contents/MacOS/Hallstream <amd64> <arm64>`.
3. `Info.plist`: `sed` `@VERSION@` → `$version` when it matches `^[0-9]+\.[0-9]+\.[0-9]+$`, else `0.0.0`; into `Contents/Info.plist`. Copy both `.lproj` folders into `Contents/Resources/`.
4. Icon: `iconutil` from an iconset made with `sips -z <n> <n> web/assets/icon/icon-512.png` for 16, 32, 128, 256, 512 and their `@2x` files up to 512 (skip `512x512@2x`, which needs a 1024 source) → `Contents/Resources/Hallstream.icns`.
5. `codesign --force --sign - dist/mac/Hallstream.app` (ad-hoc, so a dry-run build opens locally after removing quarantine).
6. Stage the DMG folder `dist/mac-dmg/`: `Hallstream.app` (copied with `ditto`), a symlink `Applications -> /Applications`, and `texts dist/mac-dmg darwin` (LEIA-ME.txt, READ-ME.txt, LICENSE.txt, THIRD-PARTY-NOTICES.txt from `notices.sh … darwin`, CAMARAS.txt, CAMERAS.txt, YouTube/).
7. Only when `HALLSTREAM_SIGN` is unset: `hdiutil create -volname Hallstream -srcfolder dist/mac-dmg -ov -format UDZO dist/Hallstream-$version-macos.dmg`. When it is set, the workflow makes the DMG after signing (Step 2.4).

**Verify** (Mac): `zsh ./build.sh 0.0.0-test macos` → exit 0; `lipo -archs dist/mac/Hallstream.app/Contents/MacOS/Hallstream` → `x86_64 arm64`; `otool -l` on each slice (`lipo -thin arm64 …`) → `minos 13.0`; `plutil -p dist/mac/Hallstream.app/Contents/Info.plist` shows `0.0.0` and the Bluetooth string; `open dist/mac/Hallstream.app` (not quarantined, so it opens) → menu bar icon, the panel opens, `GET /api/status` `data_dir` is `~/Library/Application Support/Hallstream/dados`, and macOS asks for Bluetooth permission with the Portuguese or English string. Then `zsh ./build.sh 0.0.0-test` (Windows) gives the same zip listing as on `dc2552e` (`zipinfo -1`).

#### Step 2.4: `release.yml` builds, signs and notarises the Mac app

Split the job. Keep today's job as `windows`, with its steps unchanged except: its "Check the zip" step keeps the listing check but no longer writes `SHA256SUMS`; the upload step runs on every run (tags too), uploads only the zip, and the artifact is named `windows` (not `Hallstream-<version>`); the publish and hook steps move to `publish`, and its permission drops to `contents: read`. Add:

**Job `macos`** (`runs-on: macos-latest`, `permissions: contents: read`, same `env` as today):

1. checkout, setup-go, the same "Work out the version" step copied with `shell: bash` (the runner's bash handles `[[ =~ ]]` and `${GITHUB_SHA::7}`; if the step fails there, STOP and report rather than rewriting it).
2. `go vet ./...`; `zsh ./build.sh "$VERSION" macos` with `HALLSTREAM_SIGN=1` on tags.
3. **Tags only: "Check the Apple secrets"**: each of the five secrets non-empty, else `echo "::error::<name> is not set"; exit 1` (Decision 9).
4. **Tags only: "Import the signing certificate"**: the keychain steps from BVA (`basketball-video-analyzer/app/.github/workflows/release.yml:100-125`): decode `MACOS_CERTIFICATE` to `$RUNNER_TEMP/cert.p12`; `security create-keychain`, `set-keychain-settings -lut 21600`, `unlock-keychain`, `import … -P "$MACOS_CERTIFICATE_PWD" -t cert -f pkcs12 -k … -T /usr/bin/codesign -T /usr/bin/security`, `set-key-partition-list -S apple-tool:,apple:,codesign:`, add to the user search list. Then find the identity: `security find-identity -v -p codesigning "$KEYCHAIN_PATH" | grep "Developer ID Application" | head -1`, take the quoted name into `IDENTITY`; none → fail. Decode `APPLE_API_KEY` to `$RUNNER_TEMP/AuthKey.p8` and export that path as `P8` through `$GITHUB_ENV`, with `IDENTITY` and `KEYCHAIN_PATH`.
5. **Tags only: "Sign and notarise the app"**:
   ```sh
   codesign --force --options runtime --timestamp --sign "$IDENTITY" dist/mac/Hallstream.app
   codesign --verify --strict --verbose=2 dist/mac/Hallstream.app
   ditto -c -k --keepParent dist/mac/Hallstream.app "$RUNNER_TEMP/Hallstream.zip"
   xcrun notarytool submit "$RUNNER_TEMP/Hallstream.zip" --key "$P8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER" --wait --timeout 30m --output-format json
   ```
   Save the JSON to `$RUNNER_TEMP/notary.json` and read the two fields with `plutil -extract status raw -o - "$RUNNER_TEMP/notary.json"` and `plutil -extract id raw -o - "$RUNNER_TEMP/notary.json"` (no Python). Not `Accepted` → run `xcrun notarytool log <id> …` and fail. Then `xcrun stapler staple dist/mac/Hallstream.app` and `xcrun stapler validate dist/mac/Hallstream.app`. Then `rm -rf dist/mac-dmg/Hallstream.app && ditto dist/mac/Hallstream.app dist/mac-dmg/Hallstream.app`, so the DMG gets the stapled copy.
6. **Tags only: "Make, sign and notarise the DMG"**: `hdiutil create -volname Hallstream -srcfolder dist/mac-dmg -ov -format UDZO dist/Hallstream-$VERSION-macos.dmg`; `codesign --timestamp --sign "$IDENTITY" <dmg>`; `notarytool submit <dmg> … --wait --timeout 30m` with the same status check; `xcrun stapler staple <dmg>`; `spctl -a -t open --context context:primary-signature -vv <dmg>` → `accepted`, `source=Notarized Developer ID`.
7. **"Check the DMG"** (every run): `hdiutil attach -nobrowse -readonly -mountpoint "$RUNNER_TEMP/dmg" <dmg>`; the listing (`cd` there; `find . -maxdepth 2 ! -path './Hallstream.app/*' | LC_ALL=C sort`) equals exactly `.`, `./Applications`, `./CAMARAS.txt`, `./CAMERAS.txt`, `./Hallstream.app`, `./LEIA-ME.txt`, `./LICENSE.txt`, `./READ-ME.txt`, `./THIRD-PARTY-NOTICES.txt`, `./YouTube`, `./YouTube/CONNECT-YOUTUBE.txt`, `./YouTube/LIGAR-YOUTUBE.txt`; on tags also `spctl -a -t exec -vv "$RUNNER_TEMP/dmg/Hallstream.app"` → `source=Notarized Developer ID`; `hdiutil detach`.
8. Upload the DMG as an artifact named `macos` (every run).
9. `if: always()`: delete the keychain, the `.p12` and the `.p8` (BVA's cleanup step).

**Job `publish`** (`needs: [windows, macos]` for now; PR 3 adds `linux`), `if: startsWith(github.ref, 'refs/tags/')`, `ubuntu-latest`, `permissions: contents: write`:

1. `actions/download-artifact@v4` with `path: dist` and `merge-multiple: true`, so the three files land directly in `dist/`.
2. Copy the version step into this job too, then `cd dist && sha256sum "Hallstream-$VERSION.zip" "Hallstream-$VERSION-macos.dmg" > SHA256SUMS` and `cat SHA256SUMS`. PR 3 adds `"Hallstream-$VERSION-linux-x64.tar.gz"` to that line. (The runner has no zsh; name the files, don't glob.)
3. `softprops/action-gh-release@v2` with every file plus `SHA256SUMS`, `generate_release_notes: true`.
4. The Vercel hook step, moved here unchanged.

`pull_request.paths`: add `macres/**`. Dry runs (PRs, `workflow_dispatch`) run `windows` and `macos`, keep their artifacts, and publish nothing.

**Verify**: `infra-review` on the diff. Push the branch and open the PR as a draft only if told to; the PR dry run must pass both jobs and show both artifacts. Locally: `zsh ./build.sh 0.0.0-test` still produces the identical zip listing.

### PR 3: the Linux tarball in every release

#### Step 3.1: `build.sh linux`

The `linux` branch: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags $flags -o dist/linux/Hallstream/Hallstream ./cmd/hallstream` (same `flags` as macOS); the shared texts into `dist/linux/Hallstream/` (notices with `linux`); `tar -C dist/linux -czf dist/Hallstream-$version-linux-x64.tar.gz Hallstream`, adding `--owner=0 --group=0` when `tar --version | grep -q GNU` (the runner's GNU tar has them; a Mac's BSD tar does not).

**Verify**: `zsh ./build.sh 0.0.0-test linux` → exit 0 (works on a Mac too, since the build is pure Go); `tar -tzf dist/Hallstream-0.0.0-test-linux-x64.tar.gz | LC_ALL=C sort` → exactly `Hallstream/`, `Hallstream/CAMARAS.txt`, `Hallstream/CAMERAS.txt`, `Hallstream/Hallstream`, `Hallstream/LEIA-ME.txt`, `Hallstream/LICENSE.txt`, `Hallstream/READ-ME.txt`, `Hallstream/THIRD-PARTY-NOTICES.txt`, `Hallstream/YouTube/`, `Hallstream/YouTube/CONNECT-YOUTUBE.txt`, `Hallstream/YouTube/LIGAR-YOUTUBE.txt`; the binary is executable (`tar -tvzf` shows `-rwxr-xr-x`).

#### Step 3.2: `release.yml` job `linux`

`runs-on: ubuntu-latest`, same version step, `sudo apt-get install -y --no-install-recommends zsh`, `zsh ./build.sh "$VERSION" linux`, the exact listing check from Step 3.1, `file dist/linux/Hallstream/Hallstream | grep -q "statically linked"`, upload the tarball as an artifact named `linux`. Add `linux` to `publish.needs`.

**Verify**: the PR dry run passes all three jobs with three artifacts. `GOOS=windows GOARCH=386 go build ./...` → exit 0.

### PR 4: docs

#### Step 4.1: READ-ME and LEIA-ME

`LEIA-ME.txt:14` and `READ-ME.txt:15`: list the three downloads and their requirements (Windows 10+; macOS 13 Ventura or newer, Intel or Apple chip; Linux 64-bit with BlueZ). Add a short "No Mac" / "No Linux" section to each:

- Both: a USB or capture-card camera can't be picked in "Configurar OBS" or Câmaras yet on Mac or Linux; a phone through DroidCam or VDO.Ninja works.
- Mac: open the DMG, drag Hallstream to Applications, open it from there (the autostart switch only works from Applications), allow Bluetooth when asked, the icon is in the menu bar at the top right and a click on it shows the menu, OBS opens with its window showing, the data folder is `~/Library/Application Support/Hallstream/dados`, and a second double-click does not open the panel (use the menu bar icon).
- Linux: extract the tarball where you will keep it, run `Hallstream/Hallstream`, the data is in `Hallstream/dados`, the tray needs an AppIndicator host (GNOME: the AppIndicator extension; Ubuntu has it), Bluetooth needs BlueZ and on some systems the `bluetooth` group, `zenity` gives message boxes and `wl-clipboard` or `xclip` lets the tray copy.

The same in both languages; Portuguese first.

**Verify**: `grep -n "macOS 13" LEIA-ME.txt READ-ME.txt` → one match each.

#### Step 4.2: The other docs

- `README.md:29`: Windows is unsigned (SignPath declined); the Mac app is signed and notarised; a line on each download.
- `CLAUDE.md` Commands: the project description says Windows exe → "a Windows exe, a Mac app or a Linux binary"; `./build.sh <version> [windows|macos|linux]`; the data folder on macOS inside the bundle; the release has three files and the checks per file; the five Apple secrets the release needs on tags.
- `docs/ROADMAP.md:17`: the platform row says all three ship; `:28` drop SignPath, say Windows unsigned and macOS notarised; `:55` mark "notarised macOS builds" done and Windows signing declined.
- `docs/CONTRACT.md`: already changed in Steps 1.1, 1.4 and 1.5. Re-read those sections once against the merged behaviour.
- `plans/README.md`: add the row for 014 (one row, change nothing else).

**Verify**: `go test ./...` → pass; `grep -n "build.sh <version>" CLAUDE.md` matches the new form.

### PR 5 (website repo): downloads per OS

In `/Users/kauredo/code/personal/hallstream/website` (read its `CLAUDE.md` first):

- `src/utils/assets.ts`: map `.tar.gz` to linux (`fileType` `"tar.gz"`, extend the `PlatformAsset` type); a mac file whose name has no arch is `"universal"` (extend the architecture type), not `arm64`. The page labels it "Mac (Intel e Apple)" / "Mac (Intel and Apple)".
- Tests in `src/utils/assets.test.ts` and `platform.test.ts` for the three real names from Decision 8.
- The download section offers the file for the visitor's OS first, with the other two below, and the requirements line from READ-ME. Copy in Portuguese and English per that repo's rules.

**Verify**: that repo's test command (see its `package.json`) passes; with `test/fixtures/release.json` extended with the two new assets, the page shows three downloads.

## Test plan

- `internal/store/store_test.go`: `TestDataDir` table (Step 1.1).
- `internal/desktop/autostart_test.go`: plist and `.desktop` builders, translocation, bundle path (Step 1.3).
- `internal/obsapp/obsapp_test.go` / new `launch_test.go`: `startArgs`, `macCandidates`, `linuxConfigPath`; `TestSystem` assertion updated (Step 1.4).
- `internal/web`: status has `os` (Step 1.5).
- `go test ./internal/i18n`: all new keys in both files.
- Release machinery: the exact-listing checks for the zip (unchanged), the DMG and the tarball, run in every dry run.
- Manual, on a Mac (executor if on a Mac, else owner checklist item 5): the bundle opens, menu bar icon, panel, Bluetooth prompt, OBS starts visible, autostart writes the LaunchAgent only from Applications.
- Existing tests: unchanged except `desktop_other_test.go` (build tag) and `obsapp_test.go:101-103` (assertion), both named above.

## Done criteria

- [ ] `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=linux CGO_ENABLED=0 go vet ./...` exit 0
- [ ] `go test ./...` exits 0 on macOS and Linux CI
- [ ] `GOOS=windows GOARCH=386 go build ./...` exits 0
- [ ] `git diff dc2552e..HEAD -- internal/desktop/desktop_windows.go internal/obsapp/obsapp_windows.go winres` is empty
- [ ] `zsh ./build.sh 0.0.0-test && zipinfo -1 dist/Hallstream-0.0.0-test.zip | LC_ALL=C sort` matches the `want` list in `release.yml:72`, unchanged
- [ ] `grep -rn "minimize-to-tray" internal/obsapp/*_darwin.go internal/obsapp/*_linux.go internal/obsapp/launch.go` finds only the comment in `launch.go`
- [ ] The release dry run on the stack's top PR passes `windows`, `macos` and `linux` and keeps three artifacts
- [ ] `grep -n "NSBluetoothAlwaysUsageDescription" macres/Info.plist` matches
- [ ] After the first tag: the release has `Hallstream-X.Y.Z.zip`, `Hallstream-X.Y.Z-macos.dmg`, `Hallstream-X.Y.Z-linux-x64.tar.gz` and `SHA256SUMS` with three lines; `spctl -a -t open --context context:primary-signature -vv` on the downloaded DMG says `Notarized Developer ID`
- [ ] `plans/README.md` status row updated

## STOP conditions

- Any change would alter the Windows zip's name or contents, or the behaviour of the Windows exe.
- `fyne.io/systray` crashes or shows no icon inside the bundle on macOS. Report the crash log; do not swap the tray library.
- The bundle never shows the Bluetooth permission prompt, or CoreBluetooth reports "unauthorized" with the Info.plist key present.
- The SIGINT measurement in Step 1.4 is unclean on macOS. Ship `Close: nil` there and report. Do not reach for AppleScript `quit`: it needs the Automation permission and its own usage string, which is the owner's call.
- The notary service rejects the app for a reason this plan does not cover (its log names an entitlement or an unsigned nested binary).
- The `macos-latest` runner's Xcode cannot build with `MACOSX_DEPLOYMENT_TARGET=13.0`.
- The Apple secrets are absent when you reach Step 2.4's tag-only steps for a real tag. Stop: that is owner checklist item 1, not something to work around.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- **macOS second launch** (superseded: PR 1 adds the reopen handler to `fyne.io/systray`'s app delegate class at run time, so opening the app again from Finder, or clicking its Dock icon, opens the panel; see `internal/desktop/delegatehook_darwin.go`). The original note: LaunchServices activates the running app instead of starting a second copy, so the "a second copy opens the first one's panel" behaviour (`main.go` port loop) never runs from Finder. A fix needs an `applicationShouldHandleReopen` hook, which `fyne.io/systray` does not expose (`systray_darwin.m` has no reopen handler).
- **Certificate expiry**: Developer ID certificates last five years. When it is renewed, update `MACOS_CERTIFICATE` and `MACOS_CERTIFICATE_PWD` here and in BVA. Already-notarised releases keep working.
- **Go upgrades** can raise the macOS minimum (Go 1.27 already needs 13). Check https://go.dev/wiki/MinimumRequirements and keep `LSMinimumSystemVersion`, `MACOSX_DEPLOYMENT_TARGET`, READ-ME and the website line in step.
- **New Windows-only features** must say what they do on macOS and Linux; nil funcs hide them in the panel, as `Shortcut` does.
- **OBS changes**: if a later OBS fixes the first-run CEF problem, `--minimize-to-tray` could come back on macOS and Linux. Measure on a clean OBS profile before changing `startArgs`.
- Reviewers: download the dry-run DMG artifact on a Mac, remove quarantine only if testing an unsigned build, and check the menu bar icon, the data folder and the Bluetooth prompt; on Linux, run the tarball under GNOME with and without the AppIndicator extension.

## Open questions (for the owner, before merge)

1. **Linux arm64** (Raspberry Pi 5 class machines in halls)? One more `GOARCH=arm64` line and one more file. Not built until a club asks.
2. **Does the first-run `--minimize-to-tray` problem also hit Windows?** If so, a separate small plan should start OBS unminimised when `obs-studio/global.ini` (or `user.ini`, OBS 31+) is missing.
3. **A monochrome template icon for the Mac menu bar**, which follows light and dark mode, in place of the colour `favicon-32.png`. It needs a mark drawn for 16 pt; `brand/icons.sh` would make it. The same applies to the app icon: Step 2.3 makes the `.icns` from `icon-512.png`, which is the full-bleed maskable variant (`brand/icons.sh:26`), so the Mac app icon is a square with no rounded corners. A proper one is a 1024 px render of `brand/icon.svg` with macOS's corner shape.
8. **USB and capture-card cameras on macOS and Linux**: OBS's device source there is not `dshow_input`. Porting "Configurar OBS" and Câmaras to it is its own plan; this one only says it is missing.
4. **Linux shortcut**: a `~/.local/share/applications/hallstream.desktop` menu entry would let the panel's "Criar atalho" work on Linux. Left nil for now.
5. **Ajuda texts per OS** (`painel.ajuda.dia.2`, `painel.ajuda.pc.4` name Windows). Per-OS variants or neutral wording?
6. **Close OBS on Linux** ships nil unless someone measures SIGINT on a Linux PC with OBS (Step 1.4).
7. **macOS 13 minimum**: Go 1.27 forces it. Halls with older Macs would need an older Go, which this repo does not want. Confirm that is acceptable.

## Plan review

Two cold reviews. The second (approve-with-fixes) found the missing camera-device row, the i18n test's literal-key rule, three more Windows-only strings, the Mac left-click behaviour and the CONTRACT timing; all applied. Kept as written: `HALLSTREAM_SIGN` (building the DMG once in the right order is clearer than building it twice) and `unix.IoctlGetTermios` (either works; it is already in `go.mod`). The first review gave verdict revise. Its three blockers held, and the plan now fixes them: the autostart switch sat inside the shortcut card, a nil `Close` did not hide its switch, and `watchSignals` was placed in a file the plan forbids touching. Its majors and nits were applied too. One point is kept as written: PR 5 stays in the plan as a follow-up in the website repo, because `website/src/utils/assets.ts:40-41` skips `.tar.gz` files, and the Linux download would not appear on hallstream.com without it.
