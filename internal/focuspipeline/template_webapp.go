package focuspipeline

// WebAppTemplateID is the built-in template for making a web app from one
// sentence (docs/decisions/vibe-coder-identity.md): the first work kind of
// easy mode. The person who asked cannot read code, so the plan is written
// for them, nothing is pushed or opened as a pull request, and a run is
// proven by its verify commands, never by reading the code.
const WebAppTemplateID = "webapp"

// webAppStack is the stack the template fixes (vibe-coder-identity.md §7):
// a beginner cannot choose one, and the preview, the checks and the publish
// target all assume it.
const webAppStack = "The folder holds a web app on a fixed stack: React and TypeScript on Vite with the Cloudflare Vite " +
	"plugin (`vite.config.ts`, `wrangler.jsonc`); server code is the Worker in `worker/`, written with Hono; saved data is " +
	"the D1 database bound as `DB` in `wrangler.jsonc`; styling is Tailwind through `@tailwindcss/vite`. Build on what is " +
	"there: never replace the stack, scaffold a second project, or add another framework, database or host. If the folder " +
	"is still empty, the first task sets this stack up with Cloudflare's scaffolder for React and adds Hono and Tailwind."

// webAppNoGit keeps version control with TARS: the person who asked does
// not know git, and nobody reviews a diff.
const webAppNoGit = "Do not commit, branch, push or open a pull request: TARS saves each finished version itself."

const webAppPlanInstructions = "Do not edit files. Read what the folder already has, then propose a plan for the person who asked. " +
	"They cannot read code, so write the goal line and every task title in plain words about what they will see or be able " +
	"to do on the page (\"A sign-up form with name and email\"), never file names, libraries or technical terms; put the " +
	"technical detail in each task's \"done\" text. Keep the tasks few and in order. " + webAppStack + " " +
	"Verification commands, judged by their exit code, prove the app works without anyone reading the code: always " +
	"`npm run build`, and a check of the running page for every task that changes what the page shows or does. " +
	webAppNoGit + " The person approves the plan before anything is made."

const webAppBuildInstructions = "Make the approved tasks in order in this folder. " + webAppStack + " " +
	"Run `npm run build` before you report, and fix what it reports. If something only the person can decide comes up, ask " +
	"it as a decision in plain words, with the choices spelled out. " + webAppNoGit

const webAppReviewInstructions = "Review what was made as a careful tester. Each finding names the file and line, and its " +
	"title says in plain words what goes wrong for the person using the page (\"The total is wrong after removing an item\"). " +
	"Mark a finding low only when it is cosmetic: a low finding is set aside, every other one is fixed."

const webAppFixInstructions = "Fix only what the listed findings name, in the same stack, and keep everything else as it is. " +
	"Run `npm run build` before you report. " + webAppNoGit

// WebAppTemplate is plan, make, review, with no pull request: the work
// stays in the folder TARS made for it. It is marked WebUI, so a desk with
// the scripted-browser CLI teaches its plan to prove the page in verify.
func WebAppTemplate() Template {
	return Template{
		ID: WebAppTemplateID, Name: "Web app", Builtin: true, WebUI: true,
		Description: "Make a web app from one sentence: plan, make, review. No pull request.",
		Stages: []TemplateStage{
			{ID: StagePlan, Label: "Plan", Instructions: webAppPlanInstructions},
			{ID: "make", Kind: StageBuild, Label: "Make", Instructions: webAppBuildInstructions},
			{ID: "review", Kind: StageReview, Label: "Review", Instructions: webAppReviewInstructions, FixInstructions: webAppFixInstructions},
		},
	}
}
