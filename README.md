# pocketbase-audit-log

Automatic field-level audit logging plugin for [PocketBase](https://pocketbase.io/) (`v0.41.1+`).

## Features

- **Uniform `{ old, new }` diff format** across all actions:
  - `CREATE`: `{ "field": { "old": null, "new": value } }` (empty/null fields omitted)
  - `UPDATE`: `{ "field": { "old": oldValue, "new": newValue } }` (no-op updates skipped)
  - `DELETE`: `{ "field": { "old": oldValue, "new": null } }` (empty/null fields omitted)
- **Actor & owner tracking**: records `actorId` (user ID, `"superuser"`, or `"system"`) and optional `ownerId` for user-scoped API read rules.
- **Append-only**: `ListRule`/`ViewRule` scoped to `ownerId = @request.auth.id`; `Create`/`Update`/`Delete` rules locked to `nil`.
- **Extensible**: `WithSkipFunc` (e.g. skip bulk imports), `WithIgnoredFields`, `WithRetentionDays`.

## Usage

```go
import auditlog "github.com/ReinforceZwei/pocketbase-audit-log"

auditlog.Register(app,
	auditlog.WithWatchedCollections("animes", "tags"),
	auditlog.WithOwnerField("userId"),
	auditlog.WithIgnoredFields("userId", "isImporting"),
	auditlog.WithSkipFunc(func(e *core.RecordEvent) bool {
		return e.Record.GetBool("isImporting")
	}),
)
```
