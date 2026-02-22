package tarsclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	maxChatLines   = 800
	maxStatusLines = 400
)

var slashCommands = []string{
	"/help",
	"/session",
	"/resume",
	"/new",
	"/sessions",
	"/history",
	"/export",
	"/search",
	"/status",
	"/providers",
	"/models",
	"/model",
	"/whoami",
	"/health",
	"/compact",
	"/heartbeat",
	"/skills",
	"/plugins",
	"/mcp",
	"/reload",
	"/agents",
	"/runs",
	"/run",
	"/cancel-run",
	"/spawn",
	"/gateway",
	"/browser",
	"/vault",
	"/channels",
	"/telegram",
	"/cron",
	"/project",
	"/usage",
	"/notify",
	"/trace",
	"/quit",
}

type chatDeltaMsg struct {
	chunk string
}

type chatStatusMsg struct {
	event chatEvent
}

type chatDoneMsg struct {
	result chatResult
	err    error
}

type notificationEventMsg struct {
	event notificationMessage
}

type notificationErrorMsg struct {
	err error
}

type notificationHistoryMsg struct {
	history eventsHistoryInfo
	err     error
}

type statusBootstrapMsg struct {
	status statusInfo
	err    error
}

type tarsAppModel struct {
	ctx    context.Context
	cancel context.CancelFunc

	chat       chatClient
	runtime    runtimeClient
	eventTrace eventStreamClient

	sessionID string
	verbose   bool

	state localRuntimeState

	width  int
	height int

	input textinput.Model

	chatLines   []string
	statusLines []string

	history      []string
	historyPos   int
	historyDraft string

	inflight         bool
	inflightCancel   context.CancelFunc
	inflightCanceled bool
	assistantLine    int

	asyncCh chan tea.Msg
}

func newTarsAppModel(
	ctx context.Context,
	cancel context.CancelFunc,
	chat chatClient,
	runtime runtimeClient,
	session string,
	verbose bool,
) *tarsAppModel {
	input := textinput.New()
	input.Placeholder = "Type message and press Enter"
	input.Focus()
	input.CharLimit = 0
	input.Prompt = "You > "
	input.Width = 80

	notifications := newNotificationCenter(300)
	model := &tarsAppModel{
		ctx:        ctx,
		cancel:     cancel,
		chat:       chat,
		runtime:    runtime,
		eventTrace: newEventStreamClient(runtime),
		sessionID:  strings.TrimSpace(session),
		verbose:    verbose,
		state: localRuntimeState{
			notifications:   notifications,
			chatTrace:       true,
			chatTraceFilter: "all",
		},
		input:         input,
		chatLines:     make([]string, 0, maxChatLines),
		statusLines:   make([]string, 0, maxStatusLines),
		history:       make([]string, 0, 200),
		historyPos:    0,
		assistantLine: -1,
		asyncCh:       make(chan tea.Msg, 256),
	}
	model.appendChatLine("SYSTEM > /help to show commands")
	model.appendStatusLine("trace=on filter=all")
	if model.sessionID != "" {
		model.appendStatusLine("session=" + model.sessionID)
	}
	return model
}

func (m *tarsAppModel) Init() tea.Cmd {
	m.historyPos = len(m.history)
	m.startMainSessionBootstrap()
	m.startEventStream()
	return waitAsyncMsg(m.asyncCh)
}

func (m *tarsAppModel) appendChatLine(line string) {
	text := strings.TrimRight(strings.ReplaceAll(line, "\r\n", "\n"), "\n")
	if text == "" {
		return
	}
	for _, part := range strings.Split(text, "\n") {
		m.chatLines = append(m.chatLines, part)
	}
	if len(m.chatLines) > maxChatLines {
		m.chatLines = m.chatLines[len(m.chatLines)-maxChatLines:]
	}
}

func (m *tarsAppModel) appendChatBlock(block string) {
	text := strings.TrimSpace(strings.ReplaceAll(block, "\r\n", "\n"))
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		m.appendChatLine(line)
	}
}

func (m *tarsAppModel) appendStatusLine(line string) {
	text := strings.TrimSpace(strings.ReplaceAll(line, "\r\n", "\n"))
	if text == "" {
		return
	}
	timestamp := time.Now().Format("15:04:05")
	m.statusLines = append(m.statusLines, fmt.Sprintf("%s %s", timestamp, text))
	if len(m.statusLines) > maxStatusLines {
		m.statusLines = m.statusLines[len(m.statusLines)-maxStatusLines:]
	}
}

func (m *tarsAppModel) setAssistantLine() {
	m.chatLines = append(m.chatLines, "TARS > ")
	m.assistantLine = len(m.chatLines) - 1
	if len(m.chatLines) > maxChatLines {
		offset := len(m.chatLines) - maxChatLines
		m.chatLines = m.chatLines[offset:]
		if m.assistantLine >= 0 {
			m.assistantLine -= offset
		}
		if m.assistantLine < 0 {
			m.assistantLine = -1
		}
	}
}

func (m *tarsAppModel) appendAssistantChunk(chunk string) {
	if chunk == "" {
		return
	}
	if m.assistantLine < 0 || m.assistantLine >= len(m.chatLines) {
		m.chatLines = append(m.chatLines, "TARS > "+chunk)
		m.assistantLine = len(m.chatLines) - 1
		return
	}
	m.chatLines[m.assistantLine] += chunk
}

func (m *tarsAppModel) pushHistory(value string) {
	line := strings.TrimSpace(value)
	if line == "" {
		return
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == line {
		m.historyPos = len(m.history)
		m.historyDraft = ""
		return
	}
	m.history = append(m.history, line)
	if len(m.history) > 500 {
		m.history = m.history[len(m.history)-500:]
	}
	m.historyPos = len(m.history)
	m.historyDraft = ""
}

func (m *tarsAppModel) historyPrev() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos == len(m.history) {
		m.historyDraft = m.input.Value()
	}
	if m.historyPos > 0 {
		m.historyPos--
	}
	m.input.SetValue(m.history[m.historyPos])
	m.input.CursorEnd()
}

func (m *tarsAppModel) historyNext() {
	if len(m.history) == 0 {
		return
	}
	if m.historyPos < len(m.history)-1 {
		m.historyPos++
		m.input.SetValue(m.history[m.historyPos])
		m.input.CursorEnd()
		return
	}
	m.historyPos = len(m.history)
	m.input.SetValue(m.historyDraft)
	m.input.CursorEnd()
}

func classifyStatusCategory(evt chatEvent) string {
	phase := strings.TrimSpace(evt.Phase)
	switch phase {
	case "loop_start", "before_llm", "llm_stream", "after_llm", "loop_end", "calling_llm", "streaming":
		return "llm"
	case "before_tool_call", "after_tool_call":
		return "tool"
	case "error":
		return "error"
	default:
		if strings.EqualFold(strings.TrimSpace(evt.Type), "error") {
			return "error"
		}
		return "system"
	}
}

func shouldRenderTraceEvent(state localRuntimeState, evt chatEvent) bool {
	if !state.chatTrace {
		return false
	}
	filter := strings.ToLower(strings.TrimSpace(state.chatTraceFilter))
	if filter == "" || filter == "all" {
		return true
	}
	return classifyStatusCategory(evt) == filter
}

func waitAsyncMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func completeCommandInput(current string) (string, bool) {
	value := strings.TrimRight(strings.ReplaceAll(current, "\r\n", "\n"), "\n")
	if strings.TrimSpace(value) == "" || !strings.HasPrefix(strings.TrimSpace(value), "/") {
		return current, false
	}
	hasTrailingSpace := strings.HasSuffix(value, " ")
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return current, false
	}
	if len(fields) == 1 && !hasTrailingSpace {
		return completeSingleToken(value, slashCommands)
	}
	switch fields[0] {
	case "/gateway":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"status", "reload", "restart", "summary", "runs", "channels"})
	case "/browser":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"status", "profiles", "login", "check", "run"})
	case "/vault":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"status"})
	case "/telegram":
		if len(fields) >= 2 && strings.TrimSpace(fields[1]) == "pairing" {
			return completeByPosition(value, fields, hasTrailingSpace, 2, []string{"approve"})
		}
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"pairings", "pairing"})
	case "/trace":
		if len(fields) == 1 && hasTrailingSpace {
			return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"on", "off", "filter"})
		}
		if len(fields) >= 2 && strings.TrimSpace(fields[1]) == "filter" {
			return completeByPosition(value, fields, hasTrailingSpace, 2, []string{"all", "llm", "tool", "error", "system"})
		}
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"on", "off", "filter"})
	case "/cron":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"list", "get", "runs", "add", "run", "delete", "enable", "disable"})
	case "/project":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"list", "get", "create", "activate", "archive"})
	case "/usage":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"summary", "limits", "set-limits"})
	case "/notify":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"list", "filter", "open", "clear"})
	case "/agents":
		return completeByPosition(value, fields, hasTrailingSpace, 1, []string{"--detail", "-d"})
	default:
		return current, false
	}
}

func completeByPosition(current string, fields []string, hasTrailingSpace bool, position int, options []string) (string, bool) {
	index := position
	if hasTrailingSpace {
		index = len(fields)
	} else {
		index = len(fields) - 1
	}
	if index != position {
		return current, false
	}
	prefix := ""
	if !hasTrailingSpace && len(fields) > position {
		prefix = fields[position]
	}
	match, changed := completeToken(prefix, options)
	if !changed {
		return current, false
	}
	parts := make([]string, 0, position+2)
	parts = append(parts, fields[:position]...)
	parts = append(parts, match)
	return strings.Join(parts, " ") + " ", true
}

func completeSingleToken(current string, options []string) (string, bool) {
	token := strings.TrimSpace(current)
	match, changed := completeToken(token, options)
	if !changed {
		return current, false
	}
	return match + " ", true
}

func completeToken(prefix string, options []string) (string, bool) {
	prefix = strings.TrimSpace(prefix)
	matches := make([]string, 0, len(options))
	for _, opt := range options {
		if strings.HasPrefix(opt, prefix) {
			matches = append(matches, opt)
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) == 1 {
		if matches[0] == prefix {
			return "", false
		}
		return matches[0], true
	}
	common := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, common) && common != "" {
			common = common[:len(common)-1]
		}
		if common == "" {
			break
		}
	}
	if common == "" || common == prefix {
		return "", false
	}
	return common, true
}
