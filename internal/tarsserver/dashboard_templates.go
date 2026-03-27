package tarsserver

import "html/template"

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

const projectDashboardRefreshScript = `
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
        <div class="label">Run Status</div>
        <div class="value">{{if .RunStatus}}<code>{{.RunStatus}}</code>{{else}}-{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Next Action</div>
        <div class="value">{{if .NextAction}}{{.NextAction}}{{else}}-{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Phase Note</div>
        <div class="value">{{if .PhaseNote}}{{.PhaseNote}}{{else}}-{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Pending Decision</div>
        <div class="value">{{if .PendingDecision}}{{.PendingDecision.Message}}{{else}}-{{end}}</div>
      </article>
      <article class="card">
        <div class="label">Current Blocker</div>
        <div class="value">{{if .CurrentBlocker}}{{.CurrentBlocker.Message}}{{else}}-{{end}}</div>
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
      {{if .PhaseNote}}<p class="muted">{{.PhaseNote}}</p>{{end}}
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
` + projectDashboardRefreshScript + `
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
            <div class="label">Phase Note</div>
            <div class="value">{{if .AutopilotNote}}{{.AutopilotNote}}{{else}}-{{end}}</div>
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
