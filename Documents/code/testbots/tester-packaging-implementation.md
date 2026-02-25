# Tester Packaging — Implementation Summary

**Date:** 2026-02-24
**Feature:** Configurable, named suites of testers for AutoTester
**Files changed:** 4 (1 new, 3 modified)

---

## Background

Previously, `RegisterTesters()` hard-coded all testers into a single flat list. Every AutoTester run executed all registered testers unless individual names were passed via the `--tester` flag. There was no way to define a reusable "smoke", "regression", or "nightly" grouping without repeating the same flag combinations at every call site.

---

## What Was Built

### New type: `TesterPackage`

A named, ordered collection of tester names. Packages reference testers by name — they do not own factory instances. A tester may appear in multiple packages.

```go
type TesterPackage struct {
    Name        string   // unique key, e.g. "smoke"
    Description string   // human-readable explanation
    TesterNames []string // ordered list of tester Name()s to include
}
```

### New type: `TesterPackageRegistry`

A thread-safe registry mapping package names to `TesterPackage` definitions.

```go
var GlobalPackageRegistry = &TesterPackageRegistry{...}

func RegisterPackage(pkg *TesterPackage)
func BuildPackage(packageName string) ([]Tester, error)
```

### New field: `RunConfig.PackageName`

```go
type RunConfig struct {
    // ...existing fields...

    // PackageName selects testers from a pre-defined TesterPackage.
    // Overridden by TesterNames when both are set.
    PackageName string
}
```

### Runner resolution

At the start of `runner.Run()`, if `PackageName` is set and `TesterNames` is empty, the runner resolves the package via `GlobalPackageRegistry` and populates `TesterNames`:

```go
if r.config.PackageName != "" && len(r.config.TesterNames) == 0 {
    pkg, ok := GlobalPackageRegistry.Get(r.config.PackageName)
    if !ok {
        return fmt.Errorf("package %q not found ...", r.config.PackageName)
    }
    r.config.TesterNames = pkg.TesterNames
}
```

### Built-in shared packages

Defined in `shared/go/api/testers/registertesters.go` via the new `RegisterPackages()` function:

| Package | Testers included | Intended use |
|---|---|---|
| `smoke` | `tester_database`, `tester_logger` | Pre-deploy sanity check (< 1 min) |
| `regression` | `tester_databaseutil`, `tester_logger` | Post-merge regression gate |
| `complete` | all three shared testers | Full shared-library coverage |

---

## Files Changed

| File | Change |
|---|---|
| `shared/go/api/autotester/package.go` | **New** — `TesterPackage`, `TesterPackageRegistry`, `GlobalPackageRegistry`, helpers |
| `shared/go/api/autotester/testrun.go` | Added `PackageName string` to `RunConfig` |
| `shared/go/api/autotester/runner.go` | `Run()` resolves `PackageName` → `TesterNames` at start |
| `shared/go/api/testers/registertesters.go` | Added `RegisterPackages()` with smoke/regression/complete |

---

## Usage

### Option A — via `RunConfig.PackageName` (recommended for CLI)

```go
runner := autotesters.NewTestRunner(
    autotesters.GlobalRegistry.Build(),
    &autotesters.RunConfig{
        PackageName: "smoke",
        Environment: "local",
    },
    log,
)
runner.Run(ctx)
```

### Option B — pre-flight construction via `BuildPackage`

```go
testers, err := autotesters.BuildPackage("smoke")
if err != nil {
    log.Fatal(err)
}
runner := autotesters.NewTestRunner(testers, config, log)
runner.Run(ctx)
```

### CLI (add `--package` flag to `main.go`)

```bash
# Pre-deploy gate
go run ./server/cmd/autotester/ --package=smoke

# Full nightly run
go run ./server/cmd/autotester/ --package=complete --parallel --max-parallel=8

# Regression with CI reporting
go run ./server/cmd/autotester/ --package=regression --stop-on-fail \
  --json-report=/tmp/report.json
```

---

## Migration Guide

**Step 1.** Call `RegisterPackages()` after `RegisterTesters()`:

```go
func registerAll(cfg *config.Config) {
    sharedtesters.RegisterTesters()
    // ... app-specific testers ...

    sharedtesters.RegisterPackages()   // ← add this
    // ... app-specific packages ...
}
```

**Step 2.** Add `--package` flag to `main.go`:

```go
pkg := flag.String("package", "", "Run a named tester package (e.g. smoke, complete, regression)")
// ...
&autotesters.RunConfig{
    PackageName: *pkg,
    // ... existing fields ...
}
```

**Step 3.** (Optional) Define application-specific packages:

```go
autotesters.RegisterPackage(&autotesters.TesterPackage{
    Name:        "nightly",
    Description: "Nightly: all shared + app testers",
    TesterNames: []string{
        "tester_database", "tester_databaseutil", "tester_logger",
        "user_tester",
    },
})
```

---

## Breaking Changes

None. All existing code continues to work unchanged:
- `RegisterTesters()` signature is unchanged.
- `RegisterPackages()` is opt-in; not calling it means no packages are registered.
- `RunConfig.PackageName` defaults to `""` (no-op), preserving all existing behavior.

---

## Documentation

Full specification: [`Testbot/auto-tester-v3.md`](../../../Testbot/auto-tester-v3.md) — Section 10 "Tester Packaging".
