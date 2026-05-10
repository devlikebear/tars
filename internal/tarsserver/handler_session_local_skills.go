package tarsserver

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

// localSkillItem describes a session-local skill or command surfaced for
// the Skill Inbox Session tab.
type localSkillItem struct {
	Name                  string `json:"name"`
	Slash                 string `json:"slash,omitempty"`
	Description           string `json:"description,omitempty"`
	Kind                  string `json:"kind"` // "skill" | "command"
	FilePath              string `json:"file_path"`
	HasWorkspaceCollision bool   `json:"has_workspace_collision"`
}

type localSkillCounts struct {
	Skills   int `json:"skills"`
	Commands int `json:"commands"`
}

type localSkillListResponse struct {
	Cwd    string           `json:"cwd"`
	Items  []localSkillItem `json:"items"`
	Counts localSkillCounts `json:"counts"`
}

type localSkillPromoteItem struct {
	Name string `json:"name"`
}

type localSkillPromoteRequest struct {
	Items      []localSkillPromoteItem `json:"items"`
	Mode       string                  `json:"mode"`
	OnConflict string                  `json:"on_conflict"`
}

type localSkillPromoteOutcome struct {
	skill.PromoteResult
	Error string `json:"error,omitempty"`
}

type localSkillPromoteResponse struct {
	Promoted []localSkillPromoteOutcome `json:"promoted"`
	Failed   []localSkillPromoteOutcome `json:"failed"`
}

// listLocalSkills enumerates `<cwd>/.tars/skills/` and reports collision
// against the workspace skills root. Commands under `.tars/commands/`
// are intentionally not surfaced in v1; they remain visible in the
// Session Config panel and are not promotable.
func listLocalSkills(cwd, workspaceSkillsRoot string) []localSkillItem {
	out := make([]localSkillItem, 0)
	if strings.TrimSpace(cwd) == "" {
		return out
	}
	skillsRoot := filepath.Join(filepath.Clean(cwd), ".tars", "skills")
	snapshot, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Source: skill.SourceSessionCwd, Dir: skillsRoot}},
	})
	if err != nil {
		return out
	}
	for _, def := range snapshot.Skills {
		out = append(out, localSkillItem{
			Name:                  def.Name,
			Slash:                 def.Slash,
			Description:           def.Description,
			Kind:                  "skill",
			FilePath:              def.FilePath,
			HasWorkspaceCollision: workspaceSkillExists(workspaceSkillsRoot, def.Name),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func workspaceSkillExists(root, name string) bool {
	if !filepath.IsLocal(name) {
		return false
	}
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

func sessionActiveCwdOrError(w http.ResponseWriter, reqStore *session.Store, sessionID string) (string, bool) {
	current, err := reqStore.GetCurrentDir(sessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return "", false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return "", false
	}
	return strings.TrimSpace(current), true
}

func computeLocalSkillCounts(items []localSkillItem) localSkillCounts {
	var c localSkillCounts
	for _, item := range items {
		if item.Kind == "command" {
			c.Commands++
		} else {
			c.Skills++
		}
	}
	return c
}

// handleLocalSkillsList serves GET /v1/admin/sessions/{id}/local-skills.
func handleLocalSkillsList(w http.ResponseWriter, _ *http.Request, reqStore *session.Store, sessionID, workspaceDir string) {
	cwd, ok := sessionActiveCwdOrError(w, reqStore, sessionID)
	if !ok {
		return
	}
	items := listLocalSkills(cwd, filepath.Join(workspaceDir, "skills"))
	writeJSON(w, http.StatusOK, localSkillListResponse{
		Cwd:    cwd,
		Items:  items,
		Counts: computeLocalSkillCounts(items),
	})
}

func resolvePromoteMode(raw string) (skill.PromoteMode, bool) {
	mode := skill.PromoteMode(strings.ToLower(strings.TrimSpace(raw)))
	if mode == "" {
		return skill.PromoteModeCopy, true
	}
	if mode == skill.PromoteModeCopy || mode == skill.PromoteModeMove {
		return mode, true
	}
	return "", false
}

func resolvePromotePolicy(raw string) (skill.PromoteConflictPolicy, bool) {
	policy := skill.PromoteConflictPolicy(strings.ToLower(strings.TrimSpace(raw)))
	if policy == "" {
		return skill.PromoteOnConflictRename, true
	}
	if policy == skill.PromoteOnConflictRename || policy == skill.PromoteOnConflictOverwrite || policy == skill.PromoteOnConflictAbort {
		return policy, true
	}
	return "", false
}

// handleLocalSkillsPromote serves POST /v1/admin/sessions/{id}/local-skills/promote.
func handleLocalSkillsPromote(w http.ResponseWriter, r *http.Request, reqStore *session.Store, sessionID, workspaceDir string, provider extensionsProvider, logger zerolog.Logger) {
	var req localSkillPromoteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items is required"})
		return
	}
	mode, ok := resolvePromoteMode(req.Mode)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be 'copy' or 'move'"})
		return
	}
	policy, ok := resolvePromotePolicy(req.OnConflict)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "on_conflict must be 'rename', 'overwrite', or 'abort'"})
		return
	}
	cwd, ok := sessionActiveCwdOrError(w, reqStore, sessionID)
	if !ok {
		return
	}
	if cwd == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no active cwd"})
		return
	}
	workspaceSkillsRoot := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(workspaceSkillsRoot, 0o755); err != nil {
		logger.Error().Err(err).Msg("ensure workspace skills root failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ensure workspace skills root failed"})
		return
	}

	resp := localSkillPromoteResponse{
		Promoted: make([]localSkillPromoteOutcome, 0, len(req.Items)),
		Failed:   make([]localSkillPromoteOutcome, 0),
	}
	sessionSkillsRoot := filepath.Join(cwd, ".tars", "skills")
	for _, item := range req.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			resp.Failed = append(resp.Failed, localSkillPromoteOutcome{Error: "name is required"})
			continue
		}
		result, err := skill.Promote(skill.PromoteRequest{
			SourceSkillsRoot: sessionSkillsRoot,
			TargetSkillsRoot: workspaceSkillsRoot,
			Name:             name,
			Mode:             mode,
			OnConflict:       policy,
		})
		if err != nil {
			resp.Failed = append(resp.Failed, localSkillPromoteOutcome{
				PromoteResult: skill.PromoteResult{RequestedName: name},
				Error:         err.Error(),
			})
			continue
		}
		resp.Promoted = append(resp.Promoted, localSkillPromoteOutcome{PromoteResult: result})
	}

	if len(resp.Promoted) > 0 && provider != nil {
		if reloadErr := provider.Reload(r.Context()); reloadErr != nil {
			logger.Warn().Err(reloadErr).Msg("reload extensions after local skill promote failed")
		}
	}
	status := http.StatusOK
	if len(resp.Promoted) == 0 && len(resp.Failed) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, resp)
}

// localSkillsHandlerDeps captures closure dependencies for the admin
// /v1/admin/sessions/:id/local-skills* routes.
type localSkillsHandlerDeps struct {
	provider     extensionsProvider
	workspaceDir string
}

// handleLocalSkillsDispatch is invoked from the admin sessions switch
// when pathParts[1] == "local-skills".
func handleLocalSkillsDispatch(w http.ResponseWriter, r *http.Request, reqStore *session.Store, sessionID string, pathParts []string, deps localSkillsHandlerDeps, logger zerolog.Logger) bool {
	if len(pathParts) == 2 {
		if !requireMethod(w, r, http.MethodGet) {
			return true
		}
		handleLocalSkillsList(w, r, reqStore, sessionID, deps.workspaceDir)
		return true
	}
	if len(pathParts) == 3 && pathParts[2] == "promote" {
		if !requireMethod(w, r, http.MethodPost) {
			return true
		}
		handleLocalSkillsPromote(w, r, reqStore, sessionID, deps.workspaceDir, deps.provider, logger)
		return true
	}
	return false
}
