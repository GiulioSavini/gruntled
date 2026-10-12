---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 01
subsystem: scripts, cmd/gruntled (tests)
tags: [corpus, rel-03, compare-ref, fetch, self-blast]
requires: []
provides:
  - "scripts/fetch-corpora.sh: one-time fetch of the three pinned corpora by sha, git-isolated, link-checked, ELF-stripped"
  - "scripts/test-fetch-corpora.sh: 8 offline cases over file:// fixtures"
  - "compare-ref.sh COMPARE_REF_CORPORA rows (stdout, stderr, exit) and a symlink-safe tree digest"
  - "TestCorpusBlastSelf and the shared helpers blastRun, decodeBlast, blastDoc, assertBlastEmpty"
affects: [17-02, 17-03, 17-04]
tech-stack:
  added: []
  patterns: ["one isolated git wrapper g() for every git call", "digest = sorted listing of mode+sha256 for files, 'p -> target' for links, type for the rest"]
key-files:
  created:
    - scripts/fetch-corpora.sh
    - scripts/test-fetch-corpora.sh
    - cmd/gruntled/corpus_blast_test.go
  modified:
    - scripts/compare-ref.sh
key-decisions:
  - "Escaping links are found with realpath -m on the link (the real resolution, not a lexical one). A link whose resolution fails (a loop) also counts as escaping, so the check fails closed"
  - "The digest does not add to the N/M count: 244 + 4 per corpus, as the plan's numbers require. A digest change prints DIFF corpus/<name> digest and makes the exit status non-zero on its own"
  - "For secret and denis256, the non-vacuity output comes from graph --json: the first known module in path order whose first output (by name) is a plain output block in a .tf file. The test asserts a changes[] entry that removes exactly that output, so a deletion that broke the parse would fail"
  - "decodeBlast leaves daemon_baseline out of blastDoc, because it is documented as never present in gruntled blast. note is an optional field"
  - "The fetch adds GIT_CEILING_DIRECTORIES=dest and unsets the GIT_* path, config and ssh variables, on top of the plan's isolation list"
requirements-completed: []
duration: 35min
completed: 2026-10-12
---

# Phase 17 Plan 01: Corpus Fetch, compare-ref Corpora and Self-Blast Summary

**REL-03 clause 1 is now a repeatable gate. `scripts/fetch-corpora.sh` fetched the three pinned corpora once, by sha, with git isolated from every system, global and template config. It verified each HEAD as the pinned commit, found 0 escaping symlinks, and stripped 30 ELF files (1,856,441,161 bytes) from denis256. `compare-ref.sh v0.3.0` with the three corpora prints `256/256 identical` (stdout, stderr and exit), and each corpus tree digest is unchanged. `TestCorpusBlastSelf` shows that `blast --base X X` and `blast --base copy X` are empty in text and JSON on all three corpora. Its non-vacuity row fails when the output deletion is skipped.**

## Performance

- Duration: ~35 min
- Tasks: 3
- Files: 3 created, 1 modified

## Accomplishments

### Task 1: fetch-corpora.sh and its offline self-test (a2f2f20 red, b8b5d3e green)

- scripts/fetch-corpora.sh:
  - Built-in manifest (name, https URL, sha). The whole manifest is validated before any git call or write; an invalid one exits 2.
  - Every git call goes through g() (:101-112), with GIT_CONFIG_NOSYSTEM=1, GIT_CONFIG_GLOBAL=/dev/null, GIT_TERMINAL_PROMPT=0, GIT_LFS_SKIP_SMUDGE=1, GIT_CEILING_DIRECTORIES and the `-c` list from the plan. `protocol.file.allow` is set only in FETCH_CORPORA_MANIFEST mode.
  - `git init --template=` (:205).
  - verify() (:124-168) checks:
    - HEAD equals the sha, and the object is a commit;
    - no absolute link, and no link whose real path is outside the root;
    - it reports gitlinks and `filter=` lines.
  - strip_elf() (:171-184): `find -P … -type f`, reads the first 4 bytes, and prunes .git.
  - Temp dir under dest, then `mv -T`. A trap removes the temp dir on failure. An existing dest at another HEAD is refused, never reset.
- scripts/test-fetch-corpora.sh (red first: the script did not exist, `FAIL case 1: exit 127`):
  - 1 correct sha;
  - 2 unknown sha;
  - 3 dest at another commit, untouched (digest including .git);
  - 4 idempotent rerun;
  - 5 malformed sha (4 shapes plus a bad later line): exit 2, and a git wrapper on PATH proves git was never called;
  - 6 a HOME-level `filter.x.smudge` is not run, and the attribute is reported;
  - 7 a HOME-level `init.templateDir` post-checkout hook is not run, and .git/hooks is empty;
  - 8 escaping links (relative `../../outside`, absolute `/etc/hostname`): exit 1, no dest.
  - Cases 6 and 7 first show that plain `git clone` under the same HOME does run the filter and the hook.
- Mutation checks on the script, each restored afterwards:

  | Mutant | Caught by |
  |--------|-----------|
  | GIT_CONFIG_GLOBAL=/dev/null removed | `FAIL case 6: the HOME-level filter ran` |
  | `--template=` removed | `FAIL case 7: hooks copied into the repo` |
  | escaping-link exit disabled | `FAIL case 8 (relative): exit 0, want 1` |
  | `protocol.file.allow` not passed | `FAIL case 1` (the allow-list really blocks file://) |

- Real fetch, run once over the network (55 s):

  ```sh
  bash scripts/fetch-corpora.sh
  ```

  ```
  primary: HEAD e6c55d11fd1a01e75b78d7897be36c69fa26b8cc (commit)
  primary: symlinks 68, escaping 0
  primary: gitlinks 0
  primary: filter attributes 0
  primary: stripped 0 ELF files, 0 bytes
  secret: HEAD 341e8a95b0d9bd658793094a248527cc3ebae6f2 (commit)
  secret: symlinks 0, escaping 0
  secret: gitlinks 0
  secret: filter attributes 0
  secret: stripped 0 ELF files, 0 bytes
  denis256: HEAD 726485e699a70c02dabbde629f66c0119e197357 (commit)
  denis256: symlinks 6, escaping 0
  denis256: gitlinks 5
  denis256: filter attributes 0
  denis256: stripped 30 ELF files, 1856441161 bytes
  export GRUNTLED_CORPUS=/home/giulio/.cache/gruntled-phase4/corpus/primary
  export GRUNTLED_CORPUS_SECRET=/home/giulio/.cache/gruntled-phase4/corpus/secret
  export GRUNTLED_CORPUS_DENIS256=/home/giulio/.cache/gruntled-phase4/corpus/denis256
  ```

  On disk afterwards: primary 12M, secret 1.3M, denis256 467M (mostly .git). Each `.git/HEAD` holds the pinned sha (detached), which is what corpusRequire, secretRequire and denisRequire expect.

### Task 2: compare-ref.sh corpus rows (427c425)

- COMPARE_REF_CORPORA is parsed with `IFS=:`. Each entry is checked with `realpath -e --` and `[ -d ]` before the cd and before any build. An empty entry, a missing path or a file exits 2.
- Corpus rows run with `cd --` and compare stdout, stderr and exit. Fixture rows keep their stdout-only comparison.
- `tree_listing`/`tree_digest` (:77-94):
  - `find -P` with .git pruned;
  - one NUL-sorted file list feeds both `stat -c %a` and `sha256sum`, so paths with spaces or newlines stay paired;
  - links via `-printf 'l %p -> %l'`;
  - other types via `%y %p %m`.
- GOTOOLCHAIN=local added to the offline env (T-17-01-7).
- Red, before the change: `COMPARE_REF_CORPORA=/nonexistent` was ignored, giving `244/244 identical` and exit 0.

| Run | Result |
|-----|--------|
| unset | `244/244 identical`, exit 0 |
| clean-fixture | 4 `same corpus/clean-fixture …` rows + `same corpus/clean-fixture digest 97aa7879…`, `248/248 identical` |
| link shapes: copy of clean-fixture + `dirlink -> ./modules`, `dangling -> missing/target`, `abslink -> /etc/hostname`, `zerolink -> /dev/zero` | `248/248 identical`, digest unchanged, under `timeout 900`; the listing (run alone under `timeout 60`) shows `l ./abslink -> /etc/hostname`, `l ./dangling -> missing/target`, `l ./dirlink -> ./modules`, `l ./zerolink -> /dev/zero` |
| `/nonexistent`, empty entry (`a::`), a file (go.mod) | exit 2 in 0.009 s with `COMPARE_REF_CORPORA entry is not a directory`, nothing built |
| COMPARE_REF_PERTURB=1 + clean-fixture | `DIFF golden/dependency_edges check --format text`, `247/248 identical`, exit 1 |
| the three corpora | `256/256 identical`, exit 0; denis256 check exits 1 on both binaries; digests primary 6a43fd3e…, secret 875fed9e…, denis256 ae5dea51… unchanged |

The command for the last row:

```sh
C=~/.cache/gruntled-phase4/corpus
COMPARE_REF_CORPORA="$C/primary:$C/secret:$C/denis256" bash scripts/compare-ref.sh v0.3.0
```

`git worktree list` shows only `/home/giulio/gruntled [master]` after every run. The branch is master.

### Task 3: TestCorpusBlastSelf (2072d51)

- cmd/gruntled/corpus_blast_test.go. It is env-gated through graphRepos: it skips without the env vars and fails on a wrong commit. It reads only CLI bytes, through in-process `run`, and imports no internal/ package.
- Helpers for later plans:
  - `blastRun(t, base, cur, json)`, where stderr output fails the test;
  - `decodeBlast` with DisallowUnknownFields and a trailing-data check;
  - `blastDoc`, which mirrors the docs/cli.md Blast JSON table;
  - `assertBlastEmpty`: exit 0; text from line 2 is exactly `Broken (0):\nImpacted (0):\n`; JSON version 2, kind blast, baseline true, no note, lists `[]` and not null, summary {0 0}.
- Per corpus: (checkout, checkout) and (corpusCopy or corpusCopyFiltered(graphIsELF), checkout), in text and JSON. corpusDigest of the checkout is the same before and after.
- Non-vacuity: one output block is deleted in a fresh copy, and blast must report a changes[] entry that removes exactly that output, plus a non-empty impacted.

```sh
go test -p=1 -count=1 -run TestCorpusBlastSelf -v -timeout 15m ./cmd/gruntled/
```

```
non-vacuity: output "arn" of iac.src/s3_runtime/state.tf deleted: 1 changes, 9 impacted, 0 broken, exit 0
non-vacuity: output "ecr_arn" of aws/ecr/outputs.tf deleted: 1 changes, 1 impacted, 1 broken, exit 1
non-vacuity: output "outputX" of 5270-test/main.tf deleted: 1 changes, 1 impacted, 0 broken, exit 0
--- PASS: TestCorpusBlastSelf (6.54s)
    --- PASS: TestCorpusBlastSelf/iso20022 (0.95s)
    --- PASS: TestCorpusBlastSelf/secret (0.10s)
    --- PASS: TestCorpusBlastSelf/denis256 (5.49s)
```

The peak RSS of the whole `go test` was 180,992 KB (`/usr/bin/time -v`).

Mutant: the deletion was replaced by writing the source back unchanged. All three subtests then FAIL with `non-vacuity: deleting output … must give a changes entry removing exactly it`.

Without the env vars (`env -u GRUNTLED_CORPUS -u GRUNTLED_CORPUS_SECRET -u GRUNTLED_CORPUS_DENIS256`), all three subtests SKIP. That is the CI behaviour.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 (red) | a2f2f20 | test(17-01): offline self-test for fetch-corpora.sh |
| 1 | b8b5d3e | feat(17-01): fetch-corpora.sh fetches the pinned corpora by sha, isolated and ELF-stripped |
| 2 | 427c425 | feat(17-01): compare-ref.sh corpus rows with stderr and a symlink-safe tree digest |
| 3 | 2072d51 | test(17-01): TestCorpusBlastSelf pins an empty self-blast on the three corpora |

## Gate

```sh
bash scripts/test-fetch-corpora.sh                         # all fetch-corpora cases passed
bash scripts/test-txtar-extract.sh                         # all txtar-extract cases passed
go vet ./cmd/gruntled/                                     # clean
gofmt -l cmd/gruntled/                                     # empty
bash scripts/check-architecture.sh                         # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
go test -count=1 -short -run 'TestValidationDocPins|TestCIDoc' ./cmd/gruntled/   # ok
```

shellcheck is not installed on this machine, so the shellcheck half of the Task 1 verify did not run. Everything was run serially, with `free -m` first (4.4-4.9 GB free) and one gruntled process at a time. Not run: -race, -fuzz, `go test ./...` and scripts/test-check-architecture.sh (no architecture script changed).

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-01-1 | mitigated | fetch by sha; HEAD and `cat-file -t` checked before the rename (scripts/fetch-corpora.sh:126-131); case 2; the require helpers fail on another commit |
| T-17-01-2 | mitigated | nothing executes corpus content: the script never cds into a corpus; g() isolation (:101-112); `init --template=` (:205); cases 6-7 plus mutants; the ELF strip is a size measure only (header comment) |
| T-17-01-9 | mitigated | absolute and escaping links fail the fetch (:136-161, case 8); compare-ref digest uses `find -P`, hashes `-type f` only and records `%l` (scripts/compare-ref.sh:77-94); link-shape row |
| T-17-01-10 | mitigated | GIT_CONFIG_NOSYSTEM, GIT_CONFIG_GLOBAL=/dev/null, GIT_LFS_SKIP_SMUDGE=1, `protocol.allow=never` with only https allowed (file only in manifest mode); filter= attributes reported (0 on all three) |
| T-17-01-3 | mitigated | strip uses `find -P -type f` with .git pruned (:171-184); temp dir plus `mv -T`; cases 1-3 |
| T-17-01-4 | mitigated | compare-ref digest before and after per corpus; corpusDigest in TestCorpusBlastSelf; copies under t.TempDir |
| T-17-01-5 | mitigated | `realpath -e --`, `cd --`, quoted expansions in compare-ref; the empty and file entries are refused |
| T-17-01-6 | mitigated | perturb row (exit 1); the non-vacuity mutant fails all three subtests |
| T-17-01-7 | mitigated | `GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local` in compare-ref; no network after the fetch |
| T-17-01-8 | accepted | denis256 on disk is 467M after the strip; test RSS 181 MB |

## Deviations from Plan

**1. [Count] The digest is not a counted comparison.** "A change counts as a failure" is implemented as its own failure flag. The N/M line stays 244 + 4 per corpus, so 248 and 256 hold as the plan states.

**2. [Strip count wording] 30 ELF files, 1,856,441,161 bytes (1.73 GiB).** The plan says "about 1.77 GB". graphRepo's comment in corpus_graph_test.go says 29 binaries and 1.85 GB; it was left unchanged.

**3. [Extra isolation]** g() also unsets GIT_DIR, GIT_WORK_TREE, GIT_CONFIG_PARAMETERS, GIT_CONFIG_COUNT, GIT_TEMPLATE_DIR, the ssh and askpass variables, and sets GIT_CEILING_DIRECTORIES=dest. This only adds to the plan's list.

## Known Risks / Unverified

- shellcheck was not run (not installed).
- denis256 has 5 gitlinks (submodule entries). They are reported, and they are not fetched (`submodule.recurse=false`, `--no-recurse-submodules`), as in v0.2.
- No 17-01 CI run yet: nothing is pushed.

## Self-Check: PASSED
