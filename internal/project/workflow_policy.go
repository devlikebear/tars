package project

import "strings"

type WorkflowPolicy struct{}

var DefaultWorkflowPolicy WorkflowPolicy

func (WorkflowPolicy) NormalizeBriefStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "collecting", "ready", "finalized":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "collecting"
	}
}

func (p WorkflowPolicy) HasActiveBriefStatus(status string) bool {
	switch p.NormalizeBriefStatus(status) {
	case "collecting", "ready":
		return true
	default:
		return false
	}
}

func (WorkflowPolicy) NormalizeProjectStatePhase(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "planning", "drafting", "executing", "reviewing", "blocked", "done":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "planning"
	}
}

func (WorkflowPolicy) NormalizeProjectStateStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "active", "paused", "blocked", "done":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "active"
	}
}

func (p WorkflowPolicy) DefaultProjectNextAction(brief Brief) string {
	if len(brief.OpenQuestions) > 0 {
		return strings.TrimSpace(brief.OpenQuestions[0])
	}
	if isNarrativeBriefKind(brief.Kind) {
		return "Review STORY_BIBLE.md, CHARACTERS.md, and PLOT.md, then update STATE.md with the first writing milestone."
	}
	return "Review project instructions and define the first executable milestone in STATE.md."
}

func (WorkflowPolicy) IsKickoffMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" || strings.HasPrefix(lower, "/") {
		return false
	}
	projectHints := []string{"project", "프로젝트"}
	actionHints := []string{"start", "시작", "만들", "build", "create", "개발", "구축"}
	hasProject := false
	for _, hint := range projectHints {
		if strings.Contains(lower, hint) {
			hasProject = true
			break
		}
	}
	if !hasProject {
		return false
	}
	for _, hint := range actionHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func (WorkflowPolicy) NormalizeDispatchStage(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "todo":
		return "todo", true
	case "review":
		return "review", true
	default:
		return "", false
	}
}

func (WorkflowPolicy) FilterDispatchableTasks(status string, tasks []BoardTask) []BoardTask {
	if strings.TrimSpace(status) != "todo" || len(tasks) <= 1 {
		return tasks
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == "pm-seed-bootstrap" {
			return []BoardTask{task}
		}
	}
	return tasks
}

func (WorkflowPolicy) BoardHasStatus(board Board, status string) bool {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return true
		}
	}
	return false
}

func (WorkflowPolicy) FirstTaskByStatus(board Board, status string) (BoardTask, bool) {
	for _, task := range board.Tasks {
		if task.Status == strings.TrimSpace(status) {
			return task, true
		}
	}
	return BoardTask{}, false
}

func (WorkflowPolicy) AllBoardTasksDone(board Board) bool {
	if len(board.Tasks) == 0 {
		return true
	}
	for _, task := range board.Tasks {
		if task.Status != "done" {
			return false
		}
	}
	return true
}

func (p WorkflowPolicy) ShouldAutopilotRun(item Project, board Board, state *ProjectState) bool {
	if strings.TrimSpace(item.Status) == "archived" {
		return false
	}
	if len(board.Tasks) == 0 {
		return true
	}
	if !p.AllBoardTasksDone(board) {
		return true
	}
	if state == nil {
		return true
	}
	return p.NormalizeProjectStateStatus(state.Status) != "done"
}
