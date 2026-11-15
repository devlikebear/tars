package aside

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEngine(t *testing.T) {
	for in, want := range map[string]string{"": "google", " Naver ": "naver", "duckduckgo": "duckduckgo"} {
		if got, err := NormalizeEngine(in); err != nil || got != want {
			t.Fatalf("%q: got %q, %v", in, got, err)
		}
	}
	if _, err := NormalizeEngine("bing"); err == nil {
		t.Fatal("bing should be rejected")
	}
}

func TestSearchURL(t *testing.T) {
	cases := map[string]string{
		"google":     "https://www.google.com/search?q=go+%EC%BB%A8%ED%85%8D%EC%8A%A4%ED%8A%B8",
		"naver":      "https://search.naver.com/search.naver?where=web&query=go+%EC%BB%A8%ED%85%8D%EC%8A%A4%ED%8A%B8",
		"duckduckgo": "https://html.duckduckgo.com/html/?q=go+%EC%BB%A8%ED%85%8D%EC%8A%A4%ED%8A%B8",
	}
	for engine, want := range cases {
		got, err := SearchURL(engine, "go 컨텍스트")
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v", engine, got, err)
		}
	}
	if _, err := SearchURL("bing", "x"); err == nil {
		t.Fatal("unknown engine should fail")
	}
}

func TestSearchScriptEmbedsArgumentsAsJSON(t *testing.T) {
	script := SearchScript("google", `https://www.google.com/search?q="');alert(1)//`, 3)
	if !strings.Contains(script, `"count":3`) {
		t.Fatalf("count not embedded: %s", script)
	}
	if !strings.Contains(script, `q=\"');alert(1)//"`) {
		t.Fatalf("URL not JSON-escaped: %s", script)
	}
	if !strings.Contains(script, "await closeTab(tab)") || !strings.Contains(script, resultMarker) {
		t.Fatal("script must close its tab and print the marker")
	}
	for _, engine := range Engines {
		if extractors[engine] == "" {
			t.Fatalf("no extractor for %s", engine)
		}
	}
}

func TestParseSearchOutput(t *testing.T) {
	out := "✔︎ Opened a new tab\n" + resultMarker +
		`{"blocked":false,"results":[{"title":" A ","url":"https://a.example","description":"x"},{"title":"","url":"https://skip.example"},{"title":"B","url":"https://b.example"},{"title":"C","url":"https://c.example"}]}` +
		"\n[system] last tab closed.\n"
	got, err := ParseSearchOutput(out, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "A" || got[1].URL != "https://b.example" {
		t.Fatalf("got %+v", got)
	}

	if _, err := ParseSearchOutput(resultMarker+`{"blocked":true,"results":[]}`, 5); !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	if _, err := ParseSearchOutput("Error: boom", 5); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want missing-marker error naming output, got %v", err)
	}
	if _, err := ParseSearchOutput(resultMarker+"{not json", 5); err == nil {
		t.Fatal("want decode error")
	}
}

func TestSearchRunsRepl(t *testing.T) {
	bin := fakeBinary(t)
	f := &fakeRun{stdout: resultMarker + `{"results":[{"title":"T","url":"https://t.example"}]}`}
	got, err := Client{Path: bin, run: f.run}.Search(context.Background(), "naver", "q", 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if f.gotArgs[0] != "repl" || !strings.Contains(f.gotArgs[1], "search.naver.com") {
		t.Fatalf("args %v", f.gotArgs)
	}
	if _, err := (Client{Path: bin, run: f.run}).Search(context.Background(), "bing", "q", 1); err == nil {
		t.Fatal("unknown engine should fail")
	}
	f.err = errors.New("exit status 1")
	if _, err := (Client{Path: bin, run: f.run}).Search(context.Background(), "", "q", 1); err == nil {
		t.Fatal("want repl error")
	}
}
