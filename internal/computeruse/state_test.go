package computeruse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleSnapshot() Snapshot {
	sel := true
	return Snapshot{
		Window: Window{PID: 1, WindowID: 2, App: "Finder", Title: "Downloads"},
		Elements: []Element{
			{Index: 1, Token: "s:1", Role: "AXButton", Label: "Back", Enabled: true},
			{Index: 2, Token: "s:2", Role: "AXTextField", Label: "Search", Value: "", Enabled: true},
			{Index: 3, Token: "s:3", Role: "AXButton", Label: "New Folder", Enabled: true},
			{Index: 4, Token: "s:4", Role: "AXCheckBox", Label: "Show hidden", Enabled: true, Selected: &sel},
			{Index: 5, Token: "s:5", Role: "AXSecureTextField", Label: "Password", Secure: true, Enabled: true},
			{Index: 6, Token: "s:6", Role: "AXButton", Label: strings.Repeat("L", 80), Value: strings.Repeat("V", 60), Enabled: false},
		},
		TotalElements: 6,
	}
}

func TestRenderState_Golden(t *testing.T) {
	req := Request{Goal: "Make a new folder", Inputs: map[string]string{"name": "secret-folder", "email": "a@b.c"}}
	trace := []TraceStep{{Step: 1, Op: "click", Target: "AXButton 'New Folder'", Effect: "confirmed"}}
	state, shown := RenderState(req, sampleSnapshot(), trace, RenderOptions{ExposeValues: true})
	want := "GOAL: Make a new folder\n" +
		"INPUTS AVAILABLE: email, name\n" +
		"APP: Finder\n" +
		"RECENT ACTIONS (newest last):\n" +
		"  1. click AXButton 'New Folder' → confirmed\n" +
		"SCREEN (6 elements):\n" +
		"[e1] AXButton 'Back' enabled\n" +
		"[e2] AXTextField 'Search' value='' enabled\n" +
		"[e3] AXButton 'New Folder' enabled\n" +
		"[e4] AXCheckBox 'Show hidden' enabled checked\n" +
		"[e5] AXSecureTextField 'Password' enabled\n" +
		"[e6] AXButton '" + strings.Repeat("L", 60) + "…' value='" + strings.Repeat("V", 40) + "…' disabled\n"
	if state != want {
		t.Fatalf("state mismatch:\n--- got ---\n%s\n--- want ---\n%s", state, want)
	}
	if len(shown) != 6 {
		t.Fatalf("shown = %d", len(shown))
	}
	if strings.Contains(state, "secret-folder") || strings.Contains(state, "a@b.c") {
		t.Fatal("input values leaked into state")
	}
}

func TestRenderState_ExcludesMenusAndBareContainers(t *testing.T) {
	snap := sampleSnapshot()
	snap.Elements = append(snap.Elements,
		Element{Index: 7, Token: "s:7", Role: "AXMenuBarItem", Label: "File", Enabled: true},
		Element{Index: 8, Token: "s:8", Role: "AXMenuItem", Label: "Recent Items", Enabled: true},
		Element{Index: 9, Token: "s:9", Role: "AXScrollArea", Label: "", Enabled: true},
		Element{Index: 10, Token: "s:10", Role: "AXRow", Label: "Accessibility", LabelAdopted: true, Enabled: true},
	)
	snap.TotalElements = 10
	state, shown := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{ExposeValues: true})
	if strings.Contains(state, "Recent Items") || strings.Contains(state, "AXMenuBarItem") || strings.Contains(state, "AXScrollArea") {
		t.Fatalf("menu/container leaked:\n%s", state)
	}
	if !strings.Contains(state, "[e10] AXRow 'Accessibility' enabled") || !strings.Contains(state, "SCREEN (7 elements)") {
		t.Fatalf("adopted-label row missing or count wrong:\n%s", state)
	}
	if len(shown) != 7 || shown[6].Index != 10 {
		t.Fatalf("shown = %d, last=%+v", len(shown), shown[len(shown)-1])
	}
	if strings.Contains(state, "WINDOW:") {
		t.Fatal("window title must not be sent")
	}
}

func TestRenderState_HidesValuesWhenNotExposed(t *testing.T) {
	snap := sampleSnapshot()
	snap.Elements[1].Value = "typed text"
	state, _ := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{ExposeValues: false})
	if strings.Contains(state, "typed text") || strings.Contains(state, "value=") {
		t.Fatalf("value leaked: %s", state)
	}
}

func TestRenderState_CutsAt254AndAnnounces(t *testing.T) {
	snap := Snapshot{TotalElements: 300}
	for i := 1; i <= 300; i++ {
		snap.Elements = append(snap.Elements, Element{Index: i, Token: "t", Role: "AXButton", Label: "B", Enabled: true})
	}
	state, shown := RenderState(Request{Goal: "g"}, snap, nil, RenderOptions{ExposeValues: true})
	if len(shown) != MaxChoiceElements {
		t.Fatalf("shown = %d", len(shown))
	}
	if !strings.Contains(state, "[e254]") || strings.Contains(state, "[e255]") || !strings.Contains(state, "(…46 more elements not shown; choose look to see them)") {
		t.Fatalf("cut/announce wrong:\n%s", state[len(state)-200:])
	}
}

func TestRenderState_NoInputsNoTraceOmitsSections(t *testing.T) {
	state, _ := RenderState(Request{Goal: "g"}, sampleSnapshot(), nil, RenderOptions{ExposeValues: true})
	if strings.Contains(state, "INPUTS AVAILABLE") || strings.Contains(state, "RECENT ACTIONS") {
		t.Fatalf("empty sections rendered:\n%s", state)
	}
}

func TestElementHash_ChangesWithValueNotToken(t *testing.T) {
	a := sampleSnapshot()
	b := sampleSnapshot()
	b.Elements[0].Token = "different"
	if ElementHash(a) != ElementHash(b) {
		t.Fatal("token must not affect hash")
	}
	b.Elements[1].Value = "x"
	if ElementHash(a) == ElementHash(b) {
		t.Fatal("value must affect hash")
	}
}

// The stock macOS Calculator snapshot is 143 elements of which 112 are menu
// nodes; the rendered state must carry neither those nor the bare containers.
func TestRenderState_CalculatorFixtureDropsMenuBar(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "get_window_state_calculator.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseWindowState(raw)
	if err != nil {
		t.Fatal(err)
	}
	state, shown := RenderState(Request{Goal: "compute 2+2"}, snap, nil, RenderOptions{ExposeValues: true})
	if strings.Contains(state, "AXMenu") {
		t.Fatalf("menu roles leaked into state:\n%s", state)
	}
	if len(shown) >= 40 || len(shown) == 0 {
		t.Fatalf("shown = %d; want a filtered Calculator surface well under 40", len(shown))
	}
	if n := strings.Count(state, "[e"); n >= 40 {
		t.Fatalf("element lines = %d; want fewer than 40", n)
	}
}
