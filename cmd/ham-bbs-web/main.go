package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	app, err := newServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s web app listening on %s with database %s", app.cfg.name, app.cfg.addr, app.cfg.dbFile)
	// Counter and token rows have a retention, so something has to reclaim them.
	app.startMaintenance(24 * time.Hour)
	log.Fatal(http.ListenAndServe(app.cfg.addr, app.routes()))
}
