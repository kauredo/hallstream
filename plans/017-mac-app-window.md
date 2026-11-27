# 017: The Mac app gets its own window and menus

Status: TODO. Stack in worktree `app-window`, layers below.

Repo `app/` at `ed2b8cf` (main, v0.1.6). On 2026-11-27 the owner opened v0.1.6 from Applications and found no File/Edit/View/Window/Help menus, ⌘Q doing nothing, and the panel living in a browser tab. They asked for a standalone app, then to ship it all unattended and release it as v0.1.7, so their installed v0.1.6 can update itself to it. Each 🔥 row is a pick the owner can overturn before its PR merges.

## Facts found

- Hallstream is a menu-bar program with no window: `desktop.Tray.Run` (`internal/desktop/tray.go`) runs fyne.io/systray, which runs `[NSApp run]` on the main thread and sets `SystrayAppDelegate` as the app delegate. It sets no main menu, so macOS shows only the app name and ⌘Q has no menu item to trigger.
- `internal/desktop/delegatehook_darwin.go` adds `applicationShouldHandleReopen:` (Dock click, opening the app again) and `applicationShouldTerminate:` (Dock's Quit, logout) to systray's delegate class. Terminate answers NSTerminateLater and runs "Sair" (`confirmQuit` in `cmd/hallstream/main.go` asks while the stream is live).
- The panel opens with `openBrowser(panel)` at start unless `-nobrowser` (`cmd/hallstream/main.go:325`) and from the tray's `Panel` (`main.go:359`).
- The web server stays whatever the window does: OBS's browser source loads `/overlay` from it, and phones reach the panel over the hall's wifi.
- The panel relies on browser features a web view only gives when asked: eight `<input type="file">` (logos, photos, CSV, crest, videos), three `<a download>` links (`/api/system/backup` at `painel.js:2338`, the CSV template as a `data:` URL at `:578`, the thumbnail through `miniatura.js:234`), `window.open(…, '_blank')` for the feedback form (`:3518`) and the YouTube sign-in, which opens a blank window first and then sets its location (`:2513-2516`). No `alert`/`confirm`/`prompt`.
- The Mac app is built with cgo on the macOS runner (`build.sh` macos, `.github/workflows/release.yml` job `macos`) and signed with the Hardened Runtime. The Windows exes are cross-compiled on ubuntu with no cgo; the Linux tarball is static with no cgo.
- After an in-app update, `cmd/hallstream/restart.go` starts the new copy with `-nobrowser -restarted`, because the panel lived in a browser tab that reloads itself. A panel inside the old process's window dies with it.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | 🔥 | Which systems get a window | macOS now. Windows and Linux keep the browser. | The Mac is where the owner hit this and where it can be built and tried. A Windows window (WebView2) can't be tried on this machine, and most clubs run Windows; it gets its own plan once a Windows PC can test it. Linux would need WebKitGTK and cgo, which breaks the single static binary. |
| 2 | ✅ | How | A WKWebView in an NSWindow, in a small Objective-C file in `internal/desktop` (cgo, darwin only). No new Go dependency. | The panel needs the web view's delegate hooks (file panels, downloads, links); AppKit is already linked through systray. |
| 3 | ✅ | Menus | Hallstream (Acerca do Hallstream, Ocultar ⌘H, Sair ⌘Q), Editar (Desfazer ⌘Z, Refazer ⇧⌘Z, Cortar ⌘X, Copiar ⌘C, Colar ⌘V, Selecionar tudo ⌘A), Ver (Recarregar ⌘R, Aumentar ⌘+, Diminuir ⌘−, Tamanho real ⌘0), Janela (Minimizar ⌘M, Fechar ⌘W), Ajuda (Ajuda do Hallstream: the panel's Ajuda tab). Titles from `web/i18n`. Sair goes through `[NSApp terminate:]`, so it reaches the existing terminate hook and asks while live. | The Edit menu is what makes ⌘C/⌘V work in a web view's text fields. |
| 4 | 🔥 | Closing the window | Hides it; Hallstream keeps running in the menu bar and the Dock. A Dock click, the tray's "Abrir painel" or opening the app again shows it. Quitting is ⌘Q, Hallstream > Sair, or the tray's Sair. | A volunteer closing the window mid-game must not take the overlay off air. Slack and Discord behave the same. |
| 5 | ✅ | Links | Anything not on `http://127.0.0.1:<port>`, and every new-window request (`target=_blank`, `window.open`), opens in the default browser. The panel already shows the sign-in link when its blank window isn't created (`painel.js:2520`), and that link opens outside. A blank new-window request opens nothing. Downloads are decided before the routing rule, so `data:` and `blob:` downloads stay downloads; frames inside the page load in place. The routing rule lives in Go (`desktop.inWindow`) so it is tested. | Google refuses sign-in inside embedded web views. |
| 6 | ✅ | Files | `<input type="file">` gets an NSOpenPanel (multiple when the input allows it). A download asks where to save with an NSSavePanel named after the file. | Same as a browser, minus the Downloads bar. |
| 7 | ✅ | After an in-app update | On a system with a window, the restarted copy opens its window: `restartArgs` stops adding `-nobrowser` there, and `main` treats `-restarted` as "open the window" even with `-nobrowser`, because v0.1.6's updater (the one that installs v0.1.7) always passes it. | The old window closes with the old process. |
| 8 | ✅ | Where the panel opens | `-nobrowser` still means "open nothing at start". Elsewhere the window shows at start; the tray's "Abrir painel" shows it; the browser stays one click away for anyone who wants it ("Abrir no browser" in the Ver menu). | Keeps `-nobrowser` meaning what scripts expect. |
| 9 | ✅ | Words and contract | pt.json first, en.json, CONTRACT "Desktop" lines; LEIA-ME/READ-ME Mac section. | Repo rules. |
| 10 | ✅ | Stack | 1 plan · 2 Mac app menu · 3 Mac panel window · 4 docs. | The menu alone fixes ⌘Q and is safe to land without the window. |

## Layers

### 1. `01-docs/plan-017`: this file, plans/README row.

### 2. `02-feat/mac-menu`
- `internal/desktop/menu_darwin.go` (+ inline Objective-C): `setMenus(titles)` builds the main menu on the main queue once `[NSApp run]` starts. Sair calls `[NSApp terminate:nil]`. Hide, Minimize, the Edit items and Fechar use the standard AppKit selectors (`hide:`, `performMiniaturize:`, `undo:`, `cut:`, `copy:`, `paste:`, `selectAll:`, `performClose:`), which reach whatever is focused. Ver and Ajuda call back into Go.
- `desktop.Tray` gains nothing new for this; `Tray.Run` on darwin installs the menu.
- Strings `app.menu.*` in pt and en.
- Tests: the title table has every key (`go test ./internal/i18n` covers missing keys).

### 3. `03-feat/mac-window`
- `internal/desktop/window_darwin.go` (+ `.m` or inline): `ShowPanel(url string) bool` makes or shows one NSWindow with a WKWebView loading `url`; returns false off darwin (`window_other.go`), where the caller opens the browser. Delegates: navigation policy (stay or open outside, by `inWindow`), new-window requests (open outside), open panel (NSOpenPanel), downloads (WKDownload + NSSavePanel). Window: 1280×800 first time, remembers its frame, min 420×560, closes to hidden (`releasedWhenClosed` NO).
- `internal/desktop/route.go`: `inWindow(target, panel string) bool`, tested on every system.
- `cmd/hallstream/main.go`: at start and from the tray, `desktop.ShowPanel(panel) || openBrowser(panel)`; reopen shows the window; `restartArgs` keeps `-nobrowser` only where `desktop.HasWindow` is false.
- `macres/Info.plist`: `NSAppTransportSecurity` > `NSAllowsLocalNetworking`. Apple's docs: from macOS 14, ATS blocks plain HTTP to IP addresses, 127.0.0.1 included.
- Tests: `inWindow` table; `restartArgs` with and without a window.

### 4. `04-docs/mac-window`
- LEIA-ME/READ-ME Mac section (the window, closing keeps it running, ⌘Q), README, CONTRACT, ROADMAP, plans row.

Each layer: `go vet ./...`, `GOOS=linux CGO_ENABLED=0 go vet ./...`, `go test ./...`, `GOOS=windows GOARCH=386 go build ./...`, gofmt, panel Chrome tests, and on this Mac `./build.sh <v> macos` and a run of the built app.

## Done when

On macOS, opening Hallstream.app shows the panel in its own window with Hallstream, Editar, Ver, Janela and Ajuda menus; ⌘Q quits (asking while the stream is live); ⌘C/⌘V work in the panel's fields; closing the window leaves Hallstream running and a Dock click brings it back; links, the feedback form and the YouTube sign-in open in the default browser; file pickers and the three downloads work; after an in-app update the new copy opens its window. Windows and Linux behave as in v0.1.6.

## Not tested before merge

Windows and Linux are unchanged by design. The update from v0.1.6 to v0.1.7 is tried with the owner after the release.

## Plan review

One goldfish round (revise). Fixed: the v0.1.6 updater's `-nobrowser` (decision 7), the dropped JS fallback (decision 5), the download-before-routing order, blank new windows, and the ATS key, which Apple's docs say macOS 14+ needs (the goldfish thought loopback was exempt; it isn't from macOS 14). Raycast: the installed app is a normal bundle with no `LSUIElement` and Spotlight has it (`mdls`), so the search miss reads as Raycast's index lagging a fresh install, not something the app controls. `ShowPanel` dispatches to the main queue and always returns true on macOS, so it never blocks, before or after `[NSApp run]`. The menus take the language at start and don't rebuild on a language change.
