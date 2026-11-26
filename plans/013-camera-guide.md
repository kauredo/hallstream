# Plan 013: A guide for two and three cameras, on Wi-Fi and on cable, ships in the zip

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report, do not improvise. When done, update the status row for this plan
> in `plans/README.md`, unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat b28fb61..HEAD -- build.sh .github/workflows/release.yml docs/youtube LEIA-ME.txt READ-ME.txt web/painel/painel.js web/i18n docs/CONTRACT.md CLAUDE.md`
> Plans 011 and 012 are expected to have changed the panel and the
> language files. `build.sh` and `release.yml` should be as quoted below;
> if either changed, map the change one-to-one or STOP.

## Status

- **Priority**: P2 (ROADMAP Phase B item 6)
- **Effort**: S-M (mostly writing)
- **Risk**: LOW (the release check fails loudly if the zip is wrong)
- **Depends on**: `plans/011-add-cameras.md` (Definições > Câmaras, the VDO.Ninja links in `internal/obsws/vdo.go`) and `plans/012-camera-health.md` (the "Sem imagem" and "Imagem parada" lines). Check: `ls internal/obsws/vdo.go internal/obsws/health.go` both exist, and `grep -n "painel.camaras.titulo" web/i18n/pt.json` matches. Any miss: STOP.
- **Category**: docs
- **Planned at**: commit `b28fb61`, 2026-11-26

## Why this matters

A club with one camera follows LEIA-ME.txt. A club with three has questions the program can't answer for it: whether the hall's Wi-Fi carries three phones, which phone goes on a cable, why a phone camera needs the internet when nothing else does. ROADMAP Phase B item 6 asks for a guide covering two and three cameras on Wi-Fi and on cable, with the bandwidth each needs. After this plan the zip carries that guide in both languages, the release check enforces it, and Definições > Câmaras points to it.

Owner decision this plan must honour (`plans/phase-b-decisions.md`, decision 10):

- `docs/cameras/GUIDE.md` and `GUIDE.en.md`, shipped in the zip next to the YouTube guide as `CAMARAS.txt` / `CAMERAS.txt`, and linked from Definições > Câmaras. Bandwidth numbers come from DroidCam's and VDO.Ninja's docs.

## Decisions made in this plan (the executor does not re-decide them)

1. **Where the files go in the zip**: at the zip root, next to `LEIA-ME.txt` and `READ-ME.txt`, as `CAMARAS.txt` and `CAMERAS.txt`. The YouTube guide sits in a `YouTube/` folder (`build.sh:35-39`); see "Open questions / corrections" 1.
2. **The link from Definições > Câmaras** names the file and where it is, the way the YouTube help names `YouTube\LIGAR-YOUTUBE.txt` (`web/i18n/pt.json:655`). A page in the browser can't open a file next to the exe. A second link opens the same guide on GitHub for a PC with internet.
3. **The executor writes the prose** from the outline and the facts below. Every number in the guide comes from the facts table, with the source URL in the guide's last section. No number from memory.
4. **Style**: like `docs/youtube/SETUP.md`. European Portuguese first, then British English with the same sections. Short numbered steps, buttons in bold as the screen shows them, an "If something goes wrong" section. The program's names: "Definições > Câmaras", "Configurar OBS", "Telemóvel (VDO.Ninja)".
5. **The guide assumes 1080p from the phones and 720p as the fallback**, and adds the numbers per camera into totals for two and three cameras. It tells the club to put the PC on a cable to the router whenever it can, because the PC also sends the stream to YouTube.

## Facts for the guide (each with its source)

Read 2026-11-26. The executor cites each in the guide's "Fontes" / "Sources" section.

| # | Fact | Source |
|---|---|---|
| F1 | DroidCam's quality presets in the OBS plugin, per stream, AVC: 1280x720 Low 3, Normal 4, High 6, Very High 8 Mbps; 1920x1080 Low 5, Normal 7, High 8, Very High 12 Mbps; 640x480 0.6 to 1.4 Mbps. | https://droidcam.app/obs/usage/ ("What are the quality presets for video?") |
| F2 | DroidCam: "1280x720 is the recommended option for DroidCam for optimal balance of quality and performance." | https://droidcam.app/help/ |
| F3 | DroidCam's free version: "DroidCam is free to use at standard definition. You can try HD video, but will need to purchase the pro upgrade to remove watermarks." The Windows client: "The free version of DroidCam is limited to 480p." | https://droidcam.app/obs/usage/, https://www.dev47apps.com/droidcam/hd-mode/ |
| F4 | DroidCam, several phones: "To use multiple phones, just add another DroidCam source to your scene." | https://droidcam.app/obs/usage/ |
| F5 | DroidCam Wi-Fi: "Your phone and computer must be connected to the same network/router." "If you have both 2.4Ghz and 5Ghz WiFi networks active, all devices should be on the same band." | https://droidcam.app/obs/usage/ |
| F6 | DroidCam USB: "USB connections will be more stable in general due to lack of interference and packet loss." Android needs USB debugging (Developer options, tap "Build Number" seven times, then "Allow USB Debugging"); iPhone needs Apple's drivers on Windows (iTunes or the Apple Devices app) and "Trust" on the phone. | https://droidcam.app/help/, https://droidcam.app/obs/usage/ |
| F7 | DroidCam, heat and battery: "Keep the phone at 100% charge and connected to power to avoid any battery usage and extra heat." "Place the phone in a way that will let it dissipate heat." | https://droidcam.app/help/ |
| F8 | DroidCam's port: "DroidCam uses port 4747 by default." | https://droidcam.app/help/ |
| F9 | VDO.Ninja, a phone camera at 1080p (`quality=0`): about 4000 kbps; 720p about 2500 kbps. "Audio and overhead come on top." The phone's capture depends on the device: "iPhone or iPad with Safari/WebKit 17 or newer: 1080p", "Other mobile with at least 4 threads: 720p". | https://docs.vdo.ninja/guides/default-resolution-and-bitrate.md |
| F10 | Hallstream's view link asks for `videobitrate=4000` (4 Mbps) per phone, and the phone sends no sound (`audiodevice=0`). | `internal/obsws/vdo.go` (plan 011), https://docs.vdo.ninja/advanced-settings/video-bitrate-parameters/bitrate.md |
| F11 | VDO.Ninja needs the internet even on one network: the picture "usually stays on the local network", but "Loading the page and completing the handshake still need internet access." | https://docs.vdo.ninja/readme/how-does-it-work.md |
| F12 | VDO.Ninja in OBS: "Make sure 'Shutdown source when not visible' is NOT checked." Configurar OBS already sets this (plan 011). | https://docs.vdo.ninja/guides/enabling-webrtc-sources-in-obs.md |
| F13 | The PC's own upload to YouTube: the executor reads YouTube's encoder settings page and quotes its recommended bitrate for 1080p at 30 fps. Do not guess it. | https://support.google.com/youtube/answer/2853702 |

Numbers the guide may derive (state them as sums of the facts above):

- Two phones on DroidCam at 720p Normal: 2 x 4 = 8 Mbps on the Wi-Fi. Three: 12 Mbps. At 1080p Normal: 14 and 21 Mbps (F1).
- Two phones on VDO.Ninja: 2 x 4 = 8 Mbps plus overhead. Three: 12 Mbps plus overhead (F9, F10).
- A phone on a USB cable with DroidCam uses no Wi-Fi for its picture. The docs don't say so in those words (F6 says only that USB avoids "interference and packet loss"). Write it as the plain consequence of a cable: the picture goes through the cable.

## Guide outline (both languages, same sections)

Title: "Duas ou três câmaras" / "Two or three cameras".

1. **Antes de começar** / **Before you start**: what Hallstream does with cameras (one OBS scene per camera, graphics on top, the camera buttons on Ecrãs); up to three; the three kinds of camera: DroidCam (an app on the phone), a camera plugged into the PC (a webcam, or an HDMI camera through a capture card), a phone with no app through VDO.Ninja.
2. **Escolher as câmaras** / **Choosing the cameras**: a short table, one row per kind: needs an app or not, needs internet or not (only VDO.Ninja, F11), cable possible or not, cost (DroidCam HD needs Pro, F3).
3. **Quanto a rede aguenta** / **How much the network carries**: the per-camera numbers (F1, F9, F10) and the totals for two and three cameras; the PC's YouTube upload on top (F13); one rule: if the totals pass what the hall's Wi-Fi gives, put a phone on a cable or lower one camera to 720p (F2).
4. **Duas câmaras por wifi** / **Two cameras on Wi-Fi**: numbered steps: PC on a cable to the router if possible; both phones on the same network and band (F5); phones on chargers (F7); in Definições > Câmaras, add each camera; Configurar OBS; check both lines on Ecrãs say "Com imagem".
5. **Três câmaras por wifi** / **Three cameras on Wi-Fi**: the same, plus the totals warning, and which camera to drop to 720p first (the one least on air).
6. **Com cabo** / **On cable**: a DroidCam phone by USB (F6, Android and iPhone steps); a webcam or capture card by USB; the PC by network cable. A mix: the main camera on cable, the others on Wi-Fi.
7. **Um telemóvel sem aplicação (VDO.Ninja)** / **A phone with no app (VDO.Ninja)**: in Definições > Câmaras pick "Telemóvel (VDO.Ninja)", Configurar OBS, scan the QR code on the phone, allow the camera, turn the phone sideways, keep it on the charger and its screen on. It needs the internet (F11). Only one phone per QR code: a second phone on the same code is refused by VDO.Ninja.
8. **No dia do jogo** / **On game day**: open DroidCam or the VDO.Ninja page before the game; check every line on Ecrãs; what "Sem imagem" and "Imagem parada" mean (plan 012) and the first thing to try for each.
9. **Se algo correr mal** / **If something goes wrong**: no picture from a DroidCam phone on Wi-Fi (same network and band, F5; port 4747, F8); a phone on VDO.Ninja that stays black (internet, the page closed, the screen locked); "Imagem parada" on a camera that is fine (OBS's "Deactivate when not showing" left on; turn it off); OBS says it skipped frames (the PC can't keep up: fewer cameras at 1080p, close other programs).
10. **Fontes** / **Sources**: every URL in the facts table.

## Current state

### Files and their roles (line numbers at `b28fb61`)

- `docs/youtube/SETUP.md` and `SETUP.en.md`: the YouTube guide, the style to copy. Markdown light enough to read as plain text: `#` titles, numbered steps, bold button names, bare URLs.
- `build.sh:35-41`:
  ```zsh
  rm -rf dist/YouTube
  mkdir -p dist/YouTube
  # A .txt opens with a double click on Windows; the guide is Markdown-light.
  /bin/cp -f docs/youtube/SETUP.md dist/YouTube/LIGAR-YOUTUBE.txt
  /bin/cp -f docs/youtube/SETUP.en.md dist/YouTube/CONNECT-YOUTUBE.txt
  rm -f dist/Hallstream-$version.zip
  (cd dist && zip -q -X -9 Hallstream-$version.zip "Hallstream.exe" "Hallstream (32-bit).exe" LEIA-ME.txt READ-ME.txt LICENSE.txt THIRD-PARTY-NOTICES.txt YouTube/CONNECT-YOUTUBE.txt YouTube/LIGAR-YOUTUBE.txt)
  ```
- `.github/workflows/release.yml`:
  - lines 12-19, the pull-request paths that trigger a dry run: `docs/youtube/**` is there; `docs/cameras/**` is not.
  - line 71, the exact zip contents, sorted with `LC_ALL=C sort`:
    ```
    want=$'Hallstream (32-bit).exe\nHallstream.exe\nLEIA-ME.txt\nLICENSE.txt\nREAD-ME.txt\nTHIRD-PARTY-NOTICES.txt\nYouTube/CONNECT-YOUTUBE.txt\nYouTube/LIGAR-YOUTUBE.txt'
    ```
- `CLAUDE.md:13` lists what the release checks the zip holds. It changes with the zip.
- `LEIA-ME.txt` and `READ-ME.txt`: the plain-text readmes; `LEIA-ME.txt:177` "DESCONTO DE TEMPO COM O OBS" and `:254` "DIRETOS NO YOUTUBE" are the sections nearest to cameras.
- `web/i18n/pt.json:655`: `painel.ajuda.yt.1`, the pattern for naming a file in the program folder.
- `web/painel/painel.js`: the Câmaras card from plan 011 in `renderDefinicoes`.
- `docs/CONTRACT.md`: has no section on the zip's contents. Check with `grep -n "LIGAR-YOUTUBE" docs/CONTRACT.md`; if a line exists, update it too.

### Conventions

- Guides are Markdown-light and must read well as plain `.txt` in Notepad: no tables wider than about 80 characters, no HTML, no images. A table in section 2 must stay narrow; use a list if it can't.
- Panel copy in `web/i18n/pt.json` first, then `en.json`. `go test ./internal/i18n` enforces it.
- Never read or edit `dados/` or `dist/`. Running `./build.sh` writes `dist/`; that is expected, but do not open or edit what it writes beyond `zipinfo -1`.
- Commits: `docs: camera guide …` for the guide, `build: …` for the zip.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| i18n | `go test ./internal/i18n` | pass |
| Tests | `go test ./...` | pass |
| Build the zip | `./build.sh 0.0.0-test` | ends with `ls -lh dist` |
| Zip contents | `zipinfo -1 dist/Hallstream-0.0.0-test.zip \| LC_ALL=C sort` | the 10 names in Step 3 |
| Line width | `awk 'length > 100' docs/cameras/GUIDE.md docs/cameras/GUIDE.en.md` | no table row longer than 100 (prose lines may be long) |

## Suggested executor toolkit

- `technical-writing` for the guide's structure, and the owner's writing style (`~/.claude/writing-style.md` if present) for the prose: plain words, no em dashes.
- `read-the-damn-docs` before quoting F13.

## Scope

**In scope**:

- `docs/cameras/GUIDE.md`, `docs/cameras/GUIDE.en.md` (create)
- `build.sh` (copy and zip two files)
- `.github/workflows/release.yml` (the paths list and `want`)
- `CLAUDE.md` (the zip contents sentence on line 13)
- `LEIA-ME.txt`, `READ-ME.txt` (one short paragraph each)
- `web/painel/painel.js` (one hint in the Câmaras card), `web/i18n/pt.json`, `web/i18n/en.json`
- `docs/ROADMAP.md`

**Out of scope**:

- The website's camera page (website repo).
- Any code behaviour. This plan changes copy and packaging only.
- Serving the guide from the program. See "Open questions / corrections" 2.
- `docs/youtube/*`.

## Git workflow

- Branch: `docs/camera-guide` (stacked on plan 012's branch)
- One commit per step. Do NOT push or open a PR unless told to.

## Steps

### Step 1: The Portuguese guide

Write `docs/cameras/GUIDE.md` from the outline and the facts. Every number cites its fact's source in the "Fontes" section. Open `internal/obsws/vdo.go` and copy the real `videobitrate` value into F10's sentence; if it is not 4000, use the real one and fix the totals.

Read F13's page and quote its 1080p 30 fps bitrate in section 3.

**Verify**: `grep -c "https://" docs/cameras/GUIDE.md` is at least 8; `grep -n "—" docs/cameras/GUIDE.md` finds nothing; every section heading of the outline is present (`grep -n "^## " docs/cameras/GUIDE.md` lists 10).

### Step 2: The English guide

Write `docs/cameras/GUIDE.en.md` with the same sections in the same order, in British English, button names as the English panel shows them (read `web/i18n/en.json` for "Settings > Cameras", "Set up OBS", "Phone (VDO.Ninja)").

**Verify**: `grep -c "^## " docs/cameras/GUIDE.en.md` equals the Portuguese count; `grep -n "—" docs/cameras/GUIDE.en.md` finds nothing.

### Step 3: Ship it in the zip

- `build.sh`, after the YouTube lines:
  ```zsh
  /bin/cp -f docs/cameras/GUIDE.md dist/CAMARAS.txt
  /bin/cp -f docs/cameras/GUIDE.en.md dist/CAMERAS.txt
  ```
  and add `CAMARAS.txt CAMERAS.txt` to the `zip` line after `THIRD-PARTY-NOTICES.txt`.
- `.github/workflows/release.yml`: add `- docs/cameras/**` to the pull-request paths after `docs/youtube/**`, and set `want` to the new sorted list. `LC_ALL=C` sorts upper case by byte, so the list is:
  ```
  CAMARAS.txt
  CAMERAS.txt
  Hallstream (32-bit).exe
  Hallstream.exe
  LEIA-ME.txt
  LICENSE.txt
  READ-ME.txt
  THIRD-PARTY-NOTICES.txt
  YouTube/CONNECT-YOUTUBE.txt
  YouTube/LIGAR-YOUTUBE.txt
  ```
  Write it as one `$'…\n…'` string like the current line 71.
- `CLAUDE.md:13`: add "and the camera guide in both languages" to the list of what the release checks the zip holds.

**Verify**: `./build.sh 0.0.0-test` succeeds, then `zipinfo -1 dist/Hallstream-0.0.0-test.zip | LC_ALL=C sort` prints exactly the 10 lines above, in that order. Then build the `want` string in a scratch script and compare it with that output byte for byte (copy line 71's logic into a file under your scratch folder and run it with the zip path; it must print nothing).

### Step 4: Point to it

- `web/i18n/pt.json`: `painel.camaras.guia`: "Duas ou três câmaras, por wifi ou por cabo: leia o guia <code>CAMARAS.txt</code>, na pasta do programa, ou <a href=\"https://github.com/kauredo/hallstream/blob/main/docs/cameras/GUIDE.md\" target=\"_blank\" rel=\"noopener\">abra-o na internet</a>."
- `web/i18n/en.json`: `painel.camaras.guia`: "Two or three cameras, on Wi-Fi or on cable: read the <code>CAMERAS.txt</code> guide in the program folder, or <a href=\"https://github.com/kauredo/hallstream/blob/main/docs/cameras/GUIDE.en.md\" target=\"_blank\" rel=\"noopener\">open it online</a>."
- `web/painel/painel.js`: one `<p class="hint">` with that key at the foot of the Câmaras card.
- `LEIA-ME.txt`: after "DESCONTO DE TEMPO COM O OBS", a section "DUAS OU TRÊS CÂMARAS" of two or three lines pointing to `CAMARAS.txt`. `READ-ME.txt`: the same in English ("TWO OR THREE CAMERAS", `CAMERAS.txt`), in the matching place.

**Verify**: `go test ./internal/i18n ./internal/web/` → pass; in the mock (`?mock=1&obs=ready&cams=2`), Definições > Câmaras shows the hint and the link opens the GitHub page in a new tab.

### Step 5: Roadmap

- `docs/ROADMAP.md` Phase B item 6: "Done: `CAMARAS.txt` / `CAMERAS.txt` in the zip, from `docs/cameras/`."

**Verify**: `go vet ./... && go test ./... && GOOS=windows GOARCH=386 go build ./...` → all exit 0.

## Test plan

- No Go tests: the guide is copy. The release workflow's zip check is the test; Step 3 runs the same check locally.
- `go test ./internal/i18n` covers the new key in both files.

## Done criteria

- [ ] `ls docs/cameras/GUIDE.md docs/cameras/GUIDE.en.md` lists both
- [ ] `grep -c "^## " docs/cameras/GUIDE.md` equals `grep -c "^## " docs/cameras/GUIDE.en.md`
- [ ] `grep -rn "—" docs/cameras/` finds nothing
- [ ] `grep -n "CAMARAS.txt" build.sh .github/workflows/release.yml` matches in both
- [ ] `grep -n "docs/cameras" .github/workflows/release.yml` matches
- [ ] `zipinfo -1 dist/Hallstream-0.0.0-test.zip | LC_ALL=C sort` prints the 10 names of Step 3
- [ ] `grep -n "painel.camaras.guia" web/i18n/pt.json web/i18n/en.json` matches in both
- [ ] `go test ./...` exits 0
- [ ] `plans/README.md` status row updated

## STOP conditions

- Plans 011 or 012 have not landed (the checks under "Depends on" fail).
- A fact page in the table now says something different (a new DroidCam preset table, a new VDO.Ninja default). Update the number from the page, note it in the PR description, and carry on; STOP only if a page is gone and no other official page gives the number.
- YouTube's page (F13) gives no bitrate for 1080p at 30 fps. Write the section without a YouTube number, say so in the PR, and carry on.
- `build.sh` or `release.yml` changed since `b28fb61` in a way you can't map.
- A verification fails twice after a reasonable fix.

## Maintenance notes

- Three places list the zip's contents: `build.sh`, `release.yml` (`want`) and `CLAUDE.md:13`. They change together.
- The numbers come from two vendors' pages. When DroidCam or VDO.Ninja change their defaults, the guide's section 3 changes; F10 also changes if `internal/obsws/vdo.go` changes its `videobitrate`.
- The GitHub link in `painel.camaras.guia` points at `main`. If the docs move, the link breaks; `docs/cameras/` should stay put.
- Reviewers: read the Portuguese guide in Notepad (or any plain-text viewer) at 80 columns and check nothing reads as broken Markdown.

## Open questions / corrections

1. **"Next to the YouTube guide" is ambiguous.** The YouTube guide is not at the zip root: it sits in a `YouTube/` folder as `LIGAR-YOUTUBE.txt` and `CONNECT-YOUTUBE.txt` (`build.sh:35-41`). Recommended: `CAMARAS.txt` and `CAMERAS.txt` at the zip root, next to `LEIA-ME.txt`, where a volunteer sees them on unzipping. The other choice is a `Cameras/` folder like `YouTube/`; an accented `Câmaras/` folder name is a risk in a zip opened by Windows Explorer.
2. **"Linked from Definições" can't open the file.** The panel is a browser page; it can't open a `.txt` beside the exe. Recommended: name the file and the folder, as the YouTube help does (`web/i18n/pt.json:655`), plus a link to the guide on GitHub for a PC with internet. Serving the guide from the program would mean embedding `docs/` into the exe, which `web/embed.go` doesn't do today; not worth it for one page.
3. **DroidCam's free version is 480p.** F3: HD needs DroidCam Pro, and the free Windows client is "limited to 480p". The roadmap and decisions present DroidCam as the default camera. The guide must say so; the owner may also want LEIA-ME.txt, which tells clubs to install DroidCam, to say it.
4. **DroidCam gives no Wi-Fi band advice beyond "the same band".** F5 says all devices should be on the same band. It does not say 5 GHz is better, and it gives no router distance. The guide says only what the page says.
5. **VDO.Ninja gives kbps targets, not a bandwidth rule.** F9 gives targets and says "Audio and overhead come on top", with no number for the overhead. The guide quotes 4 Mbps per phone (F10, the value Hallstream asks for) and says "plus a margin".
6. **The PC's upload to YouTube is not in DroidCam's or VDO.Ninja's docs.** Decision 10 names only those two sources. A three-camera hall on one Wi-Fi also carries the PC's stream when the PC is on Wi-Fi. Recommended: quote YouTube's own page (F13) for that one number.
