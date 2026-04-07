package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeConfig(t, `
server:
  address: ":9090"
nextcloud:
  url: "https://example.com/dav"
  username: "user"
  password: "pass"
  meal_plans_path: "/Plans"
  recipes_path: "/Recipes"
planner:
  dinner_done_by: "19:00"
  timezone: "America/New_York"
store:
  path: "mydata/weeks.json"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Address != ":9090" {
		t.Errorf("address = %q, want :9090", cfg.Server.Address)
	}
	if cfg.Nextcloud.URL != "https://example.com/dav" {
		t.Errorf("url = %q", cfg.Nextcloud.URL)
	}
	if cfg.Planner.DinnerDoneBy != "19:00" {
		t.Errorf("dinner_done_by = %q, want 19:00", cfg.Planner.DinnerDoneBy)
	}
	if cfg.Planner.Timezone != "America/New_York" {
		t.Errorf("timezone = %q, want America/New_York", cfg.Planner.Timezone)
	}
	if cfg.Store.Path != "mydata/weeks.json" {
		t.Errorf("store path = %q", cfg.Store.Path)
	}
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfig(t, `
nextcloud:
  url: "https://example.com/dav"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Address != ":8080" {
		t.Errorf("default address = %q, want :8080", cfg.Server.Address)
	}
	if cfg.Planner.DinnerDoneBy != "18:30" {
		t.Errorf("default dinner_done_by = %q, want 18:30", cfg.Planner.DinnerDoneBy)
	}
	if cfg.Planner.ShoppingEventTime != "12:00" {
		t.Errorf("default shopping time = %q", cfg.Planner.ShoppingEventTime)
	}
	if cfg.Planner.ShoppingEventDay != "Saturday" {
		t.Errorf("default shopping day = %q", cfg.Planner.ShoppingEventDay)
	}
	if cfg.Planner.Timezone != "Australia/Sydney" {
		t.Errorf("default timezone = %q, want Australia/Sydney", cfg.Planner.Timezone)
	}
	if cfg.Store.Path != "data/weeks.json" {
		t.Errorf("default store path = %q", cfg.Store.Path)
	}
}

func TestLoad_MissingURL(t *testing.T) {
	path := writeConfig(t, `
nextcloud:
  username: "user"
`)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing nextcloud.url")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfig(t, `{{{invalid yaml`)
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
