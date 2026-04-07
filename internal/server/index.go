package server

import (
	"fmt"
	"html"
	"strings"

	"github.com/aaronjauregui/chefcal/internal/model"
)

func renderIndex(weeks []*model.WeekPlan, plans []string, host string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ChefCal - Meal Planner</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 800px; margin: 2rem auto; padding: 0 1rem; color: #333; }
  h1 { color: #2d5016; }
  h2 { color: #444; border-bottom: 1px solid #ddd; padding-bottom: 0.3rem; }
  .card { background: #f9f9f9; border: 1px solid #ddd; border-radius: 8px; padding: 1rem; margin: 1rem 0; }
  .day { display: flex; justify-content: space-between; padding: 0.4rem 0; border-bottom: 1px solid #eee; }
  .day:last-child { border-bottom: none; }
  .day-name { font-weight: 600; min-width: 100px; }
  .actions { margin: 1.5rem 0; }
  button, select { padding: 0.5rem 1rem; font-size: 1rem; border-radius: 4px; border: 1px solid #ccc; cursor: pointer; }
  button { background: #2d5016; color: white; border: none; }
  button:hover { background: #3d6b20; }
  .calendar-url { background: #eef; padding: 0.5rem; border-radius: 4px; font-family: monospace; word-break: break-all; }
  .status { margin-top: 1rem; padding: 0.5rem; display: none; border-radius: 4px; }
  .status.success { display: block; background: #dfd; color: #262; }
  .status.error { display: block; background: #fdd; color: #622; }
</style>
</head>
<body>
<h1>ChefCal</h1>
`)

	// Calendar subscription URL
	calURL := fmt.Sprintf("http://%s/calendar.ics", html.EscapeString(host))
	b.WriteString(fmt.Sprintf(`<h2>Calendar Feed</h2>
<div class="card">
<p>Subscribe to this URL in Nextcloud:</p>
<div class="calendar-url">%s</div>
</div>
`, calURL))

	// Generate section
	b.WriteString(`<h2>Generate Week</h2>
<div class="card">
<form id="generate-form">
<select id="plan-select">
<option value="">Random plan</option>
`)
	for _, p := range plans {
		escaped := html.EscapeString(p)
		b.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`+"\n", escaped, escaped))
	}
	b.WriteString(`</select>
<button type="submit">Generate Next Week</button>
</form>
<div id="status" class="status"></div>
</div>
`)

	// Current weeks
	if len(weeks) > 0 {
		b.WriteString(`<h2>Current Meal Plans</h2>`)
		for _, week := range weeks {
			b.WriteString(fmt.Sprintf(`<div class="card">
<strong>Week of %s</strong> (plan: %s)
`, week.WeekStart.Format("Jan 2, 2006"), html.EscapeString(week.MealPlanName)))
			for _, day := range week.Days {
				b.WriteString(fmt.Sprintf(`<div class="day"><span class="day-name">%s</span><span>%s</span></div>
`, day.Date.Format("Monday"), html.EscapeString(day.RecipeName)))
			}
			b.WriteString("</div>\n")
		}
	} else {
		b.WriteString(`<div class="card"><p>No meal plans generated yet. Generate one above!</p></div>`)
	}

	b.WriteString(`
<script>
document.getElementById('generate-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const status = document.getElementById('status');
  const plan = document.getElementById('plan-select').value;
  const url = plan ? '/generate?plan=' + encodeURIComponent(plan) : '/generate';
  status.className = 'status';
  status.textContent = 'Generating...';
  status.style.display = 'block';
  try {
    const resp = await fetch(url, { method: 'POST' });
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text);
    }
    const data = await resp.json();
    status.className = 'status success';
    status.textContent = 'Generated week of ' + data.week_start + ' with plan "' + data.plan + '"';
    setTimeout(() => location.reload(), 1500);
  } catch (err) {
    status.className = 'status error';
    status.textContent = 'Error: ' + err.message;
  }
});
</script>
</body>
</html>`)

	return b.String()
}
