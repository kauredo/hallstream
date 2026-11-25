import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import worker from "../src/index.ts";

const env = { GITHUB_TOKEN: "test-token", GITHUB_REPO: "owner/repo" };
const realFetch = globalThis.fetch;
let ghCalls: { url: string; body: any }[] = [];

beforeEach(() => {
  ghCalls = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    ghCalls.push({ url: String(url), body: JSON.parse(String(init.body)) });
    return Response.json({ html_url: "https://github.com/owner/repo/issues/1" }, { status: 201 });
  }) as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

function post(body: unknown) {
  return worker.fetch(
    new Request("https://worker.test/feedback", {
      method: "POST",
      body: typeof body === "string" ? body : JSON.stringify(body),
    }),
    env,
  );
}

const valid = { type: "bug", title: "Crash", description: "It crashed", appVersion: "1.0.0", os: "windows", scoring: "console", sport: "basketball" };

test("a valid payload creates an issue", async () => {
  const res = await post(valid);
  assert.equal(res.status, 201);
  assert.equal(ghCalls.length, 1);
  assert.equal(ghCalls[0].body.title, "[Bug] Crash");
  assert.deepEqual(ghCalls[0].body.labels, ["bug"]);
});

for (const [name, payload] of Object.entries({
  "JSON null": "null",
  "JSON array": "[]",
  "JSON string": '"hi"',
  "numeric title": { ...valid, title: 5 },
  "object title": { ...valid, title: {} },
  "numeric description": { ...valid, description: 5 },
  "title over 200 characters": { ...valid, title: "a".repeat(201) },
  "description over 10000 characters": { ...valid, description: "a".repeat(10001) },
  "numeric os": { ...valid, os: 5 },
  "object sport": { ...valid, sport: {} },
  "appVersion over 300 characters": { ...valid, appVersion: "1".repeat(301) },
})) {
  test(`rejects ${name} with 400 and CORS headers`, async () => {
    const res = await post(payload as unknown);
    assert.equal(res.status, 400);
    assert.equal(res.headers.get("Access-Control-Allow-Origin"), "*");
    assert.equal(ghCalls.length, 0);
  });
}

test("an unknown type falls back to feedback", async () => {
  const res = await post({ ...valid, type: "<script>" });
  assert.equal(res.status, 201);
  assert.equal(ghCalls[0].body.title, "[Feedback] Crash");
  assert.deepEqual(ghCalls[0].body.labels, ["feedback"]);
  assert.ok(!ghCalls[0].body.body.includes("<script>"));
});

test("a non-string type falls back to feedback", async () => {
  const res = await post({ ...valid, type: 5 });
  assert.equal(res.status, 201);
  assert.equal(ghCalls[0].body.title, "[Feedback] Crash");
});

test("fields at the limits are accepted", async () => {
  const res = await post({ ...valid, title: "a".repeat(200), description: "a".repeat(10000), os: "o".repeat(100) });
  assert.equal(res.status, 201);
});

test("answers 502 with CORS headers when GitHub is unreachable", async () => {
  globalThis.fetch = (async () => {
    throw new TypeError("network down");
  }) as typeof fetch;
  const res = await post(valid);
  assert.equal(res.status, 502);
  assert.equal(res.headers.get("Access-Control-Allow-Origin"), "*");
});

test("answers 502 with CORS headers when GitHub replies with non-JSON", async () => {
  globalThis.fetch = (async () => new Response("<html>gateway</html>", { status: 201 })) as typeof fetch;
  const res = await post(valid);
  assert.equal(res.status, 502);
  assert.equal(res.headers.get("Access-Control-Allow-Origin"), "*");
});

test("mentions and issue references in the text do not link", async () => {
  await post({ ...valid, title: "see @alice #5", description: "ping @bob and #123", os: "@carol" });
  const { title, body } = ghCalls[0].body;
  for (const text of [title, body]) {
    assert.ok(!/@(?!​)/.test(text), text);
    assert.ok(!/#(?!​)/.test(text), text);
  }
  assert.ok(body.includes("@​bob"));
});

test("newlines in the title and short fields cannot add lines", async () => {
  await post({ ...valid, title: "a\nb", os: "x\n**Type:** forged", sport: "p\r\nq", appVersion: "1\n2" });
  const { title, body } = ghCalls[0].body;
  assert.ok(!title.includes("\n"));
  const lines = body.split("\n");
  assert.equal(lines.filter((l: string) => l.startsWith("**Type:**")).length, 1);
  assert.equal(lines.filter((l: string) => l.startsWith("**OS:**")).length, 1);
  assert.equal(lines.filter((l: string) => l.startsWith("**Sport:**")).length, 1);
  assert.equal(lines.filter((l: string) => l.startsWith("**App Version:**")).length, 1);
});
