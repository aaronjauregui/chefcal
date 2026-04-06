package nextcloud

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/studio-b12/gowebdav"
)

type Client struct {
	dav           *gowebdav.Client
	mealPlansPath string
	recipesPath   string
}

func NewClient(url, username, password, mealPlansPath, recipesPath string, insecureSkipVerify bool) *Client {
	dav := gowebdav.NewClient(url, username, password)
	if insecureSkipVerify {
		dav.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})
	}
	return &Client{
		dav:           dav,
		mealPlansPath: mealPlansPath,
		recipesPath:   recipesPath,
	}
}

func (c *Client) ListMealPlans() ([]string, error) {
	files, err := c.dav.ReadDir(c.mealPlansPath)
	if err != nil {
		return nil, fmt.Errorf("listing meal plans: %w", err)
	}

	var plans []string
	for _, f := range files {
		name := f.Name()
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(name), ".md") {
			plans = append(plans, strings.TrimSuffix(name, path.Ext(name)))
		}
	}
	return plans, nil
}

func (c *Client) ReadMealPlan(name string) (*model.MealPlan, error) {
	filePath := path.Join(c.mealPlansPath, name+".md")
	data, err := c.dav.Read(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading meal plan %q: %w", name, err)
	}

	var recipes []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		recipes = append(recipes, line)
	}

	if len(recipes) == 0 {
		return nil, fmt.Errorf("meal plan %q has no recipes", name)
	}

	return &model.MealPlan{
		Name:    name,
		Recipes: recipes,
	}, nil
}

func (c *Client) ReadRecipe(name string) (*model.Recipe, error) {
	filePath := path.Join(c.recipesPath, name, "recipe.json")
	data, err := c.dav.Read(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading recipe %q: %w", name, err)
	}

	var recipe model.Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("parsing recipe %q: %w", name, err)
	}

	return &recipe, nil
}
