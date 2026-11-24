package auditlog

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

func desiredCollection(cfg Config) *core.Collection {
	c := core.NewCollection(core.CollectionTypeBase, cfg.CollectionName, cfg.CollectionID)

	c.Fields.Add(&core.TextField{
		Name:     "collection",
		Required: true,
		Max:      100,
	})

	c.Fields.Add(&core.TextField{
		Name:     "recordId",
		Required: true,
		Max:      100,
	})

	c.Fields.Add(&core.SelectField{
		Name:      "action",
		Values:    []string{ActionCreate, ActionUpdate, ActionDelete},
		MaxSelect: 1,
		Required:  true,
	})

	c.Fields.Add(&core.TextField{
		Name: "actorId",
		Max:  100,
	})

	c.Fields.Add(&core.TextField{
		Name: "ownerId",
		Max:  100,
	})

	c.Fields.Add(&core.JSONField{
		Name:    "changes",
		MaxSize: 1 << 20, // 1MB
	})

	c.Fields.Add(&core.AutodateField{
		Name:     "created",
		OnCreate: true,
	})

	c.Indexes = append(c.Indexes,
		"CREATE INDEX idx_audit_logs_owner_created ON {{NAME}} (ownerId, created DESC)",
		"CREATE INDEX idx_audit_logs_record ON {{NAME}} (collection, recordId, created DESC)",
	)

	if cfg.OwnerField != "" {
		c.ListRule = types.Pointer("ownerId = @request.auth.id")
		c.ViewRule = types.Pointer("ownerId = @request.auth.id")
	} else {
		c.ListRule = nil
		c.ViewRule = nil
	}
	// Append-only from server hooks: clients cannot create, update, or delete audit logs.
	c.CreateRule = nil
	c.UpdateRule = nil
	c.DeleteRule = nil

	return c
}

func ensureCollection(app core.App, cfg Config) error {
	desired := desiredCollection(cfg)
	existing, err := app.FindCollectionByNameOrId(desired.Name)
	if err != nil {
		return app.Save(desired)
	}

	desired.Id = existing.Id
	desired.Created = existing.Created
	desired.MarkAsNotNew()

	if existing.Fields.String() == desired.Fields.String() &&
		existing.Indexes.String() == desired.Indexes.String() &&
		rulesEqual(existing, desired) {
		return nil
	}

	return app.Save(desired)
}

func rulesEqual(a, b *core.Collection) bool {
	return ptrEqual(a.ListRule, b.ListRule) &&
		ptrEqual(a.ViewRule, b.ViewRule) &&
		ptrEqual(a.CreateRule, b.CreateRule) &&
		ptrEqual(a.UpdateRule, b.UpdateRule) &&
		ptrEqual(a.DeleteRule, b.DeleteRule)
}

func ptrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
