# 018: The Windows app gets its own window

Status: built and tried on the club laptop; in review.

Repo `app/` at `d641629` (main, v0.1.7 tagged). Plan 017 gave the Mac app a window and left Windows on the browser, because no Windows PC could try it then. The club laptop (Windows 11, reachable over Tailscale) can now. On 2026-11-27 the owner saw another project open its page as an Edge `--app` window, asked for the same or better on Windows, and then said the better option should win. That is a native WebView2 window, with the Edge app window as the fallback. Each 🔥 row is a pick the owner can overturn before its PR merges.

## Facts found

- The Windows exes are cross-compiled on ubuntu with no cgo (`build.sh`, `.github/workflows/release.yml` job `windows`), so the window has to come from pure Go.
- WebView2 is the Edge engine for apps. It ships with Windows 11 and with Windows 10 kept up to date. `github.com/wailsapp/go-webview2` (MIT, the one Wails v2 uses, v1.0.9) drives it from pure Go and finds the runtime without Microsoft's WebView2Loader.dll unless built with `native_webview2loader`. `jchv/go-webview2`, the fork it came from, loads that DLL from memory, which antivirus tools dislike in an unsigned exe.
- The Wails package exposes the embedded browser (`edge.Chromium`): `Embed(hwnd)`, `Navigate`, settings, `ProcessFailedCallback`, `GetController().GetCoreWebView2()`. It does not wrap the new-window or navigation-starting events, though the `ICoreWebView2` it returns is the COM object itself, whose vtable has `add_NavigationStarting` at 7 and `add_NewWindowRequested` at 44 (the library's own `iCoreWebView2Vtbl`, v1.0.23).
- Any error inside the library ends the process: `errorCallback` calls `os.Exit(1)` after the callback (`pkg/edge/chromium.go:150`), on `Navigate`, resizes, controller creation and more; `WebResourceRequested` uses `log.Fatal`. A failed environment calls the callback and leaves `Embed` waiting forever. Its `pkg/edge` `init()` locks the main goroutine to the main thread and initialises COM there.
- `webviewloader.GetAvailableCoreWebView2BrowserVersionString("")` returns "" when the runtime is missing, so the program can choose before it makes a window.
- The panel's ways out (plan 017, "Facts found"): `target=_blank` links, `window.open(…, '_blank', 'noopener')` for the feedback form (`painel.js:3518`), and the YouTube sign-in, which opens a blank window and sets its `location.href` later (`painel.js:2513-2516`). Downloads are `<a download>` (same origin, `data:`, `blob:`); WebView2 handles those and `<input type="file">` the way Edge does.
- A second double-click starts a second exe, which finds the first and opens `openBrowser(panel)` (`cmd/hallstream/main.go:144-151`). With a window, that gives a browser tab next to it.
- The exe's icon resource is named `APP` (`winres/winres.json`).
- `desktop.HasWindow` means "the window dies with the process". The updater restarts without `-nobrowser` where it is true, and `openAtStart` opens the window after a `-restarted` start (plan 017, decision 7).
- `scripts/notices.sh` copies each linked module's root LICENSE. The Wails package keeps a second licence (ISC) in `webviewloader/`, which the program links.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | ✅ | How | A WebView2 window through `wailsapp/go-webview2` v1.0.23's `pkg/edge`, inside a plain Win32 window the program makes itself, run as a child process: `Hallstream.exe -window <url>`. The main program puts the child in a job object that kills it when the main program ends, and watches its exit code. | The library exits the process on any error, so it must not live in the process that serves the overlay. The child's main thread is free for the window, where the library's `init()` wants it. |
| 2 | 🔥 | Fallbacks | No WebView2 runtime, or the child exits with an error twice running (the second try 2 s later, for a data folder the old copy still holds after an update): the Edge app window (`--app`, own profile in `%LOCALAPPDATA%\Hallstream\edge`, Chrome after Edge), and Edge from then on. Neither: the default browser, as today. `HALLSTREAM_NO_WEBVIEW2=1` skips WebView2, for support and for testing the fallback. The child exits on its own if it isn't ready after 20 s, which counts as an error. The 32-bit exe always uses the Edge window. | The club PC must always reach the panel. On the club laptop the 32-bit child hung inside `Embed` with no error, so the watchdog covers a hang on 64-bit too. |
| 3 | 🔥 | Closing the window | Hides it. Hallstream keeps running in the tray; "Abrir painel" or a second double-click shows it again: the main program finds the child's window and restores and raises it. The second copy calls `AllowSetForegroundWindow` first, so Windows lets the first raise it. | Same as the Mac (017 decision 4): closing mid-game must not take the overlay off air. |
| 4 | ✅ | Window | Title "Hallstream", the exe's icon, maximised on first show, 420×560 minimum; the window procedure handles size, move, minimum size, focus and close. Right-click menu on (copy and paste), developer tools off. Data in `%LOCALAPPDATA%\Hallstream\webview2`, out of `dados/` so backups stay small. A crashed page process reloads the panel. | Volunteers paste OBS passwords and stream keys. |
| 5 | ✅ | Links | The Mac's rules, through two COM handlers the program registers itself. New-window requests (`target=_blank`, `window.open`, ctrl-click, middle-click, "open in new window") are all marked handled, so no popup opens, and the address opens in the default browser when it is `http`/`https` with a host or `mailto` (`outside`). A blank `window.open('')` returns null, and the panel shows the sign-in link, as on the Mac. A main-frame navigation off the panel (`inWindow`) is cancelled, and opens outside when the volunteer clicked it; `about:blank`, `data:` and `blob:` load. Downloads stay downloads. | Google refuses sign-in inside embedded browsers. A page that isn't the panel never loads in the window. |
| 6 | ✅ | After an in-app update | `HasWindow` is true when the WebView2 runtime is there, so the restarted copy opens its window (017 decision 7); the job object closes the old one. With the Edge fallback it stays false: that window is its own process and reloads by itself. | The old window closes with the old process. |
| 7 | ✅ | Second double-click | The second copy asks the first to show its panel (`POST /api/system/show-panel`, loopback only) and exits. If the first copy is too old to answer, it opens the browser as before. All systems. | No second panel in a browser tab. |
| 8 | ✅ | Notices | `notices.sh` also copies a LICENSE that sits in a linked package's own folder. | The ISC licence in `webviewloader/`. |
| 9 | ✅ | Words and contract | CONTRACT "Desktop" and the new endpoint; LEIA-ME/READ-ME Windows sections; README; ROADMAP. pt first, then en. | Repo rules. |
| 10 | ✅ | Stack | 1 plan and the release-job flake · 2 Edge app window and the second-copy hand-off · 3 WebView2 window · 4 docs. | The Edge window works alone and is safe to land first. |

## Layers

### 1. `01-docs/plan-018`
- This file and the plans/README row.
- `internal/web/panel_camera_fix_test.go`: `TestPanelUnsavedListStays` waits for the new row before filling it in. It failed v0.1.7's Linux release job.

### 2. `02-feat/windows-edge-window`
- `internal/desktop/appwindow.go`: `appBrowsers(env, base)` (Edge, then Chrome, 32-bit and 64-bit Program Files and LocalAppData) and `appArgs(url, profile)`. Tested on every system.
- `internal/desktop/window_windows.go`: `ShowPanel` starts the first browser found as an app window. `HasWindow` false.
- `internal/web`: `POST /api/system/show-panel` calls `Server.ShowPanel` (set by main), 403 from anything but loopback, 501 when unset. Through `ServeHTTP` like every route.
- `cmd/hallstream/main.go`: the second copy posts there and falls back to `openBrowser`.
- Tests: the endpoint (loopback, not loopback, unset), the two helpers.

### 3. `03-feat/windows-webview2-window`
- `go.mod`: `github.com/wailsapp/go-webview2 v1.0.23`.
- `internal/desktop/webview_windows.go`: `RunWindow(url) int`, the child: window class and window (hidden), `Embed`, settings, the two COM handlers, `ProcessFailedCallback`, `Navigate`, show maximised, message loop. The library's error callback exits with 3.
- `internal/desktop/window_windows.go`: `HasWindow = runtime installed && no HALLSTREAM_NO_WEBVIEW2`. `ShowPanel` raises the child's window if the child runs, else starts it in the job object; on an error exit it retries once, then falls back to Edge for good. `AllowForeground` for the second copy.
- `internal/desktop/route.go`: `outside(url) bool` next to `inWindow`, both tested on every system; `window_darwin.m` keeps its own copy of the rule.
- `cmd/hallstream/main.go`: `-window <url>` runs `desktop.RunWindow` before anything else; the second copy calls `desktop.AllowForeground`.
- `scripts/notices.sh`: decision 8.
- Tests: `outside` and `navAllowed` tables; the retry-then-fallback rule.

### 4. `04-docs/windows-window`
- LEIA-ME/READ-ME Windows sections, README, CONTRACT, ROADMAP, plans row.

Each layer: `go vet ./...`, `GOOS=linux CGO_ENABLED=0 go vet ./...`, `GOOS=windows go vet ./...`, `go test ./...`, `GOOS=windows GOARCH=386 go build ./...` and amd64, gofmt.

## Done when

On Windows with the WebView2 runtime, starting Hallstream shows the panel in its own maximised window titled Hallstream, with the exe's icon in the taskbar; closing it leaves Hallstream in the tray and "Abrir painel" or a second double-click brings it back; links, the feedback form and the YouTube sign-in open in the default browser; copy and paste work; after an in-app update the new copy opens its window. Without the runtime the panel opens as an Edge app window, and without Edge in the browser. macOS and Linux behave as in v0.1.7, apart from the second-copy hand-off.

## Tried before merge

On the club laptop over SSH, both exes (64-bit and 32-bit) started in the desktop session: screenshots of the window; close, then reopen from a second start; a `target=_blank` link and the YouTube sign-in (the child started on the mock panel) opening in the browser; a `-nobrowser -restarted` start opening the window, as an update does; `HALLSTREAM_NO_WEBVIEW2=1` giving the Edge window. Then everything removed (the exes, `%LOCALAPPDATA%\Hallstream`), with Marcador CNN on 8765 left running. A real update from a release can only be tried after this ships.

## Plan review

One goldfish round (revise). Fixed: the library's `os.Exit` on any error (the window moved to a child process in a job object), the environment failure that hangs `Embed` (the child exits, the main program retries once, then Edge), the version pin (v1.0.23, the source that was read), popups from ctrl-click, middle-click and the right-click menu (a real new-window handler instead of a page script), stray navigations (a navigation-starting handler), the foreground lock on a second double-click, a crashed page process, the window messages, and the laptop test, which now covers both exes, the fallback and a restarted start.

## Found on the club laptop

- The WebView2 browser stays white when it is made while its window is hidden, until `Show()` and `Resize()` after the window shows.
- The 32-bit child never finished `Embed` (no error, no log line). The 32-bit exe now skips WebView2 for the Edge window.
- The released v0.1.7 32-bit exe already dies with 0xC0000005 a second after "À procura da Stream Box..." on this laptop, and runs with `-demo`. That is the Bluetooth reader on 386 and is outside this plan; it needs its own.
- The second double-click restores the hidden window in front; the first start from a scheduled task only flashes the taskbar button, because the task is not the foreground process. A double-click from Explorer is.
