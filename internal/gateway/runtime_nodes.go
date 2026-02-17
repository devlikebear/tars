package gateway

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (r *Runtime) Nodes() []NodeInfo {
	return defaultNodes()
}

func (r *Runtime) NodeDescribe(name string) (NodeInfo, error) {
	key := strings.TrimSpace(name)
	if key == "" {
		return NodeInfo{}, fmt.Errorf("name is required")
	}
	for _, node := range defaultNodes() {
		if node.Name == key {
			return node, nil
		}
	}
	return NodeInfo{}, fmt.Errorf("node not found: %s", key)
}

func (r *Runtime) NodeInvoke(name string, args map[string]any) (map[string]any, error) {
	key := strings.TrimSpace(name)
	switch key {
	case "echo":
		return map[string]any{"node": key, "output": args}, nil
	case "clock.now":
		return map[string]any{"node": key, "now": r.nowFn().UTC().Format(time.RFC3339)}, nil
	case "sessions.latest":
		if r.opts.SessionStore == nil {
			return nil, fmt.Errorf("session store is not configured")
		}
		latest, err := r.opts.SessionStore.Latest()
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"node":       key,
			"session_id": latest.ID,
			"title":      latest.Title,
			"updated_at": latest.UpdatedAt.UTC().Format(time.RFC3339),
		}, nil
	default:
		return nil, fmt.Errorf("node not found: %s", key)
	}
}

func defaultNodes() []NodeInfo {
	nodes := []NodeInfo{
		{Name: "echo", Description: "Return given input payload."},
		{Name: "clock.now", Description: "Return current UTC timestamp."},
		{Name: "sessions.latest", Description: "Return latest session metadata."},
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}
