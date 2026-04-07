package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
)

func tempStorePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test_weeks.json")
}

func makeWeek(start time.Time, name string) *model.WeekPlan {
	week := &model.WeekPlan{
		WeekStart:    start,
		MealPlanName: name,
		GeneratedAt:  time.Now(),
	}
	for i := 0; i < 7; i++ {
		week.Days = append(week.Days, model.DayMeal{
			Date:       start.AddDate(0, 0, i),
			RecipeName: "Test Recipe",
			Recipe:     model.Recipe{Name: "Test Recipe"},
		})
	}
	return week
}

func TestNew_CreatesFile(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	weeks := s.GetCurrentWeeks()
	if len(weeks) != 0 {
		t.Errorf("expected 0 weeks, got %d", len(weeks))
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	weekStart := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC) // far future Saturday
	week := makeWeek(weekStart, "TestPlan")

	if err := s.Save(week); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload from disk
	s2, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}

	weeks := s2.GetCurrentWeeks()
	if len(weeks) != 1 {
		t.Fatalf("expected 1 week, got %d", len(weeks))
	}
	if weeks[0].MealPlanName != "TestPlan" {
		t.Errorf("plan name = %q, want TestPlan", weeks[0].MealPlanName)
	}
}

func TestHasWeek(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	weekStart := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC)
	if s.HasWeek(weekStart) {
		t.Error("should not have week before saving")
	}

	if err := s.Save(makeWeek(weekStart, "Plan")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !s.HasWeek(weekStart) {
		t.Error("should have week after saving")
	}
}

func TestCleanup_RemovesPastWeeks(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pastStart := time.Date(2020, 1, 4, 0, 0, 0, 0, time.UTC)
	futureStart := time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC)

	if err := s.Save(makeWeek(pastStart, "Old")); err != nil {
		t.Fatalf("Save past: %v", err)
	}
	if err := s.Save(makeWeek(futureStart, "Future")); err != nil {
		t.Fatalf("Save future: %v", err)
	}

	weeks := s.GetCurrentWeeks()
	if len(weeks) != 1 {
		t.Errorf("expected 1 current week, got %d", len(weeks))
	}

	// Past week should have been cleaned from the file
	s2, err := New(path, time.UTC)
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}
	if s2.HasWeek(pastStart) {
		t.Error("past week should have been cleaned up")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	path := tempStorePath(t)
	// Write invalid JSON
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("writing invalid file: %v", err)
	}
	_, err := New(path, time.UTC)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
