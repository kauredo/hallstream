# Contributing to Hallstream

Volunteers run Hallstream in a sports hall, often with no internet and nobody technical nearby. Every change has to keep that game day working.

## Before you start

- For anything bigger than a small fix, open an issue first, so we can agree on the approach.
- Read `CLAUDE.md` for the commands and the constraints, and `docs/CONTRACT.md` for the behaviour.
- Run `go vet ./...`, `go test ./...` and `GOOS=windows GOARCH=386 go build ./...` before you open a pull request. CI runs the same checks.

## Rules the code follows

- `docs/CONTRACT.md` describes the behaviour. Change it in the same commit as the code.
- `web/` ships exactly as it is in the repo: no npm, no build step, nothing loaded from a CDN.
- Every overlay screen and state has a fixture in `web/overlay/fixtures/`, and the Chrome test loads each one. Add a fixture with any new screen.
- Background loops start through `safely`, and one-off background work starts through `Server.once`. A panic must never close the program mid-game.
- Examples, fixtures and screenshots use invented clubs and people. Never commit real players' names or photos, and especially not minors'.

## Adding a scoreboard console

This needs someone with access to the console. The steps:

1. Record a real session. Today that is `dados/raw.log`. A capture mode for serial consoles is planned.
2. Film the board during the same session, so each byte can be matched to what the board showed.
3. Write the decoder behind the input interface (see the roadmap), with tests that replay the recording and check the score, clock, period and fouls at known moments.
4. In the pull request, name the console model, its firmware if you know it, and the sports you tested.

A decoder without a recorded session can't be merged, because nobody else could check it.

## Adding a sport

Follow phase C in `docs/ROADMAP.md`. Your pull request needs to include:
- the rules as data
- the graphics, as fixtures for every screen
- a recorded or demo game that exercises the clock and the end-of-period rules

## Translations

The string catalogs are planned in phase A. Until they exist, open an issue to say which language you can help with.

## Reporting a problem

Open an issue with:
- the Hallstream version
- the console and sport
- what you expected to happen
- `dados/marcador.log`

Remove anything personal before you attach a file.
