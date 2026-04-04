package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultKnowledgeListLimit = 200

type KnowledgeLink struct {
	Target   string `json:"target" yaml:"target"`
	Relation string `json:"relation,omitempty" yaml:"relation,omitempty"`
}

type KnowledgeNote struct {
	Slug          string          `json:"slug" yaml:"slug"`
	Title         string          `json:"title" yaml:"title"`
	Kind          string          `json:"kind,omitempty" yaml:"kind,omitempty"`
	Summary       string          `json:"summary,omitempty" yaml:"summary,omitempty"`
	Body          string          `json:"body,omitempty" yaml:"body,omitempty"`
	Tags          []string        `json:"tags,omitempty" yaml:"tags,omitempty"`
	Aliases       []string        `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Links         []KnowledgeLink `json:"links,omitempty" yaml:"links,omitempty"`
	ProjectID     string          `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	SourceSession string          `json:"source_session,omitempty" yaml:"source_session,omitempty"`
	CreatedAt     time.Time       `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	Path          string          `json:"path,omitempty" yaml:"-"`
}

type KnowledgeNotePatch struct {
	Slug          string
	Title         *string
	Kind          *string
	Summary       *string
	Body          *string
	Tags          *[]string
	Aliases       *[]string
	Links         *[]KnowledgeLink
	ProjectID     *string
	SourceSession *string
	UpdatedAt     time.Time
}

type KnowledgeListOptions struct {
	Query     string
	Kind      string
	Tag       string
	ProjectID string
	Limit     int
}

type KnowledgeGraphNode struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind,omitempty"`
	Path      string    `json:"path,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type KnowledgeGraphEdge struct {
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	Relation  string    `json:"relation,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type KnowledgeGraph struct {
	UpdatedAt time.Time            `json:"updated_at,omitempty"`
	Nodes     []KnowledgeGraphNode `json:"nodes"`
	Edges     []KnowledgeGraphEdge `json:"edges"`
}

type KnowledgeUpdate struct {
	Notes []KnowledgeNote      `json:"notes"`
	Edges []KnowledgeGraphEdge `json:"edges,omitempty"`
}

type KnowledgeStore struct {
	root     string
	semantic *Service
	nowFn    func() time.Time
}

func NewKnowledgeStore(root string, semantic *Service) *KnowledgeStore {
	return &KnowledgeStore{
		root:     strings.TrimSpace(root),
		semantic: semantic,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *KnowledgeStore) Upsert(note KnowledgeNote) (KnowledgeNote, error) {
	tags := append([]string(nil), note.Tags...)
	aliases := append([]string(nil), note.Aliases...)
	links := append([]KnowledgeLink(nil), note.Links...)
	patch := KnowledgeNotePatch{
		Slug:      note.Slug,
		UpdatedAt: note.UpdatedAt,
	}
	if strings.TrimSpace(note.Title) != "" {
		patch.Title = stringPtr(note.Title)
	}
	if strings.TrimSpace(note.Kind) != "" {
		patch.Kind = stringPtr(note.Kind)
	}
	if strings.TrimSpace(note.Summary) != "" {
		patch.Summary = stringPtr(note.Summary)
	}
	if strings.TrimSpace(note.Body) != "" {
		patch.Body = stringPtr(note.Body)
	}
	if tags != nil {
		patch.Tags = &tags
	}
	if aliases != nil {
		patch.Aliases = &aliases
	}
	if links != nil {
		patch.Links = &links
	}
	if strings.TrimSpace(note.ProjectID) != "" {
		patch.ProjectID = stringPtr(note.ProjectID)
	}
	if strings.TrimSpace(note.SourceSession) != "" {
		patch.SourceSession = stringPtr(note.SourceSession)
	}
	return s.ApplyPatch(patch)
}

func (s *KnowledgeStore) ApplyPatch(patch KnowledgeNotePatch) (KnowledgeNote, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return KnowledgeNote{}, fmt.Errorf("knowledge store is not configured")
	}
	if err := EnsureWorkspace(s.root); err != nil {
		return KnowledgeNote{}, err
	}

	slug := normalizeKnowledgeSlug(patch.Slug)
	if slug == "" && patch.Title != nil {
		slug = normalizeKnowledgeSlug(*patch.Title)
	}
	if slug == "" {
		return KnowledgeNote{}, fmt.Errorf("slug or title is required")
	}

	existing, err := s.Get(slug)
	if err != nil && !os.IsNotExist(err) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return KnowledgeNote{}, err
	}

	now := patch.UpdatedAt.UTC()
	if now.IsZero() {
		now = s.nowFn().UTC()
	}

	note := existing
	note.Slug = slug
	if existing.CreatedAt.IsZero() {
		note.CreatedAt = now
	}
	note.UpdatedAt = now

	if patch.Title != nil {
		note.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Kind != nil {
		note.Kind = normalizeKnowledgeKind(*patch.Kind)
	}
	if patch.Summary != nil {
		note.Summary = strings.TrimSpace(*patch.Summary)
	}
	if patch.Body != nil {
		note.Body = strings.TrimSpace(*patch.Body)
	}
	if patch.Tags != nil {
		note.Tags = normalizeStringList(*patch.Tags)
	}
	if patch.Aliases != nil {
		note.Aliases = normalizeStringList(*patch.Aliases)
	}
	if patch.Links != nil {
		note.Links = normalizeKnowledgeLinks(*patch.Links)
	}
	if patch.ProjectID != nil {
		note.ProjectID = strings.TrimSpace(*patch.ProjectID)
	}
	if patch.SourceSession != nil {
		note.SourceSession = strings.TrimSpace(*patch.SourceSession)
	}

	if strings.TrimSpace(note.Title) == "" {
		note.Title = titleFromSlug(note.Slug)
	}
	if strings.TrimSpace(note.Kind) == "" {
		note.Kind = "note"
	}

	path := s.notePath(note.Slug)
	if err := os.WriteFile(path, []byte(buildKnowledgeDocument(note)), 0o644); err != nil {
		return KnowledgeNote{}, fmt.Errorf("write knowledge note: %w", err)
	}
	note.Path = filepath.ToSlash(filepath.Join("memory", "wiki", "notes", filepath.Base(path)))
	if err := s.rebuildArtifacts(); err != nil {
		return KnowledgeNote{}, err
	}
	return note, nil
}

func (s *KnowledgeStore) ApplyUpdate(update KnowledgeUpdate, now time.Time) error {
	if s == nil {
		return fmt.Errorf("knowledge store is not configured")
	}
	applied := map[string]KnowledgeNote{}
	for _, note := range update.Notes {
		if note.UpdatedAt.IsZero() {
			note.UpdatedAt = now.UTC()
		}
		saved, err := s.Upsert(note)
		if err != nil {
			return err
		}
		applied[saved.Slug] = saved
	}
	for _, edge := range update.Edges {
		source := normalizeKnowledgeSlug(edge.Source)
		target := normalizeKnowledgeSlug(edge.Target)
		if source == "" || target == "" {
			continue
		}
		note, err := s.Get(source)
		if err != nil {
			continue
		}
		links := append([]KnowledgeLink(nil), note.Links...)
		links = append(links, KnowledgeLink{Target: target, Relation: edge.Relation})
		if _, err := s.ApplyPatch(KnowledgeNotePatch{
			Slug:      source,
			Links:     &links,
			UpdatedAt: now.UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *KnowledgeStore) Get(slug string) (KnowledgeNote, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return KnowledgeNote{}, fmt.Errorf("knowledge store is not configured")
	}
	path := s.notePath(normalizeKnowledgeSlug(slug))
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return KnowledgeNote{}, fmt.Errorf("knowledge note not found: %s", normalizeKnowledgeSlug(slug))
		}
		return KnowledgeNote{}, fmt.Errorf("read knowledge note: %w", err)
	}
	note, err := parseKnowledgeDocument(string(raw))
	if err != nil {
		return KnowledgeNote{}, err
	}
	note.Path = filepath.ToSlash(filepath.Join("memory", "wiki", "notes", filepath.Base(path)))
	return note, nil
}

func (s *KnowledgeStore) List(opts KnowledgeListOptions) ([]KnowledgeNote, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return []KnowledgeNote{}, nil
	}
	paths, err := filepath.Glob(filepath.Join(s.root, "memory", "wiki", "notes", "*.md"))
	if err != nil {
		return nil, fmt.Errorf("glob knowledge notes: %w", err)
	}
	query := strings.ToLower(strings.TrimSpace(opts.Query))
	kind := normalizeKnowledgeKind(opts.Kind)
	tag := strings.ToLower(strings.TrimSpace(opts.Tag))
	projectID := strings.TrimSpace(opts.ProjectID)

	items := make([]KnowledgeNote, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		note, err := parseKnowledgeDocument(string(raw))
		if err != nil {
			continue
		}
		note.Path = filepath.ToSlash(filepath.Join("memory", "wiki", "notes", filepath.Base(path)))
		if kind != "" && normalizeKnowledgeKind(note.Kind) != kind {
			continue
		}
		if projectID != "" && strings.TrimSpace(note.ProjectID) != projectID {
			continue
		}
		if tag != "" && !knowledgeHasTag(note, tag) {
			continue
		}
		if query != "" && !knowledgeMatchesQuery(note, query) {
			continue
		}
		items = append(items, note)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].Slug < items[j].Slug
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultKnowledgeListLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *KnowledgeStore) Delete(slug string) error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return fmt.Errorf("knowledge store is not configured")
	}
	path := s.notePath(normalizeKnowledgeSlug(slug))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete knowledge note: %w", err)
	}
	return s.rebuildArtifacts()
}

func (s *KnowledgeStore) Graph() (KnowledgeGraph, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return KnowledgeGraph{Nodes: []KnowledgeGraphNode{}, Edges: []KnowledgeGraphEdge{}}, nil
	}
	raw, err := os.ReadFile(filepath.Join(s.root, "memory", "wiki", "graph.json"))
	if err != nil {
		if os.IsNotExist(err) {
			if err := s.rebuildArtifacts(); err != nil {
				return KnowledgeGraph{}, err
			}
			raw, err = os.ReadFile(filepath.Join(s.root, "memory", "wiki", "graph.json"))
		}
		if err != nil {
			return KnowledgeGraph{}, fmt.Errorf("read knowledge graph: %w", err)
		}
	}
	var graph KnowledgeGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return KnowledgeGraph{}, fmt.Errorf("decode knowledge graph: %w", err)
	}
	if graph.Nodes == nil {
		graph.Nodes = []KnowledgeGraphNode{}
	}
	if graph.Edges == nil {
		graph.Edges = []KnowledgeGraphEdge{}
	}
	return graph, nil
}

func (s *KnowledgeStore) notePath(slug string) string {
	return filepath.Join(s.root, "memory", "wiki", "notes", normalizeKnowledgeSlug(slug)+".md")
}

func (s *KnowledgeStore) rebuildArtifacts() error {
	items, err := s.List(KnowledgeListOptions{Limit: 10000})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.root, "memory", "wiki", "index.md"), []byte(buildKnowledgeIndex(items)), 0o644); err != nil {
		return fmt.Errorf("write knowledge index: %w", err)
	}
	graph := buildKnowledgeGraph(items)
	encoded, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("encode knowledge graph: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "memory", "wiki", "graph.json"), append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write knowledge graph: %w", err)
	}
	return nil
}

func buildKnowledgeGraph(items []KnowledgeNote) KnowledgeGraph {
	nodes := make([]KnowledgeGraphNode, 0, len(items))
	edges := make([]KnowledgeGraphEdge, 0, len(items))
	seenEdges := map[string]struct{}{}
	var updatedAt time.Time
	for _, item := range items {
		nodes = append(nodes, KnowledgeGraphNode{
			Slug:      item.Slug,
			Title:     item.Title,
			Kind:      item.Kind,
			Path:      item.Path,
			Tags:      item.Tags,
			UpdatedAt: item.UpdatedAt.UTC(),
		})
		if item.UpdatedAt.After(updatedAt) {
			updatedAt = item.UpdatedAt.UTC()
		}
		for _, link := range item.Links {
			key := item.Slug + "|" + normalizeKnowledgeSlug(link.Target) + "|" + strings.ToLower(strings.TrimSpace(link.Relation))
			if _, exists := seenEdges[key]; exists {
				continue
			}
			seenEdges[key] = struct{}{}
			edges = append(edges, KnowledgeGraphEdge{
				Source:    item.Slug,
				Target:    normalizeKnowledgeSlug(link.Target),
				Relation:  strings.TrimSpace(link.Relation),
				UpdatedAt: item.UpdatedAt.UTC(),
			})
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Slug < nodes[j].Slug })
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Source == edges[j].Source {
			if edges[i].Target == edges[j].Target {
				return edges[i].Relation < edges[j].Relation
			}
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Source < edges[j].Source
	})
	return KnowledgeGraph{
		UpdatedAt: updatedAt,
		Nodes:     nodes,
		Edges:     edges,
	}
}

func buildKnowledgeIndex(items []KnowledgeNote) string {
	graph := buildKnowledgeGraph(items)
	byKind := map[string][]KnowledgeNote{}
	kinds := make([]string, 0)
	for _, item := range items {
		kind := normalizeKnowledgeKind(item.Kind)
		if _, exists := byKind[kind]; !exists {
			kinds = append(kinds, kind)
		}
		byKind[kind] = append(byKind[kind], item)
	}
	sort.Strings(kinds)

	var b strings.Builder
	b.WriteString("# Knowledge Base Index\n\n")
	if !graph.UpdatedAt.IsZero() {
		b.WriteString("Updated: " + graph.UpdatedAt.UTC().Format(time.RFC3339) + "\n\n")
	}
	_, _ = fmt.Fprintf(&b, "- Notes: %d\n", len(graph.Nodes))
	_, _ = fmt.Fprintf(&b, "- Relations: %d\n\n", len(graph.Edges))

	for _, kind := range kinds {
		title := kind
		if title == "" {
			title = "note"
		}
		b.WriteString("## " + titleFromSlug(title) + "\n")
		sort.SliceStable(byKind[kind], func(i, j int) bool { return byKind[kind][i].Slug < byKind[kind][j].Slug })
		for _, item := range byKind[kind] {
			_, _ = fmt.Fprintf(&b, "- [[%s]] %s", item.Slug, strings.TrimSpace(item.Title))
			if strings.TrimSpace(item.Summary) != "" {
				_, _ = fmt.Fprintf(&b, " — %s", strings.TrimSpace(item.Summary))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func buildKnowledgeDocument(note KnowledgeNote) string {
	meta := KnowledgeNote{
		Slug:          note.Slug,
		Title:         note.Title,
		Kind:          note.Kind,
		Summary:       note.Summary,
		Body:          note.Body,
		Tags:          note.Tags,
		Aliases:       note.Aliases,
		Links:         note.Links,
		ProjectID:     note.ProjectID,
		SourceSession: note.SourceSession,
		CreatedAt:     note.CreatedAt.UTC(),
		UpdatedAt:     note.UpdatedAt.UTC(),
	}
	data, _ := yaml.Marshal(meta)

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(data)
	b.WriteString("---\n")
	b.WriteString("# " + strings.TrimSpace(note.Title) + "\n\n")
	if strings.TrimSpace(note.Summary) != "" {
		b.WriteString("## Summary\n")
		b.WriteString(strings.TrimSpace(note.Summary) + "\n\n")
	}
	if strings.TrimSpace(note.Body) != "" {
		b.WriteString("## Details\n")
		b.WriteString(strings.TrimSpace(note.Body) + "\n\n")
	}
	if len(note.Links) > 0 {
		b.WriteString("## Links\n")
		for _, link := range note.Links {
			target := normalizeKnowledgeSlug(link.Target)
			if target == "" {
				continue
			}
			relation := strings.TrimSpace(link.Relation)
			if relation != "" {
				_, _ = fmt.Fprintf(&b, "- %s [[%s]]\n", relation, target)
			} else {
				_, _ = fmt.Fprintf(&b, "- [[%s]]\n", target)
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func parseKnowledgeDocument(raw string) (KnowledgeNote, error) {
	metaRaw, _, hasMeta, err := splitKnowledgeFrontmatter(strings.ReplaceAll(raw, "\r\n", "\n"))
	if err != nil {
		return KnowledgeNote{}, err
	}
	if !hasMeta {
		return KnowledgeNote{}, fmt.Errorf("knowledge note frontmatter is required")
	}
	var note KnowledgeNote
	if err := yaml.Unmarshal([]byte(metaRaw), &note); err != nil {
		return KnowledgeNote{}, fmt.Errorf("parse knowledge note frontmatter: %w", err)
	}
	note.Slug = normalizeKnowledgeSlug(note.Slug)
	note.Title = strings.TrimSpace(note.Title)
	note.Kind = normalizeKnowledgeKind(note.Kind)
	note.Summary = strings.TrimSpace(note.Summary)
	note.Body = strings.TrimSpace(note.Body)
	note.Tags = normalizeStringList(note.Tags)
	note.Aliases = normalizeStringList(note.Aliases)
	note.Links = normalizeKnowledgeLinks(note.Links)
	note.ProjectID = strings.TrimSpace(note.ProjectID)
	note.SourceSession = strings.TrimSpace(note.SourceSession)
	note.CreatedAt = note.CreatedAt.UTC()
	note.UpdatedAt = note.UpdatedAt.UTC()
	if note.Title == "" {
		note.Title = titleFromSlug(note.Slug)
	}
	if note.Kind == "" {
		note.Kind = "note"
	}
	return note, nil
}

func splitKnowledgeFrontmatter(raw string) (string, string, bool, error) {
	if !strings.HasPrefix(raw, "---\n") {
		return "", raw, false, nil
	}
	rest := raw[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated knowledge note frontmatter")
	}
	return rest[:end], rest[end+len("\n---\n"):], true, nil
}

func normalizeKnowledgeLinks(values []KnowledgeLink) []KnowledgeLink {
	if len(values) == 0 {
		return nil
	}
	out := make([]KnowledgeLink, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		target := normalizeKnowledgeSlug(value.Target)
		if target == "" {
			continue
		}
		relation := strings.TrimSpace(value.Relation)
		key := target + "|" + strings.ToLower(relation)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, KnowledgeLink{
			Target:   target,
			Relation: relation,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Target == out[j].Target {
			return out[i].Relation < out[j].Relation
		}
		return out[i].Target < out[j].Target
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeKnowledgeSlug(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeKnowledgeKind(raw string) string {
	kind := strings.TrimSpace(strings.ToLower(raw))
	if kind == "" {
		return ""
	}
	return normalizeKnowledgeSlug(kind)
}

func titleFromSlug(slug string) string {
	parts := strings.Fields(strings.ReplaceAll(strings.TrimSpace(slug), "-", " "))
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, " ")
}

func knowledgeHasTag(note KnowledgeNote, query string) bool {
	for _, tag := range note.Tags {
		if strings.Contains(strings.ToLower(strings.TrimSpace(tag)), query) {
			return true
		}
	}
	return false
}

func knowledgeMatchesQuery(note KnowledgeNote, query string) bool {
	fields := []string{
		note.Slug,
		note.Title,
		note.Kind,
		note.Summary,
		note.Body,
		note.ProjectID,
		note.SourceSession,
		strings.Join(note.Tags, " "),
		strings.Join(note.Aliases, " "),
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), query) {
			return true
		}
	}
	for _, link := range note.Links {
		if strings.Contains(strings.ToLower(link.Target), query) || strings.Contains(strings.ToLower(link.Relation), query) {
			return true
		}
	}
	return false
}

func stringPtr(value string) *string {
	v := value
	return &v
}
