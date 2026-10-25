package focuspipeline

import "strings"

// NoticePlanPushesOrOpensPR is the title of the notice card left on the
// plan gate when a task's title or done criteria pushes the branch or
// creates/opens the pull request itself (tars dogfooding fix #3): a build
// turn that does this opens the PR before the pipeline ever reaches the pr
// stage's draft gate (G3), which then finds the PR already open and has
// nothing left to draft or approve. The plan is not blocked — a developer
// who means it (a hotfix outside the normal flow) can still approve it —
// only flagged.
const NoticePlanPushesOrOpensPR = "a plan task pushes or opens a pull request"

// pushOrPRPatterns are substrings (already lowercase) that identify a task
// as pushing the branch or creating/opening a pull request, in English or
// Korean. "push" alone is included because that is how developers usually
// phrase it ("push and gh pr create", "push하고 PR 생성"); the notice is
// advisory, not a block, so a false positive only adds a card to dismiss.
var pushOrPRPatterns = []string{
	"push",
	"푸시",
	"gh pr create",
	"open a pull request",
	"open the pull request",
	"opens the pull request",
	"create a pull request",
	"creates the pull request",
	"open pr",
	"create pr",
	"pr 생성",
	"pr 열기",
	"풀 리퀘스트 생성",
	"풀 리퀘스트 열기",
}

// pushOrPRSafeMentions exempt a task that names the pr stage explicitly —
// describing the normal flow ("push and open the PR in the pr stage"), not
// proposing to do it itself.
var pushOrPRSafeMentions = []string{"pr stage", "pr 단계"}

// taskPushesOrOpensPR reports whether t's title or done criteria describes
// pushing the branch or creating/opening the pull request itself, rather
// than leaving that to the pr stage.
func taskPushesOrOpensPR(t PlanTask) bool {
	text := strings.ToLower(t.Title + " " + t.Done)
	for _, safe := range pushOrPRSafeMentions {
		if strings.Contains(text, safe) {
			return false
		}
	}
	for _, pat := range pushOrPRPatterns {
		if strings.Contains(text, pat) {
			return true
		}
	}
	return false
}

// tasksPushingOrOpeningPR is the titles of plan's tasks that push or open
// the pull request themselves, in plan order. Never nil; empty when none
// do.
func tasksPushingOrOpeningPR(tasks []PlanTask) []string {
	var out []string
	for _, t := range tasks {
		if taskPushesOrOpensPR(t) {
			out = append(out, strings.TrimSpace(t.Title))
		}
	}
	return out
}
