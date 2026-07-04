package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aaronjauregui/chefcal/internal/caldav"
	"github.com/aaronjauregui/chefcal/internal/config"
	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/nextcloud"
	"github.com/aaronjauregui/chefcal/internal/planner"
	"github.com/aaronjauregui/chefcal/internal/push"
	"github.com/aaronjauregui/chefcal/internal/server"
	"github.com/aaronjauregui/chefcal/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	generate := flag.Bool("generate", false, "generate the next unplanned week, save it, and exit")
	pushCal := flag.Bool("push", false, "reconcile stored plans to the Nextcloud calendar and exit")
	planName := flag.String("plan", "", "meal plan for -generate (random if empty)")
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

	st, err := store.New(cfg.Store.Path, p.Location())
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	// One-shot actions (suitable for a cronjob). When either is set we run the
	// requested steps and exit rather than starting the HTTP server.
	if *generate || *pushCal {
		if *generate {
			if err := generateNextWeek(p, st, *planName); err != nil {
				log.Fatalf("Failed to generate week: %v", err)
			}
		}
		if *pushCal {
			if err := pushCalendar(cfg, ig, st, p.Location()); err != nil {
				log.Fatalf("Failed to push calendar: %v", err)
			}
		}
		return
	}

	srv := server.New(nc, p, ig, st)

	addr := cfg.Server.Address
	fmt.Printf("ChefCal listening on %s\n", addr)
	fmt.Printf("  Calendar feed: http://localhost%s/calendar.ics\n", addr)
	fmt.Printf("  Web UI:        http://localhost%s/\n", addr)
	log.Fatal(http.ListenAndServe(addr, srv))
}

// generateNextWeek generates and stores the next week that isn't already
// planned, mirroring the behaviour of the HTTP /generate endpoint.
func generateNextWeek(p *planner.Planner, st *store.Store, planName string) error {
	if planName == "" {
		var err error
		planName, err = p.PickRandomPlan()
		if err != nil {
			return fmt.Errorf("picking random plan: %w", err)
		}
	}

	weekStart := planner.NextWeekStart(time.Now(), p.Location())
	for st.HasWeek(weekStart) {
		weekStart = weekStart.AddDate(0, 0, 7)
	}

	week, err := p.GenerateWeek(weekStart, planName)
	if err != nil {
		return fmt.Errorf("generating week: %w", err)
	}
	if err := st.Save(week); err != nil {
		return fmt.Errorf("saving week: %w", err)
	}

	log.Printf("Generated week starting %s with plan %q", weekStart.Format("2006-01-02"), planName)
	return nil
}

// pushCalendar reconciles the current stored plans onto the configured
// Nextcloud calendar.
func pushCalendar(cfg *config.Config, ig *ical.Generator, st *store.Store, loc *time.Location) error {
	if cfg.Nextcloud.CalendarURL == "" {
		return fmt.Errorf("nextcloud.calendar_url is required for -push")
	}

	dav := caldav.NewClient(
		cfg.Nextcloud.CalendarURL,
		cfg.Nextcloud.Username,
		cfg.Nextcloud.Password,
		cfg.Nextcloud.InsecureSkipVerify,
	)

	pusher := push.New(dav, ig, loc, cfg.Nextcloud.CalendarDisplayName)
	res, err := pusher.Reconcile(st.GetCurrentWeeks(), time.Now())
	if err != nil {
		return err
	}

	log.Printf("Push complete: %d entries published, %d stale entries removed", res.Pushed, res.Deleted)
	return nil
}
