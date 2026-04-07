package ical

import (
	"strings"
	"testing"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
)

func testPlanner(t *testing.T) *planner.Planner {
	t.Helper()
	src := &stubSource{}
	p, err := planner.NewPlanner(src, "18:30", "Australia/Sydney")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	return p
}

type stubSource struct{}

func (s *stubSource) ListMealPlans() ([]string, error)                    { return nil, nil }
func (s *stubSource) ReadMealPlan(name string) (*model.MealPlan, error)   { return nil, nil }
func (s *stubSource) ReadRecipe(name string) (*model.Recipe, error)       { return nil, nil }

func testWeek() *model.WeekPlan {
	start := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) // Saturday
	week := &model.WeekPlan{
		WeekStart:    start,
		MealPlanName: "TestPlan",
		GeneratedAt:  time.Now(),
	}
	recipes := []struct {
		name      string
		totalTime string
		url       string
		ings      []string
	}{
		{"Gyoza", "PT0H15M0S", "https://example.com/gyoza", []string{"gyoza packet"}},
		{"Ramen", "PT1H0M0S", "", []string{"noodles", "broth"}},
		{"Sushi", "PT0H45M0S", "https://example.com/sushi", []string{"rice", "fish"}},
		{"Tempura", "PT0H30M0S", "", []string{"shrimp", "flour"}},
		{"Udon", "PT0H20M0S", "", []string{"udon noodles", "dashi"}},
		{"Curry", "PT1H30M0S", "", []string{"curry roux", "potato"}},
		{"Tonkatsu", "PT0H40M0S", "", []string{"pork cutlet", "panko"}},
	}
	for i, r := range recipes {
		week.Days = append(week.Days, model.DayMeal{
			Date:       start.AddDate(0, 0, i),
			RecipeName: r.name,
			Recipe: model.Recipe{
				Name:             r.name,
				TotalTime:        r.totalTime,
				URL:              r.url,
				RecipeIngredient: r.ings,
			},
		})
	}
	return week
}

func TestGenerate_BasicStructure(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	if !strings.HasPrefix(cal, "BEGIN:VCALENDAR\r\n") {
		t.Error("should start with BEGIN:VCALENDAR")
	}
	if !strings.HasSuffix(cal, "END:VCALENDAR\r\n") {
		t.Error("should end with END:VCALENDAR")
	}
	if !strings.Contains(cal, "VERSION:2.0") {
		t.Error("should contain VERSION:2.0")
	}
}

func TestGenerate_DinnerEvents(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	// Should have 7 dinner events
	count := strings.Count(cal, "SUMMARY:Dinner: ")
	if count != 7 {
		t.Errorf("got %d dinner events, want 7", count)
	}

	if !strings.Contains(cal, "SUMMARY:Dinner: Gyoza") {
		t.Error("should contain Gyoza dinner event")
	}
	if !strings.Contains(cal, "SUMMARY:Dinner: Ramen") {
		t.Error("should contain Ramen dinner event")
	}
}

func TestGenerate_DinnerEndTime(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	// All dinner events should end at 18:30 with TZID
	if !strings.Contains(cal, "DTEND;TZID=Australia/Sydney:20260411T183000") {
		t.Error("first dinner should end at 18:30 with TZID")
	}
}

func TestGenerate_RecipeURL(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	if !strings.Contains(cal, "URL:https://example.com/gyoza") {
		t.Error("should include recipe URL field for Gyoza")
	}
	if !strings.Contains(cal, "URL:https://example.com/sushi") {
		t.Error("should include recipe URL field for Sushi")
	}
}

func TestGenerate_ShoppingEvent(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	if !strings.Contains(cal, "SUMMARY:Shopping List - TestPlan") {
		t.Error("should contain shopping list event")
	}
	// Shopping list should be grouped by dish (iCal uses \n literals and line folding)
	// Unfold lines first for easier matching
	unfolded := strings.ReplaceAll(cal, "\r\n ", "")
	if !strings.Contains(unfolded, "== Gyoza (Saturday) ==") {
		t.Error("shopping list should group ingredients by dish")
	}
	if !strings.Contains(unfolded, "== Ramen (Sunday) ==") {
		t.Error("shopping list should include Ramen section")
	}
}

func TestGenerate_Timezone(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate([]*model.WeekPlan{testWeek()})

	if !strings.Contains(cal, "Australia/Sydney") {
		t.Error("should contain configured timezone")
	}
}

func TestGenerate_EmptyWeeks(t *testing.T) {
	p := testPlanner(t)
	g, err := NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	cal := g.Generate(nil)

	if !strings.Contains(cal, "BEGIN:VCALENDAR") {
		t.Error("should still produce valid calendar")
	}
	if strings.Contains(cal, "BEGIN:VEVENT") {
		t.Error("should have no events")
	}
}

func TestNewGenerator_InvalidTime(t *testing.T) {
	p := testPlanner(t)
	_, err := NewGenerator(p, "invalid", "Saturday")
	if err == nil {
		t.Error("expected error for invalid time")
	}
}

func TestNewGenerator_InvalidDay(t *testing.T) {
	p := testPlanner(t)
	_, err := NewGenerator(p, "12:00", "Notaday")
	if err == nil {
		t.Error("expected error for invalid day")
	}
}
