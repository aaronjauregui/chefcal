package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
	"github.com/aaronjauregui/chefcal/internal/store"
)

type Server struct {
	nc      model.RecipeSource
	planner *planner.Planner
	ical    *ical.Generator
	store   *store.Store
	mux     *http.ServeMux
}

func New(nc model.RecipeSource, p *planner.Planner, ig *ical.Generator, s *store.Store) *Server {
	srv := &Server{
		nc:      nc,
		planner: p,
		ical:    ig,
		store:   s,
		mux:     http.NewServeMux(),
	}
	srv.mux.HandleFunc("GET /calendar.ics", srv.handleCalendar)
	srv.mux.HandleFunc("POST /generate", srv.handleGenerate)
	srv.mux.HandleFunc("GET /plans", srv.handlePlans)
	srv.mux.HandleFunc("GET /", srv.handleIndex)
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	weeks := s.store.GetCurrentWeeks()
	cal := s.ical.Generate(weeks)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"mealplan.ics\"")
	fmt.Fprint(w, cal)
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	planName := r.URL.Query().Get("plan")

	if planName == "" {
		var err error
		planName, err = s.planner.PickRandomPlan()
		if err != nil {
			http.Error(w, fmt.Sprintf("picking random plan: %v", err), http.StatusInternalServerError)
			return
		}
	}

	weekStart := planner.NextWeekStart(time.Now(), s.planner.Location())
	for s.store.HasWeek(weekStart) {
		weekStart = weekStart.AddDate(0, 0, 7)
	}

	week, err := s.planner.GenerateWeek(weekStart, planName)
	if err != nil {
		http.Error(w, fmt.Sprintf("generating week: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.store.Save(week); err != nil {
		http.Error(w, fmt.Sprintf("saving week: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Generated week starting %s with plan %q", weekStart.Format("2006-01-02"), planName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"week_start": weekStart.Format("2006-01-02"),
		"plan":       planName,
		"days":       formatDays(week),
	})
}

func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.nc.ListMealPlans()
	if err != nil {
		http.Error(w, fmt.Sprintf("listing plans: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plans)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	weeks := s.store.GetCurrentWeeks()
	plans, _ := s.nc.ListMealPlans()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderIndex(weeks, plans, r.Host))
}

func formatDays(week *model.WeekPlan) []map[string]string {
	days := make([]map[string]string, len(week.Days))
	for i, d := range week.Days {
		days[i] = map[string]string{
			"date":   d.Date.Format("Monday, Jan 2"),
			"recipe": d.RecipeName,
		}
	}
	return days
}
