#!/usr/bin/env node
// Agent message bus for the gruntled team: one append-only JSONL file.
//
//   node .claude/agentbus/bus.mjs post  --from planner --to executor --type handoff [--phase 12] [--ref path] "text"
//   node .claude/agentbus/bus.mjs inbox --for executor [--since <id>]   # messages to <agent> or "all"
//   node .claude/agentbus/bus.mjs tail  [-n 30]
//   node .claude/agentbus/bus.mjs hook                                   # SubagentStart/Stop hook (JSON on stdin)
//
// Types: task | plan | handoff | progress | question | answer | finding | verdict | decision | status | blocker
import { appendFileSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
export const BUS = process.env.AGENTBUS_FILE || join(here, "log", "bus.jsonl");
export const AGENTS = ["orchestrator", "planner", "executor", "sec"];
const TYPES = new Set(["task", "plan", "handoff", "progress", "question", "answer", "finding", "verdict", "decision", "status", "blocker", "lifecycle"]);

export function readAll(file = BUS) {
  if (!existsSync(file)) return [];
  return readFileSync(file, "utf8").split("\n").filter(Boolean).flatMap((l) => {
    try { return [JSON.parse(l)]; } catch { return []; }
  });
}

function nextId(all) {
  return all.length ? Math.max(...all.map((m) => m.id || 0)) + 1 : 1;
}

export function post(msg) {
  mkdirSync(dirname(BUS), { recursive: true });
  const full = { id: nextId(readAll()), ts: new Date().toISOString(), ...msg };
  appendFileSync(BUS, JSON.stringify(full) + "\n");
  return full;
}

// Map Claude Code agent_type ("gruntled-planner") to a team name ("planner").
function teamName(agentType = "") {
  const t = agentType.replace(/^gruntled-/, "");
  return AGENTS.includes(t) ? t : agentType || "unknown";
}

function parseArgs(argv) {
  const opts = {}, rest = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) opts[a.slice(2)] = argv[++i];
    else if (a === "-n") opts.n = argv[++i];
    else rest.push(a);
  }
  return { opts, text: rest.join(" ") };
}

function fmt(m) {
  const t = m.ts.slice(11, 19);
  const ph = m.phase ? ` [ph ${m.phase}]` : "";
  return `#${m.id} ${t} ${m.from} → ${m.to} (${m.type})${ph}: ${m.text}${m.ref ? `  ↳ ${m.ref}` : ""}`;
}

async function main() {
  const [cmd, ...argv] = process.argv.slice(2);
  const { opts, text } = parseArgs(argv);

  if (cmd === "post") {
    const from = opts.from, to = opts.to || "all", type = opts.type || "progress";
    if (!from || !text) { console.error("usage: post --from <agent> --to <agent|all> --type <type> \"text\""); process.exit(2); }
    if (!TYPES.has(type)) { console.error(`unknown type ${type}; one of ${[...TYPES].join(", ")}`); process.exit(2); }
    const m = post({ from, to, type, text, ...(opts.phase && { phase: opts.phase }), ...(opts.ref && { ref: opts.ref }) });
    console.log(fmt(m));
  } else if (cmd === "inbox") {
    const who = opts.for;
    if (!who) { console.error("usage: inbox --for <agent> [--since <id>]"); process.exit(2); }
    const since = Number(opts.since || 0);
    const msgs = readAll().filter((m) => m.id > since && m.type !== "lifecycle" && (m.to === who || m.to === "all") && m.from !== who);
    console.log(msgs.length ? msgs.map(fmt).join("\n") : `(no messages for ${who})`);
  } else if (cmd === "tail") {
    console.log(readAll().slice(-Number(opts.n || 30)).map(fmt).join("\n"));
  } else if (cmd === "hook") {
    let raw = "";
    for await (const c of process.stdin) raw += c;
    let ev = {};
    try { ev = JSON.parse(raw); } catch { /* ignore malformed hook input */ }
    const name = teamName(ev.agent_type);
    if (!AGENTS.includes(name)) return; // only track the team, not every helper subagent
    const started = ev.hook_event_name === "SubagentStart";
    post({
      from: name, to: "orchestrator", type: "lifecycle",
      state: started ? "working" : "done", agent_id: ev.agent_id,
      text: started ? `${name} started` : `${name} finished`,
    });
  } else {
    console.error("commands: post | inbox | tail | hook");
    process.exit(2);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
