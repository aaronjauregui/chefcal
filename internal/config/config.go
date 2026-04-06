package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Nextcloud NextcloudConfig `yaml:"nextcloud"`
	Planner   PlannerConfig   `yaml:"planner"`
	Store     StoreConfig     `yaml:"store"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
}

type NextcloudConfig struct {
	URL                string `yaml:"url"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	MealPlansPath      string `yaml:"meal_plans_path"`
	RecipesPath        string `yaml:"recipes_path"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type PlannerConfig struct {
	DinnerDoneBy      string `yaml:"dinner_done_by"`
	ShoppingEventTime string `yaml:"shopping_event_time"`
	ShoppingEventDay  string `yaml:"shopping_event_day"`
	Timezone          string `yaml:"timezone"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{Address: ":8080"},
		Planner: PlannerConfig{
			DinnerDoneBy:      "18:30",
			ShoppingEventTime: "12:00",
			ShoppingEventDay:  "Saturday",
			Timezone:          "Australia/Sydney",
		},
		Store: StoreConfig{Path: "data/weeks.json"},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Nextcloud.URL == "" {
		return nil, fmt.Errorf("nextcloud.url is required")
	}

	return cfg, nil
}
