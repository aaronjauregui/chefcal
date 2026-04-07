package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
	"github.com/aaronjauregui/chefcal/internal/store"
)

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

func setupServer(t *testing.T) (*Server, *mockSource) {
	t.Helper()

	recipes := make(map[string]*model.Recipe)
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("Recipe %d", i)
		recipes[name] = &model.Recipe{
			Name:             name,
			TotalTime:        "PT0H30M0S",
			RecipeIngredient: []string{fmt.Sprintf("ingredient %d", i)},
		}
	}

	recipeNames := make([]string, 0, 8)
	for name := range recipes {
		recipeNames = append(recipeNames, name)
	}

	src := &mockSource{
		plans: map[string]*model.MealPlan{
			"TestPlan": {Name: "TestPlan", Recipes: recipeNames},
		},
		recipes: recipes,
	}

	p, err := planner.NewPlanner(src, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	ig, err := ical.NewGenerator(p, "12:00", "Saturday")
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	storePath := filepath.Join(t.TempDir(), "weeks.json")
	st, err := store.New(storePath, time.UTC)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	srv := New(src, p, ig, st)
	return srv, src
}

func TestHandlePlans(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("GET", "/plans", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type = %q", w.Header().Get("Content-Type"))
	}

	var plans []string
	if err := json.Unmarshal(w.Body.Bytes(), &plans); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(plans) != 1 || plans[0] != "TestPlan" {
		t.Errorf("plans = %v, want [TestPlan]", plans)
	}
}

func TestHandleGenerate(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("POST", "/generate?plan=TestPlan", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["plan"] != "TestPlan" {
		t.Errorf("plan = %v, want TestPlan", resp["plan"])
	}
	if resp["week_start"] == nil {
		t.Error("missing week_start")
	}
	days, ok := resp["days"].([]any)
	if !ok || len(days) != 7 {
		t.Errorf("expected 7 days, got %v", resp["days"])
	}
}

func TestHandleGenerate_RandomPlan(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("POST", "/generate", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestHandleGenerate_InvalidPlan(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("POST", "/generate?plan=NonExistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHandleCalendar(t *testing.T) {
	srv, _ := setupServer(t)

	// Generate a week first
	req := httptest.NewRequest("POST", "/generate?plan=TestPlan", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("generate failed: %d", w.Code)
	}

	// Now fetch calendar
	req = httptest.NewRequest("GET", "/calendar.ics", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	body := w.Body.String()
	if body == "" || !contains(body, "BEGIN:VCALENDAR") {
		t.Error("invalid calendar output")
	}
}

func TestHandleCalendar_Empty(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("GET", "/calendar.ics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "BEGIN:VCALENDAR") {
		t.Error("should produce valid empty calendar")
	}
}

func TestHandleIndex(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if !contains(w.Body.String(), "ChefCal") {
		t.Error("should contain ChefCal title")
	}
}

func TestHandleIndex_NotFound(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleGenerate_SkipsExistingWeek(t *testing.T) {
	srv, _ := setupServer(t)

	// Generate first week
	req := httptest.NewRequest("POST", "/generate?plan=TestPlan", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first generate: %d", w.Code)
	}
	var resp1 map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp1)

	// Generate second week — should be a different week
	req = httptest.NewRequest("POST", "/generate?plan=TestPlan", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second generate: %d", w.Code)
	}
	var resp2 map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp2)

	if resp1["week_start"] == resp2["week_start"] {
		t.Error("second generate should target a different week")
	}
}

func TestFormatDays(t *testing.T) {
	week := &model.WeekPlan{
		Days: []model.DayMeal{
			{Date: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC), RecipeName: "Test"},
		},
	}
	days := formatDays(week)
	if len(days) != 1 {
		t.Fatalf("expected 1 day, got %d", len(days))
	}
	if days[0]["recipe"] != "Test" {
		t.Errorf("recipe = %q", days[0]["recipe"])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
