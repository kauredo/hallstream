# Hallstream

A free tool for amateur clubs that stream their games. Hallstream reads the score from the hall's scoreboard console and puts it on the stream as broadcast graphics. It also sets up OBS and YouTube, so a volunteer can run a game-day stream from one laptop.

> Hallstream is early. It scores **basketball, futsal, handball, volleyball and soccer**. The phone scorer works for all five. A **Stramatel** console and its Stream Box over Bluetooth is tested on basketball only. Futsal, handball, volleyball and soccer on the console are untested until someone records a real game, so if you have a console and a game to spare, please record one. Its interface is in **European Portuguese** and **English**. More consoles are on the [roadmap](docs/ROADMAP.md).

## What it does

- Reads the score, clock, fouls and timeouts from a Stramatel Stream Box over Bluetooth, with no operator typing. Tested on basketball only.
- Scores any of the five sports from a phone, with the rules of each sport (periods, sets, fouls, suspensions, cards).
- Serves the overlay as an OBS browser source:
  - scoreboard
  - opening and squad screens
  - timeouts, half-time and the final
  - player lower thirds
  - sponsors and promo videos
- Gives you a control panel: in its own window on Windows (WebView2, or an Edge window without it) and on a Mac, in the browser on Linux, and in the browser on a phone or tablet on the same network (with a PIN).
- Sets up OBS over obs-websocket: one scene per camera, up to four, with the overlay on top. Cameras come from DroidCam, a webcam or capture device (Windows only for now), or a phone's browser through VDO.Ninja, and you switch between them from the panel. Each camera can hide the corner graphics, the Nothing screen and timeouts can each switch to a camera of their own, and a second camera can sit in a small inset on the scoreboard. `CAMERAS.txt` in each download covers two to four cameras.
- Creates the YouTube live broadcast with a title and thumbnail, and makes it unlisted afterwards.
- Pulls squads, photos and fixtures from the Portuguese basketball federation (FPB), if you choose to. The FPB import is for Portuguese basketball only.
- Keeps working offline after the first load. A demo mode plays an invented game with no console.
- Updates itself when you click "Update now" in the panel, never while the stream is live. It checks the download against the release's `SHA256SUMS`, and on a Mac against the Developer ID, then swaps the program and restarts without closing OBS. The daily version check behind the offer stays off until you turn it on.

## Get started

1. Download the file for your computer from Releases:
   - Windows 10 or later: `Hallstream-X.Y.Z.zip`. Unzip it to a folder such as `C:\Hallstream` and open `Hallstream.exe`.
   - macOS 13 Ventura or later, Intel or Apple chip: `Hallstream-X.Y.Z-macos.dmg`. Drag Hallstream into Applications and open it from there. Its data goes in `~/Library/Application Support/Hallstream/dados`.
   - 64-bit Linux with BlueZ: `Hallstream-X.Y.Z-linux-x64.tar.gz`. Extract it and run `Hallstream/Hallstream`. The tray icon needs an AppIndicator (StatusNotifier) host, and message boxes need `zenity`.
2. Install [OBS Studio](https://obsproject.com) 31 or later. Open Hallstream and the panel opens (in your browser on Linux).
3. Follow the first-run steps in the panel. `READ-ME.txt` in each download has the full guide (`LEIA-ME.txt` in Portuguese).

The Windows exes are unsigned, since SignPath declined the project, so Windows may warn about them. The Mac app is signed with a Developer ID and notarised by Apple.

## Build from source

Hallstream needs Go 1.27 or later.

```sh
go test ./...                              # needs no network; Chrome tests skip without Chrome
go run ./cmd/hallstream -demo -nobrowser      # invented game, panel at http://127.0.0.1:8765/painel
./build.sh 0.1.0                           # Windows exes and the zip in dist/
./build.sh 0.1.0 macos                     # universal Hallstream.app and the DMG (on a Mac)
./build.sh 0.1.0 linux                     # static amd64 binary and the tarball
```

`docs/CONTRACT.md` describes every endpoint, the state shape and the rules. Change it in the same commit as the behaviour.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md). Help is most useful in these areas:
- support for another console: you need access to one
- recording a real game with a console for futsal, handball, volleyball or soccer
- another sport
- translations
- testing on real game days

## Licence

Copyright (C) 2026 Vasco Kaufmann. Hallstream is free software under the [GNU Affero General Public License, version 3 or later](LICENSE) (AGPL-3.0-or-later), with no warranty. Each download carries the licence and `THIRD-PARTY-NOTICES.txt`, the licences of the code linked into that platform's program (`scripts/notices.sh` writes it).

Stramatel, OBS and YouTube are trademarks of their owners. Hallstream is not affiliated with any of them.
