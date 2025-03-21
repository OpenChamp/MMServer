package main

import (
	"fmt"
	"log"
	"net/http"
	"openchamp/server/internal/api"
	"openchamp/server/internal/database"
	"openchamp/server/internal/util"
	"openchamp/server/internal/websocket"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const dbConnString = "postgres://postgres:password@localhost:5432/openchamp"

var dbPool *pgxpool.Pool

func main() {
	util.ConsoleTitle()
	dbPool, err := database.InitDBPool(dbConnString)
	if err != nil {
		log.Fatal(err)
	}
	setup_err := database.SetupDatabase(dbPool)
	if setup_err != nil {
		log.Fatal(setup_err)
	}
	if err != nil {
		log.Fatal(err)
	}
	go api.StartWebServer(8080, dbPool)
	go websocket.StartWebSocketServer(8081, dbPool)

	// Update Console
	for range time.Tick(5 * time.Second) {
		update_console()
	}
}

func update_console() {
	util.ConsoleTitle()

	// Webserver Checkin
	resp, err := http.Get("http://localhost:8080/")
	if err != nil {
		log.Fatal(err)
	} // if response is 200, print the result
	fmt.Println("WebServer Status: " + fmt.Sprint(resp.StatusCode))

	// WebSocket Checkin
	resp, err = http.Get("http://localhost:8081/")
	if err != nil {
		log.Fatal(err)
	} // if response is 200, print the result
	fmt.Println("WebSocket Status: " + fmt.Sprint(resp.StatusCode))
}
