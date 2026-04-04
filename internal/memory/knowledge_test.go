package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKnowledgeStore_UpsertListGetDeleteAndGraph(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := NewKnowledgeStore(root, nil)
	now := time.Date(2026, 4, 4, 7, 30, 0, 0, time.UTC)

	coffee, err := store.Upsert(KnowledgeNote{
		Slug:          "coffee-preference",
		Title:         "Coffee Preference",
		Kind:          "preference",
		Summary:       "User prefers black coffee.",
		Body:          "Keep coffee suggestions unsweetened and direct.",
		Tags:          []string{"coffee", "preference"},
		Aliases:       []string{"black coffee"},
		SourceSession: "sess-1",
		ProjectID:     "proj-alpha",
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("upsert coffee note: %v", err)
	}

	_, err = store.Upsert(KnowledgeNote{
		Slug:      "morning-routine",
		Title:     "Morning Routine",
		Kind:      "habit",
		Summary:   "Coffee comes before meetings.",
		Body:      "Schedule intense work after the first cup.",
		Tags:      []string{"routine"},
		UpdatedAt: now.Add(time.Minute),
		Links: []KnowledgeLink{
			{Target: coffee.Slug, Relation: "depends_on"},
		},
	})
	if err != nil {
		t.Fatalf("upsert routine note: %v", err)
	}

	listed, err := store.List(KnowledgeListOptions{})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(listed))
	}
	if listed[0].Slug != "morning-routine" {
		t.Fatalf("expected newest note first, got %+v", listed)
	}

	got, err := store.Get("coffee-preference")
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if got.Title != "Coffee Preference" || got.ProjectID != "proj-alpha" {
		t.Fatalf("unexpected note: %+v", got)
	}

	graph, err := store.Graph()
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("expected 2 graph nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 graph edge, got %d", len(graph.Edges))
	}
	if graph.Edges[0].Target != "coffee-preference" || graph.Edges[0].Relation != "depends_on" {
		t.Fatalf("unexpected graph edge: %+v", graph.Edges[0])
	}

	indexPath := filepath.Join(root, "memory", "wiki", "index.md")
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	indexText := string(indexRaw)
	if !strings.Contains(indexText, "[[coffee-preference]]") || !strings.Contains(indexText, "[[morning-routine]]") {
		t.Fatalf("expected wikilinks in index, got %q", indexText)
	}

	if err := store.Delete("morning-routine"); err != nil {
		t.Fatalf("delete note: %v", err)
	}

	remaining, err := store.List(KnowledgeListOptions{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Slug != "coffee-preference" {
		t.Fatalf("unexpected remaining notes: %+v", remaining)
	}

	graph, err = store.Graph()
	if err != nil {
		t.Fatalf("graph after delete: %v", err)
	}
	if len(graph.Nodes) != 1 || len(graph.Edges) != 0 {
		t.Fatalf("expected graph cleanup after delete, got %+v", graph)
	}
}

func TestKnowledgeStore_GraphOnEmptyKnowledgeBase(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := NewKnowledgeStore(root, nil)
	graph, err := store.Graph()
	if err != nil {
		t.Fatalf("graph on empty knowledge base: %v", err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("expected empty graph, got %+v", graph)
	}

	graphRaw, err := os.ReadFile(filepath.Join(root, "memory", "wiki", "graph.json"))
	if err != nil {
		t.Fatalf("read graph.json: %v", err)
	}
	if strings.Contains(string(graphRaw), `"updated_at": ""`) {
		t.Fatalf("expected repaired graph artifact without blank updated_at, got %q", string(graphRaw))
	}
}
