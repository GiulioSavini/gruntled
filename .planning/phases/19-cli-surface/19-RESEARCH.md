# Phase 19 research: CLI Surface (UX-01, UX-02, UX-03, UX-06)

Short, code-grounded; the milestone research (`.planning/research/{SUMMARY,ARCHITECTURE,STACK,PITFALLS}.md`)
already settled the design. This note records what the code looks like today and the decisions the plans pin.

## 1. What is there today

1.1 **Dispatch** (`cmd/gruntled/main.go`): `runCtx` is a `switch args[0]`; `-h/-help/--help/help` print
`topUsage` to **stderr** and exit 0; per-command `-h` goes through `parseArgs` whose `fs.Usage` writes to
**stderr**. `topUsage`, `checkUsage`, `graphUsage`, `blastUsage` (main.go), `watchUsage` (watch.go:26),
`reportUsage` (report.go:20), `rebaseUsage` (rebase.go:19) are string consts. Flags are defined inline in each
`run*` closure except `defineWatchFlags(fs) *watchOpts`. `report` defines a hidden `--depth` (usage text
"not supported by report") only to print a better error. `--format` validation is a hand-written `switch`
with a hand-written `(want text, json or sarif)` / `(want text or json)` message.

1.2 **Tests that pin help on stderr** (must move to stdout in 19-01): `e2e_test.go` TestHelpMatchesDocs
(reads `stderr` of `<cmd> -h`), `report_test.go` TestReportUsage (`report -h` and `help`),
`rules_doc_test.go` TestRuleRegistryDoc (`blast -h` fenced block), `testdata/script/usage.txtar` and
`report_usage.txtar` (`stderr '...'` after `-h`). `main_test.go` only checks exit codes.
`TestTopUsageMatchesDocs` references the identifier `topUsage` and the fenced block in `docs/cli.md`: it
stays textually unchanged if `topUsage` becomes a `var` initialised from the registry with identical bytes.

1.3 **Text/escaping**: `presenter.Text` and `BlastText` pass every repository-derived string through
`escapeTerm` (ESC = U+001B is `< 0x20`, so it becomes `\x1b`; `escape.go:21-36`). Therefore gruntled's own
text output never contains a raw ESC, and `stripSGR(Colorize(x)) == x` is an exact property. The check
summary (`presenter.Summary`) is a separate byte slice written to stderr by `runCheck` and `runReport`
(report.go:159). Blast prints nothing on stderr. Watch's informational stderr lines: banner
(`watch.go:165 "gruntled: watching ..."`) and the WSL hint (`:211`); `:215/:217/:223/:293/:316` are
degradations or errors and stay.

1.4 **TTY**: no detection anywhere. `golang.org/x/sys v0.47.0` is `// indirect` (fsnotify); importing
`x/sys/unix` directly makes it direct, `go mod tidy -diff` in CI requires the `go.mod` edit. The
architecture script forbids platform files in domain/application/interfaces (Step 3) and `infrastructure`
already has `//go:build` files (ipc, watch, statusfile), so terminal detection lives in
`internal/infrastructure/tty`. `cmd` may import infrastructure (self-test `infra-from-cmd-allowed`).
Step 9 deny-list is `^(net|net/.+|os/exec|plugin|crypto/tls)$`, so a `text/template` import fails through
`net/url` (research hard rule 2 wants a self-test case proving it: `binary-text-template`). No
`NewLazyDLL` allowlist rule exists yet (hard rule 3); Phase 21 (pipes) adds it; Phase 19 uses only
`kernel32.dll!SetConsoleMode`.

1.5 **SARIF** (`presenter/sarif.go`): `sarifRuleTable` = 4 `sarifRuleDef` (GRT001, GRT002, GRT003,
GRT100); `sarifRules()` and `ruleIndexOf` read that package var; the heading anchor is
`code: title (severity)`. GRT004's doc heading is `### GRT004: dependency output removed (error, blast only)`,
which `docRuleHeading` in `cmd/gruntled/sarif_test.go` (`\((error|warning|note)\)$`) deliberately does not
match, so TestSARIFDoc keeps comparing the check table to the four plain headings. `SARIF()` builds
results + unit/module notifications and encodes with `escapeJSON`. `blasting.Blast` returns only
`impact.Result`; the current graph (needed for the unknown-unit notes) is dropped inside it.
CI: `sarif-schema` step (ci.yml:74-87, jv against the vendored schema, expects exit 1 on
`testdata/sarif-fixture`) and `sarif-upload` job (:213-249, jq rewrites uri to the fixture prefix,
`category` not set). `ci_doc_test.go:286` pins the list of `${{` occurrences in ci.yml. No committed
blast fixture pair exists (txtar cases build trees on the fly; `testdata/blast_v1` holds output snapshots).

## 2. Decisions

- **D-19-01 registry shape.** `cmd/gruntled/commands.go`, package main:
  `type command struct { name, summary, usage string; flags func(*flag.FlagSet); values map[string][]string; hidden map[string]string; run func(ctx, args, stdout, stderr) int }`,
  `var commands = []command{check, graph, blast, watch, report, rebase}` in topUsage order,
  `var topUsage = topUsageOf(commands)` built with `strings.Builder` (never `text/template`) and
  byte-identical to today's const. `values` is the only source for enumerated flags (`format`) and the
  validation helper `enumError(cmd, flag, got)` generates the existing messages
  (`want text, json or sarif`, `want text or json`) with the join rule `", "` + `" or "`. `hidden` lists
  flags that are deliberately absent from usage with a reason (today: report `--depth`); the drift test
  still requires them in `docs/cli.md`.
- **D-19-02 help goes to stdout.** `help`, `help <cmd>`, `-h`, `--help` (top and per command) print
  usage on stdout, exit 0, nothing on stderr. Usage errors (exit 2) keep usage on stderr. `parseArgs` sets
  `fs.Usage = func(){}` and prints itself: ErrHelp -> stdout, else stderr (the flag package already
  printed its one-line error to `fs.Output()` = stderr).
- **D-19-03 colour decision is pure.** `colorEnabled(mode string, noColor bool, getenv func(string) string, isTTY bool) bool`
  in `cmd/gruntled/color.go`; precedence: `--no-color` > `--color=never|always` > `NO_COLOR` (non-empty) >
  `CLICOLOR_FORCE` (non-empty and not "0") > `TERM=dumb` > per-stream TTY. `CI` is ignored. Evaluated per
  stream (stdout for diagnostics, stderr for the summary).
- **D-19-04 colour is an edge post-processor.** `presenter.Colorize([]byte) []byte` and
  `presenter.StripSGR([]byte) []byte` in `presenter/color.go` (stdlib `bytes` only, no regexp needed) run
  in `runCheck`/`runBlast` on the finished `text` bytes, after the presenters (hence after `escapeTerm`),
  never in `renderReport`/`buildSnapshot`/the daemon, never for json/sarif/graph/status. `report` is
  conservative (reading A): `--quiet` only, daemon bytes pass through plain; `watch` likewise prints plain.
  Colour map (line-oriented, every other byte copied): a line matching `<file>:<line>:<col>: GRTnnn ` gets
  the code wrapped in `\x1b[31m`..`\x1b[0m`; lines `Broken (N):` and `Impacted (N):` wrapped in
  `\x1b[1m`..`\x1b[0m`; the stderr summary line is left plain (the stream is still decided separately so
  a later map can colour it). Keep the map small; the property test is the contract.
- **D-19-05 TTY** `internal/infrastructure/tty.SupportsColor(f *os.File) bool`: linux
  `unix.IoctlGetTermios(fd, unix.TCGETS)`, darwin `unix.TIOCGETA`, other unix false, windows
  `syscall.GetConsoleMode` then `kernel32!SetConsoleMode(mode | 0x0004)` through `syscall.NewLazyDLL`; any
  failure -> false (never raw escapes on conhost). `ModeCharDevice` is never consulted (`/dev/null`).
- **D-19-06 `--quiet`/`-q`**: check (summary), report (summary), watch (banner + WSL hint), blast (accepted,
  nothing to silence). Never a diagnostic, error, hint about a failure, or exit code.
- **D-19-07 blast SARIF**: `blastRuleTable = sarifRuleTable + GRT004` (index 4, name
  `DependencyOutputRemoved`, `headingSeverity: "error, blast only"` so the helpUri anchor is
  `#grt004-dependency-output-removed-error-blast-only`); `sarifRuleTable` untouched (len 4 pinned by a new
  explicit test next to TestSarifRuleTableOrder and by TestSARIFDoc). Shared builder
  `writeSARIF(w, table, results, notes, tool)` extracted from `SARIF`. `BlastSARIF(w, res, cur, tool)`:
  results = Broken findings in `res.Broken` order, message as check (`msg (unit U)`); notifications, all
  `note`, in this order: `no baseline: every finding is Broken` (no location, only when `!res.Baseline`),
  unknown units/modules of `cur` (same text as check), then one per Impacted unit
  `unit "<u>" is impacted (distance D, module "<m>")` with location `unitConfigFile(u)`, no region. The
  `--base` string is never passed in. `blasting.BlastDetailed` returns `Detail{Result, Cur checking.Report}`;
  `Blast` wraps it. `--depth` applies before rendering. Exit code = `res.HasErrors()` as before.
  `report --blast --format sarif` stays exit 2 and is documented as deferred.
- **D-19-08 fixture**: `cmd/gruntled/testdata/blast-fixture/{base,cur}` = `base1/cur1` of
  `testdata/script/blast_grt004.txtar` (a removed output -> one GRT004, `live/app` Broken). Used by the
  Go test and by both CI SARIF jobs (`--base .../base .../cur`, expect exit 1).

## 3. Risks carried into the threat models
Colour/SGR injection via repo content (closed by escapeTerm-before-Colorize, property test);
env-driven behaviour differences in CI (`auto` is off on non-TTY, `CI` ignored); SARIF alert churn
(Impacted never a result); rule index drift (explicit len-4 and index-4 tests); `--base` path leakage
(assert the base string is absent from the document); lazy DLL load on windows (kernel32 only, failure =
no colour); vacuity of the registry/doc drift test (negative subtests with a synthetic command).
