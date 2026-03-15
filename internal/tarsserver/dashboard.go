package tarsserver

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

type projectDashboardPageData struct {
	Project    project.Project
	State      *project.ProjectState
	Status     string
	Phase      string
	NextAction string
	Autopilot  *project.AutopilotRun
	Activity   []project.Activity
	Board      project.Board
	BoardStats []projectDashboardBoardStat
	GitHubFlow []projectDashboardGitHubFlowRow
	Reports    []projectDashboardWorkerReport
	Blockers   []projectDashboardPMItem
	Decisions  []projectDashboardPMItem
	Replans    []projectDashboardPMItem
	Sections   projectDashboardSections
	PagePath   string
	StreamPath string
}

type projectDashboardListPageData struct {
	Projects []projectDashboardListItem
}

type projectDashboardListItem struct {
	ID              string
	Name            string
	Objective       string
	Status          string
	Phase           string
	NextAction      string
	UpdatedAt       string
	DashboardPath   string
	AutopilotStatus string
	AutopilotRunID  string
	AutopilotNote   string
}

type projectDashboardBoardStat struct {
	Status string
	Count  int
}

type projectDashboardGitHubFlowRow struct {
	Task             string
	Issue            string
	Branch           string
	PR               string
	ReviewApprovedBy string
	TestStatus       string
	BuildStatus      string
	IssueStatus      string
	BranchStatus     string
	PRStatus         string
}

type projectDashboardWorkerReport struct {
	Task      string
	Agent     string
	Status    string
	Message   string
	Notes     string
	RunID     string
	Timestamp string
}

type projectDashboardPMItem struct {
	Task      string
	Status    string
	Message   string
	Timestamp string
}

type projectDashboardSectionMeta struct {
	ID string
}

type projectDashboardSections struct {
	Autopilot  projectDashboardSectionMeta
	Board      projectDashboardSectionMeta
	Activity   projectDashboardSectionMeta
	GitHubFlow projectDashboardSectionMeta
	Reports    projectDashboardSectionMeta
	Blockers   projectDashboardSectionMeta
	Decisions  projectDashboardSectionMeta
	Replans    projectDashboardSectionMeta
	RefreshIDs []string
}

type projectDashboardSectionSpec struct {
	Key      string
	ID       string
	Refresh  bool
	Populate func(*projectDashboardPageData, project.Board, []project.Activity)
}

type projectDashboardRoute struct {
	ProjectID string
	Stream    bool
}

type projectDashboardEvent struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Timestamp string `json:"timestamp"`
}

var projectDashboardSectionRegistry = []projectDashboardSectionSpec{
	{
		Key:     "autopilot",
		ID:      "autopilot-section",
		Refresh: true,
	},
	{
		Key:     "board",
		ID:      "board-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, _ []project.Activity) {
			data.BoardStats = buildProjectDashboardBoardStats(board)
		},
	},
	{
		Key:     "activity",
		ID:      "activity-section",
		Refresh: true,
	},
	{
		Key:     "github-flow",
		ID:      "github-flow-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, activity []project.Activity) {
			data.GitHubFlow = buildProjectDashboardGitHubFlow(board, activity)
		},
	},
	{
		Key:     "reports",
		ID:      "reports-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, activity []project.Activity) {
			data.Reports = buildProjectDashboardWorkerReports(board, activity)
		},
	},
	{
		Key:     "blockers",
		ID:      "blockers-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, activity []project.Activity) {
			data.Blockers = buildProjectDashboardPMItems(board, activity, project.ActivityKindBlocker)
		},
	},
	{
		Key:     "decisions",
		ID:      "decisions-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, activity []project.Activity) {
			data.Decisions = buildProjectDashboardPMItems(board, activity, project.ActivityKindDecision)
		},
	},
	{
		Key:     "replans",
		ID:      "replans-section",
		Refresh: true,
		Populate: func(data *projectDashboardPageData, board project.Board, activity []project.Activity) {
			data.Replans = buildProjectDashboardPMItems(board, activity, project.ActivityKindReplan)
		},
	},
}

type projectDashboardBroker struct {
	mu     sync.RWMutex
	nextID int
	subs   map[int]chan projectDashboardEvent
}

func newProjectDashboardBroker() *projectDashboardBroker {
	return &projectDashboardBroker{subs: map[int]chan projectDashboardEvent{}}
}

func newProjectDashboardEvent(projectID, kind string) projectDashboardEvent {
	return projectDashboardEvent{
		Type:      "project_dashboard",
		ProjectID: strings.TrimSpace(projectID),
		Kind:      strings.TrimSpace(kind),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func (b *projectDashboardBroker) subscribe() (<-chan projectDashboardEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan projectDashboardEvent, 32)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if current, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(current)
		}
	}
}

func (b *projectDashboardBroker) publish(evt projectDashboardEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		select {
		case sub <- evt:
		default:
		}
	}
}

const projectDashboardStyles = `
    :root { color-scheme: light; }
    body { margin: 0; font-family: Georgia, "Times New Roman", serif; background: #f3efe4; color: #1f1a14; }
    main { max-width: 1040px; margin: 0 auto; padding: 32px 20px 48px; }
    h1, h2, h3 { margin: 0 0 12px; }
    h1 { font-size: 2.1rem; }
    h2 { font-size: 1.1rem; letter-spacing: 0.02em; text-transform: uppercase; }
    h3 { font-size: 1.25rem; }
    p { line-height: 1.5; }
    a { color: #6b3f1d; }
    .grid { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); margin: 20px 0 28px; }
    .card { background: #fffaf0; border: 1px solid #d8ccb5; border-radius: 14px; padding: 16px; box-shadow: 0 6px 18px rgba(77, 56, 28, 0.08); }
    .label { font-size: 0.78rem; text-transform: uppercase; color: #7a6545; margin-bottom: 6px; }
    .value { font-size: 1rem; font-weight: 600; }
    .muted { color: #6a5a43; }
    .stack { display: grid; gap: 12px; }
    .stats { display: grid; gap: 12px; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); margin-bottom: 16px; }
    .stat { background: #f8f1e3; border-radius: 12px; padding: 12px; border: 1px solid #eadfc9; }
    .project-grid { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); margin-top: 20px; }
    .project-link { text-decoration: none; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 10px 8px; border-top: 1px solid #e6dac4; vertical-align: top; }
    th { font-size: 0.78rem; text-transform: uppercase; color: #7a6545; }
    ul { margin: 0; padding-left: 18px; }
    li + li { margin-top: 10px; }
    code { font-family: "SFMono-Regular", Consolas, monospace; font-size: 0.92em; }
`

var projectDashboardTemplate = template.Must(template.New("project-dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Project.Name}} | TARS</title>
  <style>
` + projectDashboardStyles + `
  </style>
</head>
<body>
  <main>
    <header class="card">
      <div class="label">Project</div>
      <h1>{{.Project.Name}}</h1>
      <p class="muted"><a href="/dashboards">All projects</a></p>
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
        <div class="value">{{.Status}}</div>
      </article>
      <article class="card">
        <div class="label">Phase</div>
        <div class="value">{{.Phase}}</div>
      </article>
      <article class="card">
        <div class="label">Next Action</div>
        <div class="value">{{if .NextAction}}{{.NextAction}}{{else}}-{{end}}</div>
      </article>
    </section>

    <section id="{{.Sections.Autopilot.ID}}" class="card">
      <h2>Autopilot</h2>
      {{if .Autopilot}}
      <div class="grid">
        <article class="stat">
          <div class="label">Status</div>
          <div class="value"><code>{{.Autopilot.Status}}</code></div>
        </article>
        <article class="stat">
          <div class="label">Iterations</div>
          <div class="value">{{.Autopilot.Iterations}}</div>
        </article>
        <article class="stat">
          <div class="label">Run ID</div>
          <div class="value"><code>{{.Autopilot.RunID}}</code></div>
        </article>
      </div>
      {{if .Autopilot.Message}}<p>{{.Autopilot.Message}}</p>{{end}}
      <p class="muted">
        {{if .Autopilot.StartedAt}}started {{.Autopilot.StartedAt}}{{end}}
        {{if .Autopilot.UpdatedAt}} · updated {{.Autopilot.UpdatedAt}}{{end}}
        {{if .Autopilot.FinishedAt}} · finished {{.Autopilot.FinishedAt}}{{end}}
      </p>
      {{else}}
      <p class="muted">No autopilot run recorded yet.</p>
      {{end}}
    </section>

    <section id="{{.Sections.Board.ID}}" class="card">
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

    <section id="{{.Sections.Activity.ID}}" class="card">
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

    <section id="{{.Sections.GitHubFlow.ID}}" class="card">
      <h2>GitHub Flow</h2>
      {{if .GitHubFlow}}
      <table>
        <thead>
          <tr>
            <th>Task</th>
            <th>Issue</th>
            <th>Branch</th>
            <th>PR</th>
            <th>Review</th>
            <th>Test</th>
            <th>Build</th>
          </tr>
        </thead>
        <tbody>
          {{range .GitHubFlow}}
          <tr>
            <td><strong>{{.Task}}</strong></td>
            <td>{{if .Issue}}{{.Issue}}{{else}}-{{end}}{{if .IssueStatus}}<div class="muted">{{.IssueStatus}}</div>{{end}}</td>
            <td>{{if .Branch}}{{.Branch}}{{else}}-{{end}}{{if .BranchStatus}}<div class="muted">{{.BranchStatus}}</div>{{end}}</td>
            <td>{{if .PR}}{{.PR}}{{else}}-{{end}}{{if .PRStatus}}<div class="muted">{{.PRStatus}}</div>{{end}}</td>
            <td>{{if .ReviewApprovedBy}}{{.ReviewApprovedBy}}{{else}}-{{end}}</td>
            <td>{{if .TestStatus}}<code>{{.TestStatus}}</code>{{else}}-{{end}}</td>
            <td>{{if .BuildStatus}}<code>{{.BuildStatus}}</code>{{else}}-{{end}}</td>
          </tr>
          {{end}}
        </tbody>
      </table>
      {{else}}
      <p class="muted">No GitHub Flow metadata recorded yet.</p>
      {{end}}
    </section>

    <section id="{{.Sections.Reports.ID}}" class="card">
      <h2>Worker Reports</h2>
      {{if .Reports}}
      <ul>
        {{range .Reports}}
        <li>
          <strong>{{.Task}}</strong>
          <span class="muted">{{.Timestamp}}</span>
          {{if .Agent}}<span class="muted">· {{.Agent}}</span>{{end}}
          {{if .Status}}<span class="muted">· {{.Status}}</span>{{end}}
          {{if .RunID}}<div class="muted"><code>{{.RunID}}</code></div>{{end}}
          <div>{{.Message}}</div>
          {{if .Notes}}<div class="muted">{{.Notes}}</div>{{end}}
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="muted">No worker reports recorded yet.</p>
      {{end}}
    </section>

    <section id="{{.Sections.Blockers.ID}}" class="card">
      <h2>Blockers</h2>
      {{if .Blockers}}
      <ul>
        {{range .Blockers}}
        <li>
          {{if .Task}}<strong>{{.Task}}</strong>{{end}}
          <span class="muted">{{.Timestamp}}</span>
          {{if .Status}}<span class="muted">· {{.Status}}</span>{{end}}
          <div>{{.Message}}</div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="muted">No blockers recorded yet.</p>
      {{end}}
    </section>

    <section id="{{.Sections.Decisions.ID}}" class="card">
      <h2>Decisions</h2>
      {{if .Decisions}}
      <ul>
        {{range .Decisions}}
        <li>
          {{if .Task}}<strong>{{.Task}}</strong>{{end}}
          <span class="muted">{{.Timestamp}}</span>
          {{if .Status}}<span class="muted">· {{.Status}}</span>{{end}}
          <div>{{.Message}}</div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="muted">No decisions recorded yet.</p>
      {{end}}
    </section>

    <section id="{{.Sections.Replans.ID}}" class="card">
      <h2>Replans</h2>
      {{if .Replans}}
      <ul>
        {{range .Replans}}
        <li>
          {{if .Task}}<strong>{{.Task}}</strong>{{end}}
          <span class="muted">{{.Timestamp}}</span>
          {{if .Status}}<span class="muted">· {{.Status}}</span>{{end}}
          <div>{{.Message}}</div>
        </li>
        {{end}}
      </ul>
      {{else}}
      <p class="muted">No replans recorded yet.</p>
      {{end}}
    </section>
  </main>
  <script>
    (() => {
      const streamPath = {{printf "%q" .StreamPath}};
      const pagePath = {{printf "%q" .PagePath}};
      const refreshIDs = [{{range $index, $id := .Sections.RefreshIDs}}{{if $index}}, {{end}}{{printf "%q" $id}}{{end}}];
      if (!streamPath || !pagePath || typeof EventSource === "undefined") {
        return;
      }
      let refreshing = false;
      async function refreshSections() {
        if (refreshing) {
          return;
        }
        refreshing = true;
        try {
          const response = await fetch(pagePath, { headers: { "X-Tars-Dashboard": "refresh" } });
          if (!response.ok) {
            return;
          }
          const html = await response.text();
          const doc = new DOMParser().parseFromString(html, "text/html");
          for (const id of refreshIDs) {
            const next = doc.getElementById(id);
            const current = document.getElementById(id);
            if (next && current) {
              current.replaceWith(next);
            }
          }
        } finally {
          refreshing = false;
        }
      }
      const source = new EventSource(streamPath);
      source.onmessage = (event) => {
        if (!event.data) {
          return;
        }
        try {
          const payload = JSON.parse(event.data);
          if (payload.type === "keepalive" || payload.kind === "connected") {
            return;
          }
        } catch (_) {
        }
        refreshSections();
      };
    })();
  </script>
</body>
</html>`))

var projectDashboardListTemplate = template.Must(template.New("project-dashboard-list").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Projects | TARS</title>
  <style>
` + projectDashboardStyles + `
  </style>
</head>
<body>
  <main>
    <header class="card">
      <div class="label">Dashboards</div>
      <h1>Projects</h1>
      <p class="muted">Browse every project dashboard in the current workspace.</p>
    </header>
    {{if .Projects}}
    <section class="project-grid">
      {{range .Projects}}
      <article class="card">
        <div class="label">Project</div>
        <h3><a class="project-link" href="{{.DashboardPath}}">{{.Name}}</a></h3>
        <div class="muted"><code>{{.ID}}</code></div>
        {{if .Objective}}<p>{{.Objective}}</p>{{end}}
        <div class="stack">
          <div>
            <div class="label">Status</div>
            <div class="value">{{.Status}}</div>
          </div>
          <div>
            <div class="label">Phase</div>
            <div class="value">{{.Phase}}</div>
          </div>
          <div>
            <div class="label">Next Action</div>
            <div class="value">{{if .NextAction}}{{.NextAction}}{{else}}-{{end}}</div>
          </div>
          <div>
            <div class="label">Autopilot</div>
            <div class="value">{{if .AutopilotStatus}}<code>{{.AutopilotStatus}}</code>{{else}}-{{end}}</div>
            {{if .AutopilotRunID}}<div class="muted"><code>{{.AutopilotRunID}}</code></div>{{end}}
            {{if .AutopilotNote}}<div class="muted">{{.AutopilotNote}}</div>{{end}}
          </div>
          <div>
            <div class="label">Updated</div>
            <div class="value">{{.UpdatedAt}}</div>
          </div>
        </div>
      </article>
      {{end}}
    </section>
    {{else}}
    <section class="card">
      <p class="muted">No projects recorded yet.</p>
    </section>
    {{end}}
  </main>
</body>
</html>`))

type projectAutopilotStatusProvider interface {
	Status(projectID string) (project.AutopilotRun, bool)
}

func newProjectDashboardHandler(store *project.Store, autopilot projectAutopilotStatusProvider, broker *projectDashboardBroker, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if isProjectDashboardListPath(r.URL.Path) {
			serveProjectDashboardList(w, store, autopilot, logger)
			return
		}
		route, ok := parseProjectDashboardPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if route.Stream {
			serveProjectDashboardStream(w, r, route.ProjectID, broker, logger)
			return
		}
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
			return
		}
		item, err := store.Get(route.ProjectID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var state *project.ProjectState
		if current, err := store.GetState(route.ProjectID); err == nil {
			state = &current
		}
		var autopilotRun *project.AutopilotRun
		if autopilot != nil {
			if current, ok := autopilot.Status(route.ProjectID); ok {
				autopilotRun = &current
			}
		}
		activity, err := store.ListRecentActivity(route.ProjectID)
		if err != nil {
			logger.Error().Err(err).Str("project_id", route.ProjectID).Msg("list project activity for dashboard failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load dashboard failed"})
			return
		}
		board, err := store.GetBoard(route.ProjectID)
		if err != nil {
			logger.Error().Err(err).Str("project_id", route.ProjectID).Msg("load project board for dashboard failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load dashboard failed"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := projectDashboardTemplate.Execute(w, buildProjectDashboardPageData(item, state, autopilotRun, activity, board)); err != nil {
			logger.Error().Err(err).Str("project_id", route.ProjectID).Msg("render project dashboard failed")
		}
	})
}

func serveProjectDashboardList(w http.ResponseWriter, store *project.Store, autopilot projectAutopilotStatusProvider, logger zerolog.Logger) {
	if store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
		return
	}
	rows, err := buildProjectDashboardList(store, autopilot)
	if err != nil {
		logger.Error().Err(err).Msg("list projects for dashboard index failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load dashboard failed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := projectDashboardListTemplate.Execute(w, projectDashboardListPageData{Projects: rows}); err != nil {
		logger.Error().Err(err).Msg("render project dashboard index failed")
	}
}

func buildProjectDashboardList(store *project.Store, autopilot projectAutopilotStatusProvider) ([]projectDashboardListItem, error) {
	projects, err := store.List()
	if err != nil {
		return nil, err
	}
	rows := make([]projectDashboardListItem, 0, len(projects))
	for _, item := range projects {
		var state *project.ProjectState
		if current, err := store.GetState(item.ID); err == nil {
			state = &current
		}
		status, phase, nextAction := project.DefaultWorkflowPolicy.ProjectStateSummary(item, state)
		row := projectDashboardListItem{
			ID:            strings.TrimSpace(item.ID),
			Name:          strings.TrimSpace(item.Name),
			Objective:     strings.TrimSpace(item.Objective),
			Status:        status,
			Phase:         phase,
			NextAction:    nextAction,
			UpdatedAt:     strings.TrimSpace(item.UpdatedAt),
			DashboardPath: fmt.Sprintf("/ui/projects/%s", strings.TrimSpace(item.ID)),
		}
		if autopilot != nil {
			if current, ok := autopilot.Status(item.ID); ok {
				row.AutopilotStatus = strings.TrimSpace(string(current.Status))
				row.AutopilotRunID = strings.TrimSpace(current.RunID)
				row.AutopilotNote = strings.TrimSpace(current.Message)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
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

func buildProjectDashboardPageData(
	item project.Project,
	state *project.ProjectState,
	autopilotRun *project.AutopilotRun,
	activity []project.Activity,
	board project.Board,
) projectDashboardPageData {
	status, phase, nextAction := project.DefaultWorkflowPolicy.ProjectStateSummary(item, state)
	data := projectDashboardPageData{
		Project:    item,
		State:      state,
		Status:     status,
		Phase:      phase,
		NextAction: nextAction,
		Autopilot:  autopilotRun,
		Activity:   activity,
		Board:      board,
		Sections:   buildProjectDashboardSections(),
		PagePath:   fmt.Sprintf("/ui/projects/%s", strings.TrimSpace(item.ID)),
		StreamPath: fmt.Sprintf("/ui/projects/%s/stream", strings.TrimSpace(item.ID)),
	}
	for _, spec := range projectDashboardSectionRegistry {
		if spec.Populate != nil {
			spec.Populate(&data, board, activity)
		}
	}
	return data
}

func buildProjectDashboardSections() projectDashboardSections {
	sections := projectDashboardSections{}
	refreshIDs := make([]string, 0, len(projectDashboardSectionRegistry))
	for _, spec := range projectDashboardSectionRegistry {
		meta := projectDashboardSectionMeta{ID: spec.ID}
		switch spec.Key {
		case "autopilot":
			sections.Autopilot = meta
		case "board":
			sections.Board = meta
		case "activity":
			sections.Activity = meta
		case "github-flow":
			sections.GitHubFlow = meta
		case "reports":
			sections.Reports = meta
		case "blockers":
			sections.Blockers = meta
		case "decisions":
			sections.Decisions = meta
		case "replans":
			sections.Replans = meta
		}
		if spec.Refresh {
			refreshIDs = append(refreshIDs, spec.ID)
		}
	}
	sections.RefreshIDs = refreshIDs
	return sections
}

func projectDashboardRefreshSectionIDs() []string {
	return append([]string(nil), buildProjectDashboardSections().RefreshIDs...)
}

func buildProjectDashboardGitHubFlow(board project.Board, activity []project.Activity) []projectDashboardGitHubFlowRow {
	rows := make([]projectDashboardGitHubFlowRow, 0, len(board.Tasks))
	statusByTaskAndKind := map[string]map[string]string{}
	for _, item := range activity {
		if strings.TrimSpace(item.TaskID) == "" || strings.TrimSpace(item.Kind) == "" {
			continue
		}
		kindMap, ok := statusByTaskAndKind[item.TaskID]
		if !ok {
			kindMap = map[string]string{}
			statusByTaskAndKind[item.TaskID] = kindMap
		}
		if _, exists := kindMap[item.Kind]; exists {
			continue
		}
		kindMap[item.Kind] = strings.TrimSpace(item.Status)
	}
	for _, task := range board.Tasks {
		kindMap := statusByTaskAndKind[task.ID]
		rows = append(rows, projectDashboardGitHubFlowRow{
			Task:             task.Title,
			Issue:            task.Issue,
			Branch:           task.Branch,
			PR:               task.PR,
			ReviewApprovedBy: task.ReviewApprovedBy,
			TestStatus:       kindMap[project.ActivityKindTestStatus],
			BuildStatus:      kindMap[project.ActivityKindBuildStatus],
			IssueStatus:      kindMap[project.ActivityKindIssueStatus],
			BranchStatus:     kindMap[project.ActivityKindBranchStatus],
			PRStatus:         kindMap[project.ActivityKindPRStatus],
		})
	}
	return rows
}

func buildProjectDashboardWorkerReports(board project.Board, activity []project.Activity) []projectDashboardWorkerReport {
	taskTitles := map[string]string{}
	for _, task := range board.Tasks {
		taskTitles[strings.TrimSpace(task.ID)] = strings.TrimSpace(task.Title)
	}
	rows := make([]projectDashboardWorkerReport, 0)
	for _, item := range activity {
		if item.Kind != project.ActivityKindAgentReport {
			continue
		}
		rows = append(rows, projectDashboardWorkerReport{
			Task:      dashboardFirstNonEmpty(taskTitles[strings.TrimSpace(item.TaskID)], strings.TrimSpace(item.TaskID), "unknown task"),
			Agent:     strings.TrimSpace(item.Agent),
			Status:    strings.TrimSpace(item.Status),
			Message:   strings.TrimSpace(item.Message),
			Notes:     strings.TrimSpace(item.Meta["notes"]),
			RunID:     strings.TrimSpace(item.Meta["run_id"]),
			Timestamp: strings.TrimSpace(item.Timestamp),
		})
	}
	return rows
}

func buildProjectDashboardPMItems(board project.Board, activity []project.Activity, kind string) []projectDashboardPMItem {
	taskTitles := map[string]string{}
	for _, task := range board.Tasks {
		taskTitles[strings.TrimSpace(task.ID)] = strings.TrimSpace(task.Title)
	}
	rows := make([]projectDashboardPMItem, 0)
	for _, item := range activity {
		if item.Kind != kind {
			continue
		}
		rows = append(rows, projectDashboardPMItem{
			Task:      dashboardFirstNonEmpty(taskTitles[strings.TrimSpace(item.TaskID)], strings.TrimSpace(item.TaskID)),
			Status:    strings.TrimSpace(item.Status),
			Message:   strings.TrimSpace(item.Message),
			Timestamp: strings.TrimSpace(item.Timestamp),
		})
	}
	return rows
}

func dashboardFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func serveProjectDashboardStream(w http.ResponseWriter, r *http.Request, projectID string, broker *projectDashboardBroker, logger zerolog.Logger) {
	if broker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard broker is not configured"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, unsubscribe := broker.subscribe()
	defer unsubscribe()

	writeEvent := func(evt projectDashboardEvent) error {
		payload, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	_ = writeEvent(newProjectDashboardEvent(projectID, "connected"))

	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, "data: {\"type\":\"%s\"}\n\n", keepaliveEventType); err != nil {
				return
			}
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if evt.ProjectID != projectID {
				continue
			}
			if err := writeEvent(evt); err != nil {
				logger.Debug().Err(err).Msg("dashboard stream write failed")
				return
			}
		}
	}
}

func isProjectDashboardListPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/dashboards", "/dashboards/":
		return true
	default:
		return false
	}
}

func parseProjectDashboardPath(path string) (projectDashboardRoute, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(path, "/ui/projects/"))
	if trimmed == "" {
		return projectDashboardRoute{}, false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 {
		projectID := strings.TrimSpace(parts[0])
		if projectID == "" {
			return projectDashboardRoute{}, false
		}
		return projectDashboardRoute{ProjectID: projectID}, true
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) == "stream" {
		projectID := strings.TrimSpace(parts[0])
		if projectID == "" {
			return projectDashboardRoute{}, false
		}
		return projectDashboardRoute{ProjectID: projectID, Stream: true}, true
	}
	return projectDashboardRoute{}, false
}
