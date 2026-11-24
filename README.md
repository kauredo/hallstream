# Hallstream

A free tool for amateur clubs that stream their games. Hallstream reads the score from the hall's scoreboard console and puts it on the stream as broadcast graphics. It also sets up OBS and YouTube, so a volunteer can run a game-day stream from one laptop.

> Hallstream is early. It works today for **basketball**, with a **Stramatel** console and its Stream Box over Bluetooth. Its interface is in **European Portuguese**. Other sports, manual scoring, more consoles and multiple cameras are on the [roadmap](docs/ROADMAP.md).

## What it does

- Reads the score, clock, fouls and timeouts from a Stramatel Stream Box over Bluetooth, with no operator typing.
- Serves the overlay as an OBS browser source:
  - scoreboard
  - opening and squad screens
  - timeouts, half-time and the final
  - player lower thirds
  - sponsors and promo videos
- Gives you a control panel in the browser, on the PC or on a phone or tablet on the same network (with a PIN).
- Sets up OBS over obs-websocket: the scene, the overlay source and the camera.
- Creates the YouTube live broadcast with a title and thumbnail, and makes it unlisted afterwards.
- Pulls squads, photos and fixtures from the Portuguese basketball federation (FPB), if you choose to.
- Keeps working offline after the first load. A demo mode plays an invented game with no console.

## Get started

1. Download the latest zip from Releases and unzip it to a folder such as `C:\Hallstream`.
2. Install [OBS Studio](https://obsproject.com) 31 or later, then open `Hallstream.exe`. The panel opens in your browser.
3. Follow the first-run steps in the panel. `LEIA-ME.txt` in the zip has the full guide, in Portuguese.

Windows may warn about an unsigned app. Signed releases are planned.

## Build from source

Hallstream needs Go 1.27 or later.

```sh
go test ./...                              # needs no network; Chrome tests skip without Chrome
go run ./cmd/hallstream -demo -nobrowser      # invented game, panel at http://127.0.0.1:8765/painel
./build.sh 0.1.0                           # Windows exes and the zip in dist/
```

`docs/CONTRACT.md` describes every endpoint, the state shape and the rules. Change it in the same commit as the behaviour.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md). Help is most useful in these areas:
- support for another console: you need access to one
- another sport
- translations
- testing on real game days

## Licence

Copyright (C) 2026 Vasco Kaufmann. Hallstream is free software under the [GNU Affero General Public License, version 3 or later](LICENSE) (AGPL-3.0-or-later), with no warranty. The release zip carries the licence and `THIRD-PARTY-NOTICES.txt`, the licences of the code linked into the exes (`scripts/notices.sh` writes it).

Stramatel, OBS and YouTube are trademarks of their owners. Hallstream is not affiliated with any of them.
