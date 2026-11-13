package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	app, err := newServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s web app listening on %s with database %s", app.cfg.name, app.cfg.addr, app.cfg.dbFile)
	// The database is shared with the classic BBS and that one is canonical. This
	// process holds the file open, so it must end rather than keep serving a copy
	// that has been replaced on disk. Exiting lets the supervisor restart it,
	// which reopens whatever is there now.
	app.watchDatabaseFile(dbFilePollInterval, func(reason string) {
		app.logBBSAction("system", "database_file_changed", "reason=%q path=%q", reason, app.cfg.dbFile)
		log.Printf("database file changed under this process (%s): exiting so the supervisor reopens %s", reason, app.cfg.dbFile)
		os.Exit(1)
	})
	// Counter and token rows have a retention, so something has to reclaim them.
	app.startMaintenance(24 * time.Hour)
	log.Fatal(http.ListenAndServe(app.cfg.addr, app.routes()))
}
