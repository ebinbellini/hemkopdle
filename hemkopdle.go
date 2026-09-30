package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ebinbellini/hemkopdle/scraper"
	"github.com/ebinbellini/hemkopdle/server"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	fmt.Println("Starting server")
	go server.Run()

	fmt.Println("Running scraper")
	go scraper.Scrape()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Activate CRON schedule for scraper")
	sched := os.Getenv("SCRAPE_CRON_SCHED")
	c := cron.New()
	_, err = c.AddFunc(sched, func() {
		fmt.Println("Running shceduled scraper")
		err := scraper.Scrape()
		if err != nil {
			log.Fatal(err)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	c.Start()

	select {}
}
