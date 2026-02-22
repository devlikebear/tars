package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var out []Project
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/projects", nil, false, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []Project{}, nil
	}
	return out, nil
}

func (c *Client) CreateProject(ctx context.Context, req ProjectCreateRequest) (Project, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Project{}, fmt.Errorf("name is required")
	}
	var out Project
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/projects", req, false, &out); err != nil {
		return Project{}, err
	}
	return out, nil
}

func (c *Client) GetProject(ctx context.Context, projectID string) (Project, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return Project{}, fmt.Errorf("project id is required")
	}
	var out Project
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(id), nil, false, &out); err != nil {
		return Project{}, err
	}
	return out, nil
}

func (c *Client) UpdateProject(ctx context.Context, projectID string, req ProjectUpdateRequest) (Project, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return Project{}, fmt.Errorf("project id is required")
	}
	var out Project
	if _, err := c.doJSON(ctx, http.MethodPatch, "/v1/projects/"+url.PathEscape(id), req, false, &out); err != nil {
		return Project{}, err
	}
	return out, nil
}

func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	_, err := c.doText(ctx, http.MethodDelete, "/v1/projects/"+url.PathEscape(id), nil, false)
	return err
}

func (c *Client) ActivateProject(ctx context.Context, projectID string, sessionID string) error {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	payload := map[string]string{}
	if s := strings.TrimSpace(sessionID); s != "" {
		payload["session_id"] = s
	}
	_, err := c.doText(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(id)+"/activate", payload, false)
	return err
}
