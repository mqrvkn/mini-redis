package main

import (
	"flag"
	"log"

	"mini-redis/internal/server"
	"mini-redis/internal/store"
)

func main() {
	addr := flag.String("addr", ":6380", "TCP address to listen on")
	flag.Parse()

	db := store.New()
	srv := server.New(*addr, db)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
