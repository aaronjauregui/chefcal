// Package push publishes ChefCal meal plans to a Nextcloud calendar over
// CalDAV. It reconciles the calendar against the current plans: entries dated
// today or later are created/updated in place, and any ChefCal-owned future
// entry no longer in the plan is removed. Past entries are never touched, so
// history is preserved.
package push

import (
	"fmt"
	"log"
	"time"

	"github.com/aaronjauregui/chefcal/internal/ical"
	"github.com/aaronjauregui/chefcal/internal/model"
)

// CalDAV is the subset of a CalDAV client the pusher needs.
type CalDAV interface {
	EnsureCalendar(displayName string) error
	List() ([]string, error)
	Put(name, ics string) error
	Delete(name string) error
}

type Pusher struct {
	dav         CalDAV
	gen         *ical.Generator
	loc         *time.Location
	displayName string
}

// Result summarises what a reconcile changed.
type Result struct {
	Pushed  int
	Deleted int
}

func New(dav CalDAV, gen *ical.Generator, loc *time.Location, displayName string) *Pusher {
	return &Pusher{dav: dav, gen: gen, loc: loc, displayName: displayName}
}

// Reconcile makes the calendar match weeks for all entries dated today or
// later. now is used to derive "today" in the pusher's location.
func (p *Pusher) Reconcile(weeks []*model.WeekPlan, now time.Time) (Result, error) {
	var res Result

	if err := p.dav.EnsureCalendar(p.displayName); err != nil {
		return res, err
	}

	today := startOfDay(now, p.loc)

	// Desired: every entry dated today or later, keyed by resource name.
	desired := make(map[string]ical.Event)
	for _, e := range p.gen.Events(weeks) {
		if startOfDay(e.Date, p.loc).Before(today) {
			continue // past entry; leave history untouched
		}
		desired[e.ResourceName] = e
	}

	// Create/update desired entries.
	for name, e := range desired {
		if err := p.dav.Put(name, e.ICS); err != nil {
			return res, err
		}
		res.Pushed++
	}

	// Delete stale future entries we own.
	existing, err := p.dav.List()
	if err != nil {
		return res, fmt.Errorf("listing calendar: %w", err)
	}
	for _, name := range existing {
		date, ok := ical.ParseResourceDate(name, p.loc)
		if !ok {
			continue // not ours
		}
		if date.Before(today) {
			continue // past entry; history
		}
		if _, want := desired[name]; want {
			continue
		}
		if err := p.dav.Delete(name); err != nil {
			return res, err
		}
		log.Printf("Deleted stale calendar entry %s", name)
		res.Deleted++
	}

	return res, nil
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
