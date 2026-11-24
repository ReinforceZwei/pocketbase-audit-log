package auditlog

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

func setupTestApp(t *testing.T, opts ...Option) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(app.Cleanup)

	// Create a sample "items" collection to audit
	items := core.NewCollection(core.CollectionTypeBase, "items")
	items.Fields.Add(
		&core.TextField{Name: "userId", Required: true},
		&core.TextField{Name: "title", Required: true},
		&core.TextField{Name: "status"},
		&core.NumberField{Name: "rating"},
		&core.BoolField{Name: "isImporting"},
		&core.TextField{Name: "secret", Hidden: true},
	)
	if err := app.Save(items); err != nil {
		t.Fatalf("failed to create items collection: %v", err)
	}

	Register(app, opts...)

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("failed to bootstrap app: %v", err)
	}
	return app
}

func decodeChanges(t *testing.T, rec *core.Record) map[string]FieldChange {
	t.Helper()
	raw := rec.GetString("changes")
	var out map[string]FieldChange
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("failed to unmarshal changes %q: %v", raw, err)
	}
	return out
}

func TestAuditLogCreateUpdateDelete(t *testing.T) {
	app := setupTestApp(t,
		WithWatchedCollections("items"),
		WithOwnerField("userId"),
		WithIgnoredFields("userId", "isImporting"),
		WithSkipFunc(func(e *core.RecordEvent) bool {
			return e.Record.GetBool("isImporting")
		}),
	)

	itemsCol, err := app.FindCollectionByNameOrId("items")
	if err != nil {
		t.Fatal(err)
	}

	// 1. CREATE
	item := core.NewRecord(itemsCol)
	item.Set("userId", "user_1")
	item.Set("title", "Frieren")
	item.Set("status", "watching")
	item.Set("secret", "top-secret")
	if err := app.Save(item); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	logs, err := app.FindAllRecords("auditLogs")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log after create, got %d", len(logs))
	}
	createLog := logs[0]
	if createLog.GetString("action") != ActionCreate {
		t.Errorf("expected action CREATE, got %q", createLog.GetString("action"))
	}
	if createLog.GetString("ownerId") != "user_1" {
		t.Errorf("expected ownerId user_1, got %q", createLog.GetString("ownerId"))
	}
	if createLog.GetString("actorId") != "system" {
		t.Errorf("expected actorId system, got %q", createLog.GetString("actorId"))
	}
	createChanges := decodeChanges(t, createLog)
	if _, ok := createChanges["secret"]; ok {
		t.Error("hidden field 'secret' should not be logged")
	}
	if _, ok := createChanges["userId"]; ok {
		t.Error("ignored field 'userId' should not be logged")
	}
	if _, ok := createChanges["rating"]; ok {
		t.Error("empty field 'rating' should be omitted on CREATE")
	}
	if ch, ok := createChanges["title"]; !ok || ch.Old != nil || ch.New != "Frieren" {
		t.Errorf("unexpected title create change: %+v", ch)
	}

	// 2. UPDATE (reload from DB as PocketBase does on update)
	item, err = app.FindRecordById("items", item.Id)
	if err != nil {
		t.Fatal(err)
	}
	item.Set("status", "completed")
	item.Set("rating", 10)
	if err := app.Save(item); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	logs, err = app.FindRecordsByFilter("auditLogs", "", "+created,+id", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs after update, got %d", len(logs))
	}
	updateChanges := decodeChanges(t, logs[1])
	if len(updateChanges) != 2 {
		t.Errorf("expected 2 changed fields (status, rating), got %d: %+v", len(updateChanges), updateChanges)
	}
	if ch := updateChanges["status"]; ch.Old != "watching" || ch.New != "completed" {
		t.Errorf("unexpected status update change: %+v", ch)
	}
	if ch := updateChanges["rating"]; ch.Old != float64(0) || ch.New != float64(10) {
		t.Errorf("unexpected rating update change: %+v", ch)
	}

	// 3. NO-OP UPDATE (reload and save without changes -> should not create a log)
	item, err = app.FindRecordById("items", item.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Save(item); err != nil {
		t.Fatalf("noop update failed: %v", err)
	}
	logs, _ = app.FindAllRecords("auditLogs")
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs after no-op update, got %d", len(logs))
	}

	// 4. DELETE (per-field old -> null, skipping already-empty fields)
	if err := app.Delete(item); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	logs, err = app.FindRecordsByFilter("auditLogs", "", "+created,+id", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 audit logs after delete, got %d", len(logs))
	}
	delLog := logs[2]
	if delLog.GetString("action") != ActionDelete {
		t.Errorf("expected action DELETE, got %q", delLog.GetString("action"))
	}
	delChanges := decodeChanges(t, delLog)
	if ch, ok := delChanges["title"]; !ok || ch.Old != "Frieren" || ch.New != nil {
		t.Errorf("unexpected title delete change: %+v", ch)
	}
	if ch, ok := delChanges["status"]; !ok || ch.Old != "completed" || ch.New != nil {
		t.Errorf("unexpected status delete change: %+v", ch)
	}

	// 5. SKIP FUNC (isImporting = true)
	imported := core.NewRecord(itemsCol)
	imported.Set("userId", "user_1")
	imported.Set("title", "Imported Anime")
	imported.Set("isImporting", true)
	if err := app.Save(imported); err != nil {
		t.Fatalf("import create failed: %v", err)
	}
	logs, _ = app.FindAllRecords("auditLogs")
	if len(logs) != 3 {
		t.Fatalf("expected 3 audit logs (skipped import), got %d", len(logs))
	}
}

func TestAuditLogHTTPRulesAndActor(t *testing.T) {
	newApp := func(t *testing.T) *tests.TestApp {
		t.Helper()
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}

		items := core.NewCollection(core.CollectionTypeBase, "items")
		items.ListRule = types.Pointer("userId = @request.auth.id")
		items.ViewRule = types.Pointer("userId = @request.auth.id")
		items.CreateRule = types.Pointer("@request.auth.id != ''")
		items.Fields.Add(
			&core.TextField{Name: "userId", Required: true},
			&core.TextField{Name: "title", Required: true},
		)
		if err := app.Save(items); err != nil {
			t.Fatal(err)
		}

		Register(app,
			WithWatchedCollections("items"),
			WithOwnerField("userId"),
			WithIgnoredFields("userId"),
		)
		if err := app.Bootstrap(); err != nil {
			t.Fatal(err)
		}
		return app
	}

	app := newApp(t)
	defer app.Cleanup()

	user, err := app.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Authenticated user creates an item via HTTP -> auditLog records actorId = user.Id
	(&tests.ApiScenario{
		Name:            "create item records actorId",
		Method:          http.MethodPost,
		URL:             "/api/collections/items/records",
		Body:            strings.NewReader(`{"userId":"` + user.Id + `","title":"Madoka"}`),
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"title":"Madoka"`},
		TestAppFactory:  func(t testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)

	logs, err := app.FindAllRecords("auditLogs")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if got := logs[0].GetString("actorId"); got != user.Id {
		t.Errorf("expected actorId %q, got %q", user.Id, got)
	}

	// 2. Owner can list their auditLogs
	(&tests.ApiScenario{
		Name:            "owner can list auditLogs",
		Method:          http.MethodGet,
		URL:             "/api/collections/auditLogs/records",
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"action":"CREATE"`, `"Madoka"`},
		TestAppFactory:  func(t testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)

	// 3. Client cannot POST directly to auditLogs (append-only)
	(&tests.ApiScenario{
		Name:            "client cannot create auditLogs directly",
		Method:          http.MethodPost,
		URL:             "/api/collections/auditLogs/records",
		Body:            strings.NewReader(`{"collection":"items","recordId":"x","action":"CREATE"}`),
		Headers:         map[string]string{"Authorization": token},
		ExpectedStatus:  http.StatusForbidden,
		ExpectedContent: []string{`"status":403`},
		TestAppFactory:        func(t testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
}
