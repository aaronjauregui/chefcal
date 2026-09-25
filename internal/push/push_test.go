package push

import (
	"sort"
	"testing"
	"time"

	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
)

type fakeDAV struct {
	existing []string          // resources already on the server
	put      map[string]string // name -> ics
	deleted  []string
	ensured  bool
}

func newFakeDAV(existing ...string) *fakeDAV {
	return &fakeDAV{existing: existing, put: map[string]string{}}
}

func (f *fakeDAV) EnsureCalendar(string) error { f.ensured = true; return nil }
func (f *fakeDAV) List() ([]string, error)     { return f.existing, nil }
func (f *fakeDAV) Put(name, ics string) error  { f.put[name] = ics; return nil }
func (f *fakeDAV) Delete(name string) error    { f.deleted = append(f.deleted, name); return nil }

type stubSource struct{}

func (stubSource) ListMealPlans() ([]string, error)             { return nil, nil }
func (stubSource) ReadMealPlan(string) (*model.MealPlan, error) { return nil, nil }
func (stubSource) ReadRecipe(string) (*model.Recipe, error)     { return nil, nil }

func newPusher(t *testing.T, dav CalDAV) (*Pusher, *time.Location) {
	t.Helper()
	p, err := planner.NewPlanner(stubSource{}, "18:30", "UTC")
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	gen, err := ical.NewGenerator(p, "12:00", "Saturday", "us", nil)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}
	return New(dav, gen, p.Location(), "Meal Plan"), p.Location()
}

// weekAt builds a 7-day plan starting at weekStart with placeholder recipes.
func weekAt(weekStart time.Time) *model.WeekPlan {
	w := &model.WeekPlan{WeekStart: weekStart, MealPlanName: "Test"}
	for i := 0; i < 7; i++ {
		date := weekStart.AddDate(0, 0, i)
		w.Days = append(w.Days, model.DayMeal{
			Date:       date,
			RecipeName: "Dish",
			Recipe:     model.Recipe{Name: "Dish", RecipeIngredient: []string{"stuff"}},
		})
	}
	return w
}

func TestReconcile_PublishesWholeCurrentWeek(t *testing.T) {
	dav := newFakeDAV()
	p, loc := newPusher(t, dav)

	now := time.Date(2026, 4, 15, 9, 0, 0, 0, loc)       // mid-week (Wednesday)
	weekStart := time.Date(2026, 4, 11, 0, 0, 0, 0, loc) // Saturday: Sat-Fri spans "now"

	res, err := p.Reconcile([]*model.WeekPlan{weekAt(weekStart)}, now)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if !dav.ensured {
		t.Error("expected EnsureCalendar to be called")
	}

	// The whole week is published: 7 dinners + 1 shopping event, including the
	// days (and the shopping event) that fall before `now`.
	if res.Pushed != 8 {
		t.Errorf("Pushed = %d, want 8 (whole week incl. past days)", res.Pushed)
	}
	for _, name := range []string{
		"chefcal-dinner-2026-04-11.ics",   // Saturday, before now
		"chefcal-dinner-2026-04-14.ics",   // Tuesday, before now
		"chefcal-dinner-2026-04-15.ics",   // now
		"chefcal-dinner-2026-04-17.ics",   // Friday
		"chefcal-shopping-2026-04-11.ics", // shopping on the (past) Saturday
	} {
		if _, ok := dav.put[name]; !ok {
			t.Errorf("expected %s to be published", name)
		}
	}
}

func TestReconcile_DeletesStaleFutureOnly(t *testing.T) {
	// A stale future entry (from an old plan) and a stale past entry both exist
	// on the server. Only the future one should be deleted.
	dav := newFakeDAV(
		"chefcal-dinner-2026-05-01.ics", // future, not in plan -> delete
		"chefcal-dinner-2026-01-01.ics", // past -> keep (history)
		"personal-note-2026-05-01.ics",  // not ours -> ignore
	)
	p, loc := newPusher(t, dav)

	now := time.Date(2026, 4, 15, 9, 0, 0, 0, loc)
	weekStart := time.Date(2026, 4, 18, 0, 0, 0, 0, loc) // wholly in the future

	if _, err := p.Reconcile([]*model.WeekPlan{weekAt(weekStart)}, now); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	sort.Strings(dav.deleted)
	if len(dav.deleted) != 1 || dav.deleted[0] != "chefcal-dinner-2026-05-01.ics" {
		t.Errorf("expected only the stale future entry deleted, got %v", dav.deleted)
	}
}

func TestReconcile_UpdatesInPlaceNoDelete(t *testing.T) {
	// The exact entries we're about to push already exist -> updated in place,
	// nothing deleted.
	weekStart := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	names := []string{
		"chefcal-dinner-2026-04-18.ics",
		"chefcal-shopping-2026-04-18.ics",
	}
	dav := newFakeDAV(names...)
	p, loc := newPusher(t, dav)

	now := time.Date(2026, 4, 15, 9, 0, 0, 0, loc)
	if _, err := p.Reconcile([]*model.WeekPlan{weekAt(weekStart)}, now); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(dav.deleted) != 0 {
		t.Errorf("expected no deletions, got %v", dav.deleted)
	}
	if _, ok := dav.put["chefcal-dinner-2026-04-18.ics"]; !ok {
		t.Error("expected the existing entry to be re-PUT (updated in place)")
	}
}
