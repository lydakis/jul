package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/lydakis/jul/server/internal/server"
)

const version = "0.0.1"

func main() {
	addr := flag.String("addr", ":8000", "HTTP listen address")
	flag.Parse()

	fmt.Printf("jul-server %s listening on %s\n", version, *addr)

	srv := server.New(server.Config{Address: *addr})
	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
