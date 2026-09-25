# ChefCal

A Go web service that generates weekly meal plans from recipes stored in Nextcloud and serves them as an iCal calendar feed. Subscribe to the feed from Nextcloud (or any calendar app) to see your dinner schedule and shopping list.

## How It Works

1. ChefCal connects to your Nextcloud instance via WebDAV (read-only)
2. It reads meal plan files (`.md`) that list recipe names, and recipe data (`recipe.json`) from your Nextcloud directories
3. When you generate a week, it randomly picks 7 distinct recipes from the chosen meal plan and assigns one per day. Weeks run Saturday–Friday, and the next week to plan starts on the coming Saturday
4. Each dinner event is timed so that cooking finishes by 18:30 (configurable), with the start time calculated from the recipe's total prep/cook time
5. A shopping list event is created at noon on the configured shopping day (Saturday by default, the first day of the week). Ingredients are deduplicated across recipes — quantities in compatible units are summed (e.g. `200g chicken` + `300g chicken` → `500g chicken`) — and presented as a consolidated list, with a per-recipe breakdown kept below for reference
6. The calendar is served as a standard `.ics` feed that any calendar app can subscribe to

ChefCal supports two delivery models:

- **Pull** (default): ChefCal runs as a long-lived HTTP server and Nextcloud subscribes to its `.ics` feed. See [API Endpoints](#api-endpoints).
- **Push**: ChefCal runs as a one-shot command (ideal for cron) that authenticates to Nextcloud over CalDAV and writes events directly onto a calendar it owns. No server to keep running, no feed to expose. See [Push Mode](#push-mode).

## Nextcloud Directory Structure

ChefCal uses the default layout from the [official Cookbook app](https://github.com/nextcloud/cookbook/). Thus, ChefCal expects the following layout in your Nextcloud files:

```
/Meal Plans/
  Japanese Chicken.md
  Comfort Food.md
  ...
/Recipes/
  Rindergoulasch/
    recipe.json
  French Onion Soup/
    recipe.json
  ...
```

### Meal Plan Files

Markdown files listing one recipe name per line. Lines starting with `#` are ignored.

```markdown
# Weekly comfort food
Rindergoulasch
French Onion Soup
Oyakodon
```

The recipe names must match directory names under `/Recipes/` (case-insensitively). Recipes that can't be read are skipped with a warning; a meal plan needs at least 7 readable recipes to generate a week.

### Recipe Files

JSON files following the schema.org `Recipe` format. The fields used by ChefCal are:

```json
{
  "name": "Rindergoulasch",
  "description": "German-style goulash stew",
  "totalTime": "PT1H10M0S",
  "recipeIngredient": [
    "700g stewing beef",
    "1 tablespoon oil",
    "..."
  ]
}
```

- `totalTime` — ISO 8601 duration used to calculate when cooking should start (defaults to 30 minutes if missing or unparseable)
- `recipeIngredient` — used for dinner event descriptions and the weekly shopping list

#### Shopping List Deduplication

The weekly shopping list consolidates ingredients across recipes. Each ingredient string is parsed into a quantity, unit, and name; entries with the same name and compatible units are summed:

- Mass (`g`, `kg`, `oz`, `lb`), volume (`ml`, `cc`, `l`, `tsp`, `tbsp`, `cup`, `fl oz`), and counts are each summed within their own dimension, converting units as needed (`1 lb` + `200g` → `654g`).
- `cc` is treated as millilitres. The size of `cup`/`tbsp`/`tsp` follows `planner.measurement_system`: `us` (cup 236ml, tbsp 14.79ml, tsp 4.93ml) or `japanese` (cup 200ml, tbsp 15ml, tsp 5ml).
- Incompatible units for the same ingredient are shown side by side (`200g + 1 cup rice`).
- Name matching is deliberately conservative: cosmetic size words (`large`, `small`) and simple plurals are folded, but material descriptors are kept, so `chicken breast`, `chicken thigh`, and `chicken stock` stay separate.

Parsing is best-effort. Anything without a recognisable quantity (`Salt to taste`, `Olive oil`) is passed through verbatim — **the list never drops an ingredient**. A per-recipe breakdown is always included below the consolidated list.

**Pantry staples** — ingredients you don't buy per recipe (water, salt, soy sauce, …) can be omitted from the consolidated list via `planner.pantry_staples`. They still appear in the per-recipe breakdown, so nothing is truly lost. Matching is by normalised name: a single-word entry must be the whole ingredient name (`water` won't drop `coconut water`), while a multi-word entry matches as a contiguous phrase (`soy sauce` also drops `light soy sauce`). Unset uses a built-in default of `[water, ice]`; an explicit empty list disables exclusion.

## Getting Started

### Prerequisites

- Go 1.22 or later
- A Nextcloud instance with WebDAV access

### Build and Run

```bash
go build -o chefcal .
cp config.yaml.example config.yaml
# Edit config.yaml with your Nextcloud credentials
./chefcal -config config.yaml
```

Or run directly:

```bash
go run . -config config.yaml
```

A `Makefile` wraps the common tasks: `make build`, `make test`, `make test-cover`, `make lint` (vet + gofmt), `make docker`, and `make run`.

### Docker

```bash
docker build -t chefcal .
docker run -p 8080:8080 -v ./config.yaml:/config.yaml -v ./data:/data chefcal
```

With the default `store.path` of `data/weeks.json`, the store lands in the mounted `/data` volume. For push mode, override the command to pass the action flags:

```bash
docker run --rm -v ./config.yaml:/config.yaml -v ./data:/data chefcal -config /config.yaml -generate -push
```

### Configuration

Copy `config.yaml.example` and edit it (the example file also documents `pantry_staples` in detail):

```yaml
server:
  address: ":8080"

nextcloud:
  url: "https://your-nextcloud.example.com/remote.php/dav/files/username"
  username: "your-username"
  password: "your-password"
  meal_plans_path: "/Meal Plans"
  recipes_path: "/Recipes"
  insecure_skip_verify: false  # set to true for self-signed certificates
  # Push mode only (see below):
  calendar_url: "https://your-nextcloud.example.com/remote.php/dav/calendars/username/chefcal"
  calendar_display_name: "Meal Plan"

planner:
  dinner_done_by: "18:30"
  shopping_event_time: "12:00"
  shopping_event_day: "Saturday"
  timezone: "Australia/Sydney"
  measurement_system: "us"
  # pantry_staples: [water, ice, salt, soy sauce]

store:
  path: "data/weeks.json"
```

| Key | Description | Default |
|-----|-------------|---------|
| `server.address` | Listen address | `:8080` |
| `nextcloud.url` | WebDAV root URL for your Nextcloud user | (required) |
| `nextcloud.username` | Nextcloud username | (required) |
| `nextcloud.password` | Nextcloud password or app token | (required) |
| `nextcloud.meal_plans_path` | Path to meal plan files | `/Meal Plans` |
| `nextcloud.recipes_path` | Path to recipe directories | `/Recipes` |
| `nextcloud.insecure_skip_verify` | Skip TLS certificate verification (for self-signed certs) | `false` |
| `nextcloud.calendar_url` | CalDAV collection URL for push mode; created if missing | (required for `-push`) |
| `nextcloud.calendar_display_name` | Display name used when creating the calendar | `Meal Plan` |
| `planner.dinner_done_by` | Target time for dinner to be ready | `18:30` |
| `planner.shopping_event_time` | Time for the shopping list event | `12:00` |
| `planner.shopping_event_day` | Day of week for the shopping list event | `Saturday` |
| `planner.timezone` | IANA timezone for calendar events | `Australia/Sydney` |
| `planner.measurement_system` | `us` or `japanese`; sizes cup/tbsp/tsp when summing the shopping list | `us` |
| `planner.pantry_staples` | Ingredient names left off the consolidated shopping list (see [Shopping List Deduplication](#shopping-list-deduplication)) | `[water, ice]` |
| `store.path` | Path to the JSON file storing generated weeks | `data/weeks.json` |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Web UI for viewing current plans and generating new ones |
| `GET` | `/calendar.ics` | iCal feed — subscribe to this from Nextcloud or any calendar app |
| `GET` | `/plans` | JSON array of available meal plan names |
| `POST` | `/generate?plan=Name` | Generate a meal plan for the next unplanned week. Omit `plan` to pick a random meal plan file |

### Subscribing in Nextcloud

1. Start ChefCal on your network
2. In Nextcloud, go to the Calendar app
3. Click "New subscription from link (read-only)"
4. Enter `http://<chefcal-host>:8080/calendar.ics`

The calendar will show dinner events for each day of the generated week(s) and a shopping list event on the configured shopping day.

### Generating a Week

From the web UI at `/`, select a meal plan (or leave it on "Random") and click "Generate Next Week".

Or via the API:

```bash
# Generate with a specific meal plan
curl -X POST 'http://localhost:8080/generate?plan=Japanese%20Chicken'

# Generate with a random meal plan
curl -X POST http://localhost:8080/generate
```

The response includes the week start date, chosen plan, and daily meals:

```json
{
  "week_start": "2026-04-11",
  "plan": "Japanese Chicken",
  "days": [
    {"date": "Saturday, Apr 11", "recipe": "Oyakodon"},
    {"date": "Sunday, Apr 12", "recipe": "Karaage"}
  ]
}
```

Weeks start on Saturday. If next week already has a plan, the service automatically targets the week after.

## Command-Line Flags

Run with no action flags to start the HTTP server (pull mode). Passing `-generate` and/or `-push` runs those one-shot actions and exits without starting the server.

| Flag | Description |
|------|-------------|
| `-config <path>` | Path to the configuration file (default `config.yaml`) |
| `-generate` | Generate the next unplanned week, save it to the store, and exit |
| `-regenerate` | Re-roll the earliest upcoming planned week in place (new recipes), and exit |
| `-plan <name>` | Meal plan for `-generate`/`-regenerate` (random for generate, the week's current plan for regenerate, if omitted) |
| `-week <YYYY-MM-DD>` | Target week start for `-generate` (overwrites any plan for that week); default is the next unplanned week. Should be a Saturday, since weeks run Saturday–Friday. Use to back-date and fill the current week |
| `-push` | Reconcile the stored plans onto the Nextcloud calendar and exit |

These actions can be combined in one invocation and run in order generate → regenerate → push, so e.g. `chefcal -regenerate -push` re-rolls this week and republishes it. Because push resource UIDs are keyed by date, the re-rolled events update in place rather than duplicating. `-regenerate` reuses the week's existing meal plan unless `-plan` overrides it.

## Push Mode

Instead of exposing an `.ics` feed for Nextcloud to pull, ChefCal can authenticate over CalDAV and write events directly onto a calendar it owns. This needs no long-running server and no exposed network endpoint, so it runs well as a local cronjob.

### Setup

1. Set `nextcloud.calendar_url` to a CalDAV collection URL. The final path segment is the calendar ID — pick any unused value; ChefCal creates the calendar on the first push if it does not exist:

   ```
   https://your-nextcloud.example.com/remote.php/dav/calendars/<username>/chefcal
   ```

2. Use a Nextcloud **app password** for `nextcloud.password` rather than your account password. The same credentials are used for reading recipes (WebDAV) and writing events (CalDAV).

Give ChefCal a **dedicated** calendar. It treats the collection as exclusively its own, so reconciliation is free to delete anything that no longer belongs.

### Running

```bash
# Plan the next unplanned week (random plan), then publish to the calendar
./chefcal -generate -push

# Publish/refresh without generating a new week (e.g. after editing a plan)
./chefcal -push
```

A typical cron setup — a new week each Saturday morning, plus a nightly reconcile so edits and retractions stay in sync:

```cron
0 6 * * SAT   cd /opt/chefcal && ./chefcal -generate -push >> cron.log 2>&1
0 3 * * *     cd /opt/chefcal && ./chefcal -push >> cron.log 2>&1
```

Running `-generate` on a Saturday plans the week starting the *following* Saturday, so there's always a week of lead time for shopping. To fill the current week instead, pass `-week` with this week's Saturday.

### How reconciliation works

Each `-push` makes the calendar match the current stored plans (weeks that have not fully passed):

- **Created / updated in place** — event UIDs and resource names are deterministic functions of `(kind, date)`, so re-pushing the same day overwrites its event rather than creating a duplicate. The **whole** of each current week is published, including days before today and the weekly shopping-list event — so back-filling the current week mid-week still publishes its aggregated shopping list even though it sits on the (now past) shopping day.
- **Deleted** — any ChefCal-owned **future** entry that is no longer in the plan (e.g. a retracted or regenerated week) is removed.
- **Never deleted** — entries dated before today are left in place as history, even if they drop out of the plan.

Because reconciliation compares against what is actually on the server, it is self-healing: if the local store is wiped or the calendar is hand-edited, the next push converges. The calendar — not the local store — is the source of truth for what is published.

## Data Persistence

Generated week plans are stored in a JSON file (configured via `store.path`). Past weeks are automatically cleaned up — only current and upcoming weeks are kept.
