package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) GetProjectBoard(ctx context.Context, projectID string) (ProjectBoard, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ProjectBoard{}, fmt.Errorf("project id is required")
	}
	var out ProjectBoard
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(id)+"/board", nil, false, &out); err != nil {
		return ProjectBoard{}, err
	}
	return out, nil
}

func (c *Client) ListProjectActivity(ctx context.Context, projectID string, limit int) ([]ProjectActivity, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return nil, fmt.Errorf("project id is required")
	}
	path := "/v1/projects/" + url.PathEscape(id) + "/activity"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out ProjectActivityList
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, false, &out); err != nil {
		return nil, err
	}
	if out.Items == nil {
		return []ProjectActivity{}, nil
	}
	return out.Items, nil
}

func (c *Client) DispatchProject(ctx context.Context, projectID string, stage string) (ProjectDispatchReport, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ProjectDispatchReport{}, fmt.Errorf("project id is required")
	}
	var out ProjectDispatchReport
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(id)+"/dispatch", map[string]string{
		"stage": strings.TrimSpace(stage),
	}, false, &out); err != nil {
		return ProjectDispatchReport{}, err
	}
	return out, nil
}

func (c *Client) StartProjectAutopilot(ctx context.Context, projectID string) (ProjectAutopilotRun, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ProjectAutopilotRun{}, fmt.Errorf("project id is required")
	}
	var out ProjectAutopilotRun
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(id)+"/autopilot", nil, false, &out); err != nil {
		return ProjectAutopilotRun{}, err
	}
	return out, nil
}

func (c *Client) GetProjectAutopilot(ctx context.Context, projectID string) (ProjectAutopilotRun, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ProjectAutopilotRun{}, fmt.Errorf("project id is required")
	}
	var out ProjectAutopilotRun
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(id)+"/autopilot", nil, false, &out); err != nil {
		return ProjectAutopilotRun{}, err
	}
	return out, nil
}
