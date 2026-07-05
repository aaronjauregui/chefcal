package nextcloud

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/aaronjauregui/chefcal/internal/model"
	"github.com/studio-b12/gowebdav"
)

// davFS is the subset of the WebDAV client the nextcloud Client uses.
type davFS interface {
	Read(path string) ([]byte, error)
	ReadDir(path string) ([]os.FileInfo, error)
}

type Client struct {
	dav           davFS
	mealPlansPath string
	recipesPath   string

	mu         sync.Mutex
	recipeDirs map[string]string // lower(dir name) -> actual dir name; lazily loaded
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
	data, err := c.dav.Read(path.Join(c.recipesPath, name, "recipe.json"))
	if err != nil {
		// WebDAV paths are case-sensitive, but recipe names in meal plans
		// often differ in case from the directory. Fall back to a
		// case-insensitive directory match before giving up.
		if actual, ok := c.resolveRecipeDir(name); ok {
			data, err = c.dav.Read(path.Join(c.recipesPath, actual, "recipe.json"))
		}
		if err != nil {
			return nil, fmt.Errorf("reading recipe %q: %w", name, err)
		}
	}

	var recipe model.Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("parsing recipe %q: %w", name, err)
	}

	return &recipe, nil
}

// resolveRecipeDir returns the actual recipe directory name that matches name
// case-insensitively, reporting false if there is no match or only the
// exact-case one (which the caller already tried). The directory listing is
// loaded once and memoised.
func (c *Client) resolveRecipeDir(name string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.recipeDirs == nil {
		files, err := c.dav.ReadDir(c.recipesPath)
		if err != nil {
			return "", false
		}
		c.recipeDirs = make(map[string]string, len(files))
		for _, f := range files {
			if f.IsDir() {
				c.recipeDirs[strings.ToLower(f.Name())] = f.Name()
			}
		}
	}

	actual, ok := c.recipeDirs[strings.ToLower(name)]
	if !ok || actual == name {
		return "", false
	}
	return actual, true
}
