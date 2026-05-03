package tasks

import (
	"context"
	"strings"
	"time"
)

const (
	databaseBusyRetryDelay = 50 * time.Millisecond
	databaseBusyRetryLimit = 20
)

func retryDatabaseBusy(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt <= databaseBusyRetryLimit; attempt++ {
		err = fn(ctx)
		if !isDatabaseBusy(err) || attempt == databaseBusyRetryLimit {
			return err
		}
		timer := time.NewTimer(databaseBusyRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isDatabaseBusy(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "SQLITE_BUSY") || strings.Contains(text, "database is locked")
}
