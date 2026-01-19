package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/lydakis/jul/server/internal/events"
	"github.com/lydakis/jul/server/internal/server"
	"github.com/lydakis/jul/server/internal/storage"
)

const version = "0.0.1"

func main() {
	addr := flag.String("addr", ":8000", "HTTP listen address")
	dbPath := flag.String("db", "var/jul/data/jul.db", "SQLite database path")
	baseURL := flag.String("base-url", "", "Public base URL (optional)")
	flag.Parse()

	fmt.Printf("jul-server %s listening on %s\n", version, *addr)

	store, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("failed to close database: %v", err)
		}
	}()

	broker := events.NewBroker()
	srv := server.New(server.Config{Address: *addr, BaseURL: *baseURL}, store, broker)
	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
