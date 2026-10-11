---
phase: 15-type-level-surface-facts
plan: 05
subsystem: cli, docs, gate
tags: [security, escaping, size-bound, rss, phase-gate]
requires: [15-04]
provides:
  - "SEC-01 end to end: crafted variable/output names through blast type tokens, the Type changes section and type_changes JSON"
  - "TestBlastSizeBound (BLAST-11) on the built binary"
  - "README blast paragraph pinned for the type-level reasons and the exit code"
  - "Phase 15 gate evidence"
affects: [16]
tech-stack:
  added: []
  patterns: ["RSS bound relative to an in-run control of the same file size"]
key-files:
  created:
    - cmd/gruntled/blast_bound_test.go
  modified:
    - cmd/gruntled/escape_test.go
    - cmd/gruntled/rules_doc_test.go
    - README.md
key-decisions:
  - "The RSS control is the same 5,000-unit chain with a module file of the same size (the 4 MiB object in a default, a tiny type change), not a tiny file: lexing and parsing a MaxFileBytes file in both trees costs ~550-610 MiB before any type fact exists, so a tiny-file control cannot isolate what Phase 15 adds (bus #308)"
  - "TestBlastSizeBound is build-tagged linux || darwin (rusage) and skipped under -short; the binary it runs is built without -race, so RSS stays meaningful in the CI -race job"
requirements-completed: [SEC-01, BLAST-11]
duration: 40min
completed: 2026-10-11
---

# Phase 15 Plan 05: SEC-01, BLAST-11 and the Phase Gate Summary

**Crafted variable and output names (ESC, U+009B, U+202E written as HCL/JSON `\u` escapes, plus a raw 0xff byte in a `.tf.json` key) reach the blast type tokens, the `Type changes` section and `type_changes` JSON escaped, and the JSON decodes back to the raw names. An object attribute name with U+202E is silent, and a raw 0xff in a `.tf` is an escaped GRT100 with no type change. On the built binary, a 5,000-unit chain with a 4 MiB variable type gives 10.2 MB of text, 11.4 MB of JSON and a peak RSS no higher than a same-size control. The phase gate is green: 244/244, perturb 243/244, snapshots identical, 60 s fuzz, six-target release build.**

## Performance

- Duration: ~40 min
- Tasks: 3
- Files: 1 created, 3 modified

## Accomplishments

### Task 1: SEC-01 end to end (cmd/gruntled/escape_test.go)
- **craftedRepo** gains unit `types`, whose module is its own directory:
  - The base main.tf (:103) declares `variable "v\u001bx\u009by\u202ez" { type = string }`, `output "o\u001b\u202e"` and `variable "w" { type = object({ "a\u202eb" = string }) }`.
  - The current main.tf (:88) changes the type to number, adds `sensitive = true` and changes w's object.
  - The current tree also gets `vars.tf.json` with `"j\u001b\u202e": {}`, a new required variable, and `bad.tf.json` (:97) with key `k` + raw `\xff`, which decodes to U+FFFD.
  - Unit `rawbad` gets a current main.tf (:101) with a raw 0xff byte in a string.
  - The Go source holds no raw control or bidi rune: everything is an HCL/JSON `\u` escape or a Go `\xff` escape.
- **blast text** (:223): the distance-1 tokens are `type variable v\x1bx\u009by\u202ez`, `sensitive output o\x1b\u202e`, `required variable j\x1b\u202e` and `required variable k�`. The Type changes lines are `types variable j\x1b\u202e: now required`, `... k�: now required`, `... v\x1bx\u009by\u202ez: type string -> number` and `types output o\x1b\u202e: sensitive false -> true`. The rawbad GRT100 appears under Broken. Nothing is reported for `w` (unparseable on both sides) or for rawbad (unknown surface in the current tree). `assertNoRawControls` passes.
- **blast json** (:287): `changes[types].type_changes` decodes to the raw names, U+FFFD included, with the right kinds and old/new values. There is no `w` entry and no changes entry for rawbad. `\u009b`, `\u007f` and `\u202e` are present as escapes, and `assertNoRawControls` passes.
- check, watch, report, check json, sarif and graph json are unchanged and green: they carry names only, so their outputs stay as before (MORE-07). watch and report stay byte-identical to check.
- The implementation landed in 15-04, so these tests passed on their first run. To prove they bite, I changed one expected token and the blast subtest failed with `blast stdout lacks "type variable v\\x1bx\\u009by\\u202ezX"`, then reverted it.

### Task 2: BLAST-11 (cmd/gruntled/blast_bound_test.go)
- **Setup:**
  - `boundTree` (:28) writes `live/u0000` (instantiating modules/big) through `live/u4999`, each depending on the previous unit with a default block that reads no output.
  - `bigObjectType` (:44) builds `object({a000000=string,...,z=T})` of 4,194,078 bytes, just under MaxFileBytes; base and cur differ only in `z`.
  - The binary is built once with `benchBuildGruntled` (no -race). `boundRun` (:56) records exit code, stdout, wall time and `Maxrss` (KiB on linux, bytes on darwin).
- **Assertions:**
  - exit 0;
  - 5,000 Impacted units at distances 1..5,000, each distance once;
  - `rendered` = len(old)+len(new) read back from JSON `type_changes`;
  - text < rendered + 5,000×200 + 1,024 (:165);
  - json < 2×rendered + 5,000×400 + 1,024 (:168);
  - the new type string appears once in text;
  - Maxrss(big) ≤ Maxrss(control) + 64×typeBytes (:175).
- Measured on linux/amd64, 22 CPUs, Go 1.27.2, WSL2. Two runs; absolute values are only logged:

| Run | text bytes | json bytes | wall (text/json) | maxrss (text/json) |
|-----|-----------:|-----------:|------------------|--------------------|
| tiny file, tiny type (logged only) | 727,658 | 759,478 | 0.60 / 0.55 s | 36 / 34 MiB |
| control: same-size file, 4 MiB default, tiny type change | 727,658 | 759,478 | 2.57 / 2.49 s | 585 / 587 MiB |
| big: 4 MiB type change | 10,234,222 | 11,384,462 | 3.07 / 3.50 s | 544 / 553 MiB |
| run 2: control / big | | | 3.77 / 4.31 s | 614 / 549 MiB |

- The rendered old+new strings total 9,506,576 bytes. The text bound is 10,531,600 bytes and the json bound 21,014,176 bytes. The RSS bound is 843 MiB (run 2: 870 MiB). The whole test takes 17-23 s.

### Task 3: README and pin
- **rules_doc_test.go:** the README blast paragraph must contain `required variable`, `types`, `` `sensitive` `` and `never change the exit code`. It was red first (88ce142).
- **README.md** (7e205b7): "Type-level changes are Impacted reasons only: they are never Broken and never change the exit code."

## Task Commits

1. `f9fd469` test(15-05): crafted variable and output names through blast type-level tokens, section and JSON
2. `c820bc6` test(15-05): blast output and peak RSS bound on a 5,000-unit chain with a 4 MiB type
3. `88ce142` test(15-05): README blast paragraph must name the type-level reasons and the exit code
4. `7e205b7` docs(15-05): README says type-level reasons never change the exit code

This summary is its own `docs(15-05): summary` commit.

## Phase 15 Gate (at 7e205b7)

```
== go test
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	67.988s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.586s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.003s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.198s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.434s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.016s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	8.977s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.057s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.069s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	5.073s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	10.900s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.132s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.094s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.042s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.030s
== vet
VET-OK
== gofmt
== check-architecture
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
== compare-ref
244/244 identical
== perturb
rc=1
DIFF golden/dependency_edges check --format text (exit ref 1, head 1)
243/244 identical
PERTURB-OK
== txtar-extract
ok blast-snapshots.sh ref no-such-ref-txtar-test
all txtar-extract cases passed
== snapshots
PROVENANCE 67c9ace9a09f46d5662acc1fffddb6d2057034e8
SNAPSHOTS-IDENTICAL
== gomod
GOMOD-UNCHANGED
```

`gofmt -l .` printed nothing.

Architecture self-tests:

```
bash scripts/test-check-architecture.sh
all architecture self-tests passed
```

Fuzz, 60 s:

```
go test -run '^$' -fuzz '^FuzzTypeSig$' -fuzztime 60s ./internal/infrastructure/tfsurface/
fuzz: elapsed: 1m1s, execs: 136397 (0/sec), new interesting: 211 (total: 382)
PASS
```

No testdata/fuzz directory was created, so there is no failing input.

Six-target release build:

```
bash scripts/build-release.sh v0.0.0-ci 7e205b765a92 <tmpdir>
gruntled_v0.0.0-ci_linux_arm64.tar.gz: OK
gruntled_v0.0.0-ci_windows_amd64.zip: OK
gruntled_v0.0.0-ci_windows_arm64.zip: OK
smoke: gruntled v0.0.0-ci (7e205b765a92)
checksums.txt  gruntled_v0.0.0-ci_{darwin,linux}_{amd64,arm64}.tar.gz  gruntled_v0.0.0-ci_windows_{amd64,arm64}.zip
```

`-race` was not run locally (no gcc). CI runs it, along with macOS, windows, staticcheck, govulncheck and the 30 s fuzz step. TestBlastSizeBound runs in the linux and darwin jobs and is excluded on windows by its build tag.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-05-1 | mitigated | escape_test.go:88-109 crafted names and invalid UTF-8; text assertions :223 (tokens, section, rawbad GRT100, nothing for w/rawbad); JSON round-trip :287/:300; assertNoRawControls on blast text and JSON |
| T-15-05-2 | mitigated | blast_bound_test.go:165/:168 linear byte bounds from the rendered old+new; :175 RSS bound against the in-run control; 5,000 distinct distances checked |
| T-15-05-3 | mitigated | the bound is relative to a control run of the same file size on the same machine (:129), absolute values only logged; build tag linux/darwin; skipped under -short; binary built without -race; two local runs passed (553 vs 843 MiB, 549 vs 870 MiB) |
| T-15-05-4 | mitigated | gate on the final code commit 7e205b7: build-release on six targets, check-architecture (Step 9), go.mod/go.sum unchanged |

## Deviations from Plan

**1. [RSS control] Same-size file instead of a tiny type (bus #308)**
- With the tiny-type control, the bound control + 64×typeBytes was 293 MiB, and the big run measured 553 MiB.
- A probe showed the 4 MiB object placed in a `default`, with no type involved, peaks at 593 MiB. That is the existing lex and parse cost of a MaxFileBytes file in two trees, not anything Phase 15 adds; with the same shape the type-changed run measured 508 MiB.
- The control is therefore the same chain with a same-size module file (the object in a default, a tiny type change), so the bound isolates the type facts. The tiny-file run is kept and logged.
- The plan's must-have wording ("a bound set at 2× the value measured here") is replaced by the relative bound the orchestrator asked for. Measured big/control ratios are 0.94 and 0.89.

**2. [TDD] The Task 1 and Task 2 tests are regression proofs, not red-first**
- The behaviour shipped in 15-02, 15-03 and 15-04, so both tests passed on their first run. A deliberate mutation of an expected escaped token proved Task 1 bites, and a repeated type string would break the Task 2 byte bound.
- Task 3's README pin was red first.

**3. [Unit layout] `types` and `rawbad` are units whose module is their own directory**
- This follows the existing `producer` pattern in craftedRepo; the plan just says "module types".

## Known Risks / Unverified

- The RSS bound has only been measured on linux/amd64 (WSL2). macOS reports Maxrss in bytes, which the test converts; the first CI macOS run will confirm it.
- `-race` was not run locally.
- Bus #308 (the RSS control) was still unanswered when this summary was written.

## Self-Check: PASSED
