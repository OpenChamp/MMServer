package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

var dbPool *pgxpool.Pool

func StartWebServer(port int, pool *pgxpool.Pool) {
	dbPool = pool
	// Default Port
	if port == 0 {
		port = 8080
	}
	// Routing
	SetupRoutes()
	// Start Server
	fmt.Println("Starting web server on :" + fmt.Sprint(port) + "...")
	if err := http.ListenAndServe(":"+fmt.Sprint(port), nil); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}
