# 021: A built-in YouTube client, with the club's own client as the fallback

Status: in review, one PR.

Repo `app/` at `f678022` (main, v0.1.9). ROADMAP Phase A item 7: "a project OAuth client that ships with the app, with 'use your own' kept as the fallback".
The owner asked on 2026-11-28 to build it straight away, while they create the Google Cloud project and the Desktop client. The client goes to Google for verification once a build carries it, since the verification needs a demo video of the login.

## Facts found

- Today the club pastes its own Desktop client into the panel. `ytSaved` (`internal/web/youtube.go:31`) keeps `client_id`, `client_secret`, `token`, `channel`, `stream` and `expired` in `dados/youtube.json`, which is sealed with DPAPI on Windows.
- `ytSaved.creds()` (line 41) is the one source of credentials for the login (`ytConnect`, line 233) and every refresh (`ytClient`, line 315). A refresh token works only with the client that minted it.
- `PUT /api/youtube/client` (line 178) answers 409 when the id changes while connected (`api.yt.desligue_primeiro`).
- `main.feedbackURL` and `main.uploadToken` come in through `-ldflags -X` in `build.sh` (lines 48, 65, 113) and reach the server as `web.Server` fields (`cmd/hallstream/main.go:293`).
- `ytSetupHtml(bare)` (`web/painel/painel.js:2529`) draws the login in Definições and in the setup wizard's YouTube step. The wizard puts `painel.wz.yt_precisa`, `yt_passo1..3` and `yt_sem_id` around it (line 3366), all about pasting an id and a secret.
- `docs/youtube/SETUP.md` ships in every zip, DMG and tarball as `YouTube/LIGAR-YOUTUBE.txt`, plus `CONNECT-YOUTUBE.txt` in English.
- Google: YouTube scopes are "sensitive", not "restricted", so verification is free and needs no security assessment. Until verification, a client in Testing lets in only its listed test users, and their logins last 7 days. Every user of one client shares that project's 10,000 units a day, and a game costs about 115 (`docs/CONTRACT.md`, Quota).

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | ✅ | How the client gets into the program | `main.youtubeClientID` and `main.youtubeClientSecret` through `-ldflags -X` from `HALLSTREAM_YOUTUBE_CLIENT_ID` and `HALLSTREAM_YOUTUBE_CLIENT_SECRET`, into `web.Server.YouTubeBuiltIn` (`youtube.Credentials`). An empty pair means no built-in client. | Same path as the feedback URL. A Desktop app secret is not confidential by Google's own rules for installed apps, and anyone can read it out of the binary. |
| 2 | 🔥 | When releases carry it | `release.yml` passes the two secrets only on `v…` tags, and only when the repo variable `HALLSTREAM_YOUTUBE_BUILTIN` is `on`. Pull request dry runs never get them. | While the client sits in Testing, it would refuse every club that isn't a test user. Building it into releases before Google verifies it would break the login for everyone. The owner turns the variable on once Google approves. |
| 3 | ✅ | Which client a login belongs to | `ytSaved` gains `client` (`"builtin"` or `"own"`). It is set when a login succeeds, and every refresh uses that client. Files from 0.1.9 and earlier have no field: with a token, they read as `own`. | A token keeps refreshing with the client that minted it, whatever the build carries next. |
| 4 | ✅ | Which client the next login uses | `use` (`"builtin"` or `"own"`) in `youtube.json`. With no field, it is `own` when the club saved its own client, else `builtin` when the build has one, else `own`. | An existing club keeps its own client. A new club on a release build gets the one-button login. |
| 5 | ✅ | Switching | `PUT /api/youtube/client` takes `{ "use": "builtin" }`, or `{ "use": "own", "client_id", "client_secret" }`, where the old body with no `use` means `own`. A switch to the other client while connected answers 409 (`desligue_primeiro`), as an id change does today. `builtin` on a build without one is 400. | One endpoint, as now. A switch while connected would leave a token the new client can't refresh. |
| 6 | ✅ | A login whose client the build no longer has | `client` `builtin` on a build without a built-in client (a dev build, say) counts as expired everywhere (`ytExpired`) but is never saved as expired, so the release build that made it still uses it. Desligar opens the club's own client fields. | Only a developer running `go run` against a release `dados/` meets this. Review caught the first version saving `expired` to disk. |
| 7 | ✅ | `GET /api/youtube` | Adds `builtin` (this build has one), `use` and `client` (the client of the saved login, `""` without one). `configured` means the client in `use` is complete. | The panel picks its layout from `builtin` and `use`. |
| 8 | 🔥 | The panel | With `builtin` and `use` `builtin`: one primary Ligar ao YouTube button and a line about the Google page. The own-client fields sit behind "Usar um cliente OAuth próprio (avançado)" (`yt-own`; renamed from "cliente Google próprio" in the critique, which read as "your own Google account"). With `use` `own` on a build with a built-in client: the fields plus "Usar o cliente do Hallstream" (`yt-builtin`). Without a built-in client: exactly as today. | A volunteer sees one button. The fallback stays one click away for a club that hits the quota or wants its own project. |
| 9 | ✅ | The wizard's YouTube step | Built-in: one line (`painel.wz.yt_builtin`) and the two existing account hints. Own: today's steps. | Today's steps ask for an id and a secret the built-in client makes unnecessary. |
| 10 | ✅ | "The app isn't verified" | `painel.wz.yt_passo3` and the guide's step 19 say the warning appears only with the club's own client. The built-in client shows none once Google verifies it. | Until then only test users log in with it, and they expect the warning. |
| 11 | ✅ | The guide | `docs/youtube/SETUP.md` (and its English copy) opens with the one-button path, then keeps today's Parts 1 and 2 under "Com um cliente Google próprio", for when the panel asks for an id and a secret or the shared quota runs out. | The same guide ships with every build, with or without the client. |
| 12 | ✅ | Words and contract | pt.json first, then en.json, and `docs/CONTRACT.md` in the same commit as the behaviour. `build.sh` and `CLAUDE.md` name the two variables. The ROADMAP marks the item built, pending verification. | Repo rules. |

## Done when

- A build with both variables connects with one button, with no id or secret typed. A build without them behaves as 0.1.9 does, and a 0.1.9 `youtube.json` keeps refreshing with the club's own client.
- `go vet ./...`, `go test ./...` and `GOOS=windows GOARCH=386 go build ./...` pass, along with the i18n test.
- Tests cover the defaults from an old file, the client a refresh uses, the 409 on a switch while connected, the 400 for `builtin` on a build without one, and `GET /api/youtube` never carrying the built-in secret.

## Out of scope

- Submitting the client for verification or the quota audit. Those are the owner's, in the Google Cloud console.
- Picking a client per game, or switching automatically when the quota runs out.
