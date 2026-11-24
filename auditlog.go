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

// MetadataFunc returns an optional custom metadata map to attach to an audit log entry.
// Returning nil or an empty map stores no metadata.
type MetadataFunc func(e *core.RecordEvent) map[string]any

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

	// OwnerCollection is the auth/owner collection name (default: "users").
	// When both OwnerField and OwnerCollection are non-empty, deleting a record
	// from OwnerCollection automatically purges its auditLogs entries.
	OwnerCollection string

	// SkipFunc is an optional predicate; when it returns true, no audit log is written.
	SkipFunc SkipFunc

	// MetadataFunc is an optional callback to attach custom metadata JSON to each entry.
	MetadataFunc MetadataFunc

	// RetentionDays automatically prunes audit log entries older than N days via
	// PocketBase's cron scheduler. 0 = keep forever. Default: 0.
	RetentionDays int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	ignored := make([]string, len(DefaultIgnoredFields))
	copy(ignored, DefaultIgnoredFields)
	return Config{
		CollectionName:  "auditLogs",
		CollectionID:    "pbc_auditlogs_plugin",
		IgnoredFields:   ignored,
		OwnerCollection: "users",
		RetentionDays:   0,
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

// WithOwnerCollection sets the owner collection name whose deleted records purge matching auditLogs.
// Default: "users". Pass "" to disable automatic purge on owner deletion.
func WithOwnerCollection(name string) Option {
	return func(c *Config) {
		c.OwnerCollection = name
	}
}

// WithSkipFunc sets a predicate to skip audit logging for matching record events.
func WithSkipFunc(fn SkipFunc) Option {
	return func(c *Config) {
		c.SkipFunc = fn
	}
}

// WithMetadataFunc sets a callback to attach custom metadata JSON to each audit log entry.
func WithMetadataFunc(fn MetadataFunc) Option {
	return func(c *Config) {
		c.MetadataFunc = fn
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
