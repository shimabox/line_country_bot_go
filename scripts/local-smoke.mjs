// Local smoke test against `wrangler dev`.
//
// It starts a mock of the LINE reply and loading animation APIs, sends signed
// webhook requests to the local Worker and checks what the mock receives: for
// every reply, the loading API first and then the reply API, with the webhook
// answered after at least REPLY_DELAY_MS and within 2 seconds. Settings come
// from .dev.vars:
//
//   CHANNEL_SECRET=any-dummy-value
//   CHANNEL_ACCESS_TOKEN=any-dummy-value
//   LINE_API_BASE_URL=http://127.0.0.1:8788   (the mock listens on this port)
//   REPLY_DELAY_MS=1000                       (optional, same default as the Worker)
//
// Usage:
//   npx wrangler dev --ip 127.0.0.1 --port 8787   # in another terminal
//   node scripts/local-smoke.mjs [worker URL, default http://127.0.0.1:8787]
//
// Only Node.js built-ins are used. Dummy credentials only: never point this at
// the real LINE API.

import { createHmac } from "node:crypto";
import { readFileSync } from "node:fs";
import { createServer } from "node:http";

const workerURL = (process.argv[2] ?? "http://127.0.0.1:8787").replace(/\/$/, "");

function readDevVars(path) {
  const vars = {};
  for (const line of readFileSync(path, "utf8").split("\n")) {
    const m = line.match(/^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(.*?)\s*$/);
    if (m) vars[m[1]] = m[2].replace(/^"(.*)"$/, "$1");
  }
  return vars;
}

const vars = readDevVars(".dev.vars");
const secret = vars.CHANNEL_SECRET;
const mockURL = new URL(vars.LINE_API_BASE_URL ?? "");
if (!secret || !["127.0.0.1", "localhost"].includes(mockURL.hostname)) {
  console.error("set CHANNEL_SECRET and a local LINE_API_BASE_URL in .dev.vars");
  process.exit(2);
}
const webhookPath = vars.WEBHOOK_PATH || "/webhook";

// The wait the Worker uses, read the same way: empty or invalid means 1000,
// and values above 1200 are capped.
function replyDelayMs(value) {
  const s = (value ?? "").trim();
  if (!/^\+?\d+$/.test(s)) return 1000;
  return Math.min(Number(s), 1200);
}
const delayMs = replyDelayMs(vars.REPLY_DELAY_MS);
const webhookTimeoutMs = 2000;

const replyPath = "/v2/bot/message/reply";
const loadingPath = "/v2/bot/chat/loading/start";
const userId = "U00000000000000000000000000000000";

// Mock reply and loading APIs: records every request to them and answers 200
// (202 for the loading API, as the real one does). Requests to other paths
// (e.g. a port scan by another local process) are answered 404 and only
// counted, so they do not disturb the checks.
const received = [];
let otherRequests = 0;
const mock = createServer((req, res) => {
  let body = "";
  req.on("data", (c) => (body += c));
  req.on("end", () => {
    if (req.url !== replyPath && req.url !== loadingPath) {
      otherRequests++;
      res.writeHead(404);
      res.end();
      return;
    }
    let json = null;
    try {
      json = JSON.parse(body);
    } catch {}
    received.push({ method: req.method, path: req.url, auth: req.headers.authorization ?? "", json, at: performance.now() });
    res.writeHead(req.url === loadingPath ? 202 : 200, { "content-type": "application/json" });
    res.end("{}");
  });
});
await new Promise((resolve) => mock.listen(Number(mockURL.port), mockURL.hostname, resolve));

function sign(body) {
  return createHmac("sha256", secret).update(body).digest("base64");
}

function event(type, extra) {
  return {
    type,
    mode: "active",
    timestamp: Date.now(),
    source: { type: "user", userId },
    webhookEventId: "01LOCALSMOKE0000000000000",
    deliveryContext: { isRedelivery: false },
    replyToken: `local-reply-token-${type}`,
    ...extra,
  };
}

async function postWebhook(events, { badSignature = false } = {}) {
  const body = JSON.stringify({ destination: "U00000000000000000000000000000000", events });
  const before = received.length;
  const start = performance.now();
  const res = await fetch(workerURL + webhookPath, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "x-line-signature": badSignature ? sign(body + "tampered") : sign(body),
    },
    body,
  });
  const text = await res.text();
  const elapsedMs = performance.now() - start;
  const all = received.slice(before);
  return {
    status: res.status,
    text,
    elapsedMs,
    received: all,
    replies: all.filter((r) => r.path === replyPath),
  };
}

// For a webhook with one reply: the mock got the loading API and then the
// reply API (only the reply API when REPLY_DELAY_MS is 0), and the webhook was
// answered after the wait and within LINE's 2-second limit.
function rhythmOK(r) {
  const paths = r.received.map((x) => x.path);
  const inTime = r.elapsedMs < webhookTimeoutMs;
  if (delayMs === 0) return inTime && paths.length === 1 && paths[0] === replyPath;
  const loading = r.received[0]?.json;
  return inTime && r.elapsedMs >= delayMs && paths.length === 2 && paths[0] === loadingPath && paths[1] === replyPath &&
    loading?.chatId === userId && loading?.loadingSeconds === 5 && Object.keys(loading).length === 2;
}

function summarize(messages) {
  return messages.map((m) => {
    switch (m.type) {
      case "text":
        return { type: "text", text: m.text };
      case "sticker":
        return { type: "sticker", packageId: m.packageId, stickerId: m.stickerId };
      case "template": {
        const t = m.template;
        return {
          type: `template/${t.type}`,
          altText: m.altText,
          title: t.title,
          text: t.text,
          thumbnailImageUrl: t.thumbnailImageUrl,
          imageSize: t.imageSize,
          actions: t.actions.map(
            (a) => `${a.type}:${a.label}→${a.uri ?? a.data ?? a.text}${a.displayText ? ` (displayText: ${a.displayText})` : ""}`,
          ),
        };
      }
      default:
        return { type: m.type };
    }
  });
}

const results = [];
let failed = false;
function record(name, r, ok) {
  const summary = r.received.map((x) => ({
    path: x.path,
    auth: x.auth.replace(/^Bearer .+$/, "Bearer <dummy>"),
    ...(x.path === loadingPath
      ? { body: x.json }
      : { replyToken: x.json?.replyToken, messages: summarize(x.json?.messages ?? []) }),
  }));
  const timing = {};
  if (r.elapsedMs !== undefined) timing.elapsedMs = Math.round(r.elapsedMs);
  const loading = r.received.find((x) => x.path === loadingPath);
  const reply = r.received.find((x) => x.path === replyPath);
  if (loading && reply) timing.loadingToReplyMs = Math.round(reply.at - loading.at);
  results.push({ scenario: name, status: r.status, ok, ...timing, received: summary });
  if (!ok) failed = true;
}

const messagesOf = (replies) => replies[0]?.json?.messages ?? [];

// 1. Health check
{
  const res = await fetch(workerURL + "/");
  record("1 GET /", { status: res.status, received: [] }, res.status === 200);
}

// 2. Country found
{
  const r = await postWebhook([event("message", { message: { id: "1", type: "text", text: "日本" } })]);
  const m = messagesOf(r.replies);
  record("2 text hit (日本)", r,
    r.status === 200 && rhythmOK(r) && r.replies.length === 1 && r.replies[0].path === replyPath && m.length === 3 &&
      m[0].text === "[日本] ですね🤔" && m[2].template?.thumbnailImageUrl?.endsWith("/flags/jp.png") &&
      m[2].template?.imageSize === "cover");
}

// 3. Country not found
{
  const r = await postWebhook([event("message", { message: { id: "2", type: "text", text: "アトランティス" } })]);
  const m = messagesOf(r.replies);
  record("3 text miss (アトランティス)", r,
    r.status === 200 && rhythmOK(r) && m.length === 2 && m[0].text === "[アトランティス] ですね🤔" &&
      m[1].text?.startsWith("む、むむっ、、わかりませんでした"));
}

// 4. Quiz
let correctData = null;
{
  const r = await postWebhook([event("message", { message: { id: "3", type: "text", text: "クイズ" } })]);
  const m = messagesOf(r.replies);
  const actions = m[0]?.template?.actions ?? [];
  correctData = actions.find((a) => a.data?.endsWith("result=1"))?.data ?? null;
  record("4 quiz (クイズ)", r,
    r.status === 200 && rhythmOK(r) && m.length === 1 && actions.length === 4 && actions.every((a) => a.type === "postback") &&
      actions.filter((a) => a.data.endsWith("result=1")).length === 1 &&
      actions.every((a) => a.displayText && a.displayText === new URLSearchParams(a.data).get("answer")));
}

// 5. Correct quiz answer
{
  const r = await postWebhook([event("postback", { postback: { data: correctData ?? "" } })]);
  const m = messagesOf(r.replies);
  const answer = new URLSearchParams(correctData ?? "").get("answer");
  record("5 postback correct", r,
    r.status === 200 && rhythmOK(r) && m.length === 4 && answer && m[0].text === `[${answer}] ですね?` &&
      m[1].text?.startsWith("む、むむっ、、\n正解です") && m[3].template?.type === "confirm");
}

// 6. Non-text message
{
  const r = await postWebhook([event("message", { message: { id: "4", type: "image" } })]);
  const m = messagesOf(r.replies);
  record("6 non-text (image)", r,
    r.status === 200 && rhythmOK(r) && m.length === 2 && m[1].type === "sticker" && m[1].packageId === "446" &&
      m[1].stickerId === "1999");
}

// 7. Invalid signature: neither the loading API nor the reply API is called.
{
  const r = await postWebhook([event("message", { message: { id: "5", type: "text", text: "日本" } })], { badSignature: true });
  record("7 invalid signature", r, r.status === 400 && r.text.trim() === "Invalid signature." && r.received.length === 0);
}

// 8. Country found whose flag is shown whole (2:1)
{
  const r = await postWebhook([event("message", { message: { id: "6", type: "text", text: "イギリス" } })]);
  const m = messagesOf(r.replies);
  record("8 text hit (イギリス)", r,
    r.status === 200 && rhythmOK(r) && m.length === 3 && m[2].template?.thumbnailImageUrl?.endsWith("/flags/gb.png") &&
      m[2].template?.imageSize === "contain");
}

// Static asset
{
  const res = await fetch(workerURL + "/flags/jp.png");
  const buf = Buffer.from(await res.arrayBuffer());
  const isPNG = buf.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]));
  const contentType = res.headers.get("content-type");
  results.push({ scenario: "GET /flags/jp.png", status: res.status, contentType, bytes: buf.length, ok: res.status === 200 && contentType === "image/png" && isPNG });
  if (!results.at(-1).ok) failed = true;
}

mock.close();
console.log(JSON.stringify({ replyDelayMs: delayMs, webhookTimeoutMs, otherRequests }));
for (const r of results) console.log(JSON.stringify(r, null, 2));
console.log(failed ? "SMOKE TEST FAILED" : "SMOKE TEST PASSED");
process.exit(failed ? 1 : 0);
