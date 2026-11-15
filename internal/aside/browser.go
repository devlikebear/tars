package aside

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// ErrNotLocal reports a browser check whose start page is not a local
// development address.
var ErrNotLocal = errors.New("the start page must be a local development address")

// Check is one browser verification: open StartURL and judge Goal there.
type Check struct {
	StartURL string
	Goal     string
}

// CheckResult is what Aside's agent concluded.
type CheckResult struct {
	// Passed is true only for a final "VERDICT: PASS" line.
	Passed bool
	// Incomplete reports a run that ended with no verdict because the
	// checker itself stopped on an error (Aside prints "• Error 429 {…}"
	// when its model is rate limited). That says nothing about the app, so
	// callers must not count it as a failed check.
	Incomplete bool
	// Reason is the text after "VERDICT: FAIL:", or why no verdict was
	// read.
	Reason string
	// Transcript is the agent's output without terminal colors.
	Transcript string
}

// LocalStartURL validates a check's start page. Verification runs with no
// one watching, in a browser signed in to the developer's real accounts, so
// it may only start on the app under development: loopback addresses and
// names ending in .localhost or .test.
func LocalStartURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: %q is not an http(s) URL", ErrNotLocal, raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".test") {
		return u, nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return u, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotLocal, host)
}

// CheckPrompt is the task given to Aside's agent. It keeps the agent on the
// app's origin and asks for a verdict line the result is read from.
func CheckPrompt(c Check, origin string) string {
	return "You are running an automated end-to-end check of a web app under development. Nobody is watching.\n" +
		"Start at " + c.StartURL + ".\n" +
		"Stay on " + origin + ": do not open, sign in to or act on any other site, and do not send messages, " +
		"make purchases or delete anything outside this app.\n" +
		"Check this goal by using the app the way a person would and looking at the result on screen:\n" +
		c.Goal + "\n\n" +
		"Finish with exactly one last line, nothing after it:\n" +
		"VERDICT: PASS\n" +
		"if the goal is met as described, or\n" +
		"VERDICT: FAIL: <one sentence on what you saw instead>\n" +
		"otherwise, including when the page does not load."
}

// Verify runs one browser check through `aside exec`.
func (c Client) Verify(ctx context.Context, check Check) (CheckResult, error) {
	u, err := LocalStartURL(check.StartURL)
	if err != nil {
		return CheckResult{}, err
	}
	origin := u.Scheme + "://" + u.Host
	out, err := c.Exec(ctx, CheckPrompt(check, origin))
	if err != nil {
		// Aside can also stop on its own model's error with a non-zero exit,
		// the "• Error 429 {…}" line then in stderr (and so in err), not in
		// the transcript: seen while dogfooding as "aside exec failed: exit
		// status 1: … • Error 429". That is the same incomplete run as one
		// that exits 0 on that line, so it is read the same way.
		if ctx.Err() == nil {
			if checkerErr := lastCheckerError(strings.Split(strings.TrimSpace(err.Error()), "\n")); checkerErr != "" {
				return CheckResult{
					Incomplete: true,
					Reason:     "the checker stopped on an error: " + checkerErr,
					Transcript: strings.TrimSpace(strings.TrimSpace(out) + "\n" + err.Error()),
				}, nil
			}
		}
		return CheckResult{Transcript: out}, err
	}
	return ParseVerdict(out), nil
}

// ParseVerdict reads the agent's last VERDICT line. A missing verdict is a
// failure: the check did not show the goal was met.
func ParseVerdict(out string) CheckResult {
	res := CheckResult{Transcript: strings.TrimSpace(out)}
	lines := strings.Split(res.Transcript, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.Trim(lines[i], "*`_ "))
		upper := strings.ToUpper(line)
		if !strings.HasPrefix(upper, "VERDICT:") {
			continue
		}
		rest := strings.TrimSpace(line[len("VERDICT:"):])
		switch {
		case strings.HasPrefix(strings.ToUpper(rest), "PASS"):
			res.Passed = true
		case strings.HasPrefix(strings.ToUpper(rest), "FAIL"):
			res.Reason = strings.TrimSpace(strings.TrimLeft(rest[len("FAIL"):], ": -"))
			if res.Reason == "" {
				res.Reason = "the agent reported a failure"
			}
		default:
			res.Reason = "unreadable verdict: " + line
		}
		return res
	}
	if checkerErr := lastCheckerError(lines); checkerErr != "" {
		res.Incomplete = true
		res.Reason = "the checker stopped on an error: " + checkerErr
		return res
	}
	res.Reason = "the agent gave no verdict"
	return res
}

// checkerErrorPattern matches the line Aside prints when its own model call
// fails ("• Error 429 {…rate_limit_error…}", "Error 529 …overloaded…").
var checkerErrorPattern = regexp.MustCompile(`^(?:[•*\-]\s*)?Error\s+([45]\d\d)\b`)

// lastCheckerError returns the status and a short form of the last non-empty
// line when it is such an error, or "". Only the last line counts: an error
// the agent recovered from earlier in the run is followed by more output.
func lastCheckerError(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		m := checkerErrorPattern.FindStringSubmatch(line)
		if m == nil {
			return ""
		}
		kind := ""
		for _, k := range []string{"rate_limit_error", "overloaded_error", "api_error", "usage_limit_exceeded", "too_many_requests"} {
			if strings.Contains(line, k) {
				kind = " " + k
				break
			}
		}
		return "Error " + m[1] + kind
	}
	return ""
}
