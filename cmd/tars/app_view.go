package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DD3FC")).
			Bold(true)
	systemLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A5B4FC")).
			Bold(true)
	helpSectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FDE68A")).
				Bold(true)
	helpCommandStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#67E8F9"))
	errorLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FCA5A5")).
			Bold(true)
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1)
	chatTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE")).Bold(true)
	statusTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F472B6")).Bold(true)
	notifyTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3E635")).Bold(true)
	inputStyle       = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F8FAFC")).
				Background(lipgloss.Color("#0F172A"))
)

func (m *tarsAppModel) View() string {
	width := m.width
	height := m.height
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 36
	}

	header := headerStyle.Render(
		fmt.Sprintf(
			"tars  server=%s  session=%s  trace=%s/%s  state=%s",
			strings.TrimSpace(m.chat.serverURL),
			displaySessionID(m.sessionID),
			onOff(m.state.chatTrace),
			traceFilterName(m.state.chatTraceFilter),
			chatStateName(m.inflight),
		),
	)
	footer := inputStyle.Render(m.input.View())

	contentHeight := height - 5
	if contentHeight < 9 {
		contentHeight = 9
	}
	if width < 120 {
		panelWidth := width - 2
		chatH := maxInt(3, contentHeight/2)
		statusH := maxInt(3, (contentHeight-chatH)/2)
		notifyH := maxInt(3, contentHeight-chatH-statusH)
		chat := m.renderChatPanel(panelWidth, chatH)
		status := m.renderStatusPanel(panelWidth, statusH)
		notify := m.renderNotifyPanel(panelWidth, notifyH)
		return lipgloss.JoinVertical(lipgloss.Left, header, chat, status, notify, footer)
	}

	leftWidth := int(float64(width) * 0.64)
	if leftWidth < 70 {
		leftWidth = 70
	}
	rightWidth := width - leftWidth - 1
	if rightWidth < 30 {
		rightWidth = 30
		leftWidth = width - rightWidth - 1
	}
	rightTop := maxInt(3, contentHeight/2)
	rightBottom := maxInt(3, contentHeight-rightTop)

	chat := m.renderChatPanel(leftWidth, contentHeight)
	status := m.renderStatusPanel(rightWidth, rightTop)
	notify := m.renderNotifyPanel(rightWidth, rightBottom)

	right := lipgloss.JoinVertical(lipgloss.Left, status, notify)
	row := lipgloss.JoinHorizontal(lipgloss.Top, chat, right)
	return lipgloss.JoinVertical(lipgloss.Left, header, row, footer)
}

func (m *tarsAppModel) renderChatPanel(width, height int) string {
	content := renderChatLines(m.chatLines, height-2)
	if strings.TrimSpace(content) == "" {
		content = "(no chat yet)"
	}
	title := chatTitleStyle.Render("Chat")
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func (m *tarsAppModel) renderStatusPanel(width, height int) string {
	content := joinTailLines(m.statusLines, height-2)
	if strings.TrimSpace(content) == "" {
		content = "(trace off)"
	}
	title := statusTitleStyle.Render(fmt.Sprintf("Status  filter=%s", traceFilterName(m.state.chatTraceFilter)))
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func (m *tarsAppModel) renderNotifyPanel(width, height int) string {
	items := m.state.notifications.filtered()
	lines := make([]string, 0, len(items)+2)
	lines = append(lines, fmt.Sprintf("unread=%d filter=%s", len(items), m.state.notifications.filterName()))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "(untitled)"
		}
		msg := strings.TrimSpace(item.Message)
		prefix := fmt.Sprintf("[%s/%s] %s", strings.TrimSpace(item.Category), strings.TrimSpace(item.Severity), title)
		if msg != "" {
			lines = append(lines, prefix+" | "+msg)
			continue
		}
		lines = append(lines, prefix)
	}
	content := joinTailLines(lines, height-2)
	if strings.TrimSpace(content) == "" {
		content = "(no notifications)"
	}
	title := notifyTitleStyle.Render("Notifications")
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func joinTailLines(lines []string, limit int) string {
	if limit <= 0 {
		limit = 1
	}
	if len(lines) == 0 {
		return ""
	}
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	return strings.Join(lines[start:], "\n")
}

func renderChatLines(lines []string, limit int) string {
	if limit <= 0 {
		limit = 1
	}
	if len(lines) == 0 {
		return ""
	}
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	styled := make([]string, 0, len(lines)-start)
	for _, line := range lines[start:] {
		styled = append(styled, formatChatLine(line))
	}
	return strings.Join(styled, "\n")
}

func formatChatLine(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "SYSTEM > commands"):
		return systemLineStyle.Render(line)
	case strings.HasSuffix(trimmed, ":") && (trimmed == "Session:" || trimmed == "Runtime:" || trimmed == "Chat:"):
		return helpSectionStyle.Render(line)
	case strings.HasPrefix(line, "  /"):
		return helpCommandStyle.Render(line)
	case strings.HasPrefix(trimmed, "ERROR >"):
		return errorLineStyle.Render(line)
	default:
		return line
	}
}

func traceFilterName(filter string) string {
	name := strings.ToLower(strings.TrimSpace(filter))
	if name == "" {
		return "all"
	}
	return name
}

func displaySessionID(sessionID string) string {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return "(new)"
	}
	return id
}

func chatStateName(inflight bool) string {
	if inflight {
		return "busy"
	}
	return "idle"
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
