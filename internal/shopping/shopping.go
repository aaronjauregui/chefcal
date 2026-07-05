// Package shopping consolidates recipe ingredients into a deduplicated
// shopping list. Ingredient strings are free-form natural language, so parsing
// is best-effort: quantities in compatible units are summed, and anything that
// cannot be parsed or combined is passed through verbatim. The list never drops
// an ingredient.
package shopping

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aaronjauregui/chefcal/internal/model"
)

// Dimension is the kind of measurement a quantity uses. Only quantities of the
// same dimension can be summed.
type Dimension int

const (
	DimNone Dimension = iota
	DimMass
	DimVolume
	DimCount
)

// System selects the measuring conventions for units whose size differs by
// locale. Metric mass/volume (g, ml, cc, ...) are identical across systems;
// only cup/tablespoon/teaspoon differ.
type System int

const (
	SystemUS System = iota
	SystemJapanese
)

// ParseSystem maps a config string to a System, defaulting to US.
func ParseSystem(s string) System {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "japanese", "japan", "jp":
		return SystemJapanese
	default:
		return SystemUS
	}
}

// jpFactors overrides the to-base (ml) factor for the locale-dependent volume
// units under the Japanese system: cup 200ml, tablespoon 15ml, teaspoon 5ml.
var jpFactors = map[string]float64{"cup": 200, "tbsp": 15, "tsp": 5}

// factor returns the to-base conversion factor for a unit under this system.
func (sys System) factor(info unitInfo) float64 {
	if sys == SystemJapanese {
		if f, ok := jpFactors[info.canonical]; ok {
			return f
		}
	}
	return info.factor
}

// Options configures how ingredients are aggregated.
type Options struct {
	System System
	// Exclude lists pantry-staple names to omit from the consolidated list,
	// matched against the normalised ingredient name. A single-word entry
	// matches only when it is the whole name (so "water" drops "water" but not
	// "coconut water"); a multi-word entry matches as a contiguous whole-word
	// phrase (so "soy sauce" also drops "light soy sauce").
	Exclude []string
}

// DefaultStaples is the built-in exclusion set used when none is configured:
// items that are essentially never bought per recipe.
func DefaultStaples() []string { return []string{"water", "ice"} }

type stapleMatcher [][]string

func compileStaples(names []string) stapleMatcher {
	var m stapleMatcher
	for _, n := range names {
		if w := strings.Fields(strings.ToLower(n)); len(w) > 0 {
			m = append(m, w)
		}
	}
	return m
}

func (m stapleMatcher) excludes(name string) bool {
	w := strings.Fields(strings.ToLower(name))
	for _, s := range m {
		if len(s) == 1 {
			if len(w) == 1 && w[0] == s[0] {
				return true
			}
		} else if containsSeq(w, s) {
			return true
		}
	}
	return false
}

func containsSeq(hay, needle []string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Item is one line of the consolidated shopping list.
type Item struct {
	Name    string   // display name, e.g. "chicken breast"
	Amount  string   // formatted quantity, e.g. "500g" or "200g + 1 cup"; empty for passthrough
	Parsed  bool     // false => Name is a verbatim passthrough of the original string
	Sources []string // dish names that contributed to this item
}

// Line renders the item as it should appear in the shopping list.
func (i Item) Line() string {
	if i.Amount == "" {
		return i.Name
	}
	return i.Amount + " " + i.Name
}

// Aggregate consolidates the ingredients of every recipe in days into a
// deduplicated, sorted shopping list. Parsed items come first (alphabetical),
// followed by verbatim passthrough items.
func Aggregate(days []model.DayMeal, opts Options) []Item {
	staples := compileStaples(opts.Exclude)

	builders := map[string]*builder{}
	var order []string

	passthrough := map[string]*Item{}
	var passOrder []string

	for _, d := range days {
		for _, raw := range d.Recipe.RecipeIngredient {
			p, ok := parse(raw, opts.System)
			if !ok {
				name := strings.TrimSpace(raw)
				key := strings.ToLower(name)
				if key == "" {
					continue
				}
				it := passthrough[key]
				if it == nil {
					it = &Item{Name: name}
					passthrough[key] = it
					passOrder = append(passOrder, key)
				}
				it.Sources = addSource(it.Sources, d.RecipeName)
				continue
			}

			key := matchKey(p.name)
			b := builders[key]
			if b == nil {
				b = &builder{name: p.name}
				builders[key] = b
				order = append(order, key)
			}
			b.add(p)
			b.sources = addSource(b.sources, d.RecipeName)
		}
	}

	var items []Item
	for _, k := range order {
		b := builders[k]
		if staples.excludes(b.name) {
			continue // pantry staple: omit from the consolidated list
		}
		items = append(items, b.finalize(opts.System))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	var pass []Item
	for _, k := range passOrder {
		it := passthrough[k]
		if staples.excludes(it.Name) {
			continue
		}
		pass = append(pass, *it)
	}
	sort.SliceStable(pass, func(i, j int) bool {
		return strings.ToLower(pass[i].Name) < strings.ToLower(pass[j].Name)
	})

	return append(items, pass...)
}

func addSource(sources []string, name string) []string {
	for _, s := range sources {
		if s == name {
			return sources
		}
	}
	return append(sources, name)
}

// ---- parsing ----

type parsed struct {
	name string    // cleaned display name
	dim  Dimension // Mass, Volume, or Count
	base float64   // value in base unit (g / ml) for mass/volume; the count otherwise
	unit string    // canonical original unit ("" for count)
}

type unitInfo struct {
	canonical string
	dim       Dimension
	factor    float64 // multiplier to the dimension's base unit (g or ml)
}

var units = map[string]unitInfo{
	"g": {"g", DimMass, 1}, "gram": {"g", DimMass, 1}, "grams": {"g", DimMass, 1},
	"kg": {"kg", DimMass, 1000}, "kilogram": {"kg", DimMass, 1000}, "kilograms": {"kg", DimMass, 1000},
	"oz": {"oz", DimMass, 28.3495}, "ounce": {"oz", DimMass, 28.3495}, "ounces": {"oz", DimMass, 28.3495},
	"lb": {"lb", DimMass, 453.592}, "lbs": {"lb", DimMass, 453.592}, "pound": {"lb", DimMass, 453.592}, "pounds": {"lb", DimMass, 453.592},

	"ml": {"ml", DimVolume, 1}, "milliliter": {"ml", DimVolume, 1}, "milliliters": {"ml", DimVolume, 1}, "millilitre": {"ml", DimVolume, 1}, "millilitres": {"ml", DimVolume, 1},
	"cc": {"cc", DimVolume, 1}, // cubic centimetre; common in Japanese recipes (= 1 ml)
	"l":  {"l", DimVolume, 1000}, "liter": {"l", DimVolume, 1000}, "liters": {"l", DimVolume, 1000}, "litre": {"l", DimVolume, 1000}, "litres": {"l", DimVolume, 1000},
	"tsp": {"tsp", DimVolume, 4.92892}, "teaspoon": {"tsp", DimVolume, 4.92892}, "teaspoons": {"tsp", DimVolume, 4.92892},
	"tbsp": {"tbsp", DimVolume, 14.7868}, "tbs": {"tbsp", DimVolume, 14.7868}, "tablespoon": {"tbsp", DimVolume, 14.7868}, "tablespoons": {"tbsp", DimVolume, 14.7868},
	"cup": {"cup", DimVolume, 236.588}, "cups": {"cup", DimVolume, 236.588},
	"floz": {"floz", DimVolume, 29.5735},
}

const fracClass = `½⅓⅔¼¾⅛⅜⅝⅞⅕⅖⅗⅘⅙⅚`

var fracRunes = map[rune]float64{
	'½': 0.5, '⅓': 1.0 / 3, '⅔': 2.0 / 3, '¼': 0.25, '¾': 0.75,
	'⅛': 0.125, '⅜': 0.375, '⅝': 0.625, '⅞': 0.875,
	'⅕': 0.2, '⅖': 0.4, '⅗': 0.6, '⅘': 0.8, '⅙': 1.0 / 6, '⅚': 5.0 / 6,
}

// a single quantity token: mixed number, fraction, decimal (optionally with a
// trailing unicode fraction), or a bare unicode fraction.
const qtyTok = `(?:\d+\s+\d+/\d+|\d+/\d+|\d+(?:\.\d+)?[` + fracClass + `]?|[` + fracClass + `])`

// leading quantity, with an optional range ("2-3", "2 to 3") whose upper bound
// is used.
var qtyRe = regexp.MustCompile(`^\s*(` + qtyTok + `)(?:(?:\s*[-–—]\s*|\s+to\s+)(` + qtyTok + `))?`)

var parenRe = regexp.MustCompile(`\([^)]*\)`)

func parse(raw string, sys System) (parsed, bool) {
	idx := qtyRe.FindStringSubmatchIndex(raw)
	if idx == nil {
		return parsed{}, false // no leading quantity: pass through verbatim
	}

	valStr := raw[idx[2]:idx[3]]
	if idx[4] != -1 { // range: use the upper bound
		valStr = raw[idx[4]:idx[5]]
	}
	val, ok := parseValue(valStr)
	if !ok {
		return parsed{}, false
	}

	rest := raw[idx[1]:]
	info, rem, hasUnit := parseUnit(rest)
	name := cleanName(rem)
	if name == "" {
		return parsed{}, false
	}

	if !hasUnit {
		return parsed{name: name, dim: DimCount, base: val}, true
	}
	return parsed{name: name, dim: info.dim, base: val * sys.factor(info), unit: info.canonical}, true
}

func parseValue(tok string) (float64, bool) {
	tok = strings.TrimSpace(tok)
	var total float64

	// trailing unicode fraction, possibly attached to a number ("1½")
	r := []rune(tok)
	if len(r) > 0 {
		if f, ok := fracRunes[r[len(r)-1]]; ok {
			total += f
			tok = strings.TrimSpace(string(r[:len(r)-1]))
			if tok == "" {
				return total, true
			}
		}
	}

	// mixed number "1 1/2"
	if fields := strings.Fields(tok); len(fields) == 2 {
		if a, err := strconv.ParseFloat(fields[0], 64); err == nil {
			if b, ok := parseFraction(fields[1]); ok {
				return total + a + b, true
			}
		}
	}
	if f, ok := parseFraction(tok); ok {
		return total + f, true
	}
	if v, err := strconv.ParseFloat(tok, 64); err == nil {
		return total + v, true
	}
	if total > 0 {
		return total, true
	}
	return 0, false
}

func parseFraction(s string) (float64, bool) {
	i := strings.IndexByte(s, '/')
	if i <= 0 {
		return 0, false
	}
	n, e1 := strconv.ParseFloat(s[:i], 64)
	d, e2 := strconv.ParseFloat(s[i+1:], 64)
	if e1 != nil || e2 != nil || d == 0 {
		return 0, false
	}
	return n / d, true
}

// parseUnit inspects the text after the quantity. If it starts with a known
// unit it returns that unit and the remaining text; otherwise it reports no
// unit (a count) and returns the text unchanged.
func parseUnit(rest string) (unitInfo, string, bool) {
	r := strings.TrimSpace(rest)
	if r == "" {
		return unitInfo{}, "", false
	}

	first, after := splitFirstWord(r)
	lf := strings.ToLower(strings.TrimRight(first, ".,"))

	if lf == "fl" { // "fl oz"
		second, after2 := splitFirstWord(after)
		ls := strings.ToLower(strings.TrimRight(second, ".,"))
		if ls == "oz" || ls == "ounce" || ls == "ounces" {
			return units["floz"], stripOf(after2), true
		}
	}
	if info, ok := units[lf]; ok {
		return info, stripOf(after), true
	}
	return unitInfo{}, r, false
}

func splitFirstWord(s string) (first, rest string) {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func stripOf(s string) string {
	if strings.HasPrefix(strings.ToLower(s), "of ") {
		return strings.TrimSpace(s[3:])
	}
	return s
}

func cleanName(s string) string {
	s = parenRe.ReplaceAllString(s, " ")
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i] // drop trailing prep ", minced"
	}
	return strings.Join(strings.Fields(s), " ")
}

// ---- name matching ----

var sizeWords = map[string]bool{
	"large": true, "small": true, "medium": true, "x-large": true, "extra-large": true,
}

// matchKey folds a name to a key used for merging. It strips cosmetic size
// words and naive plurals but deliberately keeps material descriptors (ground,
// fresh, dried, ...) so distinct products are not merged.
func matchKey(name string) string {
	words := strings.Fields(strings.ToLower(name))
	var kept []string
	for i, w := range words {
		if i < len(words)-1 && sizeWords[w] {
			continue
		}
		kept = append(kept, w)
	}
	if len(kept) > 0 {
		kept[len(kept)-1] = singular(kept[len(kept)-1])
	}
	return strings.Join(kept, " ")
}

func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 3:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "oes") && len(w) > 3:
		return w[:len(w)-2]
	case strings.HasSuffix(w, "ss"):
		return w
	case strings.HasSuffix(w, "s") && len(w) > 1:
		return w[:len(w)-1]
	}
	return w
}

// ---- aggregation ----

// dimAcc accumulates quantities of one dimension. It sums in the base unit but
// remembers the original unit so a consistent unit can be shown back to the
// user; mixed units fall back to the base unit.
type dimAcc struct {
	base  float64
	unit  string
	mixed bool
	used  bool
}

func (a *dimAcc) add(base float64, unit string) {
	if !a.used {
		a.used, a.unit = true, unit
	} else if a.unit != unit {
		a.mixed = true
	}
	a.base += base
}

type builder struct {
	name    string
	mass    dimAcc
	vol     dimAcc
	count   dimAcc
	sources []string
}

func (b *builder) add(p parsed) {
	switch p.dim {
	case DimMass:
		b.mass.add(p.base, p.unit)
	case DimVolume:
		b.vol.add(p.base, p.unit)
	case DimCount:
		b.count.add(p.base, "")
	}
}

func (b *builder) finalize(sys System) Item {
	var parts []string
	if b.mass.used {
		parts = append(parts, fmtDim(b.mass, "g", sys))
	}
	if b.vol.used {
		parts = append(parts, fmtDim(b.vol, "ml", sys))
	}
	if b.count.used {
		parts = append(parts, num2(b.count.base))
	}
	return Item{
		Name:    b.name,
		Amount:  strings.Join(parts, " + "),
		Parsed:  true,
		Sources: b.sources,
	}
}

// fmtDim formats an accumulated mass/volume. baseUnit is "g" or "ml".
func fmtDim(a dimAcc, baseUnit string, sys System) string {
	if !a.mixed && a.unit != "" {
		return fmtValUnit(a.base/sys.factor(units[a.unit]), a.unit)
	}
	if a.base >= 1000 {
		if baseUnit == "g" {
			return num2(a.base/1000) + "kg"
		}
		return num2(a.base/1000) + "L"
	}
	return num0(a.base) + baseUnit
}

func fmtValUnit(v float64, unit string) string {
	switch unit {
	case "g":
		return num0(v) + "g"
	case "kg":
		return num2(v) + "kg"
	case "oz":
		return num2(v) + " oz"
	case "lb":
		return num2(v) + " lb"
	case "ml":
		return num0(v) + "ml"
	case "cc":
		return num0(v) + "cc"
	case "l":
		return num2(v) + "L"
	case "tsp":
		return num2(v) + " tsp"
	case "tbsp":
		return num2(v) + " tbsp"
	case "cup":
		if v == 1 {
			return "1 cup"
		}
		return num2(v) + " cups"
	case "floz":
		return num2(v) + " fl oz"
	}
	return num2(v)
}

func num0(f float64) string {
	return strconv.FormatInt(int64(math.Round(f)), 10)
}

func num2(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	return s
}
