package auditlog

import (
	"log"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

func registerRetentionCron(app core.App, cfg Config) {
	app.Cron().MustAdd("auditlog_retention", "0 3 * * *", func() {
		if err := pruneOldLogs(app, cfg); err != nil {
			log.Printf("[auditlog] retention prune failed: %v", err)
		}
	})
}

func pruneOldLogs(app core.App, cfg Config) error {
	if cfg.RetentionDays <= 0 {
		return nil
	}

	cutoff, err := types.ParseDateTime(time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays))
	if err != nil {
		return err
	}

	_, err = app.DB().
		Delete(cfg.CollectionName, dbx.NewExp("created < {:cutoff}", dbx.Params{"cutoff": cutoff})).
		Execute()
	return err
}
