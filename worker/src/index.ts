// Receives feedback from the Hallstream panel (through the app's own
// POST /api/feedback) and files it as a GitHub issue, so a volunteer needs no
// GitHub account. Adapted from the bva-feedback worker.
interface Env {
  GITHUB_TOKEN: string;
  GITHUB_REPO: string;
}

const LABEL_MAP: Record<string, string> = {
  bug: "bug",
  feature: "enhancement",
  feedback: "feedback",
};

const TYPE_PREFIX: Record<string, string> = {
  bug: "[Bug]",
  feature: "[Feature Request]",
  feedback: "[Feedback]",
};

const MAX_TITLE = 200;
const MAX_DESCRIPTION = 10000;
const MAX_SHORT_FIELD = 300; // a browser user agent runs past 100

function oneLine(text: string): string {
  return text.replace(/[\r\n]+/g, " ");
}

// A zero-width space after @ and # stops GitHub from notifying users or linking issues.
function defuse(text: string): string {
  return text.replace(/[@#]/g, "$&\u200b");
}

function corsHeaders(): HeadersInit {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
  };
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    if (request.method !== "POST") {
      return Response.json(
        { error: "Method not allowed" },
        { status: 405, headers: corsHeaders() },
      );
    }

    const url = new URL(request.url);
    if (url.pathname !== "/feedback") {
      return Response.json(
        { error: "Not found" },
        { status: 404, headers: corsHeaders() },
      );
    }

    let raw: unknown;
    try {
      raw = await request.json();
    } catch {
      return Response.json(
        { error: "Invalid JSON" },
        { status: 400, headers: corsHeaders() },
      );
    }

    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
      return Response.json(
        { error: "Body must be a JSON object" },
        { status: 400, headers: corsHeaders() },
      );
    }
    const input = raw as Record<string, unknown>;

    const title =
      typeof input.title === "string" ? oneLine(input.title.trim()) : "";
    const description =
      typeof input.description === "string" ? input.description.trim() : "";
    if (!title || !description) {
      return Response.json(
        { error: "Title and description are required" },
        { status: 400, headers: corsHeaders() },
      );
    }
    if (title.length > MAX_TITLE || description.length > MAX_DESCRIPTION) {
      return Response.json(
        { error: "Title or description is too long" },
        { status: 400, headers: corsHeaders() },
      );
    }

    const shortFields: Record<string, string> = {};
    for (const key of ["appVersion", "os", "scoring", "sport"]) {
      const value = input[key];
      if (value === undefined || value === null) {
        shortFields[key] = "";
      } else if (typeof value === "string" && value.length <= MAX_SHORT_FIELD) {
        shortFields[key] = oneLine(value);
      } else {
        return Response.json(
          { error: `Invalid ${key}` },
          { status: 400, headers: corsHeaders() },
        );
      }
    }

    const type =
      typeof input.type === "string" && Object.prototype.hasOwnProperty.call(LABEL_MAP, input.type)
        ? input.type
        : "feedback";

    const issueTitle = `${TYPE_PREFIX[type]} ${defuse(title)}`;
    const issueBody = [
      defuse(description),
      "",
      "---",
      `**Type:** ${type}`,
      `**App Version:** ${defuse(shortFields.appVersion) || "unknown"}`,
      `**OS:** ${defuse(shortFields.os) || "unknown"}`,
      `**Scoring:** ${defuse(shortFields.scoring) || "unknown"}`,
      `**Sport:** ${defuse(shortFields.sport) || "unknown"}`,
      "",
      "_Submitted via in-app feedback_",
    ].join("\n");

    const labels = [LABEL_MAP[type]];

    let issue: { html_url: string };
    try {
      const ghResponse = await fetch(
        `https://api.github.com/repos/${env.GITHUB_REPO}/issues`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${env.GITHUB_TOKEN}`,
            Accept: "application/vnd.github+json",
            "Content-Type": "application/json",
            "User-Agent": "hallstream-feedback-worker",
          },
          body: JSON.stringify({ title: issueTitle, body: issueBody, labels }),
        },
      );

      if (!ghResponse.ok) {
        const err = await ghResponse.text();
        console.error("GitHub API error:", err);
        return Response.json(
          { error: "Failed to create issue" },
          { status: 502, headers: corsHeaders() },
        );
      }

      issue = await ghResponse.json();
    } catch (error) {
      console.error("GitHub request failed:", error);
      return Response.json(
        { error: "Failed to create issue" },
        { status: 502, headers: corsHeaders() },
      );
    }

    return Response.json(
      { success: true, issueUrl: issue.html_url },
      { status: 201, headers: corsHeaders() },
    );
  },
};
