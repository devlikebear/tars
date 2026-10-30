package memory

import "sort"

// rrfK is the usual reciprocal rank fusion constant: it keeps the first few
// places of every list close together, so one list cannot crowd out the other.
const rrfK = 60

// FuseRanked merges ranked lists (best first) with reciprocal rank fusion:
// an item scores 1/(rrfK+rank) in every list that has it, so items found by
// several lists rise, and each list's top items interleave. Scores from the
// different lists are never compared — semantic cosine similarity and keyword
// scores are on unrelated scales. Items with the same key are one item; the
// first one seen is kept. An empty key drops the item. limit <= 0 keeps all.
func FuseRanked[T any](key func(T) string, limit int, lists ...[]T) []T {
	type fused struct {
		item  T
		score float64
		order int
	}
	byKey := map[string]*fused{}
	all := make([]*fused, 0)
	for _, list := range lists {
		rank := 0
		seenInList := map[string]struct{}{}
		for _, item := range list {
			k := key(item)
			if k == "" {
				continue
			}
			if _, dup := seenInList[k]; dup {
				continue
			}
			seenInList[k] = struct{}{}
			rank++
			entry, ok := byKey[k]
			if !ok {
				entry = &fused{item: item, order: len(all)}
				byKey[k] = entry
				all = append(all, entry)
			}
			entry.score += 1.0 / float64(rrfK+rank)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score == all[j].score {
			return all[i].order < all[j].order
		}
		return all[i].score > all[j].score
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	out := make([]T, len(all))
	for i, entry := range all {
		out[i] = entry.item
	}
	return out
}
