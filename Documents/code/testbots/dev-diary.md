# Overview

This is the dev diary for developing Database Migration Testbot.

# Activities

## Activity 01
Date: 2026/02/24 <br>
Type: Implementation <br>
- The testbot implementation is implemented by Qwen.
- The implementation document is created: `tester-migration-implement-v1.md`

## Activity 02
Date: 2026/02/24 <br>
TYpe: Bug <br>

**Description** <br>
A few functions were created but never used in shared/go/api/testers/tester-migration/tester_migration_state.go:
```go
    getAppliedMigrations(...)
    getCurrentVersion(...)
    hasPendingMigrations(...)
    getMigrationStatus(...)
    listTables(...)
    listMigrationFiles(...)
```

shared/go/api/testers/tester-migration/tester_migration.go has unused parameters:
```go
    clearMigrationsDir(ctx context.Context): 'ctx' unused
    buildMigrationsPool(ctx context.Context): 'ctx' unused
```

## Activity 03
Date: 2026/02/24 <br>
Type: Bug <br>

**Description** <br>
The implementation did not register the tester to shared/go/api/testers/registertester.go

**Analysis** <br>
The reason is that the initial requirement (the one that was written by a human user)
did not say it.

**How to Prevent** <br>
The doc `testbot.md` should require it.

# References
[1] testbot.md: ./Testbot/testbot.md

## Activity 04
Date: 2026/02/24 <br>
Type: New Feature <br>

**Description** <br>
The autotester (refer to [1] for its document), which is implemented in shared/go/api/autotester, requires testers register with the autotester in order for autotester to run them. Refer to shared/go/api/testers/registertesters.go for an example.

We may view the registration as a tester packaging mechnism. Currently, it is hard coded. We need to make it configurable so that users can chery pick the ones to run.

Thinking along this direction, autotester should support multiple tester packaging, one, for instance, for smoke test, one for complete test, one for regression test, etc.

Please do the following:
- Implement this Tester Packaging feature
- Update [1] and add a Change Log section for this update. Save the updated version to auto-tester-v3.md

References:
[1] auto-tester-v2.md: ./Testbot/auto-tester-v2.md

Assistant: Claude Code  
