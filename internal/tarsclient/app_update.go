package tarsclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func runTUI(
	ctx context.Context,
	stdin io.Reader,
	stdout io.Writer,
	chat chatClient,
	runtime runtimeClient,
	session string,
	verbose bool,
) error {
	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := newTarsAppModel(appCtx, cancel, chat, runtime, session, verbose)
	program := tea.NewProgram(
		model,
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
		tea.WithAltScreen(),
	)
	_, err := program.Run()
	return err
}

func (m *tarsAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.input.Width = maxInt(20, typed.Width-8)
		return m, nil

	case tea.KeyMsg:
		switch typed.Type {
		case tea.KeyCtrlC:
			m.cancelInFlight("interrupt")
			m.cancel()
			return m, tea.Quit
		case tea.KeyEsc:
			if m.inflight {
				m.cancelInFlight("stream canceled")
			}
			m.input.SetValue("")
			return m, nil
		case tea.KeyUp:
			m.historyPrev()
			return m, nil
		case tea.KeyDown:
			m.historyNext()
			return m, nil
		case tea.KeyTab:
			next, changed := completeCommandInput(m.input.Value())
			if changed {
				m.input.SetValue(next)
				m.input.CursorEnd()
			}
			return m, nil
		case tea.KeyEnter:
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			m.historyPos = len(m.history)
			m.historyDraft = ""
			if line == "" {
				return m, nil
			}
			m.pushHistory(line)
			return m.handleSubmit(line)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(typed)
		return m, cmd

	case chatDeltaMsg:
		m.appendAssistantChunk(typed.chunk)
		return m, waitAsyncMsg(m.asyncCh)

	case chatStatusMsg:
		if shouldRenderTraceEvent(m.state, typed.event) {
			label := formatChatStatusEvent(typed.event, m.verbose)
			if strings.TrimSpace(label) != "" {
				m.appendStatusLine(label)
			}
		}
		return m, waitAsyncMsg(m.asyncCh)

	case chatDoneMsg:
		m.inflight = false
		m.inflightCancel = nil
		m.assistantLine = -1
		if strings.TrimSpace(typed.result.SessionID) != "" && strings.TrimSpace(typed.result.SessionID) != strings.TrimSpace(m.sessionID) {
			m.sessionID = strings.TrimSpace(typed.result.SessionID)
			m.appendStatusLine("session=" + m.sessionID)
		}
		if typed.err != nil {
			if !(m.inflightCanceled && errors.Is(typed.err, context.Canceled)) {
				m.appendChatLine("ERROR > " + formatRuntimeError(typed.err))
			}
		}
		m.inflightCanceled = false
		return m, waitAsyncMsg(m.asyncCh)

	case notificationEventMsg:
		m.state.notifications.add(typed.event)
		return m, waitAsyncMsg(m.asyncCh)

	case notificationErrorMsg:
		m.appendStatusLine("notification stream error: " + formatRuntimeError(typed.err))
		return m, waitAsyncMsg(m.asyncCh)
	}

	return m, nil
}

func (m *tarsAppModel) handleSubmit(line string) (tea.Model, tea.Cmd) {
	if isExitCommand(line) {
		m.cancelInFlight("interrupt")
		m.cancel()
		return m, tea.Quit
	}

	if strings.HasPrefix(strings.TrimSpace(line), "/") {
		stdoutBuf := &bytes.Buffer{}
		stderrBuf := &bytes.Buffer{}
		handled, nextSession, err := executeCommandWithState(
			m.ctx,
			m.runtime,
			line,
			m.sessionID,
			stdoutBuf,
			stderrBuf,
			&m.state,
		)
		out := strings.TrimSpace(stdoutBuf.String())
		if out != "" {
			m.appendChatBlock(out)
		}
		if errOut := strings.TrimSpace(stderrBuf.String()); errOut != "" {
			for _, ln := range strings.Split(errOut, "\n") {
				trim := strings.TrimSpace(ln)
				if strings.HasPrefix(trim, "session=") {
					m.appendStatusLine(trim)
				} else if trim != "" {
					m.appendStatusLine(trim)
				}
			}
		}
		if err != nil {
			m.appendChatLine("ERROR > " + formatRuntimeError(err))
			return m, nil
		}
		if handled {
			if strings.TrimSpace(nextSession) != "" {
				m.sessionID = strings.TrimSpace(nextSession)
			}
			return m, nil
		}
	}

	if m.inflight {
		m.appendStatusLine("chat stream already running")
		return m, nil
	}

	m.appendChatLine("You > " + line)
	m.setAssistantLine()

	reqCtx, cancel := context.WithCancel(m.ctx)
	m.inflight = true
	m.inflightCancel = cancel
	m.inflightCanceled = false
	m.startChatStream(reqCtx, chatRequest{Message: line, SessionID: m.sessionID})
	return m, nil
}

func isExitCommand(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "exit", "quit", "/exit", "/quit":
		return true
	default:
		return false
	}
}

func (m *tarsAppModel) startChatStream(ctx context.Context, req chatRequest) {
	asyncCh := m.asyncCh
	client := m.chat
	go func() {
		result, err := client.stream(ctx, req,
			func(evt chatEvent) {
				select {
				case asyncCh <- chatStatusMsg{event: evt}:
				case <-ctx.Done():
				}
			},
			func(chunk string) {
				select {
				case asyncCh <- chatDeltaMsg{chunk: chunk}:
				case <-ctx.Done():
				}
			},
		)
		select {
		case asyncCh <- chatDoneMsg{result: result, err: err}:
		case <-ctx.Done():
			asyncCh <- chatDoneMsg{result: result, err: err}
		}
	}()
}

func (m *tarsAppModel) startEventStream() {
	ctx := m.ctx
	asyncCh := m.asyncCh
	client := m.eventTrace
	m.appendStatusLine("notification stream connecting")
	go client.consume(ctx,
		func(evt notificationMessage) {
			select {
			case asyncCh <- notificationEventMsg{event: evt}:
			case <-ctx.Done():
			}
		},
		func(err error) {
			select {
			case asyncCh <- notificationErrorMsg{err: err}:
			case <-ctx.Done():
			}
		},
	)
}

func (m *tarsAppModel) cancelInFlight(reason string) {
	if !m.inflight || m.inflightCancel == nil {
		return
	}
	m.inflightCanceled = true
	m.appendStatusLine(reason)
	m.inflightCancel()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
