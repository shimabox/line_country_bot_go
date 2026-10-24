// CPU time benchmark for the Worker's wasm on Node.js.
//
// workers-go starts a new Go wasm instance for every request, so the time
// spent in main is paid on every request. This script runs build/app.wasm the
// same way the generated build/worker.mjs does and reports, per scenario, the
// median CPU time (user + system, process.cpuUsage) of:
//
//   start   new Go() + WebAssembly.Instance + go.run until workers.ready
//   handle  binding.handleRequest until its Response is read
//   total   start + handle of the same run
//
// The module is compiled once, as the Workers runtime does. The scenarios take
// turns; the first 2 runs of each warm up the JIT and are left out of the
// median. Node's numbers are smaller
// than the ones measured in production (workerd), so compare builds with each
// other on the same machine instead of reading the absolute values.
//
// Usage:
//   npm run build
//   node scripts/bench-cpu.mjs <path/to/app.wasm> [runs, default 12] [--verbose]
//   npm run bench:cpu -- build/app.wasm
//
// --verbose also prints every sample (start/handle) to stderr.
//
// Reply API calls go to a local mock server. Only dummy credentials are used.

import { createHmac } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { createServer } from "node:http";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { setFlagsFromString } from "node:v8";
import { runInNewContext } from "node:vm";

const warmup = 2;

const args = process.argv.slice(2);
const verbose = args.includes("--verbose");
const [wasmPath, runsArg] = args.filter((a) => a !== "--verbose");
if (!wasmPath) {
  console.error("usage: node scripts/bench-cpu.mjs <path/to/app.wasm> [runs] [--verbose]");
  process.exit(2);
}
const runs = Number(runsArg ?? 12);
if (!Number.isInteger(runs) || runs <= warmup) {
  console.error(`runs must be an integer greater than ${warmup}`);
  process.exit(2);
}

// wasm_exec.js: the one workers-assets-gen writes to build/, or the one that
// ships with the Go toolchain.
function findWasmExec() {
  const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const candidates = [join(repoRoot, "build", "wasm_exec.js")];
  try {
    const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
    if (goroot) candidates.push(join(goroot, "lib", "wasm", "wasm_exec.js"));
  } catch {}
  const found = candidates.find((p) => existsSync(p));
  if (!found) {
    console.error(
      "wasm_exec.js not found. Run `npm run build` (it writes build/wasm_exec.js)\n" +
        "or install Go so that $(go env GOROOT)/lib/wasm/wasm_exec.js exists. Looked at:\n  " +
        candidates.join("\n  "),
    );
    process.exit(2);
  }
  return found;
}
createRequire(import.meta.url)(findWasmExec());

// Same helper build/worker.mjs defines for the workers-go JS shim.
globalThis.tryCatch = (fn) => {
  try {
    return { result: fn() };
  } catch (e) {
    return { error: e };
  }
};

// Node's fetch needs duplex: "half" to build a Request with a body.
const NodeRequest = globalThis.Request;
globalThis.Request = class extends NodeRequest {
  constructor(input, init) {
    super(input, init && init.body != null ? { duplex: "half", ...init } : init);
  }
};

// Mock reply API.
const mock = createServer((req, res) => {
  req.resume();
  req.on("end", () => {
    res.writeHead(200, { "content-type": "application/json" });
    res.end("{}");
  });
});
await new Promise((r) => mock.listen(0, "127.0.0.1", r));
const mockURL = `http://127.0.0.1:${mock.address().port}`;

const secret = "bench-dummy-secret";
const env = {
  CHANNEL_SECRET: secret,
  CHANNEL_ACCESS_TOKEN: "bench-dummy-token",
  WEBHOOK_PATH: "/webhook",
  PUBLIC_BASE_URL: "https://bot.example.com",
  LINE_API_BASE_URL: mockURL,
};
const ctx = { waitUntil() {}, passThroughOnException() {} };

function webhookRequest(text) {
  const body = JSON.stringify({
    destination: "U00000000000000000000000000000000",
    events: [
      {
        type: "message",
        mode: "active",
        timestamp: Date.now(),
        source: { type: "user", userId: "U00000000000000000000000000000000" },
        webhookEventId: "01BENCH00000000000000000000",
        deliveryContext: { isRedelivery: false },
        replyToken: "bench-reply-token",
        message: { id: "1", type: "text", text },
      },
    ],
  });
  const signature = createHmac("sha256", secret).update(body).digest("base64");
  return () =>
    new Request("https://bot.example.com/webhook", {
      method: "POST",
      headers: { "content-type": "application/json", "x-line-signature": signature },
      body,
    });
}

const scenarios = [
  { name: "GET /", request: () => new Request("https://bot.example.com/"), status: 200 },
  { name: "webhook 日本", request: webhookRequest("日本"), status: 200 },
  { name: "webhook クイズ", request: webhookRequest("クイズ"), status: 200 },
];

const mod = await WebAssembly.compile(readFileSync(wasmPath));

function cpuMs(from) {
  const d = process.cpuUsage(from);
  return (d.user + d.system) / 1000;
}

async function runOnce(scenario) {
  const req = scenario.request();
  const binding = {};
  const t0 = process.cpuUsage();
  const go = new Go();
  let ready;
  const readyPromise = new Promise((r) => (ready = r));
  const instance = new WebAssembly.Instance(mod, {
    ...go.importObject,
    workers: { ready: () => ready() },
  });
  go.run(instance, { env, ctx, binding, connect() {} });
  await readyPromise;
  const start = cpuMs(t0);

  const t1 = process.cpuUsage();
  const res = await binding.handleRequest(req);
  await res.arrayBuffer();
  const handle = cpuMs(t1);
  if (res.status !== scenario.status) {
    throw new Error(`${scenario.name}: status ${res.status}, want ${scenario.status}`);
  }
  return { start, handle };
}

function median(xs) {
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

// The Go program's log lines go to stderr so stdout has only the results.
console.log = console.error;

// Every run leaves a Go instance and its memory behind. Collecting them
// between runs keeps the garbage collector out of the measured spans.
setFlagsFromString("--expose-gc");
const gc = runInNewContext("gc");

// Scenarios take turns so that each sees the same JIT state.
const samples = scenarios.map(() => []);
for (let i = 0; i < runs; i++) {
  for (const [j, scenario] of scenarios.entries()) {
    gc();
    samples[j].push(await runOnce(scenario));
  }
}
mock.close();

const results = scenarios.map((scenario, j) => {
  const kept = samples[j].slice(warmup);
  if (verbose) {
    process.stderr.write(`${scenario.name}: ${samples[j].map((s) => `${s.start.toFixed(2)}/${s.handle.toFixed(2)}`).join(" ")}\n`);
  }
  return {
    scenario: scenario.name,
    start: median(kept.map((s) => s.start)),
    handle: median(kept.map((s) => s.handle)),
    total: median(kept.map((s) => s.start + s.handle)),
  };
});

const out = (s) => process.stdout.write(s + "\n");
const fmt = (v) => v.toFixed(2);
out(`wasm: ${wasmPath}`);
out(`runs: ${runs} (median of the last ${runs - warmup}), CPU ms (user+system)`);
out("scenario\tstart\thandle\ttotal");
for (const r of results) out(`${r.scenario}\t${fmt(r.start)}\t${fmt(r.handle)}\t${fmt(r.total)}`);
out(JSON.stringify({ wasm: wasmPath, runs, results }));
process.exit(0);
