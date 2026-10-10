package main

import (
	"log"

	"exht-proxy/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
