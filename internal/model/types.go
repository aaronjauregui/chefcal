package model

import (
	"time"
)

// RecipeSource abstracts access to meal plans and recipes.
type RecipeSource interface {
	ListMealPlans() ([]string, error)
	ReadMealPlan(name string) (*MealPlan, error)
	ReadRecipe(name string) (*Recipe, error)
}

type Recipe struct {
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
	Description        string      `json:"description"`
	URL                string      `json:"url"`
	Image              string      `json:"image"`
	PrepTime           string      `json:"prepTime"`
	CookTime           string      `json:"cookTime"`
	TotalTime          string      `json:"totalTime"`
	RecipeCategory     string      `json:"recipeCategory"`
	Keywords           string      `json:"keywords"`
	RecipeYield        int         `json:"recipeYield"`
	RecipeIngredient   []string    `json:"recipeIngredient"`
	RecipeInstructions []string    `json:"recipeInstructions"`
}

type MealPlan struct {
	Name    string   // filename without .md
	Recipes []string // recipe names parsed from the markdown
}

type DayMeal struct {
	Date       time.Time `json:"date"`
	RecipeName string    `json:"recipe_name"`
	Recipe     Recipe    `json:"recipe"`
}

type WeekPlan struct {
	WeekStart    time.Time `json:"week_start"` // Saturday
	MealPlanName string    `json:"meal_plan_name"`
	Days         []DayMeal `json:"days"`       // 7 days, Sat-Fri
	GeneratedAt  time.Time `json:"generated_at"`
}
