package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// The focus PR stages' facts (ADR §5): a read-only `gh pr view` run in the
// session's working folder, so gh picks the PR of the checked-out branch.
// gh is a soft dependency: missing, logged out, slow or unreadable all come
// back as ProbeUnavailable, and the developer passes the stage by hand. The
// probe never writes — pushing, opening and merging are the agent's turns
// after a gate.

// focusGHFields are the PR fields the probe asks for: the review facts,
// plus the head commit and branch (which PR this is; what was merged) and
// the author (whose comments are trusted).
const focusGHFields = "number,url,state,mergeStateStatus,statusCheckRollup,reviews,comments,headRefOid,headRefName,author"

// focusGHTimeout bounds one probe; gh reaches the network.
var focusGHTimeout = 20 * time.Second

// focusPRProber probes PR number in dir, or with number 0 the PR of the
// branch checked out there.
type focusPRProber func(ctx context.Context, dir string, number int) focuspipeline.PRProbe

// probeFocusPR is the server's prober: gh from the person's PATH.
func probeFocusPR(ctx context.Context, dir string, number int) focuspipeline.PRProbe {
	return runFocusGH(ctx, dir, "gh", number)
}

func unavailablePR(format string, args ...any) focuspipeline.PRProbe {
	return focuspipeline.PRProbe{Status: focuspipeline.ProbeUnavailable, Error: fmt.Sprintf(format, args...)}
}

func runFocusGH(ctx context.Context, dir, bin string, number int) focuspipeline.PRProbe {
	if strings.TrimSpace(dir) == "" {
		return unavailablePR("the session has no working folder")
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return unavailablePR("gh not found: install the GitHub CLI and run `gh auth login`")
	}
	ctx, cancel := context.WithTimeout(ctx, focusGHTimeout)
	defer cancel()
	args := []string{"pr", "view"}
	if number > 0 {
		// Pinned once found: never the branch's latest (maybe old) PR.
		args = append(args, strconv.Itoa(number))
	}
	args = append(args, "--json", focusGHFields)
	cmd := exec.CommandContext(ctx, path, args...) // NOSONAR: gh is the person's own toolchain, resolved from their PATH; the arguments are fixed.
	cmd.Dir = dir
	// Never wait on a prompt nobody can answer.
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1")
	cmd.WaitDelay = releaseKillGrace
	killProcessGroupOnCancel(cmd, releaseKillGrace)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return unavailablePR("gh timed out after %s", focusGHTimeout)
	case ctx.Err() != nil:
		return unavailablePR("probe cancelled")
	case err != nil:
		return classifyFocusGHFailure(stderr.String())
	}
	return parseFocusPRView(stdout.Bytes())
}

// classifyFocusGHFailure reads a failed gh run's stderr: no PR for the
// branch yet is a fact (ProbeNone); anything else makes the probe
// unavailable with gh's own words.
func classifyFocusGHFailure(stderr string) focuspipeline.PRProbe {
	msg := strings.TrimSpace(stderr)
	if strings.Contains(strings.ToLower(msg), "no pull requests found") {
		return focuspipeline.PRProbe{Status: focuspipeline.ProbeNone}
	}
	if msg == "" {
		msg = "gh failed"
	}
	if r := []rune(msg); len(r) > 500 {
		msg = string(r[:500]) + "…"
	}
	return unavailablePR("%s", msg)
}

type ghAuthor struct {
	Login string `json:"login"`
}

type ghPRView struct {
	Number            int      `json:"number"`
	URL               string   `json:"url"`
	State             string   `json:"state"`
	MergeStateStatus  string   `json:"mergeStateStatus"`
	HeadRefOid        string   `json:"headRefOid"`
	HeadRefName       string   `json:"headRefName"`
	Author            ghAuthor `json:"author"`
	StatusCheckRollup []struct {
		Typename   string `json:"__typename"`
		Name       string `json:"name"`
		Context    string `json:"context"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
		DetailsURL string `json:"detailsUrl"`
		TargetURL  string `json:"targetUrl"`
		StartedAt  string `json:"startedAt"`
		CreatedAt  string `json:"createdAt"`
	} `json:"statusCheckRollup"`
	Reviews []struct {
		ID                string   `json:"id"`
		Author            ghAuthor `json:"author"`
		AuthorAssociation string   `json:"authorAssociation"`
		State             string   `json:"state"`
		Body              string   `json:"body"`
	} `json:"reviews"`
	Comments []struct {
		ID                string   `json:"id"`
		Author            ghAuthor `json:"author"`
		AuthorAssociation string   `json:"authorAssociation"`
		Body              string   `json:"body"`
		URL               string   `json:"url"`
	} `json:"comments"`
}

// trustedCommenter: the PR's author, or an owner, member or collaborator
// of the repository. Anyone else's text never reaches a turn verbatim.
func trustedCommenter(association, login, prAuthor string) bool {
	switch strings.ToUpper(association) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	}
	return login != "" && strings.EqualFold(login, prAuthor)
}

// parseFocusPRView maps `gh pr view --json` output to a probe. Output it
// cannot read is unavailable, never a guess.
func parseFocusPRView(raw []byte) focuspipeline.PRProbe {
	var v ghPRView
	if err := json.Unmarshal(raw, &v); err != nil {
		return unavailablePR("gh returned unreadable json: %v", err)
	}
	if v.Number <= 0 {
		return unavailablePR("gh returned json without a PR number")
	}
	probe := focuspipeline.PRProbe{
		Status: focuspipeline.ProbeFound, Number: v.Number, URL: v.URL,
		State: strings.ToUpper(v.State), MergeState: v.MergeStateStatus,
		HeadOID: v.HeadRefOid, HeadRef: v.HeadRefName,
	}
	for _, c := range v.StatusCheckRollup {
		check := focuspipeline.PRCheck{Name: c.Name, URL: c.DetailsURL, StartedAt: c.StartedAt}
		if c.Typename == "StatusContext" {
			// A commit status: context, state and creation time; no
			// status/conclusion pair.
			check.Name, check.URL = c.Context, c.TargetURL
			if check.StartedAt == "" {
				check.StartedAt = c.CreatedAt
			}
			check.State = statusContextState(c.State)
		} else {
			check.State = checkRunState(c.Status, c.Conclusion)
		}
		probe.Checks = append(probe.Checks, check)
	}
	// A review asks for a change when it requests changes (its body may be
	// empty: the feedback is inline) or comments with a body; approvals and
	// dismissed reviews ask for nothing.
	for _, r := range v.Reviews {
		requested := r.State == "CHANGES_REQUESTED"
		if !requested && (r.State != "COMMENTED" || strings.TrimSpace(r.Body) == "") {
			continue
		}
		probe.Comments = append(probe.Comments, focuspipeline.PRComment{
			ID: r.ID, Author: r.Author.Login, Body: r.Body, ChangesRequested: requested,
			Trusted: trustedCommenter(r.AuthorAssociation, r.Author.Login, v.Author.Login),
		})
	}
	for _, c := range v.Comments {
		probe.Comments = append(probe.Comments, focuspipeline.PRComment{
			ID: c.ID, Author: c.Author.Login, Body: c.Body, URL: c.URL,
			Trusted: trustedCommenter(c.AuthorAssociation, c.Author.Login, v.Author.Login),
		})
	}
	return probe
}

func checkRunState(status, conclusion string) string {
	if !strings.EqualFold(status, "COMPLETED") {
		return focuspipeline.CheckPending
	}
	switch strings.ToUpper(conclusion) {
	case "SUCCESS", "NEUTRAL":
		return focuspipeline.CheckPass
	case "SKIPPED":
		return focuspipeline.CheckSkipped
	default:
		return focuspipeline.CheckFail
	}
}

func statusContextState(state string) string {
	switch strings.ToUpper(state) {
	case "SUCCESS", "NEUTRAL":
		return focuspipeline.CheckPass
	case "SKIPPED":
		return focuspipeline.CheckSkipped
	case "FAILURE", "ERROR":
		return focuspipeline.CheckFail
	default:
		// PENDING, EXPECTED, or a state this probe does not know: wait,
		// never invent a failure.
		return focuspipeline.CheckPending
	}
}
