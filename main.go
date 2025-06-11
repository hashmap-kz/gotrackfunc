package main

import (
	"flag"
	"log"

	"github.com/hashmap-kz/gotrackfunc/internal/gtf"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("Usage: gotrackfunc ./... or gotrackfunc file.go")
	}
	gtf.RunApp(args)
}
