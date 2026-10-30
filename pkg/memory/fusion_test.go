package memory

import (
	"reflect"
	"strings"
	"testing"
)

func TestFuseRanked_InterleavesListsAndRewardsAgreement(t *testing.T) {
	semantic := []string{"birthday", "spicy", "cafe"}
	keyword := []string{"release", "cafe"}

	got := FuseRanked(func(s string) string { return s }, 0, semantic, keyword)

	// "cafe" is in both lists, so it outranks every item found by one list
	// alone; the two first places come next, ahead of second places.
	want := []string{"cafe", "birthday", "release", "spicy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestFuseRanked_DedupesByKeyAndKeepsFirstItem(t *testing.T) {
	type item struct{ key, source string }
	a := []item{{"Same Fact", "semantic"}}
	b := []item{{"same fact ", "keyword"}, {"other", "keyword"}}

	got := FuseRanked(func(i item) string { return strings.ToLower(strings.TrimSpace(i.key)) }, 0, a, b)

	if len(got) != 2 || got[0].source != "semantic" || got[1].key != "other" {
		t.Fatalf("expected deduped list led by the first-seen item, got %#v", got)
	}
}

func TestFuseRanked_LimitAndEmptyKeys(t *testing.T) {
	got := FuseRanked(func(s string) string { return s }, 2, []string{"a", "", "b", "c"}, []string{"d"})
	if !reflect.DeepEqual(got, []string{"a", "d"}) {
		t.Fatalf("expected limit 2 and empty keys skipped, got %v", got)
	}
	if out := FuseRanked[string](func(s string) string { return s }, 3); len(out) != 0 {
		t.Fatalf("expected empty result with no lists, got %v", out)
	}
}
