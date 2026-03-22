package tarsserver

import (
	"encoding/json"
	"fmt"
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
