# hallstream-feedback

A Cloudflare Worker that turns feedback sent from Hallstream's Ajuda tab into a GitHub issue on `kauredo/hallstream`, so a volunteer needs no GitHub account. Adapted from the `bva-feedback` worker.

```
Ajuda form in the panel
  → POST /api/feedback (the Hallstream app, which adds the version, system, scoring and sport)
    → POST /feedback (this worker)
      → GitHub Issues API: a labelled issue
```

## API

`POST /feedback` with a JSON object:

```json
{
  "type": "bug" | "feature" | "feedback",
  "title": "The clock froze in the second quarter",
  "description": "What happened, and what you expected",
  "appVersion": "0.1.0",
  "os": "windows",
  "scoring": "console",
  "sport": "basketball"
}
```

It answers `201 { "success": true, "issueUrl": "…" }`, or `400` for a body it refuses (missing title or description, a title over 200 characters, a description over 10,000, a short field over 300) and `502` when GitHub refuses. Titles get a `[Bug]`, `[Feature Request]` or `[Feedback]` prefix, and the labels `bug`, `enhancement` or `feedback`. Every `@` and `#` gets a zero-width space after it, so a report can't mention people or link issues.

## Deploy

1. A fine-grained GitHub token with **Issues: Read and write** on `kauredo/hallstream` only.
2. From this folder:

   ```bash
   npm install
   npx wrangler login
   npx wrangler secret put GITHUB_TOKEN   # paste the token
   npx wrangler deploy
   ```

   Wrangler prints the worker's URL, such as `https://hallstream-feedback.<account>.workers.dev`.
3. Give the app the URL: set the repository variable `HALLSTREAM_FEEDBACK_URL` to `<that URL>/feedback` (`gh variable set HALLSTREAM_FEEDBACK_URL`). The release workflow builds it into the exe. A build without it shows only the "Write on GitHub" button.

## Test

`npm test` runs `node --test`, with GitHub's API faked.
