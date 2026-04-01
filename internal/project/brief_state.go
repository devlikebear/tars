package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/session"
	"gopkg.in/yaml.v3"
)

const (
	projectStateDocumentName = "STATE.md"
	projectBriefDocumentName = "BRIEF.md"
)

type Brief struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title,omitempty"`
	Goal               string   `json:"goal,omitempty"`
	Kind               string   `json:"kind,omitempty"`
	Genre              string   `json:"genre,omitempty"`
	TargetLength       string   `json:"target_length,omitempty"`
	Cadence            string   `json:"cadence,omitempty"`
	TargetInstallments string   `json:"target_installments,omitempty"`
	Premise            string   `json:"premise,omitempty"`
	PlotSeed           string   `json:"plot_seed,omitempty"`
	StylePreferences   string   `json:"style_preferences,omitempty"`
	Constraints        []string `json:"constraints,omitempty"`
	MustHave           []string `json:"must_have,omitempty"`
	MustAvoid          []string `json:"must_avoid,omitempty"`
	OpenQuestions      []string `json:"open_questions,omitempty"`
	Decisions          []string `json:"decisions,omitempty"`
	Status             string   `json:"status,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Body               string   `json:"body,omitempty"`
	Path               string   `json:"path,omitempty"`
}

var briefFinalizeLocks sync.Map

type BriefUpdateInput struct {
	Title              *string
	Goal               *string
	Kind               *string
	Genre              *string
	TargetLength       *string
	Cadence            *string
	TargetInstallments *string
	Premise            *string
	PlotSeed           *string
	StylePreferences   *string
	Constraints        []string
	MustHave           []string
	MustAvoid          []string
	OpenQuestions      []string
	Decisions          []string
	Status             *string
	Summary            *string
	Body               *string
}

type ProjectState struct {
	ProjectID         string   `json:"project_id"`
	Goal              string   `json:"goal,omitempty"`
	Phase             string   `json:"phase,omitempty"`
	Status            string   `json:"status,omitempty"`
	PhaseNumber       int      `json:"phase_number,omitempty"`
	NextAction        string   `json:"next_action,omitempty"`
	RemainingTasks    []string `json:"remaining_tasks,omitempty"`
	CompletionSummary string   `json:"completion_summary,omitempty"`
	LastRunSummary    string   `json:"last_run_summary,omitempty"`
	LastRunAt         string   `json:"last_run_at,omitempty"`
	StopReason        string   `json:"stop_reason,omitempty"`
	Body              string   `json:"body,omitempty"`
	Path              string   `json:"path,omitempty"`
}

type ProjectStateUpdateInput struct {
	Goal              *string
	Phase             *string
	Status            *string
	PhaseNumber       *int
	NextAction        *string
	RemainingTasks    []string
	CompletionSummary *string
	LastRunSummary    *string
	LastRunAt         *string
	StopReason        *string
	Body              *string
}

func (s *Store) BriefPath(id string) string {
	return filepath.Join(s.workspaceDir, "_shared", "project_briefs", strings.TrimSpace(id), projectBriefDocumentName)
}

func (s *Store) StatePath(projectID string) string {
	return filepath.Join(s.workspaceDir, "projects", strings.TrimSpace(projectID), projectStateDocumentName)
}

func (s *Store) ProjectFilePath(projectID, name string) string {
	return filepath.Join(s.workspaceDir, "projects", strings.TrimSpace(projectID), strings.TrimSpace(name))
}

func (s *Store) GetBrief(id string) (Brief, error) {
	if s == nil {
		return Brief{}, fmt.Errorf("project store is nil")
	}
	briefID := strings.TrimSpace(id)
	if briefID == "" {
		return Brief{}, fmt.Errorf("brief id is required")
	}
	raw, err := os.ReadFile(s.BriefPath(briefID))
	if err != nil {
		if os.IsNotExist(err) {
			return Brief{}, fmt.Errorf("brief not found: %s", briefID)
		}
		return Brief{}, err
	}
	item, err := parseBriefDocument(string(raw))
	if err != nil {
		return Brief{}, err
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = briefID
	}
	item.Path = filepath.Dir(s.BriefPath(briefID))
	return item, nil
}

func (s *Store) UpdateBrief(id string, input BriefUpdateInput) (Brief, error) {
	if s == nil {
		return Brief{}, fmt.Errorf("project store is nil")
	}
	briefID := strings.TrimSpace(id)
	if briefID == "" {
		return Brief{}, fmt.Errorf("brief id is required")
	}
	item, err := s.GetBrief(briefID)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "brief not found") {
			return Brief{}, err
		}
		item = Brief{ID: briefID, Status: "collecting"}
	}
	applyBriefUpdateInput(&item, input)
	item.ID = briefID
	item.Status = normalizeBriefStatus(item.Status)
	item.Kind = normalizeBriefKind(item.Kind)
	if err := s.writeBrief(item); err != nil {
		return Brief{}, err
	}
	return s.GetBrief(briefID)
}

func (s *Store) GetState(projectID string) (ProjectState, error) {
	if s == nil {
		return ProjectState{}, fmt.Errorf("project store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectState{}, fmt.Errorf("project id is required")
	}
	if _, err := s.Get(projectID); err != nil {
		return ProjectState{}, err
	}
	raw, err := os.ReadFile(s.StatePath(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectState{}, fmt.Errorf("project state not found: %s", projectID)
		}
		return ProjectState{}, err
	}
	item, err := parseProjectStateDocument(string(raw))
	if err != nil {
		return ProjectState{}, err
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		item.ProjectID = projectID
	}
	item.Path = filepath.Dir(s.StatePath(projectID))
	return item, nil
}

func (s *Store) UpdateState(projectID string, input ProjectStateUpdateInput) (ProjectState, error) {
	if s == nil {
		return ProjectState{}, fmt.Errorf("project store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectState{}, fmt.Errorf("project id is required")
	}
	item, err := s.GetState(projectID)
	hadState := err == nil
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "project state not found") {
			return ProjectState{}, err
		}
		if _, getErr := s.Get(projectID); getErr != nil {
			return ProjectState{}, getErr
		}
		item = DefaultWorkflowPolicy.DefaultProjectState(projectID)
	}
	before := item
	applyProjectStateUpdateInput(&item, input)
	item.ProjectID = projectID
	item.Phase = normalizeProjectStatePhase(item.Phase)
	item.Status = normalizeProjectStateStatus(item.Status)
	if err := s.writeState(item); err != nil {
		return ProjectState{}, err
	}
	updated, err := s.GetState(projectID)
	if err != nil {
		return ProjectState{}, err
	}
	if hadState && !projectStateActivityChanged(before, updated) {
		return updated, nil
	}
	message := "Project state updated"
	if !hadState {
		message = "Project state initialized"
	}
	if err := s.appendSystemActivity(projectID, ActivityAppendInput{
		Kind:    ActivityKindStateChanged,
		Status:  updated.Status,
		Message: message,
		Meta: map[string]string{
			"phase":       updated.Phase,
			"next_action": updated.NextAction,
		},
	}); err != nil {
		return ProjectState{}, err
	}
	return updated, nil
}

func (s *Store) FinalizeBrief(id string, sessionStore *session.Store) (Project, Brief, error) {
	if s == nil {
		return Project{}, Brief{}, fmt.Errorf("project store is nil")
	}
	unlock := lockBriefFinalize(s.workspaceDir, id)
	defer unlock()
	brief, err := s.GetBrief(id)
	if err != nil {
		return Project{}, Brief{}, err
	}
	if normalizeBriefStatus(brief.Status) == "finalized" {
		return Project{}, Brief{}, fmt.Errorf("brief already finalized: %s", strings.TrimSpace(id))
	}
	projectName := strings.TrimSpace(brief.Title)
	if projectName == "" {
		projectName = strings.TrimSpace(brief.Goal)
	}
	if projectName == "" {
		projectName = "Untitled Project"
	}
	created, err := s.Create(CreateInput{
		Name:         projectName,
		Objective:    strings.TrimSpace(brief.Goal),
		Instructions: buildProjectInstructionsFromBrief(brief),
	})
	if err != nil {
		return Project{}, Brief{}, err
	}
	initial := DefaultWorkflowPolicy.InitialProjectState(brief).stateInput()
	initial.Goal = stringValuePtr(strings.TrimSpace(brief.Goal))
	initial.RemainingTasks = append([]string(nil), brief.OpenQuestions...)
	_, err = s.UpdateState(created.ID, initial)
	if err != nil {
		return Project{}, Brief{}, err
	}
	if isNarrativeBriefKind(brief.Kind) {
		if err := s.seedNarrativeProjectDocs(created.ID, brief); err != nil {
			return Project{}, Brief{}, err
		}
	}
	if sessionStore != nil {
		sess, err := sessionStore.Create(projectName)
		if err == nil {
			_ = sessionStore.SetProjectID(sess.ID, created.ID)
			created.SessionID = sess.ID
			_, _ = s.Update(created.ID, UpdateInput{SessionID: &sess.ID})
		}
	}
	finalizedStatus := "finalized"
	finalized, err := s.UpdateBrief(id, BriefUpdateInput{Status: &finalizedStatus})
	if err != nil {
		return Project{}, Brief{}, err
	}
	return created, finalized, nil
}

func lockBriefFinalize(workspaceDir, id string) func() {
	key := strings.TrimSpace(workspaceDir) + "::" + strings.TrimSpace(id)
	actual, _ := briefFinalizeLocks.LoadOrStore(key, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Store) writeBrief(item Brief) error {
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		return fmt.Errorf("brief id is required")
	}
	item.Kind = normalizeBriefKind(item.Kind)
	item.Status = normalizeBriefStatus(item.Status)
	dir := filepath.Dir(s.BriefPath(item.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.BriefPath(item.ID), []byte(buildBriefDocument(item)), 0o644)
}

func (s *Store) writeState(item ProjectState) error {
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	if item.ProjectID == "" {
		return fmt.Errorf("project id is required")
	}
	item.Phase = normalizeProjectStatePhase(item.Phase)
	item.Status = normalizeProjectStateStatus(item.Status)
	dir := filepath.Dir(s.StatePath(item.ProjectID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.StatePath(item.ProjectID), []byte(buildProjectStateDocument(item)), 0o644)
}

func (s *Store) writeProjectAuxFile(projectID, name, content string) error {
	path := s.ProjectFilePath(projectID, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644)
}

func (s *Store) seedNarrativeProjectDocs(projectID string, brief Brief) error {
	if err := s.writeProjectAuxFile(projectID, "STORY_BIBLE.md", buildStoryBible(brief)); err != nil {
		return err
	}
	if err := s.writeProjectAuxFile(projectID, "CHARACTERS.md", buildCharactersSkeleton(brief)); err != nil {
		return err
	}
	return s.writeProjectAuxFile(projectID, "PLOT.md", buildPlotSkeleton(brief))
}

func parseBriefDocument(raw string) (Brief, error) {
	metaRaw, body, hasMeta, err := splitFrontmatter(raw)
	if err != nil {
		return Brief{}, err
	}
	if !hasMeta {
		return Brief{Status: "collecting", Kind: "other", Body: strings.TrimSpace(raw)}, nil
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(metaRaw), &parsed); err != nil {
		return Brief{}, fmt.Errorf("parse brief frontmatter: %w", err)
	}
	item := Brief{
		ID:                 mapString(parsed, "id"),
		Title:              mapString(parsed, "title"),
		Goal:               mapString(parsed, "goal"),
		Kind:               normalizeBriefKind(mapString(parsed, "kind")),
		Genre:              mapString(parsed, "genre"),
		TargetLength:       mapString(parsed, "target_length", "target-length"),
		Cadence:            mapString(parsed, "cadence"),
		TargetInstallments: mapString(parsed, "target_installments", "target-installments"),
		Premise:            mapString(parsed, "premise"),
		PlotSeed:           mapString(parsed, "plot_seed", "plot-seed"),
		StylePreferences:   mapString(parsed, "style_preferences", "style-preferences"),
		Constraints:        mapStringList(parsed, "constraints"),
		MustHave:           mapStringList(parsed, "must_have", "must-have"),
		MustAvoid:          mapStringList(parsed, "must_avoid", "must-avoid"),
		OpenQuestions:      mapStringList(parsed, "open_questions", "open-questions"),
		Decisions:          mapStringList(parsed, "decisions"),
		Status:             normalizeBriefStatus(mapString(parsed, "status")),
		Summary:            mapString(parsed, "summary"),
		Body:               strings.TrimSpace(body),
	}
	return item, nil
}

func buildBriefDocument(item Brief) string {
	var b strings.Builder
	b.WriteString("---\n")
	_, _ = fmt.Fprintf(&b, "id: %s\n", quoteYAML(strings.TrimSpace(item.ID)))
	if v := strings.TrimSpace(item.Title); v != "" {
		_, _ = fmt.Fprintf(&b, "title: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(item.Goal); v != "" {
		_, _ = fmt.Fprintf(&b, "goal: %s\n", quoteYAML(v))
	}
	_, _ = fmt.Fprintf(&b, "kind: %s\n", quoteYAML(normalizeBriefKind(item.Kind)))
	_, _ = fmt.Fprintf(&b, "status: %s\n", quoteYAML(normalizeBriefStatus(item.Status)))
	writeOptionalYAMLField(&b, "genre", item.Genre)
	writeOptionalYAMLField(&b, "target_length", item.TargetLength)
	writeOptionalYAMLField(&b, "cadence", item.Cadence)
	writeOptionalYAMLField(&b, "target_installments", item.TargetInstallments)
	writeOptionalYAMLField(&b, "premise", item.Premise)
	writeOptionalYAMLField(&b, "plot_seed", item.PlotSeed)
	writeOptionalYAMLField(&b, "style_preferences", item.StylePreferences)
	writeDocumentList(&b, "constraints", item.Constraints)
	writeDocumentList(&b, "must_have", item.MustHave)
	writeDocumentList(&b, "must_avoid", item.MustAvoid)
	writeDocumentList(&b, "open_questions", item.OpenQuestions)
	writeDocumentList(&b, "decisions", item.Decisions)
	writeOptionalYAMLField(&b, "summary", item.Summary)
	b.WriteString("---\n")
	if body := strings.TrimSpace(item.Body); body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func parseProjectStateDocument(raw string) (ProjectState, error) {
	metaRaw, body, hasMeta, err := splitFrontmatter(raw)
	if err != nil {
		return ProjectState{}, err
	}
	if !hasMeta {
		item := DefaultWorkflowPolicy.DefaultProjectState("")
		item.Body = strings.TrimSpace(raw)
		return item, nil
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(metaRaw), &parsed); err != nil {
		return ProjectState{}, fmt.Errorf("parse state frontmatter: %w", err)
	}
	return ProjectState{
		ProjectID:         mapString(parsed, "project_id", "project-id"),
		Goal:              mapString(parsed, "goal"),
		Phase:             normalizeProjectStatePhase(mapString(parsed, "phase")),
		Status:            normalizeProjectStateStatus(mapString(parsed, "status")),
		PhaseNumber:       mapInt(parsed, "phase_number"),
		NextAction:        mapString(parsed, "next_action", "next-action"),
		RemainingTasks:    mapStringList(parsed, "remaining_tasks", "remaining-tasks"),
		CompletionSummary: mapString(parsed, "completion_summary", "completion-summary"),
		LastRunSummary:    mapString(parsed, "last_run_summary", "last-run-summary"),
		LastRunAt:         mapString(parsed, "last_run_at", "last-run-at"),
		StopReason:        mapString(parsed, "stop_reason", "stop-reason"),
		Body:              strings.TrimSpace(body),
	}, nil
}

func buildProjectStateDocument(item ProjectState) string {
	var b strings.Builder
	b.WriteString("---\n")
	_, _ = fmt.Fprintf(&b, "project_id: %s\n", quoteYAML(strings.TrimSpace(item.ProjectID)))
	_, _ = fmt.Fprintf(&b, "phase: %s\n", quoteYAML(normalizeProjectStatePhase(item.Phase)))
	_, _ = fmt.Fprintf(&b, "status: %s\n", quoteYAML(normalizeProjectStateStatus(item.Status)))
	if item.PhaseNumber > 0 {
		_, _ = fmt.Fprintf(&b, "phase_number: %d\n", item.PhaseNumber)
	}
	writeOptionalYAMLField(&b, "goal", item.Goal)
	writeOptionalYAMLField(&b, "next_action", item.NextAction)
	writeDocumentList(&b, "remaining_tasks", item.RemainingTasks)
	writeOptionalYAMLField(&b, "completion_summary", item.CompletionSummary)
	writeOptionalYAMLField(&b, "last_run_summary", item.LastRunSummary)
	writeOptionalYAMLField(&b, "last_run_at", item.LastRunAt)
	writeOptionalYAMLField(&b, "stop_reason", item.StopReason)
	b.WriteString("---\n")
	if body := strings.TrimSpace(item.Body); body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func applyBriefUpdateInput(item *Brief, input BriefUpdateInput) {
	if item == nil {
		return
	}
	assignStringPtr(&item.Title, input.Title)
	assignStringPtr(&item.Goal, input.Goal)
	if input.Kind != nil {
		item.Kind = normalizeBriefKind(*input.Kind)
	}
	assignStringPtr(&item.Genre, input.Genre)
	assignStringPtr(&item.TargetLength, input.TargetLength)
	assignStringPtr(&item.Cadence, input.Cadence)
	assignStringPtr(&item.TargetInstallments, input.TargetInstallments)
	assignStringPtr(&item.Premise, input.Premise)
	assignStringPtr(&item.PlotSeed, input.PlotSeed)
	assignStringPtr(&item.StylePreferences, input.StylePreferences)
	if len(input.Constraints) > 0 {
		item.Constraints = normalizeList(input.Constraints)
	}
	if len(input.MustHave) > 0 {
		item.MustHave = normalizeList(input.MustHave)
	}
	if len(input.MustAvoid) > 0 {
		item.MustAvoid = normalizeList(input.MustAvoid)
	}
	if len(input.OpenQuestions) > 0 {
		item.OpenQuestions = normalizeList(input.OpenQuestions)
	}
	if len(input.Decisions) > 0 {
		item.Decisions = normalizeList(input.Decisions)
	}
	if input.Status != nil {
		item.Status = normalizeBriefStatus(*input.Status)
	}
	assignStringPtr(&item.Summary, input.Summary)
	assignStringPtr(&item.Body, input.Body)
}

func applyProjectStateUpdateInput(item *ProjectState, input ProjectStateUpdateInput) {
	if item == nil {
		return
	}
	assignStringPtr(&item.Goal, input.Goal)
	if input.Phase != nil {
		item.Phase = normalizeProjectStatePhase(*input.Phase)
	}
	if input.Status != nil {
		item.Status = normalizeProjectStateStatus(*input.Status)
	}
	if input.PhaseNumber != nil {
		item.PhaseNumber = *input.PhaseNumber
	}
	assignStringPtr(&item.NextAction, input.NextAction)
	if len(input.RemainingTasks) > 0 {
		item.RemainingTasks = normalizeList(input.RemainingTasks)
	}
	assignStringPtr(&item.CompletionSummary, input.CompletionSummary)
	assignStringPtr(&item.LastRunSummary, input.LastRunSummary)
	assignStringPtr(&item.LastRunAt, input.LastRunAt)
	assignStringPtr(&item.StopReason, input.StopReason)
	assignStringPtr(&item.Body, input.Body)
}

func assignStringPtr(target *string, value *string) {
	if target == nil || value == nil {
		return
	}
	*target = strings.TrimSpace(*value)
}

func stringValuePtr(value string) *string {
	v := strings.TrimSpace(value)
	return &v
}

func writeOptionalYAMLField(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	_, _ = fmt.Fprintf(b, "%s: %s\n", key, quoteYAML(strings.TrimSpace(value)))
}

func normalizeBriefKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "novel", "serial", "research", "other":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "other"
	}
}

func normalizeBriefStatus(raw string) string {
	return DefaultWorkflowPolicy.NormalizeBriefStatus(raw)
}

func normalizeProjectStatePhase(raw string) string {
	return DefaultWorkflowPolicy.NormalizeProjectStatePhase(raw)
}

func normalizeProjectStateStatus(raw string) string {
	return DefaultWorkflowPolicy.NormalizeProjectStateStatus(raw)
}

func buildProjectInstructionsFromBrief(brief Brief) string {
	parts := make([]string, 0, 8)
	if v := strings.TrimSpace(brief.Summary); v != "" {
		parts = append(parts, "Summary: "+v)
	}
	if v := strings.TrimSpace(brief.Premise); v != "" {
		parts = append(parts, "Premise: "+v)
	}
	if v := strings.TrimSpace(brief.PlotSeed); v != "" {
		parts = append(parts, "Plot Seed: "+v)
	}
	if v := strings.TrimSpace(brief.StylePreferences); v != "" {
		parts = append(parts, "Style Preferences: "+v)
	}
	if len(brief.Constraints) > 0 {
		parts = append(parts, "Constraints: "+strings.Join(brief.Constraints, ", "))
	}
	if len(brief.MustHave) > 0 {
		parts = append(parts, "Must Have: "+strings.Join(brief.MustHave, ", "))
	}
	if len(brief.MustAvoid) > 0 {
		parts = append(parts, "Must Avoid: "+strings.Join(brief.MustAvoid, ", "))
	}
	if body := strings.TrimSpace(brief.Body); body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

func defaultProjectNextAction(brief Brief) string {
	return DefaultWorkflowPolicy.DefaultProjectNextAction(brief)
}

func isNarrativeBriefKind(kind string) bool {
	switch normalizeBriefKind(kind) {
	case "novel", "serial":
		return true
	default:
		return false
	}
}

func buildStoryBible(brief Brief) string {
	var b strings.Builder
	b.WriteString("# Story Bible\n\n")
	if v := strings.TrimSpace(brief.Genre); v != "" {
		b.WriteString("## Genre\n\n" + v + "\n\n")
	}
	if v := strings.TrimSpace(brief.Premise); v != "" {
		b.WriteString("## Premise\n\n" + v + "\n\n")
	}
	if v := strings.TrimSpace(brief.StylePreferences); v != "" {
		b.WriteString("## Style Preferences\n\n" + v + "\n\n")
	}
	if len(brief.Constraints) > 0 {
		b.WriteString("## Constraints\n\n")
		for _, item := range brief.Constraints {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	if len(brief.MustHave) > 0 {
		b.WriteString("## Must Have\n\n")
		for _, item := range brief.MustHave {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	if len(brief.MustAvoid) > 0 {
		b.WriteString("## Must Avoid\n\n")
		for _, item := range brief.MustAvoid {
			b.WriteString("- " + item + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func buildCharactersSkeleton(brief Brief) string {
	return strings.TrimSpace(`# Characters

## Protagonist
- Role:
- Goal:
- Conflict:
- Voice:

## Supporting Cast
- Name:
- Role:
- Relationship:
`)
}

func buildPlotSkeleton(brief Brief) string {
	var b strings.Builder
	b.WriteString("# Plot\n\n")
	if v := strings.TrimSpace(brief.PlotSeed); v != "" {
		b.WriteString("## Plot Seed\n\n" + v + "\n\n")
	}
	if v := strings.TrimSpace(brief.TargetInstallments); v != "" {
		b.WriteString("## Target Installments\n\n" + v + "\n\n")
	}
	if v := strings.TrimSpace(brief.Cadence); v != "" {
		b.WriteString("## Cadence\n\n" + v + "\n\n")
	}
	if len(brief.OpenQuestions) > 0 {
		b.WriteString("## Open Questions\n\n")
		for _, item := range brief.OpenQuestions {
			b.WriteString("- " + item + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}
