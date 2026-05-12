package main

import (
	"flag"
	"log"

	"github.com/johndemoss/mocktracegen/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	srv := server.New(*addr)
	log.Fatal(srv.ListenAndServe())
}
