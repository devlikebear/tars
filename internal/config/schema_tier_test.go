package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateTierDoc = flag.Bool("update-tier-doc", false, "rewrite docs/config-field-tiers.md")

func TestEveryConfigFieldHasATier(t *testing.T) {
	inSchema := map[string]bool{}
	for _, field := range Schema() {
		inSchema[field.Key] = true
		switch field.Tier {
		case FieldTierBasic, FieldTierAdvanced, FieldTierFile, FieldTierRemove:
		default:
			t.Errorf("%s has no tier; classify it in configFieldTiers", field.Key)
		}
	}
	for key := range configFieldTiers {
		if !inSchema[key] {
			t.Errorf("configFieldTiers lists %s, which is not in Schema()", key)
		}
	}
}

func TestConfigFieldShowWhenIsSatisfiable(t *testing.T) {
	byKey := map[string]FieldMeta{}
	for _, field := range Schema() {
		byKey[field.Key] = field
	}
	for _, field := range Schema() {
		for _, cond := range field.ShowWhen {
			parent, ok := byKey[cond.Key]
			if !ok {
				t.Errorf("%s: show_when names unknown field %s", field.Key, cond.Key)
				continue
			}
			if parent.Tier != FieldTierBasic && parent.Tier != FieldTierAdvanced {
				t.Errorf("%s: show_when names %s, which the console does not show (tier %s)", field.Key, cond.Key, parent.Tier)
			}
			if len(cond.Values) == 0 {
				t.Errorf("%s: show_when on %s has no values", field.Key, cond.Key)
			}
			for _, v := range cond.Values {
				if !valueAllowed(parent, v) {
					t.Errorf("%s: show_when value %q is not a value of %s", field.Key, v, cond.Key)
				}
			}
			if field.Tier == FieldTierBasic && parent.Tier == FieldTierAdvanced {
				t.Errorf("%s is basic but only shows when advanced field %s holds", field.Key, cond.Key)
			}
		}
		// A chain of conditions must end at a field shown unconditionally.
		seen := map[string]bool{field.Key: true}
		for cur := field; len(cur.ShowWhen) > 0; {
			next := cur.ShowWhen[0].Key
			if seen[next] {
				t.Errorf("%s: show_when cycle through %s", field.Key, next)
				break
			}
			seen[next] = true
			cur = byKey[next]
		}
	}
}

func valueAllowed(field FieldMeta, v string) bool {
	switch field.Type {
	case "bool":
		return v == "true" || v == "false"
	case "select":
		for _, opt := range field.Options {
			if opt == v {
				return true
			}
		}
		return false
	}
	return true
}

func TestRemoveTierFieldsHaveANote(t *testing.T) {
	for key, tier := range configFieldTiers {
		if (tier.tier == FieldTierRemove || tier.tier == FieldTierFile) && tier.note == "" {
			t.Errorf("%s is %s without a note saying why", key, tier.tier)
		}
	}
}

func TestConfigFieldTierDocIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "config-field-tiers.md")
	want := renderConfigFieldTierDoc()
	if *updateTierDoc {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s is stale; run go test ./internal/config -run TestConfigFieldTierDocIsCurrent -update-tier-doc", path)
	}
}

func renderConfigFieldTierDoc() string {
	fields := Schema()
	counts := map[string]int{}
	var sections []string
	bySection := map[string][]FieldMeta{}
	for _, field := range fields {
		counts[field.Tier]++
		if _, ok := bySection[field.Section]; !ok {
			sections = append(sections, field.Section)
		}
		bySection[field.Section] = append(bySection[field.Section], field)
	}

	var b strings.Builder
	b.WriteString("# Config field tiers\n\n")
	b.WriteString("<!-- Generated from internal/config/schema_tier.go by\n")
	b.WriteString("     go test ./internal/config -run TestConfigFieldTierDocIsCurrent -update-tier-doc -->\n\n")
	b.WriteString("Where the console shows each field of `/console/config` (#1240). The schema\n")
	b.WriteString("(`GET /v1/admin/config/schema`) carries the same `tier` and `show_when`.\n\n")
	b.WriteString("| Tier | Meaning | Fields |\n|---|---|---|\n")
	for _, row := range [][2]string{
		{FieldTierBasic, "Most people touch it at least once; shown up front"},
		{FieldTierAdvanced, "Inside a collapsed Advanced group"},
		{FieldTierFile, "Not in the console; YAML and `config/tars.config.example.yaml` only"},
		{FieldTierRemove, "Nothing reads it or it was replaced; to be removed, hidden until then"},
	} {
		fmt.Fprintf(&b, "| %s | %s | %d |\n", row[0], row[1], counts[row[0]])
	}
	fmt.Fprintf(&b, "\nTotal: %d fields in %d sections.\n\n", len(fields), len(sections))
	b.WriteString("`show_when` lists conditions on other fields; the field shows when any one holds\n")
	b.WriteString("and the field it names is itself shown.\n")

	for _, section := range sections {
		fmt.Fprintf(&b, "\n## %s\n\n| YAML path | Tier | Shown when | Why |\n|---|---|---|---|\n", section)
		for _, field := range bySection[section] {
			var conds []string
			for _, c := range field.ShowWhen {
				conds = append(conds, fmt.Sprintf("`%s` = %s", preferredYAMLPathForKey(c.Key), strings.Join(c.Values, " / ")))
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", field.Path, field.Tier, strings.Join(conds, " or "), configFieldTiers[field.Key].note)
		}
	}
	return b.String()
}
