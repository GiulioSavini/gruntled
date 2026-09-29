# Validation of gruntled v0.1

gruntled v0.1 is a falsifiable experiment, not a feature list. The claim has four
parts. On the unmutated primary corpus, `gruntled check` reports nothing. When an output
is renamed or deleted in a target module, gruntled reports every reference that no longer
resolves. Plain `terragrunt hcl validate` does not report those injected breaks. And
`gruntled check` is faster than `terragrunt hcl validate` on the same repository. If any
part fails, the idea is wrong. This document records one full run of that experiment and
how to rerun it.

## Outcome

All five claims passed on the pinned primary corpus. The denis256 secondary check also
passed, but it passed with a known recall limitation: gruntled reports none of the 8
hand-derived references on that corpus (0/8). See
[Secondary corpus](#secondary-corpus-denis256terragrunt-tests).

| Requirement | Claim | Result | Evidence |
|---|---|---|---|
| VALID-02 | Golden tests assert the exact expected diagnostic set on fixture repositories | PASS | [VALID-02](#valid-02-golden-fixtures) |
| VALID-03 | Zero diagnostics on the unmutated primary corpus | PASS | [VALID-03](#valid-03-unmutated-corpus) |
| VALID-04 | Every reference broken by an injected rename or deletion is reported, and nothing else | PASS | [VALID-04](#valid-04-injected-mutations) |
| VALID-05 | Plain `terragrunt hcl validate` does not report those mutations | PASS | [VALID-05](#valid-05-terragrunt-hcl-validate-on-the-same-mutated-trees) |
| VALID-06 | `gruntled check` is faster than `terragrunt hcl validate` on the same repository | PASS | [VALID-06](#valid-06-benchmark) |
| Secondary corpus denis256 (supplementary to VALID-02) | No panic, deterministic output, and an exact GRT001 set | PASS (0 false positives, 0/8 recall) | [Secondary corpus](#secondary-corpus-denis256terragrunt-tests) |

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
DENIS256 resolved=249
DENIS256 module_unknown=157
DENIS256 config_unknown=740
DENIS256 unknown_modules=4
DENIS256 errors=4
DENIS256 warnings=0
DENIS256 grt100=4
DENIS256 grt001=0
DENIS256 unknown_reason include-target=719
DENIS256 unknown_reason remote-source=132
DENIS256 unknown_reason generate-may-declare-outputs=14
DENIS256 unknown_reason include-dynamic-path=11
DENIS256 unknown_reason source-dynamic-path=9
DENIS256 unknown_reason syntax-error=5
DENIS256 unknown_reason invalid-include=2
DENIS256 unknown_reason config-too-deep=1
DENIS256 unknown_reason include-not-found=1
DENIS256 unknown_reason json-config-unsupported=1
DENIS256 unknown_reason source-outside-repo=1
DENIS256 unknown_reason unit-dir-overlays-module=1
DENIS256 grt100 encryption/terragrunt.hcl:14:20
DENIS256 grt100 include-error/terragrunt.hcl:28:19
DENIS256 grt100 issue-3368/terragrunt.hcl:24:68
DENIS256 grt100 scaffold/test1/.boilerplate/terragrunt.hcl:7:24
```

That is 897 unknown units out of 1146, 719 of them `include-target`.

**How the expectation was made.** It was derived by hand from the corpus text and the
locked DIAG-03 table. The starting point was the 9 would-be hits of the Phase 2 stress
run, which ran without DIAG-03. Every position was self-checked against the file bytes
before gruntled ran.

**Independence.** The classification of each row (GRT001 or silent, suffix or not) is
independent of gruntled: it comes from the HCL and the locked DIAG-03 rules. The
candidate list is not independent. It came from the Phase 2 stress run, which used
gruntled's own parser, so a breakage that run did not flag could be missing from it. No
extra GRT001 was found in this run (`grt001=0`), so there is nothing to classify as a
false positive.

**Deviation from the original request.** The request asked for 9 GRT001 hits. The
disabled-dependency reference is silent under the locked DIAG-03 row 2
(`enabled` not literally `true`), which applies before mocks are consulted. So 8 were
expected, not 9.

**Result: 0 of the 8 are reported. This is a known limitation, not a pass on recall.**
Gap-closure plan 02-13 added a conservative include-target rule. When any include in the
repository has a dynamic file name, or names a `terragrunt.hcl`, every include-free unit
may be the target of that include, so gruntled marks it `config-unknown: include-target`
and does not check it. Under DIAG-03 a reference whose referring unit or target unit is
unknown is silent. On denis256 this rule covers every one of the 8 references. It keeps
precision intact (0 GRT001, no false positive, the `enabled = false` reference silent),
but recall on the known references is 0/8. The assertion was amended after 02-13 to an
empty GRT001 set, with each of the 8 references required to sit on an unknown referring
unit, target unit or target module. The 8 rows stay in `denisExpected` as the regression
target for a v2 refinement that narrows the include-target rule.

| `file:line:col` | dependency / output | Expected under DIAG-03 | Suffix expected | Actual in this run | Justification |
|---|---|---|---|---|---|
| `issue-2163/app/terragrunt.hcl:14:24` | `app_service_plan01` / `asp_id` | GRT001 (mock-only key) | no | not reported: target `issue-2163/module` is `config-unknown/include-target` | The module declares no outputs. `asp_id` is only mocked, and the allowed commands are `["validate", "plan"]`, so apply fails loudly instead of masking. |
| `issue-2405/app/terragrunt.hcl:16:21` | `vpc` / `vpc_id` | GRT001 (mock-only key) | yes | not reported: unit and target `config-unknown/include-target` | `vpc/main.tf` is empty. `vpc_id` exists only in `mock_outputs`, and the allowed-commands list is misplaced inside the mock map, so apply would use the mock. |
| `issue-2405/app/terragrunt.hcl:17:21` | `vpc` / `private_subnets` | GRT001 (mock-only key) | yes | not reported: unit and target `config-unknown/include-target` | Same fixture and facts as the row above, for `private_subnets`. |
| `issue-2631/main/terragrunt.hcl:9:9` | `dep` / `a` | GRT001 (genuine fixture bug) | no | not reported: unit and target `config-unknown/include-target` | The module declares only `y`, and there is no `mock_outputs` at all. |
| `issue-2718/app/terragrunt.hcl:23:25` | `vpc_main` / `aws_subnet_public_output` | GRT001 (mock-only key) | yes | not reported: target `issue-2718/vpc` is `config-unknown/include-target` | `vpc/main.tf` is empty. The key exists only in `mock_outputs`. The merge and allowed-commands settings are misplaced inside the mock map, so every command gets the mocks, apply included. |
| `mock-output/module1/terragrunt.hcl:22:16` | `module2` / `subnets` | GRT001 (mock-only key) | yes | not reported: unit and target `config-unknown/include-target` | `module2` declares only `hello` and `attribute`. `subnets` is only mocked, the `"shallow"` strategy merges mocks into state, and apply is allowed, so apply silently injects the mock. |
| `mocks/module1/terragrunt.hcl:10:12` | `module2` / `vpc_id2` | GRT001 (genuine fixture bug) | no | not reported: unit and target `config-unknown/include-target` | A typo for `vpc_id`. `module2/main.tf` is empty, and `mock_outputs` is `yamldecode(file(...))`, which is not literal (so no suffix) and defines only `vpc_id` anyway. |
| `optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:17:12` | `vpc` / `vpc_id` | GRT001 (mock-only key) | yes | not reported: unit and target `config-unknown/include-target` | `enabled = true` literally. The module has one resource and zero outputs. `vpc_id` is only mocked, and there is no allowed-commands list, so apply returns the mock. |
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
  claim. On that corpus gruntled reports 0 of 8 known references, because the 02-13
  include-target rule marks 719 of 1146 units unknown. Narrowing that rule is a v2
  candidate.
- GRT001 only. Units that gruntled marks unknown are not checked (the Phase 2 policy).
  In this run that was 3 of 65 units on the primary corpus and 897 of 1146 on denis256.
- The timing claim is relative, on one machine only, under the load described above. The
  absolute numbers are not a performance specification.
