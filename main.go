package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/aaronjauregui/chefcal/internal/config"
	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/nextcloud"
	"github.com/aaronjauregui/chefcal/internal/planner"
	"github.com/aaronjauregui/chefcal/internal/server"
	"github.com/aaronjauregui/chefcal/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	nc := nextcloud.NewClient(
		cfg.Nextcloud.URL,
		cfg.Nextcloud.Username,
		cfg.Nextcloud.Password,
		cfg.Nextcloud.MealPlansPath,
		cfg.Nextcloud.RecipesPath,
		cfg.Nextcloud.InsecureSkipVerify,
	)

	p, err := planner.NewPlanner(nc, cfg.Planner.DinnerDoneBy, cfg.Planner.Timezone)
	if err != nil {
		log.Fatalf("Failed to create planner: %v", err)
	}

	ig, err := ical.NewGenerator(p, cfg.Planner.ShoppingEventTime, cfg.Planner.ShoppingEventDay)
	if err != nil {
		log.Fatalf("Failed to create ical generator: %v", err)
	}

	st, err := store.New(cfg.Store.Path)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	srv := server.New(nc, p, ig, st)

	addr := cfg.Server.Address
	fmt.Printf("ChefCal listening on %s\n", addr)
	fmt.Printf("  Calendar feed: http://localhost%s/calendar.ics\n", addr)
	fmt.Printf("  Web UI:        http://localhost%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}
