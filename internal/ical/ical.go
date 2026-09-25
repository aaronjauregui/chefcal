package ical

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
	"github.com/aaronjauregui/chefcal/internal/shopping"
)

type Generator struct {
	planner           *planner.Planner
	shoppingEventTime planner.TimeOfDay
	shoppingEventDay  time.Weekday
	measurementSystem shopping.System
	pantryStaples     []string
}

func NewGenerator(p *planner.Planner, shoppingTime string, shoppingDay string, measurementSystem string, pantryStaples []string) (*Generator, error) {
	tod, err := planner.ParseTimeOfDay(shoppingTime)
	if err != nil {
		return nil, err
	}

	day, err := parseWeekday(shoppingDay)
	if err != nil {
		return nil, err
	}

	// A nil list means "use the built-in default"; an explicit empty list
	// disables exclusion entirely.
	staples := pantryStaples
	if staples == nil {
		staples = shopping.DefaultStaples()
	}

	return &Generator{
		planner:           p,
		shoppingEventTime: tod,
		shoppingEventDay:  day,
		measurementSystem: shopping.ParseSystem(measurementSystem),
		pantryStaples:     staples,
	}, nil
}

func (g *Generator) Generate(weeks []*model.WeekPlan) string {
	var b strings.Builder
	g.writeHeader(&b, true)

	for _, week := range weeks {
		for i := range week.Days {
			g.writeDinnerEvent(&b, &week.Days[i])
		}
		g.writeShoppingEvent(&b, week)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// Event is a single calendar entry rendered as a standalone VCALENDAR
// document, ready to be PUT to a CalDAV collection as its own resource.
type Event struct {
	UID          string
	ResourceName string // e.g. "chefcal-dinner-2026-07-11.ics"
	Date         time.Time
	ICS          string
}

// Events renders each dinner and shopping entry as its own standalone
// VCALENDAR document for CalDAV publishing. Resource names and UIDs are
// deterministic functions of (kind, date), so re-pushing updates in place.
func (g *Generator) Events(weeks []*model.WeekPlan) []Event {
	var events []Event
	for _, week := range weeks {
		for i := range week.Days {
			day := &week.Days[i]
			events = append(events, g.buildEvent("dinner", day.Date, func(b *strings.Builder) {
				g.writeDinnerEvent(b, day)
			}))
		}
		shoppingDate := g.findDay(week.WeekStart, g.shoppingEventDay)
		events = append(events, g.buildEvent("shopping", shoppingDate, func(b *strings.Builder) {
			g.writeShoppingEvent(b, week)
		}))
	}
	return events
}

func (g *Generator) buildEvent(kind string, date time.Time, writeVEvent func(*strings.Builder)) Event {
	var b strings.Builder
	g.writeHeader(&b, false)
	writeVEvent(&b)
	b.WriteString("END:VCALENDAR\r\n")
	return Event{
		UID:          eventUID(kind, date),
		ResourceName: resourceName(kind, date),
		Date:         date,
		ICS:          b.String(),
	}
}

// writeHeader writes the VCALENDAR preamble. feed controls whether the
// publish-only metadata (METHOD, X-WR-*) is included: it is required for the
// subscribable .ics feed but forbidden on a CalDAV calendar object, which a
// server such as SabreDAV rejects if METHOD is present.
func (g *Generator) writeHeader(b *strings.Builder, feed bool) {
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//ChefCal//Meal Planner//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	if feed {
		b.WriteString("METHOD:PUBLISH\r\n")
		b.WriteString("X-WR-CALNAME:Meal Plan\r\n")
		writeField(b, "X-WR-TIMEZONE", g.planner.Location().String())
	}
}

func (g *Generator) writeDinnerEvent(b *strings.Builder, day *model.DayMeal) {
	end := g.planner.DinnerEndTime(day.Date)
	start := g.planner.DinnerStartTime(day.Date, &day.Recipe)
	uid := eventUID("dinner", day.Date)
	tzName := g.planner.Location().String()

	b.WriteString("BEGIN:VEVENT\r\n")
	writeField(b, "UID", uid)
	writeField(b, "DTSTAMP", formatDateTimeUTC(time.Now()))
	writeField(b, fmt.Sprintf("DTSTART;TZID=%s", tzName), formatDateTimeLocal(start))
	writeField(b, fmt.Sprintf("DTEND;TZID=%s", tzName), formatDateTimeLocal(end))
	writeField(b, "SUMMARY", fmt.Sprintf("Dinner: %s", day.RecipeName))

	var desc strings.Builder
	if day.Recipe.Description != "" {
		desc.WriteString(day.Recipe.Description)
		desc.WriteString("\\n\\n")
	}
	desc.WriteString("Ingredients:\\n")
	for _, ing := range day.Recipe.RecipeIngredient {
		desc.WriteString("- ")
		desc.WriteString(ing)
		desc.WriteString("\\n")
	}
	if day.Recipe.URL != "" {
		desc.WriteString("\\n")
		desc.WriteString(day.Recipe.URL)
	}
	writeField(b, "DESCRIPTION", desc.String())
	if day.Recipe.URL != "" {
		writeField(b, "URL", day.Recipe.URL)
	}
	b.WriteString("END:VEVENT\r\n")
}

func (g *Generator) writeShoppingEvent(b *strings.Builder, week *model.WeekPlan) {
	shoppingDate := g.findDay(week.WeekStart, g.shoppingEventDay)
	start := time.Date(shoppingDate.Year(), shoppingDate.Month(), shoppingDate.Day(),
		g.shoppingEventTime.Hour, g.shoppingEventTime.Minute, 0, 0, g.planner.Location())
	end := start.Add(1 * time.Hour)
	uid := eventUID("shopping", shoppingDate)
	tzName := g.planner.Location().String()

	var desc strings.Builder
	desc.WriteString(fmt.Sprintf("Shopping list for week of %s\\n", week.WeekStart.Format("Jan 2")))
	desc.WriteString(fmt.Sprintf("Meal plan: %s\\n\\n", week.MealPlanName))

	desc.WriteString("SHOPPING LIST\\n")
	opts := shopping.Options{System: g.measurementSystem, Exclude: g.pantryStaples}
	for _, item := range shopping.Aggregate(week.Days, opts) {
		desc.WriteString("- ")
		desc.WriteString(item.Line())
		desc.WriteString("\\n")
	}
	desc.WriteString("\\n")

	desc.WriteString("By recipe:\\n")
	for _, day := range week.Days {
		desc.WriteString(fmt.Sprintf("== %s (%s) ==\\n", day.RecipeName, day.Date.Format("Monday")))
		for _, ing := range day.Recipe.RecipeIngredient {
			desc.WriteString("- ")
			desc.WriteString(ing)
			desc.WriteString("\\n")
		}
		desc.WriteString("\\n")
	}

	b.WriteString("BEGIN:VEVENT\r\n")
	writeField(b, "UID", uid)
	writeField(b, "DTSTAMP", formatDateTimeUTC(time.Now()))
	writeField(b, fmt.Sprintf("DTSTART;TZID=%s", tzName), formatDateTimeLocal(start))
	writeField(b, fmt.Sprintf("DTEND;TZID=%s", tzName), formatDateTimeLocal(end))
	writeField(b, "SUMMARY", fmt.Sprintf("Shopping List - %s", week.MealPlanName))
	writeField(b, "DESCRIPTION", desc.String())
	b.WriteString("END:VEVENT\r\n")
}

// findDay returns the date for the given weekday within the 7-day span
// starting at weekStart.
func (g *Generator) findDay(weekStart time.Time, day time.Weekday) time.Time {
	offset := (int(day) - int(weekStart.Weekday()) + 7) % 7
	return weekStart.AddDate(0, 0, offset)
}

func formatDateTimeUTC(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func formatDateTimeLocal(t time.Time) string {
	return t.Format("20060102T150405")
}

// eventUID is a deterministic function of (kind, date) so that re-pushing
// the same entry updates the existing calendar event in place rather than
// creating a duplicate.
func eventUID(kind string, date time.Time) string {
	return fmt.Sprintf("%s-%s@chefcal", kind, date.Format("2006-01-02"))
}

// resourceName is the CalDAV resource basename for an entry. The date is
// embedded so reconciliation can tell an entry's date from its href alone,
// without fetching the body.
func resourceName(kind string, date time.Time) string {
	return fmt.Sprintf("chefcal-%s-%s.ics", kind, date.Format("2006-01-02"))
}

// ParseResourceDate extracts the entry date from a ChefCal resource name
// (as produced by resourceName), interpreted at midnight in loc. It reports
// false for names ChefCal did not create.
func ParseResourceDate(name string, loc *time.Location) (time.Time, bool) {
	if !strings.HasPrefix(name, "chefcal-") || !strings.HasSuffix(name, ".ics") {
		return time.Time{}, false
	}
	base := strings.TrimSuffix(name, ".ics")
	parts := strings.Split(base, "-")
	if len(parts) < 4 {
		return time.Time{}, false
	}
	dateStr := strings.Join(parts[len(parts)-3:], "-")
	t, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// writeField writes an iCal property with RFC 5545 line folding.
// Folds at 75 octets, respecting UTF-8 character boundaries.
func writeField(b *strings.Builder, key, value string) {
	line := fmt.Sprintf("%s:%s", key, value)
	for len(line) > 75 {
		cut := 75
		// Walk back to a valid UTF-8 boundary
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if cut == 0 {
			cut = 75 // shouldn't happen with valid UTF-8, but avoid infinite loop
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(s) {
	case "sunday":
		return time.Sunday, nil
	case "monday":
		return time.Monday, nil
	case "tuesday":
		return time.Tuesday, nil
	case "wednesday":
		return time.Wednesday, nil
	case "thursday":
		return time.Thursday, nil
	case "friday":
		return time.Friday, nil
	case "saturday":
		return time.Saturday, nil
	default:
		return 0, fmt.Errorf("unknown weekday: %q", s)
	}
}
