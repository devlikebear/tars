package tarsserver

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

type projectDashboardPageData struct {
	Project    project.Project
	State      *project.ProjectState
	Activity   []project.Activity
	Board      project.Board
	BoardStats []projectDashboardBoardStat
}

type projectDashboardBoardStat struct {
	Status string
	Count  int
}

var projectDashboardTemplate = template.Must(template.New("project-dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Project.Name}} | TARS</title>
  <style>
    :root { color-scheme: light; }
    body { margin: 0; font-family: Georgia, "Times New Roman", serif; background: #f3efe4; color: #1f1a14; }
    main { max-width: 1040px; margin: 0 auto; padding: 32px 20px 48px; }
    h1, h2 { margin: 0 0 12px; }
    h1 { font-size: 2.1rem; }
    h2 { font-size: 1.1rem; letter-spacing: 0.02em; text-transform: uppercase; }
    p { line-height: 1.5; }
    .grid { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); margin: 20px 0 28px; }
    .card { background: #fffaf0; border: 1px solid #d8ccb5; border-radius: 14px; padding: 16px; box-shadow: 0 6px 18px rgba(77, 56, 28, 0.08); }
    .label { font-size: 0.78rem; text-transform: uppercase; color: #7a6545; margin-bottom: 6px; }
    .value { font-size: 1rem; font-weight: 600; }
    .muted { color: #6a5a43; }
    .stack { display: grid; gap: 12px; }
    .stats { display: grid; gap: 12px; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); margin-bottom: 16px; }
    .stat { background: #f8f1e3; border-radius: 12px; padding: 12px; border: 1px solid #eadfc9; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 10px 8px; border-top: 1px solid #e6dac4; vertical-align: top; }
    th { font-size: 0.78rem; text-transform: uppercase; color: #7a6545; }
    ul { margin: 0; padding-left: 18px; }
    li + li { margin-top: 10px; }
    code { font-family: "SFMono-Regular", Consolas, monospace; font-size: 0.92em; }
  </style>
</head>
<body>
  <main>
    <header class="card">
      <div class="label">Project</div>
      <h1>{{.Project.Name}}</h1>
      {{if .Project.Objective}}<p>{{.Project.Objective}}</p>{{end}}
      {{if .Project.Body}}<p class="muted">{{.Project.Body}}</p>{{end}}
    </header>

    <section class="grid">
      <article class="card">
        <div class="label">Project ID</div>
        <div class="value"><code>{{.Project.ID}}</code></div>
      </article>
      <article class="card">
        <div class="label">Status</div>
        <div class="value">{{if .State}}{{.State.Status}}{{else}}{{.Project.Status}}{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Phase</div>
        <div class="value">{{if .State}}{{.State.Phase}}{{else}}planning{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Next Action</div>
        <div class="value">{{if and .State .State.NextAction}}{{.State.NextAction}}{{else}}-{{end}}</div>
      </article>
    </section>

    <section class="card">
      <h2>Board</h2>
      <div class="stats">
        {{range .BoardStats}}
        <article class="stat">
          <div class="label">{{.Status}}</div>
          <div class="value">{{.Count}}</div>
          {{if gt .Count 0}}<div class="muted">{{.Count}} active</div>{{end}}
        </article>
        {{end}}
      </div>
      {{if .Board.Tasks}}
      <table>
        <thead>
          <tr>
            <th>Task</th>
            <th>Status</th>
            <th>Assignee</th>
          </tr>
        </thead>
        <tbody>
          {{range .Board.Tasks}}
          <tr>
            <td>
              <strong>{{.Title}}</strong>
              {{if .Role}}<div class="muted">{{.Role}}</div>{{end}}
            </td>
            <td><code>{{.Status}}</code></td>
            <td>{{if .Assignee}}{{.Assignee}}{{else}}-{{end}}</td>
          </tr>
          {{end}}
        </tbody>
      </table>
      {{else}}
      <p class="muted">No board tasks recorded yet.</p>
      {{end}}
    </section>

    <section class="card">
      <h2>Recent Activity</h2>
      {{if .Activity}}
      <ul>
        {{range .Activity}}
        <li>
          <strong>{{.Kind}}</strong>
          <span class="muted">{{.Timestamp}}</span>
          {{if .Agent}}<span class="muted">· {{.Agent}}</span>{{end}}
          {{if .Status}}<span class="muted">· {{.Status}}</span>{{end}}
          <div>{{.Message}}</div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="muted">No activity recorded yet.</p>
      {{end}}
    </section>
  </main>
</body>
</html>`))

func newProjectDashboardHandler(store *project.Store, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
			return
		}
		projectID, ok := parseProjectDashboardPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		item, err := store.Get(projectID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var state *project.ProjectState
		if current, err := store.GetState(projectID); err == nil {
			state = &current
		}
		activity, err := store.ListActivity(projectID, 20)
		if err != nil {
			logger.Error().Err(err).Str("project_id", projectID).Msg("list project activity for dashboard failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load dashboard failed"})
			return
		}
		board, err := store.GetBoard(projectID)
		if err != nil {
			logger.Error().Err(err).Str("project_id", projectID).Msg("load project board for dashboard failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load dashboard failed"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := projectDashboardTemplate.Execute(w, projectDashboardPageData{
			Project:    item,
			State:      state,
			Activity:   activity,
			Board:      board,
			BoardStats: buildProjectDashboardBoardStats(board),
		}); err != nil {
			logger.Error().Err(err).Str("project_id", projectID).Msg("render project dashboard failed")
		}
	})
}

func buildProjectDashboardBoardStats(board project.Board) []projectDashboardBoardStat {
	stats := make([]projectDashboardBoardStat, 0, len(board.Columns))
	counts := make(map[string]int, len(board.Columns))
	for _, task := range board.Tasks {
		counts[task.Status]++
	}
	for _, column := range board.Columns {
		stats = append(stats, projectDashboardBoardStat{
			Status: column,
			Count:  counts[column],
		})
	}
	return stats
}

func parseProjectDashboardPath(path string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(path, "/ui/projects/"))
	if trimmed == "" {
		return "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 1 {
		return "", false
	}
	projectID := strings.TrimSpace(parts[0])
	if projectID == "" {
		return "", false
	}
	return projectID, true
}
