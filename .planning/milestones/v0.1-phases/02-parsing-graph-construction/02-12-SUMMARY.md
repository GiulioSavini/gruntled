---
phase: 02-parsing-graph-construction
plan: 12
subsystem: hclconv-limits, tfsurface, terragrunt-walk
tags: [go, hcl, gap-cycle-1, stack-overflow, fifo, opentofu, hardening]

requires:
  - phase: 02-parsing-graph-construction
    provides: "hclconv limits API and CheckNativeDepth bracket/quote/heredoc/template/unary depth guard (02-10); ReadFileLimited (02-10)"
provides:
  - "CheckNativeDepth counts pending ternary `?` tokens per newline-significant frame (matching hclsyntax.parseTernaryConditional's own recursion), so both the true-branch chain and the else-chain shapes of a ternary bomb are refused, not just bracket/quote nesting"
  - "hclconv.MaxExpressionChain (10,000): caps binary-operator/`.`/`[` links per expression frame, refusing the 2M-long `+`-chain shape before it builds a multi-GB left-leaning AST"
  - "hclconv.ErrNotRegularFile: ReadFileLimited refuses any non-regular file (FIFO, socket, device) at fs.Stat, before Open, so a FIFO can never block the reader open forever; the open handle is re-checked with f.Stat too"
  - "tfsurface.keepModuleFiles reads every .tf/.tf.json/.tofu/.tofu.json file as a union surface instead of applying OpenTofu precedence (dropping the .tf twin) -- the union can only over-count declared names, never manufacture a false GRT001"
affects: [02-13, 02-14, 03-grt001-diagnostic-cli]

tech-stack:
  added: []
  patterns:
    - "Frame-stack depth accounting in CheckNativeDepth: frames[] push/pop on bracket/quote/heredoc/template opens and closes; each frame tracks a chain counter (binary op/`.`/`[` links) and a questions counter (pending `?`), both checked against their max on every increment; pending `?` are released (folded back to zero) at a comma, the frame's own closer, or -- for a newline-significant frame (file body, block body, non-for object constructor) -- a newline, exactly mirroring where hclsyntax's own recursive descent unwinds"
    - "Fail-toward-unknown/over-count-never-under-count for anything statically ambiguous: a .tf/.tofu pair is read as the union of both views (may over-count, per G20) rather than guessing which binary runs; a broken file in either view still makes the whole surface unknown"
    - "Non-regular-file refusal happens at Stat time in every reader (hclconv.ReadFileLimited, tfsurface module files, terragrunt discoverUnits), never at Open, so a FIFO/socket/device can never hang a scan"

key-files:
  created:
    - internal/infrastructure/hclconv/limits_unix_test.go
    - internal/infrastructure/tfsurface/realfs_unix_test.go (FIFO real-FS regression, unix-only)
    - internal/infrastructure/terragrunt/walk_unix_test.go (FIFO real-FS regression, unix-only)
    - internal/infrastructure/terragrunt/limits_test.go
  modified:
    - internal/infrastructure/hclconv/limits.go
    - internal/infrastructure/hclconv/limits_test.go
    - internal/infrastructure/tfsurface/reader.go
    - internal/infrastructure/tfsurface/reader_test.go
    - internal/infrastructure/terragrunt/walk.go
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/testhelpers_test.go
    - internal/infrastructure/terragrunt/loader_test.go

key-decisions:
  - "A pending `?` is counted per bracket/quote/heredoc/template frame and released only at a comma, that frame's own closer, or a newline in a newline-significant frame (body, non-for object constructor) -- never at a `:` -- because hclsyntax's parseTernaryConditional recurses into ParseExpression for BOTH branches of a ternary, so a `:` never ends the Go-stack recursion the else-chain `a?b:a?b:...1` triggers; popping on `:` would leave the counter at 1 while the real parser recursed once per `?` and still stack-overflowed"
  - "MaxExpressionChain is set to 10,000: at least 2 orders of magnitude above any realistic hand-written or generated expression (the deliberately adversarial dense-ternary regression test -- one ternary per attribute/object item/function argument, with trailing comments -- still passes under it), while the 2,000,000-link G17 repro is refused; a `[` counts as one chain link of the enclosing frame before it opens its own bracket frame, so postfix index chains `x[a][a]...` are covered without double-counting"
  - "Every tie in the depth/chain accounting goes to over-counting (refuse), never under-counting: an unnecessary config-too-deep/module-file-too-deep verdict costs an unknown unit or module, never a false GRT001, while under-counting could hand a hostile file back to hclsyntax's recursive parser, which Go cannot recover from (a fatal stack overflow, not a panic)"
  - "ReadFileLimited's Stat-time regular-file check runs before any Open call: opening a FIFO for reading blocks until a writer appears, so the check must reject before that syscall, not after; the already-open handle is re-checked with f.Stat() too, since a symlink can resolve to a FIFO only visible after Stat-following at Open time on some FS implementations"
  - "tfsurface.keepModuleFiles drops OpenTofu-precedence exclusion entirely: gruntled cannot know statically which binary (Terraform, which ignores .tofu, or OpenTofu, which ignores a .tf twin) will run a given module, so it reads both files and unions their declared names. This can only make the surface look bigger (never triggers a spurious missing-output diagnostic); a broken file on either side of the pair still fails the whole surface toward module-file-unreadable/syntax-error, per the project's existing under-count-never rule"

patterns-established:
  - "Adversarial-input regressions for a size/depth guard split into two tiers: an always-on MapFS-based test at a size that runs in milliseconds (asserting the exact boundary), and a GRUNTLED_HEAVY_TESTS=1-gated test using the reviewer's literal multi-MB repro (asserting no crash/hang within a generous timeout), so CI stays fast while the exact crash shape stays covered locally and can be re-run on demand"
  - "Non-regular-file handling is tested at three layers with the same double: hclconv (MapFS pipe/socket/device Mode rows, always-on; a real unix FIFO with a 10s hang guard, unix-only), tfsurface (module file), and terragrunt walk (unit file) -- each layer's own test double (readFailFS, countingFS, countingReadFS) was updated to intercept Open so injection/counting still works now that ReadFileLimited opens before the final read"

requirements-covered: [PARSE-05, PARSE-06, GRAPH-05]
