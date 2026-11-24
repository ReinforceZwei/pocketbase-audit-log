package auditlog

import (
	"log"
	"sync"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

func registerHooks(app core.App, cfg Config) {
	ignored := buildIgnoredSet(cfg.IgnoredFields)
	tags := cfg.WatchedCollections

	// Keyed by *core.Record pointer identity during the lifecycle of a single save/delete.
	var actorStore sync.Map    // *core.Record -> string
	var snapshotStore sync.Map // *core.Record -> *core.Record

	// Capture HTTP actorId on request hooks (cleaned up on return).
	app.OnRecordCreateRequest(tags...).BindFunc(func(e *core.RecordRequestEvent) error {
		actorStore.Store(e.Record, resolveActorID(e.Auth))
		defer actorStore.Delete(e.Record)
		return e.Next()
	})
	app.OnRecordUpdateRequest(tags...).BindFunc(func(e *core.RecordRequestEvent) error {
		actorStore.Store(e.Record, resolveActorID(e.Auth))
		defer actorStore.Delete(e.Record)
		return e.Next()
	})
	app.OnRecordDeleteRequest(tags...).BindFunc(func(e *core.RecordRequestEvent) error {
		actorStore.Store(e.Record, resolveActorID(e.Auth))
		defer actorStore.Delete(e.Record)
		return e.Next()
	})

	// Capture pre-save Original() before PocketBase resets it after save.
	app.OnRecordUpdate(tags...).BindFunc(func(e *core.RecordEvent) error {
		if orig := e.Record.Original(); orig != nil {
			snapshotStore.Store(e.Record, orig)
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateError(tags...).BindFunc(func(e *core.RecordErrorEvent) error {
		snapshotStore.Delete(e.Record)
		return e.Next()
	})

	app.OnRecordDelete(tags...).BindFunc(func(e *core.RecordEvent) error {
		if orig := e.Record.Original(); orig != nil {
			snapshotStore.Store(e.Record, orig)
		} else {
			snapshotStore.Store(e.Record, e.Record)
		}
		return e.Next()
	})
	app.OnRecordAfterDeleteError(tags...).BindFunc(func(e *core.RecordErrorEvent) error {
		snapshotStore.Delete(e.Record)
		return e.Next()
	})

	// Write audit log entries only after the DB operation succeeds.
	app.OnRecordAfterCreateSuccess(tags...).BindFunc(func(e *core.RecordEvent) error {
		if shouldSkip(e, cfg) {
			return e.Next()
		}
		changes := buildCreateDiff(e.Record, ignored)
		if len(changes) > 0 {
			actorID := loadActor(&actorStore, e.Record)
			writeAuditLog(e.App, cfg, e.Record, ActionCreate, actorID, changes)
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess(tags...).BindFunc(func(e *core.RecordEvent) error {
		origVal, _ := snapshotStore.LoadAndDelete(e.Record)
		if shouldSkip(e, cfg) {
			return e.Next()
		}
		var oldRecord *core.Record
		if orig, ok := origVal.(*core.Record); ok {
			oldRecord = orig
		} else {
			oldRecord = e.Record.Original()
		}
		changes := buildUpdateDiff(oldRecord, e.Record, ignored)
		if len(changes) > 0 {
			actorID := loadActor(&actorStore, e.Record)
			writeAuditLog(e.App, cfg, e.Record, ActionUpdate, actorID, changes)
		}
		return e.Next()
	})

	app.OnRecordAfterDeleteSuccess(tags...).BindFunc(func(e *core.RecordEvent) error {
		origVal, _ := snapshotStore.LoadAndDelete(e.Record)
		if shouldSkip(e, cfg) {
			return e.Next()
		}
		target := e.Record
		if orig, ok := origVal.(*core.Record); ok && orig != nil {
			target = orig
		}
		changes := buildDeleteDiff(target, ignored)
		if len(changes) > 0 {
			actorID := loadActor(&actorStore, e.Record)
			writeAuditLog(e.App, cfg, target, ActionDelete, actorID, changes)
		}
		return e.Next()
	})
}

func shouldSkip(e *core.RecordEvent, cfg Config) bool {
	if e.Record == nil || e.Record.Collection().Name == cfg.CollectionName {
		return true
	}
	if cfg.SkipFunc != nil && cfg.SkipFunc(e) {
		return true
	}
	return false
}

func resolveActorID(auth *core.Record) string {
	if auth == nil {
		return "anonymous"
	}
	if auth.IsSuperuser() {
		return "superuser"
	}
	return auth.Id
}

func loadActor(store *sync.Map, record *core.Record) string {
	if val, ok := store.Load(record); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return "system"
}

func writeAuditLog(
	app core.App,
	cfg Config,
	record *core.Record,
	action string,
	actorID string,
	changes map[string]FieldChange,
) {
	col, err := app.FindCollectionByNameOrId(cfg.CollectionName)
	if err != nil {
		log.Printf("[auditlog] failed to find collection %q: %v", cfg.CollectionName, err)
		return
	}

	entry := core.NewRecord(col)
	entry.Set("collection", record.Collection().Name)
	entry.Set("recordId", record.Id)
	entry.Set("action", action)
	entry.Set("actorId", actorID)
	if cfg.OwnerField != "" {
		entry.Set("ownerId", record.GetString(cfg.OwnerField))
	}
	entry.Set("changes", changes)

	if err := app.Save(entry); err != nil {
		log.Printf("[auditlog] failed to save audit log for %s/%s: %v", record.Collection().Name, record.Id, err)
	}
}
