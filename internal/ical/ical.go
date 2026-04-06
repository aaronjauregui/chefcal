package ical

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/aaronjauregui/chefcal/internal/planner"
)

type Generator struct {
	planner             *planner.Planner
	shoppingEventTime   planner.TimeOfDay
	shoppingEventDay    time.Weekday
}

func NewGenerator(p *planner.Planner, shoppingTime string, shoppingDay string) (*Generator, error) {
	tod, err := planner.ParseTimeOfDay(shoppingTime)
	if err != nil {
		return nil, err
	}

	day, err := parseWeekday(shoppingDay)
	if err != nil {
		return nil, err
	}

	return &Generator{
		planner:           p,
		shoppingEventTime: tod,
		shoppingEventDay:  day,
	}, nil
}

func (g *Generator) Generate(weeks []*model.WeekPlan) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//ChefCal//Meal Planner//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("X-WR-CALNAME:Meal Plan\r\n")
	writeField(&b, "X-WR-TIMEZONE", g.planner.Location().String())

	for _, week := range weeks {
		for _, day := range week.Days {
			g.writeDinnerEvent(&b, &day)
		}
		g.writeShoppingEvent(&b, week)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func (g *Generator) writeDinnerEvent(b *strings.Builder, day *model.DayMeal) {
	end := g.planner.DinnerEndTime(day.Date)
	start := g.planner.DinnerStartTime(day.Date, &day.Recipe)
	uid := eventUID("dinner", day.Date)

	b.WriteString("BEGIN:VEVENT\r\n")
	writeField(b, "UID", uid)
	writeField(b, "DTSTAMP", formatDateTime(time.Now()))
	writeField(b, "DTSTART", formatDateTime(start))
	writeField(b, "DTEND", formatDateTime(end))
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
	satDate := g.findDay(week.WeekStart, g.shoppingEventDay)
	start := time.Date(satDate.Year(), satDate.Month(), satDate.Day(),
		g.shoppingEventTime.Hour, g.shoppingEventTime.Minute, 0, 0, g.planner.Location())
	end := start.Add(1 * time.Hour)
	uid := eventUID("shopping", week.WeekStart)

	var desc strings.Builder
	desc.WriteString(fmt.Sprintf("Shopping list for week of %s\\n", week.WeekStart.Format("Jan 2")))
	desc.WriteString(fmt.Sprintf("Meal plan: %s\\n\\n", week.MealPlanName))
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
	writeField(b, "DTSTAMP", formatDateTime(time.Now()))
	writeField(b, "DTSTART", formatDateTime(start))
	writeField(b, "DTEND", formatDateTime(end))
	writeField(b, "SUMMARY", fmt.Sprintf("Shopping List - %s", week.MealPlanName))
	writeField(b, "DESCRIPTION", desc.String())
	b.WriteString("END:VEVENT\r\n")
}

func (g *Generator) findDay(weekStart time.Time, day time.Weekday) time.Time {
	// weekStart is Monday (Weekday == 1)
	offset := int(day) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return weekStart.AddDate(0, 0, offset)
}

func formatDateTime(t time.Time) string {
	return t.Format("20060102T150405")
}

func eventUID(prefix string, date time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s", prefix, date.Format("2006-01-02"))))
	return fmt.Sprintf("%x@chefcal", h[:8])
}

func writeField(b *strings.Builder, key, value string) {
	line := fmt.Sprintf("%s:%s", key, value)
	// iCal line folding: max 75 octets per line
	for len(line) > 75 {
		b.WriteString(line[:75])
		b.WriteString("\r\n ")
		line = line[75:]
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
