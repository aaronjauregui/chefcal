package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aaronjauregui/chefcal/internal/model"
)

type Store struct {
	path     string
	location *time.Location
	mu       sync.RWMutex
	weeks    map[string]*model.WeekPlan // key: week start date "2006-01-02"
}

func New(path string, location *time.Location) (*Store, error) {
	s := &Store{
		path:     path,
		location: location,
		weeks:    make(map[string]*model.WeekPlan),
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating store directory: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("reading store: %w", err)
	}

	if err := json.Unmarshal(data, &s.weeks); err != nil {
		return nil, fmt.Errorf("parsing store: %w", err)
	}

	return s, nil
}

func (s *Store) Save(week *model.WeekPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := week.WeekStart.Format("2006-01-02")
	s.weeks[key] = week
	s.cleanup()
	return s.persist()
}

func (s *Store) GetCurrentWeeks() []*model.WeekPlan {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	var result []*model.WeekPlan
	for _, w := range s.weeks {
		weekEnd := w.WeekStart.AddDate(0, 0, 6)
		if !weekEnd.Before(today) {
			result = append(result, w)
		}
	}
	return result
}

func (s *Store) HasWeek(weekStart time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := weekStart.Format("2006-01-02")
	_, ok := s.weeks[key]
	return ok
}

// cleanup removes weeks that have fully passed.
func (s *Store) cleanup() {
	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	for key, w := range s.weeks {
		weekEnd := w.WeekStart.AddDate(0, 0, 6)
		if weekEnd.Before(today) {
			delete(s.weeks, key)
		}
	}
}

// persist writes the store to disk atomically via temp file + rename.
func (s *Store) persist() error {
	data, err := json.MarshalIndent(s.weeks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling store: %w", err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "weeks-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
