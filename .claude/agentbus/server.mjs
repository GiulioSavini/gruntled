#!/usr/bin/env node
// Local GUI for the agent bus.  node .claude/agentbus/server.mjs  →  http://127.0.0.1:4777
// Zero dependencies. Binds to loopback only and rejects foreign Host/Origin headers
// (DNS-rebinding / CSRF), since the page can also post messages to the team.
import { createServer } from "node:http";
import { existsSync, readFileSync, statSync, watchFile } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { BUS, post, readAll } from "./bus.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..", "..");
const PORT = Number(process.env.AGENTBUS_PORT || 4777);
const HOST = "127.0.0.1";
const allowedHosts = new Set([`127.0.0.1:${PORT}`, `localhost:${PORT}`]);

function gsdState() {
  const f = join(repo, ".planning", "STATE.md");
  if (!existsSync(f)) return {};
  const src = readFileSync(f, "utf8");
  const fm = src.match(/^---\n([\s\S]*?)\n---/)?.[1] || "";
  const get = (k) => fm.match(new RegExp(`^\\s*${k}:\\s*"?(.*?)"?$`, "m"))?.[1];
  let commits = [];
  try {
    commits = execFileSync("git", ["-C", repo, "log", "--oneline", "-8"], { encoding: "utf8" }).trim().split("\n");
  } catch { /* not a git checkout */ }
  return {
    milestone: get("milestone"), milestone_name: get("milestone_name"), status: get("status"),
    stopped_at: get("stopped_at"), last_activity: get("last_activity"),
    completed_phases: get("completed_phases"), total_phases: get("total_phases"),
    completed_plans: get("completed_plans"), total_plans: get("total_plans"), commits,
  };
}

const clients = new Set();
let lastSize = existsSync(BUS) ? statSync(BUS).size : 0;
let lastId = readAll().reduce((m, x) => Math.max(m, x.id || 0), 0);
watchFile(BUS, { interval: 400 }, (cur) => {
  if (cur.size === lastSize) return;
  lastSize = cur.size;
  const fresh = readAll().filter((m) => m.id > lastId);
  if (!fresh.length) return;
  lastId = fresh[fresh.length - 1].id;
  for (const res of clients) for (const m of fresh) res.write(`event: msg\ndata: ${JSON.stringify(m)}\n\n`);
});
setInterval(() => {
  const s = JSON.stringify(gsdState());
  for (const res of clients) res.write(`event: state\ndata: ${s}\n\n`);
}, 5000);

createServer((req, res) => {
  if (!allowedHosts.has(req.headers.host || "")) { res.writeHead(403).end("forbidden host"); return; }
  const url = new URL(req.url, `http://${req.headers.host}`);

  if (req.method === "GET" && url.pathname === "/") {
    res.writeHead(200, { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" });
    res.end(readFileSync(join(here, "index.html")));
  } else if (req.method === "GET" && url.pathname === "/api/messages") {
    res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(readAll()));
  } else if (req.method === "GET" && url.pathname === "/api/state") {
    res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(gsdState()));
  } else if (req.method === "GET" && url.pathname === "/events") {
    res.writeHead(200, { "content-type": "text/event-stream", "cache-control": "no-cache", connection: "keep-alive" });
    res.write(": hello\n\n");
    clients.add(res);
    const ping = setInterval(() => res.write(": ping\n\n"), 15000);
    req.on("close", () => { clearInterval(ping); clients.delete(res); });
  } else if (req.method === "POST" && url.pathname === "/api/post") {
    const origin = req.headers.origin;
    if (origin && !allowedHosts.has(origin.replace(/^https?:\/\//, ""))) { res.writeHead(403).end("forbidden origin"); return; }
    let body = "";
    req.on("data", (c) => { body += c; if (body.length > 8192) req.destroy(); });
    req.on("end", () => {
      try {
        const { to = "orchestrator", text } = JSON.parse(body);
        if (!text || typeof text !== "string") throw new Error("text required");
        const m = post({ from: "giulio", to: String(to).slice(0, 32), type: "task", text: text.slice(0, 4000) });
        res.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(m));
      } catch (e) {
        res.writeHead(400).end(String(e.message));
      }
    });
  } else {
    res.writeHead(404).end("not found");
  }
}).listen(PORT, HOST, () => console.log(`agentbus GUI → http://${HOST}:${PORT}  (bus: ${BUS})`));
