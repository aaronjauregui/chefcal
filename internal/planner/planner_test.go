package planner

import (
	"fmt"
	"testing"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
)

// mockSource implements model.RecipeSource for testing.
type mockSource struct {
	plans   map[string]*model.MealPlan
	recipes map[string]*model.Recipe
}

func (m *mockSource) ListMealPlans() ([]string, error) {
	var names []string
	for k := range m.plans {
		names = append(names, k)
	}
	return names, nil
}

func (m *mockSource) ReadMealPlan(name string) (*model.MealPlan, error) {
	p, ok := m.plans[name]
	if !ok {
		return nil, fmt.Errorf("plan %q not found", name)
	}
	return p, nil
}

func (m *mockSource) ReadRecipe(name string) (*model.Recipe, error) {
	r, ok := m.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found", name)
	}
	return r, nil
}

func newTestSource() *mockSource {
	recipes := map[string]*model.Recipe{
		"Recipe A": {Name: "Recipe A", TotalTime: "PT1H0M0S", RecipeIngredient: []string{"ing1"}},
		"Recipe B": {Name: "Recipe B", TotalTime: "PT0H30M0S", RecipeIngredient: []string{"ing2"}},
		"Recipe C": {Name: "Recipe C", TotalTime: "PT0H45M0S", RecipeIngredient: []string{"ing3"}},
		"Recipe D": {Name: "Recipe D", TotalTime: "PT0H20M0S", RecipeIngredient: []string{"ing4"}},
		"Recipe E": {Name: "Recipe E", TotalTime: "PT1H30M0S", RecipeIngredient: []string{"ing5"}},
		"Recipe F": {Name: "Recipe F", TotalTime: "PT0H15M0S", RecipeIngredient: []string{"ing6"}},
		"Recipe G": {Name: "Recipe G", TotalTime: "PT0H50M0S", RecipeIngredient: []string{"ing7"}},
		"Missing":  {Name: "Missing", TotalTime: "PT0H10M0S"},
	}
	plans := map[string]*model.MealPlan{
		"TestPlan": {
			Name:    "TestPlan",
			Recipes: []string{"Recipe A", "Recipe B", "Recipe C", "Recipe D", "Recipe E", "Recipe F", "Recipe G"},
		},
		"SmallPlan": {
			Name:    "SmallPlan",
			Recipes: []string{"Recipe A", "Recipe B"},
		},
		"WithMissing": {
			Name:    "WithMissing",
			Recipes: []string{"Recipe A", "Recipe B", "Recipe C", "Recipe D", "Recipe E", "Recipe F", "Recipe G", "DoesNotExist"},
		},
	}
	return &mockSource{plans: plans, recipes: recipes}
}

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"PT1H0M0S", 1 * time.Hour},
		{"PT0H30M0S", 30 * time.Minute},
		{"PT1H10M0S", 1*time.Hour + 10*time.Minute},
		{"PT0H0M30S", 30 * time.Second},
		{"PT2H", 2 * time.Hour},
		{"PT45M", 45 * time.Minute},
		{"P1DT2H", 26 * time.Hour},
		{"P2D", 48 * time.Hour},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseISO8601Duration(tt.input)
			if got != tt.want {
				t.Errorf("ParseISO8601Duration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTimeOfDay(t *testing.T) {
	tod, err := ParseTimeOfDay("18:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tod.Hour != 18 || tod.Minute != 30 {
		t.Errorf("got %d:%d, want 18:30", tod.Hour, tod.Minute)
	}

	_, err = ParseTimeOfDay("invalid")
	if err == nil {
		t.Error("expected error for invalid time")
	}
}

func TestNextWeekStart(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name     string
		from     time.Time
		wantDay  time.Weekday
		wantDate string
	}{
		{
			name:     "from Monday",
			from:     time.Date(2026, 4, 6, 0, 0, 0, 0, loc), // Monday
			wantDay:  time.Saturday,
			wantDate: "2026-04-11",
		},
		{
			name:     "from Friday",
			from:     time.Date(2026, 4, 10, 0, 0, 0, 0, loc), // Friday
			wantDay:  time.Saturday,
			wantDate: "2026-04-11",
		},
		{
			name:     "from Saturday gives next Saturday",
			from:     time.Date(2026, 4, 11, 0, 0, 0, 0, loc), // Saturday
			wantDay:  time.Saturday,
			wantDate: "2026-04-18",
		},
		{
			name:     "from Sunday",
			from:     time.Date(2026, 4, 12, 0, 0, 0, 0, loc), // Sunday
			wantDay:  time.Saturday,
			wantDate: "2026-04-18",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextWeekStart(tt.from, loc)
			if got.Weekday() != tt.wantDay {
				t.Errorf("got weekday %v, want %v", got.Weekday(), tt.wantDay)
			}
			if got.Format("2006-01-02") != tt.wantDate {
				t.Errorf("got %s, want %s", got.Format("2006-01-02"), tt.wantDate)
			}
		})
	}
}

func TestGenerateWeek_UniqueRecipes(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	weekStart := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	week, err := p.GenerateWeek(weekStart, "TestPlan")
	if err != nil {
		t.Fatalf("GenerateWeek: %v", err)
	}

	if len(week.Days) != 7 {
		t.Fatalf("got %d days, want 7", len(week.Days))
	}

	seen := make(map[string]bool)
	for _, d := range week.Days {
		if seen[d.RecipeName] {
			t.Errorf("duplicate recipe: %s", d.RecipeName)
		}
		seen[d.RecipeName] = true
	}
}

func TestGenerateWeek_TooFewRecipes(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	weekStart := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	_, err = p.GenerateWeek(weekStart, "SmallPlan")
	if err == nil {
		t.Error("expected error for plan with fewer than 7 recipes")
	}
}

func TestGenerateWeek_SkipsMissingRecipes(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	weekStart := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	week, err := p.GenerateWeek(weekStart, "WithMissing")
	if err != nil {
		t.Fatalf("GenerateWeek: %v", err)
	}

	for _, d := range week.Days {
		if d.RecipeName == "DoesNotExist" {
			t.Error("missing recipe should have been skipped")
		}
	}
}

func TestGenerateWeek_DatesStartOnWeekStart(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	weekStart := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC) // Saturday
	week, err := p.GenerateWeek(weekStart, "TestPlan")
	if err != nil {
		t.Fatalf("GenerateWeek: %v", err)
	}

	if week.Days[0].Date != weekStart {
		t.Errorf("first day = %v, want %v", week.Days[0].Date, weekStart)
	}
	lastDay := weekStart.AddDate(0, 0, 6)
	if week.Days[6].Date != lastDay {
		t.Errorf("last day = %v, want %v", week.Days[6].Date, lastDay)
	}
}

func TestDinnerTimes(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "Australia/Sydney")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	date := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	recipe := &model.Recipe{TotalTime: "PT1H0M0S"}

	end := p.DinnerEndTime(date)
	if end.Hour() != 18 || end.Minute() != 30 {
		t.Errorf("dinner end = %v, want 18:30", end)
	}

	start := p.DinnerStartTime(date, recipe)
	if start.Hour() != 17 || start.Minute() != 30 {
		t.Errorf("dinner start = %v, want 17:30", start)
	}
}

func TestDinnerStartTime_DefaultDuration(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	date := time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
	recipe := &model.Recipe{TotalTime: ""}

	start := p.DinnerStartTime(date, recipe)
	if start.Hour() != 18 || start.Minute() != 0 {
		t.Errorf("dinner start = %v, want 18:00 (30min default)", start)
	}
}

func TestPickRandomPlan(t *testing.T) {
	src := newTestSource()
	p, err := NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	name, err := p.PickRandomPlan()
	if err != nil {
		t.Fatalf("PickRandomPlan: %v", err)
	}
	if name == "" {
		t.Error("expected a plan name")
	}
}

func TestNewPlanner_InvalidTimezone(t *testing.T) {
	src := newTestSource()
	_, err := NewPlanner(src, "18:30", "Invalid/Zone")
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}
