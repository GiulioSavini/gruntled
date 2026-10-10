# Milestones

## v0.3 — Watch & Blast (shipped 2026-10-10)

**Phases:** 8-12 (24 plans, phase 12 = audit gap closure)
**Archive:** `milestones/v0.3-ROADMAP.md`, `milestones/v0.3-REQUIREMENTS.md`, `milestones/v0.3-MILESTONE-AUDIT.md`

Turned the one-shot `check` into live feedback, and added a blast radius for a change.

**Key accomplishments:**
- Incremental index: a persistent parse store with `Invalidate`, proven equal to a full rescan by a rapid
  stateful property test over an in-memory filesystem, including editor-pattern directory names and
  symlinked includes (both found missing by the milestone audit and closed in phase 12)
- `gruntled blast --base <dir>`: disjoint, sorted Broken and Impacted sets (one-hop `variable`/`output`
  surface changes), text and json, multiset baseline so duplicate findings are not hidden
- `gruntled watch`: fsnotify (or `--poll`, and windows) with ~150 ms debounce, single indexer goroutine,
  30 s safety net, atomic one-line status file at a per-repository path outside the repository
- `gruntled report` over a raw-syscall AF_UNIX socket (report file on windows), byte-identical to `check`;
  crash-safe single instance via lock + stale socket recovery
- No-net/no-exec binary proof still holds on all six targets with watcher and socket linked; every text
  and JSON output escapes terminal control and bidi runes from repository names
- Process: first milestone run by the agent team (orchestrator, planner, executor, sec) on GSD Core; the
  cross-phase audit found a BLOCKER and a MEDIUM that every per-phase verification had passed

**Known limitations (accepted, documented in `docs/cli.md`):** darwin native watcher delays new files after
a dangling symlink by up to 30 s (fsnotify#787); 64 MiB report response cap; Windows runtime dir relies on
`%LocalAppData%` ACLs.

---

## v0.2 — CI-Ready (shipped 2026-10-08)

**Phases:** 5-7 (17 plans)
**Archive:** `milestones/v0.2-ROADMAP.md`, `milestones/v0.2-REQUIREMENTS.md`, `milestones/v0.2-MILESTONE-AUDIT.md`

Made the validated `check` engine installable and usable in pre-commit and CI.

**Key accomplishments:**
- `GRT002` (dependency to no unit) and `GRT003` (dependency cycle) as pure graph analyzers, validated on
  the pinned corpus: zero findings on iso20022 and secret, exactly 4 hand-checked `GRT002` on denis256,
  13/13 injected mutations caught, confirmed against terragrunt v1.1.6 and an independent oracle
- `gruntled graph --json`: one deterministic document with schema version, units, modules, edges and
  unresolved dependencies, all paths repository-relative
- `gruntled check --format sarif`: SARIF 2.1.0 with one rule per `GRT` code, validated in CI against the
  vendored OASIS schema and accepted by GitHub code scanning
- `gruntled-check` pre-commit hook, `docs/ci.md` recipes (release download, pre-commit, GitHub Actions
  plain and SARIF, GitLab CI), proven by a `recipe-check` CI job
- Release `v0.2.0`: static binaries for 6 targets (linux/darwin/windows x amd64/arm64) with
  `checksums.txt`, `gruntled --version` injected at build time, tag-triggered `release.yml` green on
  first real run

**Tech debt:** bookkeeping and Nyquist gaps listed in `milestones/v0.2-MILESTONE-AUDIT.md`
(no requirement gaps; README v0.1 text fixed after the audit).

---

## v0.1 — Falsifiable experiment (shipped 2026-09-29)

**Phases:** 1-4 (26 plans)
**Archive:** `milestones/v0.1-ROADMAP.md`, `milestones/v0.1-REQUIREMENTS.md`

Delivered `gruntled check` with `GRT001` and `GRT100` on an in-house structural HCL decoder.
On the real corpus: 0 false positives, every injected mutation caught (denis256 8/8 after
313f856), faster than `terragrunt hcl validate` in 3 of 3 runs.
