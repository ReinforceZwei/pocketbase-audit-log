// Package auditlog provides automatic field-level audit logging for PocketBase collections.
//
// Register the plugin with a PocketBase app:
//
//	auditlog.Register(app,
//		auditlog.WithWatchedCollections("animes", "tags"),
//		auditlog.WithOwnerField("userId"),
//	)
package auditlog

import (
	"log"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

const (
	ActionCreate = "CREATE"
	ActionUpdate = "UPDATE"
	ActionDelete = "DELETE"
)

// DefaultIgnoredFields are always excluded from audit diffs unless overridden.
var DefaultIgnoredFields = []string{"id", "created", "updated"}

// SkipFunc decides whether a record event should be skipped from audit logging.
// Return true to skip logging the event.
type SkipFunc func(e *core.RecordEvent) bool

// Config holds all customizable settings for the audit log plugin.
type Config struct {
	// CollectionName is the name of the collection that stores audit logs.
	// Default: "auditLogs"
	CollectionName string

	// CollectionID is the stable ID for the auditLogs collection.
	// Default: "pbc_auditlogs_plugin"
	CollectionID string

	// WatchedCollections lists the collection names to audit.
	WatchedCollections []string

	// IgnoredFields lists field names excluded from diffs.
	// Default: ["id", "created", "updated"]
	IgnoredFields []string

	// OwnerField is the field name on watched records that identifies the owner
	// (e.g. "userId"). When non-empty, its value is copied to auditLogs.ownerId
	// and List/View rules restrict access to ownerId = @request.auth.id.
	// When empty, List/View rules are nil (superuser-only).
	OwnerField string

	// SkipFunc is an optional predicate; when it returns true, no audit log is written.
	SkipFunc SkipFunc

	// RetentionDays automatically prunes audit log entries older than N days via
	// PocketBase's cron scheduler. 0 = keep forever. Default: 0.
	RetentionDays int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	ignored := make([]string, len(DefaultIgnoredFields))
	copy(ignored, DefaultIgnoredFields)
	return Config{
		CollectionName: "auditLogs",
		CollectionID:   "pbc_auditlogs_plugin",
		IgnoredFields:  ignored,
		RetentionDays:  0,
	}
}

// Option is a functional option for configuring the plugin.
type Option func(*Config)

// WithCollectionName overrides the audit log collection name.
func WithCollectionName(name string) Option {
	return func(c *Config) {
		if name != "" {
			c.CollectionName = name
		}
	}
}

// WithCollectionID overrides the audit log collection ID.
func WithCollectionID(id string) Option {
	return func(c *Config) {
		if id != "" {
			c.CollectionID = id
		}
	}
}

// WithWatchedCollections sets the collection names to audit.
func WithWatchedCollections(collections ...string) Option {
	return func(c *Config) {
		c.WatchedCollections = append([]string(nil), collections...)
	}
}

// WithIgnoredFields appends additional field names to ignore in diffs.
func WithIgnoredFields(fields ...string) Option {
	return func(c *Config) {
		c.IgnoredFields = append(c.IgnoredFields, fields...)
	}
}

// WithOwnerField sets the record field name used to populate ownerId and scope API read rules.
func WithOwnerField(field string) Option {
	return func(c *Config) {
		c.OwnerField = field
	}
}

// WithSkipFunc sets a predicate to skip audit logging for matching record events.
func WithSkipFunc(fn SkipFunc) Option {
	return func(c *Config) {
		c.SkipFunc = fn
	}
}

// WithRetentionDays enables daily cron pruning of audit logs older than the given days.
// Values <= 0 disable pruning (keep forever).
func WithRetentionDays(days int) Option {
	return func(c *Config) {
		c.RetentionDays = days
	}
}

// Register wires the audit log plugin into a PocketBase app.
// Must be called once before app.Start().
func Register(app core.App, opts ...Option) {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.CollectionName == "" || cfg.CollectionID == "" {
		log.Fatalf("auditlog: collection name and id must not be empty")
	}

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return ensureCollection(app, cfg)
	})

	if len(cfg.WatchedCollections) > 0 {
		registerHooks(app, cfg)
	}

	if cfg.RetentionDays > 0 {
		registerRetentionCron(app, cfg)
	}
}
