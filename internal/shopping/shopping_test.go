package shopping

import (
	"testing"

	"github.com/aaronjauregui/chefcal/internal/model"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in       string
		wantOK   bool
		wantName string
		wantDim  Dimension
		wantBase float64 // base unit: g for mass, ml for volume, count otherwise
	}{
		{"700g stewing beef", true, "stewing beef", DimMass, 700},
		{"200g of chicken", true, "chicken", DimMass, 200},
		{"1kg potatoes", true, "potatoes", DimMass, 1000},
		{"1 lb ground beef", true, "ground beef", DimMass, 453.592},
		{"2 cups flour", true, "flour", DimVolume, 473.176},
		{"1 tablespoon oil", true, "oil", DimVolume, 14.7868},
		{"1/2 cup sugar", true, "sugar", DimVolume, 118.294},
		{"1 1/2 cups milk", true, "milk", DimVolume, 354.882},
		{"½ cup rice", true, "rice", DimVolume, 118.294},
		{"2 cloves garlic, minced", true, "cloves garlic", DimCount, 2},
		{"2 eggs", true, "eggs", DimCount, 2},
		{"2-3 large potatoes", true, "large potatoes", DimCount, 3}, // range -> upper bound
		{"1 (400g) can chopped tomatoes", true, "can chopped tomatoes", DimCount, 1},
		{"Salt to taste", false, "", DimNone, 0},
		{"Olive oil", false, "", DimNone, 0},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			p, ok := parse(tt.in, SystemUS)
			if ok != tt.wantOK {
				t.Fatalf("parse(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if p.name != tt.wantName {
				t.Errorf("name = %q, want %q", p.name, tt.wantName)
			}
			if p.dim != tt.wantDim {
				t.Errorf("dim = %v, want %v", p.dim, tt.wantDim)
			}
			if !approx(p.base, tt.wantBase) {
				t.Errorf("base = %v, want %v", p.base, tt.wantBase)
			}
		})
	}
}

func TestMatchKey(t *testing.T) {
	tests := []struct{ a, b string }{
		{"onion", "onions"},
		{"large onion", "onion"},
		{"tomatoes", "tomato"},
		{"berries", "berry"},
		{"Large Onion", "onion"}, // case-insensitive + size-word strip
	}
	for _, tt := range tests {
		if matchKey(tt.a) != matchKey(tt.b) {
			t.Errorf("matchKey(%q)=%q != matchKey(%q)=%q", tt.a, matchKey(tt.a), tt.b, matchKey(tt.b))
		}
	}

	// These must NOT collapse together.
	distinct := []struct{ a, b string }{
		{"chicken breast", "chicken thigh"},
		{"chicken breast", "chicken stock"},
		{"ground beef", "beef"},
	}
	for _, tt := range distinct {
		if matchKey(tt.a) == matchKey(tt.b) {
			t.Errorf("matchKey merged distinct items: %q and %q", tt.a, tt.b)
		}
	}
}

// days builds a []DayMeal where each entry is one recipe with the given
// ingredient lines.
func days(recipes ...[]string) []model.DayMeal {
	var out []model.DayMeal
	for i, ings := range recipes {
		out = append(out, model.DayMeal{
			RecipeName: string(rune('A' + i)),
			Recipe:     model.Recipe{RecipeIngredient: ings},
		})
	}
	return out
}

// find returns the item with the given display name.
func find(items []Item, name string) (Item, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return Item{}, false
}

func TestAggregate_SumsSameUnit(t *testing.T) {
	items := Aggregate(days(
		[]string{"200g chicken"},
		[]string{"300g chicken"},
	), SystemUS)
	it, ok := find(items, "chicken")
	if !ok {
		t.Fatalf("chicken not found in %v", items)
	}
	if it.Line() != "500g chicken" {
		t.Errorf("Line = %q, want %q", it.Line(), "500g chicken")
	}
	if len(it.Sources) != 2 {
		t.Errorf("Sources = %v, want 2", it.Sources)
	}
}

func TestAggregate_ConvertsWithinDimension(t *testing.T) {
	// 1lb (453.592g) + 200g = 653.592g -> mixed units fall back to grams.
	items := Aggregate(days(
		[]string{"1 lb beef"},
		[]string{"200g beef"},
	), SystemUS)
	it, _ := find(items, "beef")
	if it.Line() != "654g beef" {
		t.Errorf("Line = %q, want %q", it.Line(), "654g beef")
	}
}

func TestAggregate_IncompatibleUnitsKeptSeparate(t *testing.T) {
	// cups (volume) and grams (mass) cannot combine.
	items := Aggregate(days(
		[]string{"1 cup rice"},
		[]string{"200g rice"},
	), SystemUS)
	it, _ := find(items, "rice")
	// mass bucket is formatted before volume bucket
	if it.Line() != "200g + 1 cup rice" {
		t.Errorf("Line = %q, want %q", it.Line(), "200g + 1 cup rice")
	}
}

func TestAggregate_CountMerges(t *testing.T) {
	items := Aggregate(days(
		[]string{"2 eggs"},
		[]string{"1 egg"},
	), SystemUS)
	it, ok := find(items, "eggs") // first-seen display form
	if !ok {
		t.Fatalf("eggs not found in %v", items)
	}
	if it.Line() != "3 eggs" {
		t.Errorf("Line = %q, want %q", it.Line(), "3 eggs")
	}
}

func TestAggregate_PassthroughNeverDropped(t *testing.T) {
	items := Aggregate(days(
		[]string{"Salt to taste", "200g chicken"},
		[]string{"Salt to taste"},
	), SystemUS)
	it, ok := find(items, "Salt to taste")
	if !ok {
		t.Fatalf("passthrough item lost: %v", items)
	}
	if it.Parsed {
		t.Error("passthrough item should have Parsed=false")
	}
	if it.Line() != "Salt to taste" {
		t.Errorf("Line = %q, want verbatim", it.Line())
	}
	// deduped to a single line despite appearing twice
	count := 0
	for _, x := range items {
		if x.Name == "Salt to taste" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("passthrough appeared %d times, want 1", count)
	}
}

func TestAggregate_DistinctNamesNotMerged(t *testing.T) {
	items := Aggregate(days(
		[]string{"200g chicken breast"},
		[]string{"200g chicken thigh"},
	), SystemUS)
	if _, ok := find(items, "chicken breast"); !ok {
		t.Error("chicken breast missing")
	}
	if _, ok := find(items, "chicken thigh"); !ok {
		t.Error("chicken thigh missing")
	}
}

func TestAggregate_CCSumsAsVolume(t *testing.T) {
	items := Aggregate(days(
		[]string{"200cc water"},
		[]string{"300cc water"},
	), SystemUS)
	it, ok := find(items, "water")
	if !ok {
		t.Fatalf("water not found in %v", items)
	}
	if it.Line() != "500cc water" {
		t.Errorf("Line = %q, want %q", it.Line(), "500cc water")
	}
}

func TestAggregate_JapaneseCupIs200ml(t *testing.T) {
	us, _ := find(Aggregate(days([]string{"2 cups rice"}), SystemUS), "rice")
	if us.Line() != "2 cups rice" {
		t.Errorf("US Line = %q, want %q", us.Line(), "2 cups rice")
	}

	// Japanese cup is 200ml, so 2 cups = 400ml. Mixing with a metric volume
	// forces the base-unit fallback and proves the factor: 400ml + 100ml.
	jp, _ := find(Aggregate(days(
		[]string{"2 cups rice"},
		[]string{"100ml rice"},
	), SystemJapanese), "rice")
	if jp.Line() != "500ml rice" {
		t.Errorf("JP Line = %q, want %q (2 JP cups=400ml + 100ml)", jp.Line(), "500ml rice")
	}
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.01
}
