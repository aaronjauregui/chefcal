package planner

import (
	"fmt"
	"log"
	"math/rand/v2"
	"regexp"
	"strconv"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/nextcloud"
)

type Planner struct {
	nc           *nextcloud.Client
	dinnerDoneBy TimeOfDay
	location     *time.Location
}

type TimeOfDay struct {
	Hour   int
	Minute int
}

func ParseTimeOfDay(s string) (TimeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return TimeOfDay{}, fmt.Errorf("parsing time of day %q: %w", s, err)
	}
	return TimeOfDay{Hour: t.Hour(), Minute: t.Minute()}, nil
}

func NewPlanner(nc *nextcloud.Client, dinnerDoneBy string, timezone string) (*Planner, error) {
	tod, err := ParseTimeOfDay(dinnerDoneBy)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("loading timezone %q: %w", timezone, err)
	}
	return &Planner{nc: nc, dinnerDoneBy: tod, location: loc}, nil
}

func (p *Planner) Location() *time.Location {
	return p.location
}

func (p *Planner) GenerateWeek(weekStart time.Time, planName string) (*model.WeekPlan, error) {
	plan, err := p.nc.ReadMealPlan(planName)
	if err != nil {
		return nil, err
	}

	week := &model.WeekPlan{
		WeekStart:    weekStart,
		MealPlanName: planName,
		GeneratedAt:  time.Now(),
	}

	// Filter to only recipes that exist in Nextcloud
	var available []string
	for _, name := range plan.Recipes {
		if _, err := p.nc.ReadRecipe(name); err != nil {
			log.Printf("Skipping recipe %q: %v", name, err)
			continue
		}
		available = append(available, name)
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no valid recipes found in meal plan %q", planName)
	}
	if len(available) < 7 {
		return nil, fmt.Errorf("meal plan %q has only %d valid recipes, need at least 7", planName, len(available))
	}

	// Shuffle and pick 7 unique recipes
	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	picked := available[:7]

	for i := 0; i < 7; i++ {
		date := weekStart.AddDate(0, 0, i)
		recipeName := picked[i]

		recipe, _ := p.nc.ReadRecipe(recipeName)

		week.Days = append(week.Days, model.DayMeal{
			Date:       date,
			RecipeName: recipeName,
			Recipe:     *recipe,
		})
	}

	return week, nil
}

func (p *Planner) PickRandomPlan() (string, error) {
	plans, err := p.nc.ListMealPlans()
	if err != nil {
		return "", err
	}
	if len(plans) == 0 {
		return "", fmt.Errorf("no meal plans found")
	}
	return plans[rand.IntN(len(plans))], nil
}

func (p *Planner) DinnerEndTime(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(),
		p.dinnerDoneBy.Hour, p.dinnerDoneBy.Minute, 0, 0, p.location)
}

func (p *Planner) DinnerStartTime(date time.Time, recipe *model.Recipe) time.Time {
	end := p.DinnerEndTime(date)
	dur := ParseISO8601Duration(recipe.TotalTime)
	if dur == 0 {
		dur = 30 * time.Minute // default if unparseable
	}
	return end.Add(-dur)
}

// NextWeekStart returns the Saturday of the next week relative to the given time.
func NextWeekStart(from time.Time, loc *time.Location) time.Time {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	daysUntilSaturday := (int(time.Saturday) - int(from.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	return from.AddDate(0, 0, daysUntilSaturday)
}

var iso8601Re = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

func ParseISO8601Duration(s string) time.Duration {
	matches := iso8601Re.FindStringSubmatch(s)
	if matches == nil {
		return 0
	}

	var d time.Duration
	if matches[1] != "" {
		h, _ := strconv.Atoi(matches[1])
		d += time.Duration(h) * time.Hour
	}
	if matches[2] != "" {
		m, _ := strconv.Atoi(matches[2])
		d += time.Duration(m) * time.Minute
	}
	if matches[3] != "" {
		sec, _ := strconv.Atoi(matches[3])
		d += time.Duration(sec) * time.Second
	}
	return d
}
