# Validation of gruntled v0.1 and v0.2

v0.2 adds GRT002 (missing dependency target) and GRT003 (dependency cycle). Their
validation on the real corpus is recorded in
[v0.2: GRT002 and GRT003 on the real corpus](#v02-grt002-and-grt003-on-the-real-corpus-more-06),
at the end of this document. Everything before it is the v0.1 record, unchanged.
v0.4's blast is validated on the three real corpora in
[v0.4: blast on the real corpora (REL-03)](#v04-blast-on-the-real-corpora-rel-03).

gruntled v0.1 is a falsifiable experiment, not a feature list. The claim has four
parts. On the unmutated primary corpus, `gruntled check` reports nothing. When an output
is renamed or deleted in a target module, gruntled reports every reference that no longer
resolves. Plain `terragrunt hcl validate` does not report those injected breaks. And
`gruntled check` is faster than `terragrunt hcl validate` on the same repository. If any
part fails, the idea is wrong. This document records one full run of that experiment and
how to rerun it.

## Outcome

All five claims passed on the pinned primary corpus. The denis256 secondary check also
passes: gruntled reports exactly the 8 hand-derived references on that corpus (8/8) and
nothing else. See [Secondary corpus](#secondary-corpus-denis256terragrunt-tests).

| Requirement | Claim | Result | Evidence |
|---|---|---|---|
| VALID-02 | Golden tests assert the exact expected diagnostic set on fixture repositories | PASS | [VALID-02](#valid-02-golden-fixtures) |
| VALID-03 | Zero diagnostics on the unmutated primary corpus | PASS | [VALID-03](#valid-03-unmutated-corpus) |
| VALID-04 | Every reference broken by an injected rename or deletion is reported, and nothing else | PASS | [VALID-04](#valid-04-injected-mutations) |
| VALID-05 | Plain `terragrunt hcl validate` does not report those mutations | PASS | [VALID-05](#valid-05-terragrunt-hcl-validate-on-the-same-mutated-trees) |
| VALID-06 | `gruntled check` is faster than `terragrunt hcl validate` on the same repository | PASS | [VALID-06](#valid-06-benchmark) |
| Secondary corpus denis256 (supplementary to VALID-02) | No panic, deterministic output, and an exact GRT001 set | PASS: exactly the 8 known references reported (8/8), 0 false positives | [Secondary corpus](#secondary-corpus-denis256terragrunt-tests) |

## Environment

From the `VALIDATION-BENCH` lines of the recorded run:

| Field | Value |
|---|---|
| CPU | 13th Gen Intel(R) Core(TM) i5-1335U |
| Logical CPUs visible to Go | 4 |
| RAM (MemTotal) | 6069492 kB |
| Kernel | 6.6.87.2-microsoft-standard-WSL2 (Windows Subsystem for Linux 2) |
| GOOS/GOARCH | linux/amd64 |
| Go | go1.27.0 |
| gruntled commit | `11aa6a7113c8af24d6daba3f81b420ea4598754d` (`git rev-parse HEAD` at run time) |

The machine was busy during the recorded run. Its load average was 13.74 just before
the run and 17.06 when the benchmark finished, on 4 logical CPUs. `top` showed about 47%
I/O wait and 2 GiB of swap in use. The benchmark was run a third time once the load had
fallen to about 3. See [VALID-06](#valid-06-benchmark) for all three runs and what the
load does to the timings.

## Pinned tools

**terragrunt v1.1.6.**

- Download: `https://github.com/gruntwork-io/terragrunt/releases/download/v1.1.6/terragrunt_linux_amd64`
- SHA256: `d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04`
- The checksum was verified against the release's `SHA256SUMS` file when the binary was
  downloaded (PREP.md §1). The tests verify it again on every run and fail, not skip, on a
  mismatch.

Measured in this run, verbatim from the `VALIDATION-TG` log lines:

```
VALIDATION-TG sha256=d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04 path=/home/giulio/.cache/gruntled-phase4/bin/terragrunt_linux_amd64
VALIDATION-TG version=terragrunt version v1.1.6
```

Both match the pins.

**OpenTofu v1.12.6.** SHA256
`5dc43da4f750f33873dc25e94587128709e819e544b7be9016b255316153c3a8`, verified against
`tofu_1.12.6_SHA256SUMS` (PREP.md §1). It sits beside terragrunt on the restricted `PATH`,
but plain `hcl validate` does not need it.

**Why only plain `hcl validate`.** `terragrunt hcl validate --inputs --strict` already
exits 1 on the clean primary corpus, with 11 "inputs passed in by terragrunt are unused"
errors that have nothing to do with `dependency.outputs`. `--inputs` also writes a 3.5 MB
`.terragrunt-cache/` into the repository it checks (PREP.md §4.1-4.2). Plain
`hcl validate` exits 0 on the clean corpus and writes nothing, so it is the only fair
comparator. No run in this experiment passes `--inputs` or `--strict`.

## Corpus

- Repository: `https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws`
- Commit: `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc` (detached HEAD). The tests fail, not
  skip, if `GRUNTLED_CORPUS` is at another commit.
- Licence: MIT-0.
- Shape as gruntled saw it in this run (TestCorpusClean log):
  `units=65 resolved=62 module_unknown=0 config_unknown=3 unknown_modules=0`.
  The 3 unknown units are all `config-unknown: invalid-dependency`:
  `iac.cicd/codebuild_project`, `iac.src/scheduler_recover`, `iac.src/scheduler_timeout`.

Every test works on a scratch copy. A tree digest of the corpus checkout is taken before
and after each test and did not change.

Corpora not used for any VALID claim (PREP.md §3):

- `cds-snc/secret` and `cds-snc/gc-articles`. Both use the legacy unnamed
  `include { ... }` block, which terragrunt v1.1.6 rejects ("Missing name for include").
  `gc-articles`' `prod/*` units also use remote `git::` sources keyed off an unset
  environment variable. terragrunt exits 1 on both repositories before any
  `dependency.outputs` question arises, so neither is a usable comparator.
- `denis256/terragrunt-tests` is used only as a secondary exact-set corpus. It is a
  deliberately broken fixture suite, so a zero-diagnostic claim makes no sense there, and
  terragrunt v1.1.6 crashes on it after about 66 s (`ERROR "not a string"`, PREP.md §3),
  so it cannot be a mutation or timing corpus either.

## VALID-02 Golden fixtures

The golden tests are always on and run in CI. `go test -count=1 -v -run TestGolden
./cmd/gruntled` passed in this run.

Hand-written fixtures, as txtar archives under `cmd/gruntled/testdata/golden/`. Each was
written for this project; nothing is copied from an external repository.

| Fixture | What it proves |
|---|---|
| `live_clean` | A unit is not its module, includes via `find_in_parent_folders`, two environments, no diagnostics. |
| `live_broken` | Renamed output references are caught (2 GRT001), valid sibling references stay silent. |
| `shared_include` | A broken reference in a shared include is reported once per including unit (3 GRT001 at `_envcommon/app.hcl:6:13`). |
| `remote_local_mix` | Remote-source units are unknown and silent; the local break is reported. |
| `tf_json_surface` | Outputs declared in `.tf.json` files are read. |
| `symlinked_module_files` | Symlinked `.tf` files in a module directory are read. |
| `mock_shapes` | DIAG-03: mocks never suppress a report; `skip_outputs` and `enabled = false` are silent. |
| `lazy_guards` | References guarded by `try`, `can`, a ternary, `&&` or a `for` are silent. |
| `syntax_errors_mixed` | Exactly one GRT100 per broken file, at the `@`; a broken module surface is silent. |
| `dependency_edges` | Missing dir, stack, non-default file name and undeclared label cases; a duplicate label is `config-unknown: invalid-dependency`. |

Full-scale synthetic trees from the VALID-01 generator (`synthrepo`), compared with
`Manifest.Expected`:

| Spec | Units | Include depth | Fanout | Injected `BadOutputRef` |
|---|---|---|---|---|
| `seed1_depth4_fanout5` | 400 | 4 | 5 | 12 |
| `seed2_depth2_fanout8` | 400 | 2 | 8 | 25 |
| `seed3_depth3_fanout3` | 600 | 3 | 3 | 8 |
| `clean_full_scale` | 500 | 4 | 6 | 0 |

The rule is exact set equality. Every position is exact, GRT100 included: one GRT100 per
broken file, following the `FirstSyntaxError` contract, at the hand-counted `@`. Exit
codes and unknown-unit sets are also exact, and each fixture runs twice with
byte-identical JSON. Every expected diagnostic was computed by hand and self-checked
against the fixture bytes before gruntled runs. None was taken from gruntled's output,
and there is no `-update` flag.

## Secondary corpus: denis256/terragrunt-tests

- Repository: `https://github.com/denis256/terragrunt-tests`
- Commit: `726485e699a70c02dabbde629f66c0119e197357`. The checkout has `HEAD` on
  `refs/heads/master`; the test resolves the ref and fails on any other commit.
- Licence: MIT.
- Environment variable: `GRUNTLED_CORPUS_DENIS256`.

**What is asserted:** no panic, exit 1, byte-identical JSON across two runs, only the codes
GRT001 and GRT100, the exact GRT001 set, nothing at the silent position, and an unchanged
corpus digest.

**What is only logged** (DENIS256 block of this run):

```
DENIS256 units=1146
DENIS256 resolved=852
DENIS256 module_unknown=219
DENIS256 config_unknown=75
DENIS256 unknown_modules=30
DENIS256 errors=16
DENIS256 warnings=0
DENIS256 grt100=8
DENIS256 grt001=8
DENIS256 unknown_reason remote-source=182
DENIS256 unknown_reason include-target=54
DENIS256 unknown_reason generate-may-declare-outputs=21
DENIS256 unknown_reason source-dynamic-path=13
DENIS256 unknown_reason include-dynamic-path=11
DENIS256 unknown_reason syntax-error=5
DENIS256 unknown_reason invalid-include=2
DENIS256 unknown_reason source-outside-repo=2
DENIS256 unknown_reason config-too-deep=1
DENIS256 unknown_reason include-not-found=1
DENIS256 unknown_reason json-config-unsupported=1
DENIS256 unknown_reason unit-dir-overlays-module=1
DENIS256 grt100 autoinclude-bugs/case1-object-key-leak/units/app/main.tf:1:26
DENIS256 grt100 broken-dependencies/dependency/main.tf:8:17
DENIS256 grt100 broken-dependencies/dependency2/main.tf:4:23
DENIS256 grt100 encryption/terragrunt.hcl:14:20
DENIS256 grt100 include-error/terragrunt.hcl:28:19
DENIS256 grt100 issue-3368/terragrunt.hcl:24:68
DENIS256 grt100 scaffold/test1/.boilerplate/terragrunt.hcl:7:24
DENIS256 grt100 stack-autoincludes/object-computed-key-leak/units/app/main.tf:3:16
```

That is 294 unknown units out of 1146, 54 of them `include-target`. The 8 GRT100 are
all genuine syntax errors in the corpus fixtures (for example `some broken code` in
`broken-dependencies/dependency/main.tf`).

**How the expectation was made.** It was derived by hand from the corpus text and the
locked DIAG-03 table. The starting point was the 9 would-be hits of the Phase 2 stress
run, which ran without DIAG-03. Every position was self-checked against the file bytes
before gruntled ran.

**Independence.** The classification of each row (GRT001 or silent, suffix or not) is
independent of gruntled: it comes from the HCL and the locked DIAG-03 rules. The
candidate list is not independent. It came from the Phase 2 stress run, which used
gruntled's own parser, so a breakage that run did not flag could be missing from it. The
run reports the 8 expected GRT001 and no other (`grt001=8`), so there is nothing to
classify as a false positive.

**Deviation from the original request.** The request asked for 9 GRT001 hits. The
disabled-dependency reference is silent under the locked DIAG-03 row 2
(`enabled` not literally `true`), which applies before mocks are consulted. So 8 were
expected, not 9.

**Result: all 8 are reported, and nothing else.** Gap-closure plan 02-13 added a
conservative include-target rule. When an include cannot be evaluated and its file name
may be `terragrunt.hcl`, every include-free unit may be its target, so gruntled marks it
`config-unknown: include-target` and, under DIAG-03, stays silent on it. On denis256 seven
such includes first hid all 8 references (719 of 1146 units `include-target`, 0/8). None
of the seven can name an in-repo unit: five are `find_in_parent_folders` calls with no
argument or a plain file name that find nothing inside the repository, and two
reference `local.*`, which Terragrunt 1.1.6 rejects in an include path ("Unknown
variable") because it decodes includes before locals. Only `values` is in scope there,
which the pinned binary confirms. Such includes now mark nothing, and the exact-set
assertion from the original request is back.

| `file:line:col` | dependency / output | Expected under DIAG-03 | Suffix expected | Actual in this run | Justification |
|---|---|---|---|---|---|
| `issue-2163/app/terragrunt.hcl:14:24` | `app_service_plan01` / `asp_id` | GRT001 (mock-only key) | no | reported | The module declares no outputs. `asp_id` is only mocked, and the allowed commands are `["validate", "plan"]`, so apply fails loudly instead of masking. |
| `issue-2405/app/terragrunt.hcl:16:21` | `vpc` / `vpc_id` | GRT001 (mock-only key) | yes | reported | `vpc/main.tf` is empty. `vpc_id` exists only in `mock_outputs`, and the allowed-commands list is misplaced inside the mock map, so apply would use the mock. |
| `issue-2405/app/terragrunt.hcl:17:21` | `vpc` / `private_subnets` | GRT001 (mock-only key) | yes | reported | Same fixture and facts as the row above, for `private_subnets`. |
| `issue-2631/main/terragrunt.hcl:9:9` | `dep` / `a` | GRT001 (genuine fixture bug) | no | reported | The module declares only `y`, and there is no `mock_outputs` at all. |
| `issue-2718/app/terragrunt.hcl:23:25` | `vpc_main` / `aws_subnet_public_output` | GRT001 (mock-only key) | yes | reported | `vpc/main.tf` is empty. The key exists only in `mock_outputs`. The merge and allowed-commands settings are misplaced inside the mock map, so every command gets the mocks, apply included. |
| `mock-output/module1/terragrunt.hcl:22:16` | `module2` / `subnets` | GRT001 (mock-only key) | yes | reported | `module2` declares only `hello` and `attribute`. `subnets` is only mocked, the `"shallow"` strategy merges mocks into state, and apply is allowed, so apply silently injects the mock. |
| `mocks/module1/terragrunt.hcl:10:12` | `module2` / `vpc_id2` | GRT001 (genuine fixture bug) | no | reported | A typo for `vpc_id`. `module2/main.tf` is empty, and `mock_outputs` is `yamldecode(file(...))`, which is not literal (so no suffix) and defines only `vpc_id` anyway. |
| `optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:17:12` | `vpc` / `vpc_id` | GRT001 (mock-only key) | yes | reported | `enabled = true` literally. The module has one resource and zero outputs. `vpc_id` is only mocked, and there is no allowed-commands list, so apply returns the mock. |
| `optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:18:12` | `db` / `db` | silent (DIAG-03 row 2) | no | silent | `enabled = false`: row 2 of DIAG-03 is silent before mocks are consulted. The Phase 2 stress run listed it only because it ran without DIAG-03. |

In words: 2 genuine fixture bugs (`issue-2631/main` `a`, `mocks/module1` `vpc_id2`),
6 keys that exist only in `mock_outputs` on enabled dependencies and would be reported
under "mocks never suppress", and 1 reference that is silent by DIAG-03 row 2 because its
dependency has `enabled = false`.

## VALID-03 Unmutated corpus

TestCorpusClean runs gruntled in process on the corpus and on a scratch copy:

```
summary: units=65 resolved=62 module_unknown=0 config_unknown=3 unknown_modules=0 errors=0 warnings=0
```

Exit 0, 0 diagnostics of any code. The JSON on the scratch copy is byte-identical to the
JSON on the corpus.

The shipped binary, run the way a user runs it:

```
$ go build -trimpath -o /tmp/gruntled ./cmd/gruntled
$ /tmp/gruntled check $HOME/.cache/gruntled-phase4/corpus/primary; echo "exit=$?"
gruntled: checked 65 units (3 unknown): 0 errors, 0 warnings
exit=0
```

stdout was empty (0 bytes); the line above is stderr.

```
$ git -C $HOME/.cache/gruntled-phase4/corpus/primary status --porcelain | wc -l
0
```

## VALID-04 Injected mutations

The mutations were injected. No maintained public corpus was found with an organic
instance of this bug class, because `apply` catches it before anyone commits it. Each
mutation is applied to a fresh scratch copy.

**Oracle.** The expected set is computed by a textual scan of every `terragrunt.hcl` for
`dependency.<label>.outputs.<name>`, where `<label>` is a dependency block whose
`config_path` points at the mutated module. It does not use gruntled's parser. It was
cross-checked with `grep` before any gruntled run:

```
$ grep -rn 'dependency\.s3\.outputs\.role_name\b' --include=terragrunt.hcl .
$ grep -rn 'dependency\.mq\.outputs\.region\b' --include=terragrunt.hcl .
```

which gives 8 and 3 matches, all at column 16.

### Mutation 1: rename `role_name` in `iac.src/s3_runtime`

```diff
--- a/iac.src/s3_runtime/state.tf
+++ b/iac.src/s3_runtime/state.tf
@@ -40 +40 @@
-output "role_name" {
+output "role_name_renamed" {
```

| Expected (oracle) | Actual (gruntled) |
|---|---|
| `iac.src/ecr_health/terragrunt.hcl:13:16` iac.src/ecr_health | `iac.src/ecr_health/terragrunt.hcl:13:16` iac.src/ecr_health |
| `iac.src/ecr_inbox/terragrunt.hcl:13:16` iac.src/ecr_inbox | `iac.src/ecr_inbox/terragrunt.hcl:13:16` iac.src/ecr_inbox |
| `iac.src/ecr_outbox/terragrunt.hcl:13:16` iac.src/ecr_outbox | `iac.src/ecr_outbox/terragrunt.hcl:13:16` iac.src/ecr_outbox |
| `iac.src/ecr_process/terragrunt.hcl:13:16` iac.src/ecr_process | `iac.src/ecr_process/terragrunt.hcl:13:16` iac.src/ecr_process |
| `iac.src/ecr_recover/terragrunt.hcl:13:16` iac.src/ecr_recover | `iac.src/ecr_recover/terragrunt.hcl:13:16` iac.src/ecr_recover |
| `iac.src/ecr_release/terragrunt.hcl:13:16` iac.src/ecr_release | `iac.src/ecr_release/terragrunt.hcl:13:16` iac.src/ecr_release |
| `iac.src/ecr_timeout/terragrunt.hcl:13:16` iac.src/ecr_timeout | `iac.src/ecr_timeout/terragrunt.hcl:13:16` iac.src/ecr_timeout |
| `iac.src/ecr_uuid/terragrunt.hcl:13:16` iac.src/ecr_uuid | `iac.src/ecr_uuid/terragrunt.hcl:13:16` iac.src/ecr_uuid |

8 of 8, 0 missing, 0 extra, every one GRT001 severity error. One message, verbatim:

```
iac.src/ecr_health/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
```

### Mutation 2: delete `region` from `iac.mq/mq_broker`

```diff
--- a/iac.mq/mq_broker/state.tf
+++ b/iac.mq/mq_broker/state.tf
@@ -17,4 +16,0 @@
-output "region" {
-  value = data.aws_region.this.name
-}
-
```

| Expected (oracle) | Actual (gruntled) |
|---|---|
| `iac.mq/ecr_mq_generator/terragrunt.hcl:14:16` iac.mq/ecr_mq_generator | `iac.mq/ecr_mq_generator/terragrunt.hcl:14:16` iac.mq/ecr_mq_generator |
| `iac.mq/ecr_mq_reader/terragrunt.hcl:14:16` iac.mq/ecr_mq_reader | `iac.mq/ecr_mq_reader/terragrunt.hcl:14:16` iac.mq/ecr_mq_reader |
| `iac.mq/ecr_mq_writer/terragrunt.hcl:14:16` iac.mq/ecr_mq_writer | `iac.mq/ecr_mq_writer/terragrunt.hcl:14:16` iac.mq/ecr_mq_writer |

3 of 3, 0 missing, 0 extra, every one GRT001 severity error. One message, verbatim:

```
iac.mq/ecr_mq_generator/terragrunt.hcl:14:16: GRT001 dependency "mq" output "region" is not declared by module "iac.mq/mq_broker" (target unit "iac.mq/mq_broker"); mock_outputs supplies it, so apply would silently use the mock value
```

After each mutation the test restores the original bytes and runs gruntled again: exit 0,
0 diagnostics, JSON byte-identical to the state before the mutation.

## VALID-05 terragrunt hcl validate on the same mutated trees

Command, run with `cmd.Dir` set to the scratch copy:

```
terragrunt hcl validate --working-dir <copy> --no-color
```

Environment: exactly `PATH=<pinned bin dir>`, `HOME=<temp dir>`, `TMPDIR=<temp dir>`,
`TG_NON_INTERACTIVE=true`. Nothing is inherited, so no `TG_*` variable can change the
command.

| Mutation | terragrunt, unmutated copy | terragrunt, mutated copy | Output lines added by the mutation | gruntled on the same mutated copy |
|---|---|---|---|---|
| rename `role_name` | exit 0, 12 lines | exit 0, 12 lines | none | exit 1, 8 GRT001 |
| delete `region` | exit 0, 12 lines | exit 0, 12 lines | none | exit 1, 3 GRT001 |

The output is compared after normalization (root dir replaced, ANSI escapes and the
leading timestamp removed). The 12 lines are the same on the unmutated and mutated
copies. They are warnings the corpus always produces: one about two `dependency` blocks
addressing the same dependency, and 11 of the form "Config .../s3_runtime/terragrunt.hcl
is a dependency of .../ecr_health/terragrunt.hcl that has no outputs, but mock outputs
provided". Those 11 come from the absence of state, not from the modules' declared
outputs, so they are identical before and after the mutation. terragrunt wrote nothing
into either copy (tree digest unchanged).

## VALID-06 Benchmark

**Method.** TestBenchmarkVsTerragrunt builds the gruntled binary with
`go build -trimpath` and times two external processes on the same scratch copy of the
corpus, with the same environment (the minimal one from VALID-05) and the same working
directory:

```
gruntled check <copy>
terragrunt hcl validate --working-dir <copy> --no-color
```

Each tool runs 3 untimed warm-ups (which also warm the page cache for both). Then 21
samples of each are taken, interleaved: on even rounds gruntled runs first, on odd rounds
terragrunt runs first, so drift and thermal effects hit both. Each run is timed with
`time.Now`/`time.Since` around `cmd.Run`. Every run must exit 0, or the test fails, so
an erroring run can never count as a fast one. The verdict compares medians. The p25 and
p75 are nearest-rank quantiles.

**Runs.** The plan allows at most 3 runs of the benchmark test, all reported, with the
majority as the verdict. It was run exactly 3 times:

1. Run 1, the harness check when the test was written (load average 8.80 on 4 CPUs).
2. Run 2, inside the full experiment run above (load average 17.06).
3. Run 3, after waiting for the load average to fall below 3 (3.78 when it finished).

The third run was taken because runs 1 and 2 were on a heavily loaded machine, not
because of the verdict, which was the same in all three. No run was discarded.

| Run | Tool | min ms | p25 ms | median ms | p75 ms | max ms | terragrunt / gruntled median | Verdict |
|---|---|---|---|---|---|---|---|---|
| 1 | gruntled | 56.459 | 205.590 | 233.311 | 277.410 | 548.180 | 2.81 | PASS |
| 1 | terragrunt | 353.467 | 493.831 | 656.260 | 800.808 | 1451.640 | | |
| 2 | gruntled | 191.782 | 253.544 | 300.006 | 359.944 | 510.411 | 2.13 | PASS |
| 2 | terragrunt | 425.783 | 496.115 | 639.756 | 856.746 | 1540.056 | | |
| 3 | gruntled | 88.638 | 115.804 | 150.653 | 185.111 | 293.673 | 1.99 | PASS |
| 3 | terragrunt | 234.099 | 275.546 | 299.080 | 424.680 | 837.667 | | |

Verdict: PASS in 3 of 3 runs. gruntled's median was 1.99 to 2.81 times lower.

Full sorted sample lists in milliseconds, verbatim from the `VALIDATION-BENCH` lines:

```
run 1 loadavg=8.80 5.99 4.51 6/1062 559454
run 1 gruntled_samples_ms=56.459,64.764,162.258,200.516,201.947,205.590,213.426,215.166,216.521,216.673,233.311,240.277,254.069,257.588,266.658,277.410,281.595,281.610,334.401,365.370,548.180
run 1 terragrunt_samples_ms=353.467,364.779,378.316,436.201,445.585,493.831,538.534,546.770,611.542,640.918,656.260,694.662,707.302,726.546,730.465,800.808,916.729,966.018,971.542,1261.582,1451.640
run 2 loadavg=17.06 9.09 5.70 8/1064 561484
run 2 gruntled_samples_ms=191.782,196.089,213.546,237.413,244.139,253.544,259.064,264.676,286.013,292.767,300.006,321.659,324.260,331.384,352.901,359.944,382.587,390.395,416.102,426.201,510.411
run 2 terragrunt_samples_ms=425.783,432.169,476.089,490.542,495.824,496.115,516.591,528.367,537.844,615.809,639.756,646.352,738.359,753.423,825.030,856.746,1040.475,1101.875,1116.087,1304.119,1540.056
run 3 loadavg=3.78 4.76 5.67 4/1010 569080
run 3 gruntled_samples_ms=88.638,94.881,103.708,104.965,111.087,115.804,116.803,118.844,129.439,135.697,150.653,151.625,155.062,179.543,183.031,185.111,192.423,199.893,201.671,221.823,293.673
run 3 terragrunt_samples_ms=234.099,244.718,245.010,248.863,264.425,275.546,282.192,286.689,292.908,295.727,299.080,305.678,310.917,343.628,388.005,424.680,456.040,483.963,579.139,705.755,837.667
```

**Supplementary `go test -bench`** (`-benchtime=20x`, load average about 4.1, right
after run 3):

```
BenchmarkGruntledCheckCorpus-4     	      20	 157551111 ns/op
BenchmarkTerragruntHCLValidate-4   	      20	 274757333 ns/op
```

These are not the VALID-06 comparison. The gruntled number is in process, without process
start; the terragrunt number is a whole process. They exist so `go test -bench` users get
ns/op figures.

**Comparison with the PREP baseline.** PREP.md §3 measured plain `hcl validate` on this
corpus at a mean of 0.148 s over 5 runs of `/usr/bin/time`. That measurement is not
directly comparable. PREP ran with 12 logical CPUs and 7.6 GiB of RAM visible to WSL2;
this run had 4 logical CPUs and 5.8 GiB (the WSL2 limits changed between the two), and
this machine was never idle. The least loaded run here (run 3) gave terragrunt a median
of 0.299 s, about twice PREP's figure. Both tools slowed down together under load, and
the ratio stayed between 1.99 and 2.81 in every run.

**Caveats.**

- Small corpus: 65 units. Process start and one HCL walk dominate both tools, so this
  measures the overhead of a typical small repository, not scaling.
- One machine, under WSL2, and busy in every run. The absolute numbers vary by a factor
  of two between runs; only the relative result was stable.
- Wall time only. CPU time and memory were not measured.
- terragrunt's `hcl validate` does a different job (it parses and validates all HCL),
  so this compares the two commands a user would run, not the same algorithm.

## Reproduce

The corpora can be fetched in one step: `bash scripts/fetch-corpora.sh` fetches all three
at their pinned commits into `~/.cache/gruntled-phase4/corpus` (verified, ELF binaries
stripped) and prints the `export` lines. The manual steps below remain as a fallback.

1. Download terragrunt v1.1.6 and check its SHA256:

   ```
   mkdir -p ~/.cache/gruntled-phase4/bin && cd ~/.cache/gruntled-phase4/bin
   curl -fsSLo terragrunt_linux_amd64 https://github.com/gruntwork-io/terragrunt/releases/download/v1.1.6/terragrunt_linux_amd64
   echo "d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04  terragrunt_linux_amd64" | sha256sum -c
   chmod +x terragrunt_linux_amd64 && ln -sf terragrunt_linux_amd64 terragrunt
   ```

2. Clone the primary corpus at the pinned commit (detached):

   ```
   git clone https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws ~/.cache/gruntled-phase4/corpus/primary
   git -C ~/.cache/gruntled-phase4/corpus/primary checkout --detach e6c55d11fd1a01e75b78d7897be36c69fa26b8cc
   ```

3. Export the two variables:

   ```
   export GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary
   export GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt
   ```

4. Optionally, the secondary corpus:

   ```
   git clone https://github.com/denis256/terragrunt-tests ~/.cache/gruntled-phase4/corpus/denis256
   git -C ~/.cache/gruntled-phase4/corpus/denis256 reset --hard 726485e699a70c02dabbde629f66c0119e197357
   export GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256
   ```

5. From the gruntled repository root, run:

   ```
   go test -count=1 -v -run 'TestGolden' ./cmd/gruntled
   go test -count=1 -v -run 'TestCorpusClean|TestCorpusMutation|TestTerragruntGap|TestDenis256Corpus|TestBenchmarkVsTerragrunt' ./cmd/gruntled
   go test -count=1 -run '^$' -bench 'GruntledCheckCorpus|TerragruntHCLValidate' -benchtime=20x ./cmd/gruntled
   go build -trimpath -o /tmp/gruntled ./cmd/gruntled && /tmp/gruntled check "$GRUNTLED_CORPUS"; echo "exit=$?"
   ```

The corpus, terragrunt and benchmark tests skip when their variables are unset, and they
are not part of CI by design. The golden tests need no external asset and run in CI on
every push.

## Limitations

- One primary corpus of 65 units (summary.units in this run). A second real corpus with
  terragrunt-compatible syntax was not found; see [Corpus](#corpus).
- Two mutation kinds: one rename and one deletion, each in a different module.
- The denis256 check is a single secondary exact-set run, not a zero-false-positive
  claim. The include-target rule still marks 54 of its 1146 units unknown.
- GRT001 only. Units that gruntled marks unknown are not checked (the Phase 2 policy).
  In this run that was 3 of 65 units on the primary corpus and 294 of 1146 on denis256.
- The timing claim is relative, on one machine only, under the load described above. The
  absolute numbers are not a performance specification.

## v0.2: GRT002 and GRT003 on the real corpus (MORE-06)

The claim has three parts. On the unmutated corpus, gruntled reports no GRT002 or GRT003
on iso20022 and cds-snc/secret, and on denis256 (a suite of deliberately broken
reproductions) exactly the set an independent oracle derives. Every injected graph error
(a missing `config_path` directory, a missing `dependencies` path, a back edge closing a
cycle, a self-loop, and on iso20022 a module-only directory) adds exactly one expected
diagnostic and nothing else. An independent oracle agrees on each result.

### Outcome

| Claim | Result |
|---|---|
| iso20022 unmutated: zero GRT002/GRT003, GRT001 baseline unchanged (65 units, 3 unknown, 0 diagnostics, exit 0) | PASS |
| secret unmutated: zero GRT002/GRT003 (4 units, 1 unknown, 0 diagnostics, exit 0) | PASS |
| denis256 unmutated: GRT002/GRT003 set equals `denisExpectedGraph` exactly (4 GRT002, 0 GRT003), GRT001 still exactly 8 | PASS |
| 13 mutations (5 iso20022, 4 secret, 4 denis256): each adds exactly its diagnostic, exit 1, every other diagnostic unchanged, revert gives byte-identical JSON | PASS, 13/13 |
| Textual oracle agrees on the unmutated sets and on every mutation delta | PASS, with 12 documented oracle-only findings on denis256 (below) |
| Pinned terragrunt v1.1.6 agrees on every mutation | PASS, 13/13 (denis256 on the fixture subtree) |

Recorded run: 2026-09-30, gruntled commit `d05fd1af101fc68f2035f9f1e225033bcfa2b46f`,
go1.27.0 linux/amd64, same WSL2 machine as above. Test wall time: 57 s.

### Pinned commits

| Corpus | Repository | Commit | Variable |
|---|---|---|---|
| iso20022 (primary) | aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws | `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc` | `GRUNTLED_CORPUS` |
| secret | cds-snc/secret | `341e8a95b0d9bd658793094a248527cc3ebae6f2` | `GRUNTLED_CORPUS_SECRET` |
| denis256 | denis256/terragrunt-tests | `726485e699a70c02dabbde629f66c0119e197357` | `GRUNTLED_CORPUS_DENIS256` |

Each test fails (not skips) when a checkout is at another commit. The checkouts are never
written: every mutation and every terragrunt run happens on a scratch copy, and a digest
of each checkout is compared before and after.

### Command

```
GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary \
GRUNTLED_CORPUS_SECRET=$HOME/.cache/gruntled-phase4/corpus/secret \
GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256 \
GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt_linux_amd64 \
go test -count=1 -v -run 'TestCorpus|TestDenis256|TestSecret' ./cmd/gruntled
```

`TestCorpusGraphClean` and `TestCorpusGraphMutation` are the v0.2 tests; the same command
reruns the v0.1 corpus tests. Without the variables every one of them skips.

### Oracles

**Pinned terragrunt v1.1.6** (the SHA256 above, checked on every run). The PATH
`terragrunt` on this machine is v0.99.3 and is never used. The command, run in a scratch
copy with a minimal environment (`PATH` = the pinned bin dir, fresh `HOME` and `TMPDIR`):

```
terragrunt run --all --non-interactive --no-auto-init --no-color --tf-path <bin>/tofu -- version
```

The signal is the queue-construction error, not the exit code:

- missing or config-less target: `You attempted to run terragrunt in a folder that does
  not contain a terragrunt.hcl file ...` followed by `Path: "<copy>/<target>/terragrunt.hcl"`;
- cycle or self-loop: `ERROR  cycle detected during queue construction`.

Both stop terragrunt before any unit runs (0 units print a tofu version). On the unmutated
copies neither message appears; the queue is built and units run `version` (36 on
iso20022, 2 on secret, 0 on the denis256 subtree). The exit code is still 1 there, for
reasons outside the graph: units whose inputs read `dependency.X.outputs` fail with
`There is no variable named "dependency"` because there is no state. `--no-auto-init`
keeps tofu from prompting for the S3 backend.

- `terragrunt hcl validate` is not an oracle for these codes: it exits 0 with a missing
  `config_path` directory. `dag graph` only prints a WARN on a cycle and exits 0.
- secret's unnamed `include {}`: `run --all -- version` handles it on v1.1.6 (queue built,
  2 units ran); no renaming was needed, so the terragrunt oracle runs on the unmodified
  secret tree too.
- denis256 as a whole cannot be queued: terragrunt fails first on stack and function
  errors unrelated to the graph (for example `get_repo_root` in `6288/stacks` outside a git
  repository, `values.env` in `6289`). Its mutations are all in `issue-2565`, so terragrunt
  runs on that subtree of the mutated copy; the textual oracle covers the whole tree.

**Textual oracle** (`graphOracle` in `cmd/gruntled/corpus_graph_test.go`). It shares no
code with gruntled's parser, loader or analyzers:

- a byte scanner that masks strings (with their `${...}` templates), comments and
  heredocs, plus regexps for top-level `dependency "x" {`, `dependencies {` and
  `include {` blocks;
- include targets it can resolve: a literal path, a `${get_terragrunt_dir()}/...` path,
  `find_in_parent_folders("name")` and `find_in_parent_folders()` (as `terragrunt.hcl`),
  searched from the parent directory up; `no_merge` includes contribute nothing; a label
  in the unit file overrides the same label in an include;
- only literal `config_path` values and literal `paths` entries, optionally prefixed by
  `${get_terragrunt_dir()}`, resolved with `path.Join` against the unit directory; a
  block whose `enabled` is not literally `true` is dropped; empty, absolute, outside the
  repository or `*.hcl`/`*.json` targets are dropped;
- targets classified with `os.Stat` (missing dir; dir without `terragrunt.hcl` or
  `terragrunt.hcl.json`; unit);
- cycles by mutual reachability (one BFS per node), not Tarjan; ring vs `among` and the
  smallest-unit attribution follow the documented message rules.

Everything the oracle leaves out is counted and logged. On denis256: 63 non-literal
`config_path`, 8 unresolvable, 6 disabled blocks, 26 non-literal and 5 unresolvable paths
entries, 7 includes it cannot resolve, 5 `find_in_parent_folders` that find nothing, 1
unreadable include, 1 `terragrunt.hcl.json` unit not scanned. On iso20022 and secret it
leaves nothing out.

**tsort.** The oracle's unit pairs are also fed to `tsort` (uutils coreutils 0.8.0 on
this machine; self-pairs dropped because tsort treats `a a` as a node): no loop on the
three unmutated corpora (117, 3 and 660 edges), `input contains a loop` after each back-edge
mutation.

### Unmutated results

| Corpus | Units | GRT002 | GRT003 | Oracle | terragrunt |
|---|---|---|---|---|---|
| iso20022 | 65 (62 resolved, 3 config-unknown) | 0 | 0 | 0 findings, 117 edges | queue built, 36 units ran |
| secret | 4 (3 resolved, 1 config-unknown) | 0 | 0 | 0 findings, 3 edges | queue built, 2 units ran |
| denis256 | 1146 (852 resolved, 75 config-unknown, 219 module-unknown) | 4 | 0 | 16 findings, 660 edges | cannot queue the whole repository (see Oracles) |

denis256, the exact set (`denisExpectedGraph`), every entry checked by hand:

| Position | Message | Why it is right |
|---|---|---|
| `5728-broken-includes/test.hcl:1:37` | `dependency "borked" config_path resolves to "5728-broken-includes/not-here": directory does not exist` | Reproduction of "inclusion of broken dependencies": the included `test.hcl` points `${get_terragrunt_dir()}/not-here` at nothing on purpose; reported for the including unit at the include's position |
| `hcl/terragrunt.hcl:19:17` | `dependency "vpc" config_path resolves to "vpc": directory does not exist` | `../vpc` from `hcl` is the repository root's `vpc`, which does not exist |
| `module-output-broken/m1/terragrunt.hcl:3:17` | `dependency "m2" config_path resolves to "module-output-broken/m2": directory does not exist` | The fixture holds only `app` and `m1` |
| `tf-lint-regeneration/dev/template/terragrunt.hcl:24:45` | `dependency "vpc" config_path resolves to "tf-lint-regeneration/vpc": directory does not exist` | A template that `copy-app.sh` copies to `dev/apps/app-N`, where `../../vpc` exists (all 100 copies are silent); in place it points at nothing. `skip_outputs = "true"` does not gate GRT002 |

The oracle reports 12 more (`denisGraphOracleOnly`), all `directory does not exist` at
column 45: `perf-tests/code-v2/app-template/terragrunt.hcl` and
`perf-tests-v2/test/app-template/terragrunt.hcl` lines 6, 13, 20, 27, 34 (`../../deps/dep-1`
to `dep-5`), and `perf-tests/code-v2/dependency-template/terragrunt.hcl:6` and
`perf-tests-v2/test/dependency-template/terragrunt.hcl:6` (`../common`). The four units are
config-unknown (`include-dynamic-path`: `find_in_parent_folders()` finds no parent
`terragrunt.hcl`), and a config-unknown unit carries no dependencies in gruntled. They are
templates that `init.sh` copies under `code/`, where the targets exist. Terragrunt would
fail on them in place, so these are known false negatives of gruntled's conservative
rule, never false positives. There is no other disagreement.

### Mutations

Each row is applied alone to a fresh copy. File and position are in the copy.
"Synthetic" means the mutation adds a block or a list entry rather than editing an
existing value.

| Repo | Mutation | Expected (and gruntled) diagnostic | Synthetic | Textual oracle | terragrunt v1.1.6 |
|---|---|---|---|---|---|
| iso20022 | `iac.src/ecr_health` `config_path` `../s3_runtime` -> `../s3_runtime_gone` | GRT002 `iac.src/ecr_health/terragrunt.hcl:2:18` `dependency "s3" config_path resolves to "iac.src/s3_runtime_gone": directory does not exist` | no | same, exactly | `Path: ".../iac.src/s3_runtime_gone/terragrunt.hcl"` |
| iso20022 | `dependencies { paths = ["../gone_unit"] }` in `ecr_health` | GRT002 `iac.src/ecr_health/terragrunt.hcl:13:12` `dependencies path "../gone_unit" resolves to "iac.src/gone_unit": directory does not exist` | yes (iso20022 has no `dependencies` block) | same | `Path: ".../iac.src/gone_unit/terragrunt.hcl"` |
| iso20022 | `dependency "back" { config_path = "../lambda_health" }` in `ecr_health` (`lambda_health` already depends on it) | GRT003 `iac.src/ecr_health/terragrunt.hcl:13:17` `dependency cycle: "iac.src/ecr_health" -> "iac.src/lambda_health" -> "iac.src/ecr_health"` | yes | same; tsort: loop | `cycle detected during queue construction` |
| iso20022 | `dependency "self" { config_path = "../ecr_health" }` in `ecr_health` | GRT003 `iac.src/ecr_health/terragrunt.hcl:13:17` `dependency cycle: "iac.src/ecr_health" -> "iac.src/ecr_health"` | yes | same | `cycle detected during queue construction` |
| iso20022 | `ecr_health` `config_path` -> `../s3_crr` (module only: `.tf` files, no `terragrunt.hcl`) | GRT002 `iac.src/ecr_health/terragrunt.hcl:2:18` `dependency "s3" config_path resolves to "iac.src/s3_crr": directory has no terragrunt.hcl` | no | same | `Path: ".../iac.src/s3_crr/terragrunt.hcl"` |
| secret | `terragrunt/ecr` `config_path = "../acm"` -> `"../acm_gone"` | GRT002 `terragrunt/ecr/terragrunt.hcl:12:17` `dependency "acm" config_path resolves to "terragrunt/acm_gone": directory does not exist` | no | same | `Path: ".../terragrunt/acm_gone/terragrunt.hcl"` |
| secret | `ecr` `paths = ["../acm"]` -> `["../acm", "../gone"]` | GRT002 `terragrunt/ecr/terragrunt.hcl:8:22` `dependencies path "../gone" resolves to "terragrunt/gone": directory does not exist` | yes (entry added to a real block) | same | `Path: ".../terragrunt/gone/terragrunt.hcl"` |
| secret | `dependency "back" { config_path = "../ecr" }` in `terragrunt/acm` | GRT003 `terragrunt/acm/terragrunt.hcl:8:17` `dependency cycle: "terragrunt/acm" -> "terragrunt/ecr" -> "terragrunt/acm"` | yes | same; tsort: loop | `cycle detected during queue construction` |
| secret | `dependency "self" { config_path = "../ecr" }` in `terragrunt/ecr` | GRT003 `terragrunt/ecr/terragrunt.hcl:8:17` `dependency cycle: "terragrunt/ecr" -> "terragrunt/ecr"` | yes | same | `cycle detected during queue construction` |
| denis256 | `issue-2565/B` `config_path = "../A"` -> `"../A_gone"` | GRT002 `issue-2565/B/terragrunt.hcl:5:17` `dependency "A" config_path resolves to "issue-2565/A_gone": directory does not exist` | no | same | `Path: ".../issue-2565/A_gone/terragrunt.hcl"` (subtree) |
| denis256 | `dependencies { paths = ["../gone"] }` in `issue-2565/B` | GRT002 `issue-2565/B/terragrunt.hcl:5:12` `dependencies path "../gone" resolves to "issue-2565/gone": directory does not exist` | yes | same | `Path: ".../issue-2565/gone/terragrunt.hcl"` (subtree) |
| denis256 | `dependency "C" { config_path = "../C" }` in `issue-2565/A` (C -> B -> A exists) | GRT003 `issue-2565/A/terragrunt.hcl:5:17` `dependency cycle: "issue-2565/A" -> "issue-2565/C" -> "issue-2565/B" -> "issue-2565/A"` | yes | same; tsort: loop | `cycle detected during queue construction` (subtree) |
| denis256 | `issue-2565/B` `config_path = "../A"` -> `"../B"` | GRT003 `issue-2565/B/terragrunt.hcl:5:17` `dependency cycle: "issue-2565/B" -> "issue-2565/B"` | no | same | `cycle detected during queue construction` (subtree) |

For every row: exit 1; every GRT001 and GRT100 diagnostic identical to the baseline
(iso20022 and secret have none; denis256 keeps its 8 GRT001 and 8 GRT100); writing the
original bytes back gives JSON byte-identical to the baseline. The mutated blocks have no
`dependency.X.outputs` references, except the retargets of a referenced block (iso20022
`dependency "s3"` twice, secret `dependency "acm"` in `ecr`): those references then point
at a non-unit, which is DIAG-03 row 4 (silent), and they resolved before the mutation, so
GRT001 stays at zero either way. The denis256 `issue-2565/B` block has no references.

The denis256 copy leaves out the 29 ELF terragrunt binaries the repository checks in
(1.85 GB, more than the tmpfs holds). gruntled never reads them; fidelity is proven by
requiring gruntled's JSON on the copy to equal the JSON on the checkout, byte for byte.

### Resolved: `config_path = ""` (plan 05-07)

Through 05-06 gruntled resolved an empty block `config_path` to the unit's own directory
and reported a GRT003 self-loop, and kept a `dependencies { paths }` entry `""` unresolved
and silent. The pinned terragrunt v1.1.6 does the opposite on both. Two-unit scratch trees
(`a` holding the case, `b` empty), `run --all --non-interactive --no-auto-init -- version`:

| Case in `a/terragrunt.hcl` | terragrunt v1.1.6 | gruntled since 05-07 |
|----------------------------|-------------------|----------------------|
| `dependency "x" { config_path = "" }` | `ERROR  skipping dependency "x" in "<tree>/a": config_path could not be resolved`, no cycle message (exit 1; a single-unit `run` in `a` exits 0) | unresolved (`config-path-empty`), silent: no edge, no GRT002, no GRT003 |
| `dependencies { paths = [""] }` | `ERROR  cycle detected during queue construction` | self-edge: GRT003 `dependency cycle: "a" -> "a"` at the `""` entry |
| `dependencies { paths = ["../b", ""] }` | `ERROR  cycle detected during queue construction` | edge to `b` plus the self-edge: GRT003 at the `""` entry, no GRT002 (`b` is a unit) |

gruntled now matches terragrunt per case: the empty check applies to dependency blocks
only, while a paths entry `""` resolves to the unit's own directory, like `"."`. The
`grt002_missing_target` golden pins both (silent block, GRT003 on the paths entry), and
the loader tests cover the plain, `"${""}"` and include-inherited forms. No corpus holds
an active empty `config_path` or paths entry (the only two `config_path = ""`, in denis256
`deep-merge-fix/common.hcl`, are commented out), so the corpus counts are unchanged.

### The include-merge correction

`dependencies { paths }` from includes are a union in both shallow and deep merge, not
"highest-precedence block wins" (terragrunt v1.1.6 `pkg/config/include.go` `Merge` calls
`ModuleDependencies.Merge`, which appends absent paths; `DeepMerge` also unions). gruntled
implements the union with de-duplication by target, and the textual oracle unions
independently.

One corpus unit exercises it: denis256 `render-json/dependencies/app` declares
`paths = ["../d1"]` and includes `include.hcl`, which declares `paths = ["../d2"]`. Both
the oracle and gruntled give the unit two edges (d1 and d2); both targets exist, so
nothing is reported. On a scratch copy of that fixture with `d2` removed, gruntled
reports `dependencies/app/include.hcl:2:12: GRT002 dependencies path "../d2" resolves to
"dependencies/d2": directory does not exist`, and the pinned terragrunt (run on the
fixture) builds the queue and then fails with
`Found paths in the 'dependencies' block that do not exist: [../d2 (<tree>/dependencies/d2)]`.
The inherited path is kept, as the union says; with "highest-precedence block wins" it
would have been dropped. Note the different terragrunt message: an inherited missing path
fails at run time, a unit's own missing path fails during queue construction.

### Limitations found on the corpus

- **Config-unknown units hide dependencies.** 3 of 65 iso20022 units, 1 of 4 secret and
  75 of 1146 denis256 units carry no edges. On denis256 this hides the 12 oracle-only
  GRT002 above; a cycle through such a unit would be hidden the same way. No cycle exists
  in the oracle's graph of the three corpora.
- **Symlink aliases are not canonicalised.** iso20022 has 68 symlinks, all `global.tf`
  files. denis256 has 2 directory symlinks (`pr-3562/fixture2/t1` and `t2` ->
  `../template`) and 4 symlinked `terragrunt.hcl`. None is part of a finding or of a
  cycle; aliasing can only cause false negatives.
- **`exclude {}` is not modelled.** iso20022 and secret have none. denis256 has 11
  `exclude {` blocks (7 in `terragrunt.hcl` under `feature-flags/` and
  `stacks-test/stack-feature-flag/units/`, 3 in `feature-flags/exclude-example/*/environment.hcl`,
  1 in a `terragrunt.stack.hcl`). None of those units is in the GRT002/GRT003 set, so the
  limitation does not change this result.
- **Mutation coverage.** 13 mutations of four kinds (plus module-only on iso20022), in one
  unit per repository. Shared-include attribution and paths-list merging are covered by
  goldens, not by the corpus.

## v0.4 daemon memory (DAEMON-10)

Since v0.4 the watch daemon keeps a blast baseline: the `checking.Report` of its first
index (or of the last `gruntled rebase`), next to the last Report and the served snapshot,
which carries the pre-rendered check output and the blast view. The baseline is a Report
only, never a loader, parse cache or file system (TestBaselineHoldsOnlyReport). This
section records what that costs.

**Method.** TestDaemonMemorySynthrepo (`cmd/gruntled/daemon_mem_test.go`) generates a
synthrepo tree (`synthrepo.Spec{Units: n, IncludeDepth: 3, DependencyFanout: 3, Seed: 16}`)
and measures `HeapInuse` after two `runtime.GC()` calls:

- `report_heap`: one Report, indexed through the daemon's path (`watching.Indexer` over
  one invalidating loader on an `os.Root`), with the indexer dropped;
- `publisher_heap`: a publisher that published that Report, then the Report of one edit
  (an output of `unit-000` renamed, reindexed with that dirty path), holding the baseline,
  the last Report and the served snapshot sealed as the daemon seals it (`ipc.Seal`);
- `view_bytes`: the blast view's text plus JSON bytes.

The test asserts `publisher_heap <= 2.5 x report_heap + view_bytes + 1 MiB`, which a pinned
parse cache (many times one Report) would break. The 5,000-unit size is skipped under
`-short`.

**Result.** One local run, on its own (`free -m` showed 6.9 GB available):

| Units | Report heap | Publisher heap | Publisher / Report | View bytes | Report heap per unit |
|---:|---:|---:|---:|---:|---:|
| 65 | 344 KiB | 856 KiB | 2.49 | 20,424 | 5.4 KB |
| 500 | 1.12 MiB | 2.70 MiB | 2.41 | 151,221 | 2.3 KB |
| 5,000 | 10.9 MiB | 25.9 MiB | 2.37 | 1,609,736 | 2.3 KB |

```
VALIDATION-BENCH goos=linux goarch=amd64
VALIDATION-BENCH cpus=8
VALIDATION-BENCH go=go1.27.2
VALIDATION-BENCH cpu_model=Intel(R) Core(TM) Ultra 7 155H
VALIDATION-BENCH mem_total=8132820 kB
VALIDATION-BENCH kernel=6.6.87.2-microsoft-standard-WSL2
VALIDATION-BENCH loadavg=0.19 0.27 0.38 2/246 146219
VALIDATION-BENCH daemon_mem units=65 report_heap=352256 publisher_heap=876544 view_bytes=20424 ratio=2.49
VALIDATION-BENCH daemon_mem units=500 report_heap=1171456 publisher_heap=2826240 view_bytes=151221 ratio=2.41
VALIDATION-BENCH daemon_mem units=5000 report_heap=11460608 publisher_heap=27172864 view_bytes=1609736 ratio=2.37
```

The ARCHITECTURE estimate was about 1-3 KB per unit for the baseline: measured, one
Report costs about 2.3 KB per unit from 500 units up (the 65-unit figure is dominated by
fixed overhead), so the baseline is about 11 MiB at 5,000 units. The whole publisher state
is two Reports plus the snapshot (check output, blast view and its sealed response line),
about 2.4 x one Report.

The worst-case blast view is bounded separately. TestReportBlastSizeBound (BLAST-11, CI,
skipped under `-short`) runs the built daemon on the 5,000-unit chain whose seed module
changes one attribute of a ~4 MiB variable type. `report --blast` stays within the
`blast` byte bounds, and the daemon's peak RSS stays within an in-run control daemon's
+ 64 x the type bytes. In the same local run: control 627 MiB, big 635 MiB, bound
883 MiB; text 10,234,273 bytes and JSON 11,384,568 bytes, under the 64 MiB response cap.

**Command.**

```sh
free -m   # at least 4 GB available; run it alone
GOMAXPROCS=2 GOFLAGS=-p=2 timeout 900 go test -count=1 -run TestDaemonMemorySynthrepo -v ./cmd/gruntled/
```

## v0.4: blast on the real corpora (REL-03)

The claim has three parts, on each of the three pinned corpora:

1. `check` is byte-identical to v0.3.0, and `blast --base` of a tree against itself is empty.
2. Reformat-only changes give an empty blast.
3. Every mutation of a fixed set is caught with exact equality: Broken units and GRT004
   sites, Impacted units with distance, source and via, and `changes[]` with its name
   lists and `type_changes`.

The expected results come from an independent textual model of each corpus, which shares
no code with gruntled. They never come from gruntled's own output.

### Outcome

| Claim | Result | Evidence |
|---|---|---|
| `check` text, JSON and SARIF and `graph --json` byte-identical to v0.3.0 (stdout, stderr, exit) | PASS: 256/256 identical, corpus digests unchanged | `scripts/compare-ref.sh` with `COMPARE_REF_CORPORA` |
| `blast --base X X` and `blast --base <copy> X` are empty (text and JSON) | PASS on all three | TestCorpusBlastSelf |
| The independent model agrees with `graph --json` except for pinned, predicate-checked exclusions | PASS: 110 rows; included modules below | TestCorpusBlastModel |
| Reformat-only transforms R1-R9 give exit 0 with Broken, Impacted and `changes` all empty | PASS on all three, guard green | TestCorpusReformat |
| Mutations M1-M7 and M9 caught with exact equality; M1 confirmed by v0.3.0 | PASS: 4,345 of 4,345 | TestCorpusMutation17 |
| Propagation to distance 2 and more (M8) | PASS: up to distance 5 on denis256 | TestCorpusMutation17/<corpus>/M8 |
| The daemon path (`report --blast`, `rebase`) agrees with `blast --base` | PASS: one site per class, R8, the shared `global.tf` edit and one rebase cycle per corpus | TestCorpusDaemonSample |

Exact equality has four named exceptions:

- type_changes.old is compared exactly for an absent type (any) and for the primitives string, number and bool; for any other type it is only checked to be non-empty and different from new.
- GRT004 messages are compared by prefix; the M1 dual oracle against v0.3.0 compares the full message after rewriting `was removed from` to `is not declared by`.
- Text output is checked by tokens once per class; the JSON document is the authority and is compared exactly.
- The M5 SENTINEL canonical string is pinned from HEAD after a hand check against the D-15 rendering rules.

### Pinned commits

| Corpus | Repository | Commit | Variable |
|---|---|---|---|
| iso20022 | aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws | `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc` | `GRUNTLED_CORPUS` |
| secret | cds-snc/secret | `341e8a95b0d9bd658793094a248527cc3ebae6f2` | `GRUNTLED_CORPUS_SECRET` |
| denis256 | denis256/terragrunt-tests | `726485e699a70c02dabbde629f66c0119e197357` | `GRUNTLED_CORPUS_DENIS256` |

The reference binary is v0.3.0 at `e870e2984760f0a674bc87697e3f69329613758e`.

### Command

```sh
bash scripts/fetch-corpora.sh            # once, the only network step
export GRUNTLED_CORPUS=… GRUNTLED_CORPUS_SECRET=… GRUNTLED_CORPUS_DENIS256=…   # its export lines
bash scripts/corpus-gate.sh              # offline, serial, about 50 minutes
```

`scripts/fetch-corpora.sh`:
- fetches each corpus by its sha, with git isolated from system, global and template
  config;
- verifies HEAD;
- fails on escaping symlinks;
- deletes the checked-in ELF binaries of denis256 (30 files, 1,856,441,161 bytes).

`scripts/corpus-gate.sh`:
- re-executes itself under `unshare -rn` and proves the isolation from `/proc/net/dev`;
- builds v0.3.0 from the pinned tag commit;
- runs 31 shards one at a time, under a memory floor of 1500 MB and an address-space cap.

`.github/workflows/corpus.yml` runs the same two scripts on demand and weekly. `ci.yml`
does not run them.

### The independent model and its exclusions

The model (`cmd/gruntled/corpus_model_test.go`) is computed from source text. It finds:
- units;
- `terraform.source`, with the `//` subdirectory split, plus Terragrunt's documented path
  functions;
- live `dependency` edges: `enabled` absent or literally true, `skip_outputs` absent or
  literally false, and a target that is a unit;
- reference sites, classified plain, guarded or other;
- module surfaces;
- a BFS whose via is the smallest predecessor one level closer.

Every difference with `graph --json` must be covered by a pinned row: path, category,
direction and file:line evidence. The test re-evaluates each row's category as a predicate
over source text, binds it to the reason gruntled itself gives, and fails on an uncovered
difference, a stale row or a changed count. An exclusion removes the item from the
universe, and every mutation whose module, seeds or cone touches it. It never copies
gruntled's value into the model.

The per-item rows live in test code (`corpusExclusions`). Totals per category:

| Category | iso20022 | secret | denis256 | Total |
|---|---:|---:|---:|---:|
| `include target` | 0 | 1 | 52 | 53 |
| `generate block may declare outputs` | 0 | 0 | 21 | 21 |
| `reference shape other` | 0 | 0 | 16 | 16 |
| `module syntax error the oracle does not detect` | 0 | 0 | 4 | 4 |
| `duplicate dependency label in one file` | 3 | 0 | 0 | 3 |
| `deep-merged dependency (non-literal facts)` | 0 | 0 | 2 | 2 |
| `more than one unlabeled include` | 0 | 0 | 2 | 2 |
| `source not modelled` | 0 | 0 | 2 | 2 |
| `unit config syntax error the oracle does not detect` | 0 | 0 | 2 | 2 |
| `config_path absolute or unresolvable` | 0 | 0 | 1 | 1 |
| `include path not modelled` | 0 | 0 | 1 | 1 |
| `module dir contains the unit dir` | 0 | 0 | 1 | 1 |
| `nesting over the 1000-level cap` | 0 | 0 | 1 | 1 |
| `terragrunt.hcl.json unit` | 0 | 0 | 1 | 1 |

Each category matches gruntled's documented reason for the unit or module. Examples:
`include-target`, `generate-may-declare-outputs`, `invalid-dependency`, `syntax-error` and
`config-too-deep`. The two resolved-unit rows are documented lower bounds: an absolute
`config_path`, and deep-merged `enabled`/`skip_outputs`.

Included modules: eligible to the model, not excluded, and with a cone that avoids every
excluded unit. The share is taken of the modules `graph --json` knows.

| Corpus | Included | Known | Share |
|---|---:|---:|---:|
| iso20022 | 62 | 62 | 1.000 |
| secret | 3 | 3 | 1.000 |
| denis256 | 795 | 818 | 0.972 |

On denis256, 15 known modules are not eligible: 14 have a seed whose `generate` block may
change the surface, and 1 has an include the model does not evaluate. The other 8 modules
in the 818 vs 795 gap belong to excluded units.

### Mutations

| Class | Edit | Expected |
|---|---|---|
| M1 | referenced output removed | `removed_outputs`, GRT004 at every firing site and no GRT001, exit 1; v0.3.0's GRT001 set equals HEAD's GRT004 set |
| M2 | unreferenced output removed | `removed_outputs`, Broken empty |
| M3 | `variable "gruntled_m17_req" {}` in a new file | `added_variables`, `variable_required` absent → required |
| M4 | `default` removed | `variable_required` optional → required |
| M5 | `type` set to `object({gruntled_m17 = string})` | `variable_type` old → SENTINEL |
| M6 | `type = string` on the base, `type = number` on the current tree | `output_type` string → number; the base against the pristine tree is empty |
| M7 | `sensitive` flipped | `output_sensitive` |
| M9 | M4 and M5 on the shared iso20022 `iac.src/global.tf` | one `changes[]` entry per linking module |
| M8 | not an edit | Impacted at distance 2 and more equal the model's BFS |

The recorded counts (sites = caught):

```
CORPUS-MUTATION iso20022 M1 sites=4 caught=4 excluded_by_touch=0
CORPUS-MUTATION iso20022 M2 sites=392 caught=392 excluded_by_touch=0
CORPUS-MUTATION iso20022 M3 sites=62 caught=62 excluded_by_touch=0
CORPUS-MUTATION iso20022 M4 sites=0 caught=0 excluded_by_touch=0 excluded=map[declared-in-shared-link:372]
CORPUS-MUTATION iso20022 M5 sites=75 caught=75 excluded_by_touch=0 excluded=map[declared-in-shared-link:372]
CORPUS-MUTATION iso20022 M6 sites=396 caught=396 excluded_by_touch=0
CORPUS-MUTATION iso20022 M7 sites=396 caught=396 excluded_by_touch=0
CORPUS-MUTATION iso20022 M9 sites=12 caught=12 excluded_by_touch=0 excluded=map[linker-of-excluded-units:3]
CORPUS-MUTATION iso20022 M8 depth>=2=67 depth>=3=0 depth>=4=0 depth>=5=0
CORPUS-MUTATION secret M1 sites=3 caught=3 excluded_by_touch=0
CORPUS-MUTATION secret M2 sites=1 caught=1 excluded_by_touch=0
CORPUS-MUTATION secret M3 sites=3 caught=3 excluded_by_touch=0
CORPUS-MUTATION secret M4 sites=4 caught=4 excluded_by_touch=0
CORPUS-MUTATION secret M5 sites=8 caught=8 excluded_by_touch=0
CORPUS-MUTATION secret M6 sites=4 caught=4 excluded_by_touch=0
CORPUS-MUTATION secret M7 sites=4 caught=4 excluded_by_touch=0
CORPUS-MUTATION secret M9 sites=0 caught=0 excluded_by_touch=0 (no file shared by symlink)
CORPUS-MUTATION secret M8 depth>=2=14 depth>=3=0 depth>=4=0 depth>=5=0
CORPUS-MUTATION denis256 M1 sites=87 caught=87 excluded_by_touch=8
CORPUS-MUTATION denis256 M2 sites=387 caught=387 excluded_by_touch=0
CORPUS-MUTATION denis256 M3 sites=795 caught=795 excluded_by_touch=0
CORPUS-MUTATION denis256 M4 sites=41 caught=41 excluded_by_touch=0
CORPUS-MUTATION denis256 M5 sites=708 caught=708 excluded_by_touch=0 excluded=map[type-grammar:1]
CORPUS-MUTATION denis256 M6 sites=481 caught=481 excluded_by_touch=0 excluded=map[type-declared:1]
CORPUS-MUTATION denis256 M7 sites=482 caught=482 excluded_by_touch=0
CORPUS-MUTATION denis256 M9 sites=0 caught=0 excluded_by_touch=0 (no file shared by symlink)
CORPUS-MUTATION denis256 M8 depth>=2=667 depth>=3=147 depth>=4=67 depth>=5=12
```

- **iso20022 M4 has no module-local site.** Every iso20022 default is in the shared
  `global.tf`, which M9 covers.
- **The 8 denis256 `excluded_by_touch`** are the (module, output) pairs of the
  `reference shape other` rows.

**The M1 dual oracle.**
- `GRUNTLED_REF_BIN` must print `gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)`.
- It must also pass a behavioural fingerprint: `rebase` is an unknown command and
  `blast --depth` is an undefined flag. A HEAD build with v0.3.0 ldflags fails that check.

**SENTINEL hand check.** The sentinel `object({gruntled_m17 = string})` has one attribute.
The renderer writes `object({`, then each name in sorted order quoted with
`strconv.Quote`, then `=` with no whitespace, then the type (`string`), then `})`. That
gives `object({"gruntled_m17"=string})`, which is the string HEAD prints.

### Reformat-only transforms

Each transform is corpus-wide over the included modules' regular `.tf` files. None
touches a string, heredoc or comment interior.

| Transform | What it does |
|---|---|
| R1 | whitespace |
| R2 | comments |
| R3 | attribute reorder |
| R4 | `optional()` default, two-sided, added and changed |
| R5 | `.tf` to `.tf.json` for simple blocks, both directions |
| R6 | blocks moved to another file |
| R7 | `deprecated` added |
| R8 | R1∘R2∘R3∘R6∘R7 |
| R9 | symlinked module files materialised |

Each must give exit 0 and an empty Broken, Impacted and `changes`. A guard also requires
the same known modules and names, the same unit statuses, no new GRT100, and at least one
edit per corpus:

| Transform | iso20022 | secret | denis256 |
|---|---:|---:|---:|
| R1 | 1443 | 54 | 1520 |
| R2 | 1413 | 44 | 2556 |
| R3 | 0 (471 inside R8) | 5 | 43 |
| R4 | 62 | 3 | 795 |
| R5 | 469 | 9 | 1103 |
| R6 | 471 | 12 | 1191 |
| R7 | 471 | 12 | 1191 |
| R8 | 5211 | 159 | 10875 |
| R9 | 62 | 0 | 0 |

### The daemon sample

TestCorpusDaemonSample starts one built `gruntled watch --poll` at a time on a working copy.
For each class it applies the first eligible site, then R8, then (iso20022) M9. After each
edit it waits until the daemon has indexed the final tree. `report --blast` (text and
JSON) must then equal `blast --base <pristine copy> <working copy>` after the exact-label
normaliser, with the same exit code. After the revert the view must be empty again.

Then comes one rebase cycle:
- the M1 edit is applied;
- `rebase --expect <gen>` accepts its GRT004 findings: iso20022 3, secret 2, denis256 1;
- the view is empty;
- the revert shows `+output <y>` at exit 0.

The daemon is stopped with SIGTERM, then SIGKILL after 5 seconds, and no process may still
mention the temp directories. The run passed on all three corpora: 18 s, 8 s and 38 s,
147 MB RSS.

### Recorded gate run

2026-10-12, commit `853bb17`, 15:18:50 to 16:09:10 (wall 3020 s), run alone. Machine
(from `lscpu`, `/proc/meminfo`, `uname -r` and `go version`): Intel(R) Core(TM) Ultra 7
155H, 8 logical CPUs, MemTotal 8132820 kB, kernel 6.6.87.2-microsoft-standard-WSL2, go1.27.2.
Gate output (excerpt: the first lines, the shards over 60 seconds, and the last lines):

```
corpus-gate: network=isolated (/proc/net/dev: lo)
corpus-gate: GRUNTLED_REF_BIN gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)
corpus-gate: free -m: Mem: 7942 912 5864 3 1360 7029
256/256 identical
corpus-gate: shard compare-ref ok 8s
corpus-gate: shard TestCorpusBlastSelf ok 5s
corpus-gate: shard TestCorpusBlastModel ok 8s
corpus-gate: shard TestCorpusReformat ok 30s
corpus-gate: shard TestCorpusMutation17/iso20022/M2 ok 65s
corpus-gate: shard TestCorpusMutation17/iso20022/M6 ok 125s
corpus-gate: shard TestCorpusMutation17/denis256/M2 ok 289s
corpus-gate: shard TestCorpusMutation17/denis256/M3 ok 591s
corpus-gate: shard TestCorpusMutation17/denis256/M5 ok 540s
corpus-gate: shard TestCorpusMutation17/denis256/M6 ok 746s
corpus-gate: shard TestCorpusMutation17/denis256/M7 ok 378s
corpus-gate: free -m: Mem: 7942 1187 4022 3 2926 6754
corpus-gate: 31/31
```

All 31 shards passed. The other shards took between 0 and 96 seconds each. Peak RSS was
about 150 MB per shard.

### Limitations

- **The universe is restricted.** On denis256, 23 of the 818 known modules are outside it.
  These are the modules with a surface-changing `generate` or a seed or cone that meets an
  excluded unit (the table above). Their changes are not claimed.
- **R5 converts only simple blocks.** Those are single-line values with no heredoc,
  directive or nested block (iso20022 skipped 2 blocks, secret 3, denis256 88).
- **No corpus declares `optional()` or an output `type`.** R4 and M6 therefore inject
  them on both sides.
- **The daemon sample is one site per class, not the full set.** The windows report-file
  path is covered by phase 16 CI.
- **The gate needs unprivileged user namespaces for `network=isolated`.** ubuntu-latest's
  AppArmor blocks them by default (run 38262035294). corpus.yml therefore runs
  `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` before the gate, and it
  keeps `CORPUS_GATE_NONET=1`, so a runner where isolation still fails stops instead of
  running with network.
