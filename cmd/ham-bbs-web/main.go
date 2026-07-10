package main

import (
	"log"
	"net/http"
)

func main() {
	app, err := newServer()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s web app listening on %s with database %s", app.cfg.name, app.cfg.addr, app.cfg.dbFile)
	log.Fatal(http.ListenAndServe(app.cfg.addr, app.routes()))
}
