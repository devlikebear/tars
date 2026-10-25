package skillhub

import (
	"context"
	"errors"
	"testing"
)

// pathStubSource is an external hub whose skill "foo" lives in directory
// skills/foo-dir. Once byNameGone is set, a lookup by name fails the way
// openclaw's used to (it guessed skills/<name>), so only a lookup by the
// recorded path can find the skill.
type pathStubSource struct {
	externalStubSource
	byNameGone bool
	pathCalls  []string
}

func (s *pathStubSource) FindSkillByName(ctx context.Context, name string) (*RegistryEntry, error) {
	if s.byNameGone {
		return nil, errors.New("404 skills/" + name + "/SKILL.md")
	}
	return s.externalStubSource.FindSkillByName(ctx, name)
}

func (s *pathStubSource) FindSkillByPath(_ context.Context, path string) (*RegistryEntry, error) {
	s.pathCalls = append(s.pathCalls, path)
	for _, e := range s.entries {
		if e.Path == path {
			e := e
			return &e, nil
		}
	}
	return nil, errors.New("404 " + path)
}

func newPathStub() *pathStubSource {
	src := &pathStubSource{externalStubSource: *newExternalStub()}
	src.entries = map[string]RegistryEntry{"foo": {Name: "foo", Path: "skills/foo-dir"}}
	return src
}

func TestUpdate_ExternalSkillLooksUpRecordedPath(t *testing.T) {
	src := newPathStub()
	inst := newInstallerWithSource(t, src)
	if _, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true}); err != nil {
		t.Fatalf("install: %v", err)
	}
	skills, err := inst.List()
	if err != nil || len(skills) != 1 || skills[0].Path != "skills/foo-dir" {
		t.Fatalf("installed row = %+v, %v; want path skills/foo-dir", skills, err)
	}

	src.byNameGone = true
	result, err := inst.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v (result %+v)", err, result)
	}
	if len(src.pathCalls) != 1 || src.pathCalls[0] != "skills/foo-dir" {
		t.Fatalf("FindSkillByPath calls = %v, want [skills/foo-dir]", src.pathCalls)
	}
	for _, d := range append(result.Skipped, result.Failed...) {
		if d.Err != nil {
			t.Fatalf("update diagnostic %s: %v", d.Name, d.Err)
		}
	}
}

func TestUpdate_ExternalSkillPathNowHoldsAnotherSkill(t *testing.T) {
	src := newPathStub()
	inst := newInstallerWithSource(t, src)
	if _, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true}); err != nil {
		t.Fatalf("install: %v", err)
	}
	src.entries = map[string]RegistryEntry{"bar": {Name: "bar", Path: "skills/foo-dir"}}

	result, _ := inst.Update(context.Background())
	if len(result.Updated) != 0 {
		t.Fatalf("updated %v; a renamed upstream skill must not replace foo", result.Updated)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Err == nil || !contains(result.Skipped[0].Err.Error(), `"bar"`) {
		t.Fatalf("skipped = %+v, want the rename reported", result.Skipped)
	}
}
