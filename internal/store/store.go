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
	path  string
	mu    sync.RWMutex
	weeks map[string]*model.WeekPlan // key: week start date "2006-01-02"
}

func New(path string) (*Store, error) {
	s := &Store{
		path:  path,
		weeks: make(map[string]*model.WeekPlan),
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

	today := time.Now().Truncate(24 * time.Hour)
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
	today := time.Now().Truncate(24 * time.Hour)
	for key, w := range s.weeks {
		weekEnd := w.WeekStart.AddDate(0, 0, 6)
		if weekEnd.Before(today) {
			delete(s.weeks, key)
		}
	}
}

func (s *Store) persist() error {
	data, err := json.MarshalIndent(s.weeks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling store: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}
