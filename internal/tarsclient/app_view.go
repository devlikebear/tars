package tarsclient

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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
	userLineStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F8FAFC")).Bold(true)
	assistantStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4B5FD"))
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
	contentWidth := innerPanelWidth(width)
	content := renderPanelLines(m.chatLines, contentWidth, height-2, func(original, segment string) string {
		return formatChatLine(original, segment)
	})
	if strings.TrimSpace(content) == "" {
		content = "(no chat yet)"
	}
	title := chatTitleStyle.Render("Chat")
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func (m *tarsAppModel) renderStatusPanel(width, height int) string {
	content := renderPanelLines(m.statusLines, innerPanelWidth(width), height-2, nil)
	if strings.TrimSpace(content) == "" {
		content = "(trace off)"
	}
	title := statusTitleStyle.Render(fmt.Sprintf("Status  filter=%s", traceFilterName(m.state.chatTraceFilter)))
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func (m *tarsAppModel) renderNotifyPanel(width, height int) string {
	items := m.state.notifications.filtered()
	lines := make([]string, 0, len(items)+2)
	lines = append(lines, fmt.Sprintf("unread=%d total=%d filter=%s", m.state.notifications.unreadCount(), len(items), m.state.notifications.filterName()))
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
	content := renderPanelLines(lines, innerPanelWidth(width), height-2, nil)
	if strings.TrimSpace(content) == "" {
		content = "(no notifications)"
	}
	title := notifyTitleStyle.Render("Notifications")
	return panelStyle.Width(width).Height(height).Render(title + "\n" + content)
}

func innerPanelWidth(width int) int {
	if width <= 4 {
		return 1
	}
	return width - 4
}

func renderPanelLines(lines []string, width, limit int, formatter func(original, segment string) string) string {
	if width <= 0 {
		width = 1
	}
	if limit <= 0 {
		limit = 1
	}
	if len(lines) == 0 {
		return ""
	}
	visual := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, segment := range wrapVisualLine(line, width) {
			if formatter != nil {
				visual = append(visual, formatter(line, segment))
				continue
			}
			visual = append(visual, segment)
		}
	}
	start := 0
	if len(visual) > limit {
		start = len(visual) - limit
	}
	return strings.Join(visual[start:], "\n")
}

func wrapVisualLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	runes := []rune(line)
	parts := make([]string, 0, len(runes)/maxInt(1, width)+1)
	start := 0
	currentWidth := 0
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if rw <= 0 {
			rw = 1
		}
		if currentWidth > 0 && currentWidth+rw > width {
			parts = append(parts, string(runes[start:i]))
			start = i
			currentWidth = 0
		}
		currentWidth += rw
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

func formatChatLine(original, segment string) string {
	trimmed := strings.TrimSpace(original)
	switch {
	case strings.HasPrefix(trimmed, "SYSTEM > commands"):
		return systemLineStyle.Render(segment)
	case strings.HasSuffix(trimmed, ":") && (trimmed == "Session:" || trimmed == "Runtime:" || trimmed == "Chat:"):
		return helpSectionStyle.Render(segment)
	case strings.HasPrefix(original, "  /"):
		return helpCommandStyle.Render(segment)
	case strings.HasPrefix(trimmed, "You >"):
		return userLineStyle.Render(segment)
	case strings.HasPrefix(trimmed, "TARS >"):
		return assistantStyle.Render(segment)
	case strings.HasPrefix(trimmed, "ERROR >"):
		return errorLineStyle.Render(segment)
	default:
		return segment
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
