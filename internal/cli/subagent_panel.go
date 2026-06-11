package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
)

const (
	subagentPreviewHead = 2
	subagentPreviewTail = 2
	subagentMaxLines    = 200
)

type subagentPanel struct {
	alias           string
	skill           string
	id              string
	transcriptIdx   int
	status          string
	usageSummary    string
	answer          string
	lines           []string
	reasoningBuf    strings.Builder
	textBuf         strings.Builder
	reasoningBlocks []string
	textBlocks      []string
	textStreamed    bool
	expanded        bool
	completed       bool
}

type subagentPanels struct {
	order []string
	byID  map[string]*subagentPanel
}

func (m *chatTUI) updateSubagent(info event.Subagent) {
	m.applySubagentInfo(info)
}

func (m *chatTUI) updateSubagentEvent(info *event.Subagent) *subagentPanel {
	if info == nil {
		return nil
	}
	return m.applySubagentInfo(*info)
}

func (m *chatTUI) applySubagentInfo(info event.Subagent) *subagentPanel {
	sub := m.panelForSubagentInfo(info)
	if sub == nil {
		return nil
	}
	if info.Alias != "" {
		sub.alias = info.Alias
	}
	if info.Skill != "" {
		sub.skill = info.Skill
	}
	if info.ID != "" {
		sub.id = info.ID
	}
	if info.State != "" {
		sub.status = string(info.State)
		sub.completed = info.State == event.SubagentCompleted || info.State == event.SubagentFailed || info.State == event.SubagentCanceled
		if sub.completed {
			sub.expanded = true
		}
	}
	if sub.completed {
		m.flushSubagentBufs(sub)
	}
	m.subagent = sub
	return sub
}

func (m *chatTUI) panelForSubagentInfo(info event.Subagent) *subagentPanel {
	if info.ID == "" {
		if m.subagent == nil || (info.State == event.SubagentRunning && m.subagent.completed) {
			m.subagent = &subagentPanel{transcriptIdx: -1}
		}
		return m.subagent
	}
	if m.subagents.byID == nil {
		m.subagents.byID = map[string]*subagentPanel{}
	}
	sub := m.subagents.byID[info.ID]
	if sub == nil {
		sub = &subagentPanel{id: info.ID, transcriptIdx: -1}
		m.subagents.byID[info.ID] = sub
		m.subagents.order = append([]string{info.ID}, m.subagents.order...)
	}
	return sub
}

func (m *chatTUI) ensureSubagentTranscript(sub *subagentPanel) {
	if sub == nil {
		return
	}
	if sub.transcriptIdx >= 0 && sub.transcriptIdx < len(m.transcript) {
		return
	}
	m.commitSpacer()
	sub.transcriptIdx = len(m.transcript)
	m.commitLine(m.renderSubagentTranscriptBlock(sub))
}

func (m *chatTUI) refreshSubagentTranscript(sub *subagentPanel) {
	if sub == nil {
		return
	}
	m.flushSubagentBufs(sub)
	if sub.transcriptIdx < 0 || sub.transcriptIdx >= len(m.transcript) {
		m.ensureSubagentTranscript(sub)
		return
	}
	m.transcript[sub.transcriptIdx] = m.renderSubagentTranscriptBlock(sub)
	m.transcriptDirty = true
}

func (m *chatTUI) flushSubagentBufs(sub *subagentPanel) {
	if sub == nil {
		return
	}
	if sub.reasoningBuf.Len() > 0 {
		text := strings.TrimSpace(sub.reasoningBuf.String())
		sub.reasoningBuf.Reset()
		if text != "" {
			m.appendSubagentReasoningBlock(sub, text)
		}
	}
	if sub.textBuf.Len() > 0 {
		text := strings.TrimSpace(sub.textBuf.String())
		sub.textBuf.Reset()
		if text != "" {
			m.appendSubagentTextBlock(sub, text)
		}
	}
}

func (m *chatTUI) appendSubagentReasoningBlock(sub *subagentPanel, text string) {
	text = strings.TrimSpace(text)
	if sub == nil || text == "" {
		return
	}
	block := normalizeSubagentStreamText(text)
	if len(sub.reasoningBlocks) == 0 {
		sub.lines = capAppend(sub.lines, dim("  ▎ "+block), subagentMaxLines)
		sub.reasoningBlocks = append(sub.reasoningBlocks, block)
		return
	}
	idx := len(sub.reasoningBlocks) - 1
	if len(sub.lines) > 0 {
		last := len(sub.lines) - 1
		merged := joinSubagentStreamText(sub.reasoningBlocks[idx], block)
		sub.reasoningBlocks[idx] = merged
		sub.lines[last] = dim("  ▎ " + merged)
	}
}

func (m *chatTUI) appendSubagentTextBlock(sub *subagentPanel, text string) {
	text = strings.TrimSpace(text)
	if sub == nil || text == "" {
		return
	}
	block := normalizeSubagentStreamText(text)
	if len(sub.textBlocks) == 0 {
		sub.lines = capAppend(sub.lines, block, subagentMaxLines)
		sub.textBlocks = append(sub.textBlocks, block)
		return
	}
	idx := len(sub.textBlocks) - 1
	if len(sub.lines) > 0 {
		last := len(sub.lines) - 1
		merged := joinSubagentStreamText(sub.textBlocks[idx], block)
		sub.textBlocks[idx] = merged
		sub.lines[last] = merged
	}
}

func normalizeSubagentStreamText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func joinSubagentStreamText(prev, next string) string {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if prev == "" {
		return next
	}
	if next == "" {
		return prev
	}
	if subagentNeedsSpace(prev, next) {
		return prev + " " + next
	}
	return prev + next
}

func subagentNeedsSpace(prev, next string) bool {
	a, okA := lastRune(prev)
	b, okB := firstRune(next)
	if !okA || !okB {
		return false
	}
	if unicode.IsPunct(b) || unicode.IsSymbol(b) {
		return false
	}
	if isASCIIRunRune(a) && isASCIIRunRune(b) {
		return true
	}
	if unicode.IsSpace(a) || unicode.IsSpace(b) {
		return false
	}
	if (unicode.IsLetter(a) || unicode.IsDigit(a)) && (unicode.IsLetter(b) || unicode.IsDigit(b)) && (a <= unicode.MaxASCII || b <= unicode.MaxASCII) {
		return true
	}
	return false
}

func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func lastRune(s string) (rune, bool) {
	var last rune
	ok := false
	for _, r := range s {
		last = r
		ok = true
	}
	return last, ok
}

func isASCIIRunRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
}

func resetSubagentStreamBlocks(sub *subagentPanel) {
	if sub == nil {
		return
	}
	sub.textBlocks = nil
	sub.reasoningBlocks = nil
}

func (m *chatTUI) appendSubagentPresentationEvent(sub *subagentPanel, kind event.Kind, text, toolName, args, errText string) {
	if sub == nil {
		return
	}
	switch kind {
	case event.Reasoning:
		if sub.textBuf.Len() > 0 {
			m.flushSubagentBufs(sub)
		}
		sub.textBlocks = nil
		sub.reasoningBuf.WriteString(text)
	case event.Text:
		if sub.reasoningBuf.Len() > 0 {
			m.flushSubagentBufs(sub)
		}
		sub.reasoningBlocks = nil
		sub.textStreamed = true
		sub.textBuf.WriteString(text)
	case event.Message:
		if strings.TrimSpace(text) != "" {
			sub.answer = strings.TrimSpace(text)
		}
		if !sub.textStreamed && strings.TrimSpace(text) != "" {
			sub.textBuf.WriteString(text)
		}
		sub.textStreamed = false
		m.flushSubagentBufs(sub)
		resetSubagentStreamBlocks(sub)
	case event.ToolDispatch:
		m.flushSubagentBufs(sub)
		sub.textStreamed = false
		resetSubagentStreamBlocks(sub)
		sub.lines = capAppend(sub.lines, formatSubagentDispatchLine(toolName, args), subagentMaxLines)
	case event.ToolResult:
		if errText != "" {
			m.flushSubagentBufs(sub)
			sub.textStreamed = false
			resetSubagentStreamBlocks(sub)
			sub.lines = capAppend(sub.lines, formatSubagentBlockedLine(toolName, errText), subagentMaxLines)
		}
	}
}

func (m chatTUI) renderSubagentTranscriptBlock(sub *subagentPanel) string {
	if sub == nil {
		return ""
	}
	m.flushSubagentBufs(sub)
	state := event.SubagentState(strings.TrimSpace(sub.status))
	if state == "" {
		state = event.SubagentRunning
	}
	status := subagentStateLabel(state)
	title := strings.TrimSpace(sub.alias)
	if title == "" {
		if sub.skill != "" {
			title = "/" + sub.skill
		} else {
			title = i18n.M.SubagentTitle
		}
	}

	var b strings.Builder
	parts := []string{bold(accent("[" + i18n.M.SkillPickerSubagent + "]")), bold(green(title))}
	if sub.skill != "" {
		parts = append(parts, bold(yellow("/"+sub.skill)))
	}
	head := []string{strings.Join(parts, " "), status}
	if sub.usageSummary != "" {
		head = append(head, dim(sub.usageSummary))
	}
	toggle := dim(i18n.M.SubagentToggleExpand)
	if sub.expanded {
		toggle = dim(i18n.M.SubagentToggleCollapse)
	}
	head = append(head, toggle)
	b.WriteString(ansi.Truncate(strings.Join(head, " · "), m.subagentDisplayWidth(), "…"))
	b.WriteByte('\n')

	var meta []string
	if sub.id != "" {
		meta = append(meta, "id "+sub.id)
	}
	if len(meta) > 0 && sub.expanded {
		b.WriteString(dim(ansi.Truncate("  "+strings.Join(meta, " · "), m.subagentDisplayWidth(), "…")))
		b.WriteByte('\n')
	}

	lines := subagentBodyLines(sub)
	hadLines := len(lines) > 0
	if !sub.expanded && !sub.completed {
		lines = collapseSubagentLines(lines)
	}
	lines = m.wrapSubagentDisplayLines(lines)
	if len(lines) == 0 {
		if !hadLines {
			b.WriteString(dim("  " + i18n.M.SubagentWaitingForActivity))
		}
	} else {
		for i, line := range lines {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
	}
	return todoPanelStyle.Width(max(m.width, 10)).Render(strings.TrimRight(b.String(), "\n"))
}

func capAppend(lines []string, line string, maxLen int) []string {
	if len(lines) >= maxLen {
		return lines
	}
	return append(lines, line)
}

func collapseSubagentLines(lines []string) []string {
	if len(lines) <= subagentPreviewHead+subagentPreviewTail+1 {
		return append([]string(nil), lines...)
	}
	collapsed := append([]string(nil), lines[:subagentPreviewHead]...)
	omitted := len(lines) - subagentPreviewHead - subagentPreviewTail
	collapsed = append(collapsed, dim(fmt.Sprintf(i18n.M.SubagentLinesOmittedFmt, omitted)))
	collapsed = append(collapsed, lines[len(lines)-subagentPreviewTail:]...)
	return collapsed
}

func subagentBodyLines(sub *subagentPanel) []string {
	if sub == nil {
		return nil
	}
	lines := append([]string(nil), sub.lines...)
	if strings.TrimSpace(sub.answer) != "" && sub.completed {
		if len(lines) > 0 {
			lines = append(lines, dim("  ---"))
		}
		lines = append(lines, green(i18n.M.SubagentAnswerLabel))
		lines = append(lines, strings.Split(strings.TrimSpace(sub.answer), "\n")...)
	}
	return lines
}

func subagentUsageSummary(u *provider.Usage, p *provider.Pricing) string {
	if u == nil || u.TotalTokens == 0 {
		return ""
	}
	parts := []string{shortTokens(u.TotalTokens) + " tok"}
	if u.ReasoningTokens > 0 {
		parts = append(parts, shortTokens(u.ReasoningTokens)+" reasoning")
	}
	if p != nil {
		if cost := p.Cost(u); cost > 0 {
			parts = append(parts, fmt.Sprintf("%s%.4f", p.Symbol(), cost))
		}
	}
	return strings.Join(parts, " · ")
}
