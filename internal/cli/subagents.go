package cli

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

// subagentPicker is the interactive /subagents surface. Stage 0 lists retained
// runs newest-first from the controller; stage 1 shows controller-owned detail
// for the selected run.
type subagentPicker struct {
	allItems     []control.SubagentSummary
	items        []control.SubagentSummary
	sel          int
	filter       string
	filterEdit   bool
	detail       *control.SubagentDetail
	detailID     string
	detailScroll int
}

func (m *chatTUI) openSubagents() {
	items := m.ctrl.ListSubagents()
	m.completion = completion{}
	m.subagentPicker = &subagentPicker{}
	m.subagentPicker.setItems(items, "")
}

func (p *subagentPicker) closeDetail() {
	p.detail = nil
	p.detailID = ""
	p.detailScroll = 0
}

func (p *subagentPicker) selectedID() string {
	if len(p.items) == 0 || p.sel < 0 || p.sel >= len(p.items) {
		return ""
	}
	return p.items[p.sel].ID
}

func (p *subagentPicker) setItems(items []control.SubagentSummary, selectedID string) {
	p.allItems = append([]control.SubagentSummary(nil), items...)
	p.items = filterSubagentSummaries(items, p.filter)
	if len(p.items) == 0 {
		p.sel = 0
		return
	}
	if selectedID == "" && p.sel >= 0 && p.sel < len(p.items) {
		selectedID = p.items[p.sel].ID
	}
	for i, item := range p.items {
		if item.ID == selectedID {
			p.sel = i
			return
		}
	}
	p.sel = clampSel(p.sel, p.items)
}

func filterSubagentSummaries(items []control.SubagentSummary, query string) []control.SubagentSummary {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return append([]control.SubagentSummary(nil), items...)
	}
	out := make([]control.SubagentSummary, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Alias), q) ||
			strings.Contains(strings.ToLower(item.Skill), q) ||
			strings.Contains(strings.ToLower(string(item.State)), q) ||
			strings.Contains(strings.ToLower(control.SubagentStateText(item.State)), q) {
			out = append(out, item)
		}
	}
	return out
}

func (m *chatTUI) refreshSubagentItems(p *subagentPicker, selectedID string) {
	if p == nil || m.ctrl == nil {
		return
	}
	p.setItems(m.ctrl.ListSubagents(), selectedID)
}

func (m chatTUI) handleSubagentsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.subagentPicker
	if p == nil {
		return m, nil
	}
	key := msg.Key()
	switch {
	case msg.String() == "ctrl+d":
		return m, tea.Quit
	case p.detail == nil && p.filterEdit:
		switch key.Code {
		case tea.KeyEsc:
			p.filterEdit = false
		case tea.KeyEnter:
			p.filterEdit = false
		case tea.KeyBackspace, tea.KeyDelete:
			selectedID := p.selectedID()
			p.filter = trimLastRune(p.filter)
			p.setItems(p.allItems, selectedID)
		case tea.KeySpace:
			selectedID := p.selectedID()
			p.filter += " "
			p.setItems(p.allItems, selectedID)
		default:
			if text := key.Text; text != "" {
				selectedID := p.selectedID()
				p.filter += text
				p.setItems(p.allItems, selectedID)
			}
		}
		return m, nil
	case key.Code == tea.KeyEsc:
		m.subagentPicker = nil
	case key.Text == "q":
		m.subagentPicker = nil
	case p.detail == nil && key.Text == "/":
		p.filterEdit = true
	case key.Code == tea.KeyUp || key.Text == "k":
		if p.detail != nil {
			if p.detailScroll > 0 {
				p.detailScroll--
			}
		} else if p.sel > 0 {
			p.sel--
		}
	case key.Code == tea.KeyDown || key.Text == "j":
		if p.detail != nil {
			if maxScroll := m.subagentDetailScrollLimit(*p.detail); p.detailScroll < maxScroll {
				p.detailScroll++
			}
		} else if p.sel < len(p.items)-1 {
			p.sel++
		}
	case key.Code == tea.KeyPgUp:
		if p.detail != nil {
			p.detailScroll -= max(1, m.subagentDetailBodyRows(*p.detail)-1)
			m.clampSubagentDetailScroll(p)
		}
	case key.Code == tea.KeyPgDown || (key.Code == tea.KeySpace && p.detail != nil):
		if p.detail != nil {
			p.detailScroll += max(1, m.subagentDetailBodyRows(*p.detail)-1)
			m.clampSubagentDetailScroll(p)
		}
	case key.Code == tea.KeyHome:
		if p.detail != nil {
			p.detailScroll = 0
		}
	case key.Code == tea.KeyEnd:
		if p.detail != nil {
			p.detailScroll = m.subagentDetailScrollLimit(*p.detail)
		}
	case key.Code == tea.KeyEnter:
		if p.detail != nil {
			return m, nil
		}
		if len(p.items) == 0 {
			return m, nil
		}
		detail, ok := m.ctrl.SubagentDetail(p.items[p.sel].ID)
		if !ok {
			m.notice(fmt.Sprintf(i18n.M.SubagentNotFoundFmt, p.items[p.sel].ID))
			m.subagentPicker = nil
			return m, nil
		}
		p.detail = &detail
		p.detailID = detail.ID
		p.detailScroll = 0
	case key.Text == "r":
		m.refreshSubagentItems(p, p.selectedID())
		if len(p.items) == 0 {
			p.sel = 0
			p.closeDetail()
			return m, nil
		}
		if p.detail != nil && p.detailID != "" {
			if detail, ok := m.ctrl.SubagentDetail(p.detailID); ok {
				p.detail = &detail
				m.clampSubagentDetailScroll(p)
			} else {
				p.closeDetail()
			}
		}
	case key.Text == "c":
		if len(p.items) == 0 || (p.detail != nil && !p.detail.Cancelable) {
			return m, nil
		}
		id := p.items[p.sel].ID
		if p.detail != nil {
			id = p.detail.ID
		}
		m.ctrl.CancelSubagent(id)
		m.refreshSubagentItems(p, id)
		if detail, ok := m.ctrl.SubagentDetail(id); ok {
			p.detail = &detail
			p.detailID = detail.ID
			m.clampSubagentDetailScroll(p)
		}
	}
	return m, nil
}

func (m chatTUI) handleSubagentsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	p := m.subagentPicker
	if p == nil || p.detail == nil {
		return m, nil
	}
	mouse := msg.Mouse()
	switch mouse.Button {
	case tea.MouseWheelUp:
		if p.detailScroll > 0 {
			p.detailScroll--
		}
	case tea.MouseWheelDown:
		if maxScroll := m.subagentDetailScrollLimit(*p.detail); p.detailScroll < maxScroll {
			p.detailScroll++
		}
	}
	return m, nil
}

func (m chatTUI) renderSubagents() string {
	p := m.subagentPicker
	if p == nil {
		return ""
	}
	if p.detail != nil {
		if m.ctrl != nil && p.detailID != "" {
			if detail, ok := m.ctrl.SubagentDetail(p.detailID); ok {
				p.detail = &detail
				m.clampSubagentDetailScroll(p)
			}
		}
		return m.renderSubagentDetail(*p.detail, p.detailScroll)
	}

	w := max(m.width, 10)
	maxRows := m.subagentsPanelRows()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", accent(i18n.M.SubagentsTitle), dim(fmt.Sprintf(i18n.M.SubagentsShowingFmt, len(p.items), len(p.allItems))))
	if p.filter != "" || p.filterEdit {
		label := p.filter
		if label == "" {
			label = i18n.M.SubagentsFilterPlaceholder
		}
		prefix := i18n.M.SubagentsFilterLabel
		if p.filterEdit {
			prefix = i18n.M.SubagentsFilterEditLabel
		}
		b.WriteString(dim(prefix + label))
		b.WriteByte('\n')
	}
	if len(p.items) == 0 {
		if p.filter != "" {
			b.WriteString(dim(i18n.M.SubagentsNoMatch))
		} else {
			b.WriteString(dim(i18n.M.SubagentsNone))
		}
		b.WriteByte('\n')
		if p.filterEdit {
			b.WriteString(dim(i18n.M.SubagentsFilterExitHint))
		} else {
			b.WriteString(dim(i18n.M.SubagentsFilterIdleHint))
		}
		return choicePanelStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))
	}
	itemRows := maxRows - 4
	if itemRows < 1 {
		itemRows = 1
	}
	start := 0
	if p.sel >= itemRows {
		start = p.sel - itemRows + 1
	}
	end := min(len(p.items), start+itemRows)
	for i := start; i < end; i++ {
		item := p.items[i]
		label := subagentSummaryLabel(item, w)
		b.WriteString(rowLine(i == p.sel, i+1, "", label, item.State == event.SubagentRunning))
		b.WriteByte('\n')
	}
	if start > 0 || end < len(p.items) {
		b.WriteString(dim(fmt.Sprintf(i18n.M.SubagentsWindowFmt, start+1, end, len(p.items))))
		b.WriteByte('\n')
	}
	if p.filterEdit {
		b.WriteString(dim(i18n.M.SubagentsFilterEditingHint))
	} else {
		b.WriteString(dim(i18n.M.SubagentsFilterIdleHint))
	}
	return choicePanelStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))
}

func (m chatTUI) renderSubagentDetail(d control.SubagentDetail, scrollOffset int) string {
	w := max(m.width, 10)
	maxContentRows := max(1, m.subagentDetailTranscriptRows())
	header := m.subagentDetailHeaderLines(d, w, maxContentRows)
	bodyRows := m.subagentDetailBodyRows(d)
	body := scrollSubagentLines(m.subagentDetailBodyLines(d), bodyRows, scrollOffset)
	scrollLimit := m.subagentDetailScrollLimit(d)
	lines := make([]string, 0, len(header)+max(1, len(body))+1)
	for _, line := range header {
		lines = append(lines, ansi.Truncate(line, m.subagentDisplayWidth(), "…"))
	}
	if len(body) == 0 && bodyRows > 0 {
		body = []string{dim(i18n.M.SubagentNoEvents)}
	}
	lines = append(lines, body...)
	if scrollLimit > 0 {
		footer := fmt.Sprintf(i18n.M.SubagentDetailScrollFooterFmt, min(scrollOffset, scrollLimit), scrollLimit)
		if d.Cancelable {
			footer += i18n.M.SubagentDetailCancelSuffix
		}
		lines = append(lines, dim(footer))
	} else if d.Cancelable {
		lines = append(lines, dim(i18n.M.SubagentDetailCloseCancelHint))
	} else {
		lines = append(lines, dim(i18n.M.SubagentDetailCloseHint))
	}
	return choicePanelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func (m chatTUI) renderSubagentDetailScreen() string {
	p := m.subagentPicker
	if p == nil || p.detail == nil {
		return ""
	}
	if m.ctrl != nil && p.detailID != "" {
		if detail, ok := m.ctrl.SubagentDetail(p.detailID); ok {
			p.detail = &detail
			m.clampSubagentDetailScroll(p)
		}
	}
	return m.renderSubagentDetail(*p.detail, p.detailScroll)
}

func (m chatTUI) subagentDisplayWidth() int {
	if w := m.width - 6; w > 10 {
		return w
	}
	return max(m.width, 10)
}

func (m chatTUI) wrapSubagentDisplayLines(lines []string) []string {
	width := m.subagentDisplayWidth()
	var out []string
	for _, line := range lines {
		wrapped := clampWidth(line, width)
		if wrapped == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return out
}

func scrollSubagentLines(lines []string, maxRows, offset int) []string {
	if maxRows <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= maxRows {
		return lines
	}
	limit := len(lines) - maxRows
	if offset < 0 {
		offset = 0
	}
	if offset > limit {
		offset = limit
	}
	return lines[offset : offset+maxRows]
}

func (m chatTUI) subagentsPanelRows() int {
	if m.height <= 0 {
		return 12
	}
	boxW := max(m.width, 10)
	reserved := strings.Count(inputBoxStyle.Width(boxW).Render(m.input.View()), "\n") + 1 + 2
	for _, panel := range []string{
		m.renderTodoPanel(),
		m.renderApprovalBanner(),
		m.renderChooser(),
		m.renderRewind(),
		m.renderResumePicker(),
		m.renderCompletion(),
	} {
		if panel != "" {
			reserved += strings.Count(panel, "\n") + 1
		}
	}
	rows := m.height - reserved
	if rows < 4 {
		return 4
	}
	return rows
}

func (m chatTUI) subagentDetailTranscriptRows() int {
	if m.height <= 0 {
		return 12
	}
	rows := m.height - 1
	if rows < 4 {
		return 4
	}
	return rows
}

func (m chatTUI) subagentDetailBodyRows(d control.SubagentDetail) int {
	maxContentRows := max(1, m.subagentDetailTranscriptRows()-2)
	header := m.subagentDetailHeaderLines(d, max(m.width, 10), maxContentRows)
	body := maxContentRows - len(header) - 1
	if body < 1 {
		return 0
	}
	return body
}

func (m chatTUI) subagentDetailBodyLines(d control.SubagentDetail) []string {
	return m.wrapSubagentDisplayLines(m.subagentDetailTranscriptLines(d))
}

func (m chatTUI) subagentDetailScrollLimit(d control.SubagentDetail) int {
	lines := m.subagentDetailBodyLines(d)
	bodyRows := m.subagentDetailBodyRows(d)
	if bodyRows <= 0 || len(lines) <= bodyRows {
		return 0
	}
	return len(lines) - bodyRows
}

func (m chatTUI) clampSubagentDetailScroll(p *subagentPicker) {
	if p == nil || p.detail == nil {
		return
	}
	limit := m.subagentDetailScrollLimit(*p.detail)
	if p.detailScroll < 0 {
		p.detailScroll = 0
	}
	if p.detailScroll > limit {
		p.detailScroll = limit
	}
}

func (m chatTUI) subagentDetailHeaderLines(d control.SubagentDetail, w, maxContentRows int) []string {
	title := strings.TrimSpace(d.Alias)
	if title == "" {
		title = d.Skill
	}
	if title == "" {
		title = i18n.M.SubagentTitle
	}
	lines := []string{fmt.Sprintf("%s %s %s", accent(i18n.M.SubagentTitle), bold(title), dim(fmt.Sprintf("/%s · %s", d.Skill, control.SubagentStateText(d.State))))}
	minRemaining := 2
	if maxContentRows-len(lines) <= minRemaining {
		return lines
	}
	lines = append(lines, dim(fmt.Sprintf(i18n.M.SubagentIDFmt, d.ID)))
	if maxContentRows-len(lines) <= minRemaining {
		return lines
	}
	if d.StartedAt.IsZero() {
		lines = append(lines, dim(i18n.M.SubagentStartedUnknown))
	} else {
		line := fmt.Sprintf(i18n.M.SubagentStartedFmt, d.StartedAt.Format(time.RFC3339))
		if !d.EndedAt.IsZero() {
			line = fmt.Sprintf(i18n.M.SubagentStartedEndedFmt, d.StartedAt.Format(time.RFC3339), d.EndedAt.Format(time.RFC3339))
		}
		lines = append(lines, dim(line))
	}
	if d.Err != "" && maxContentRows-len(lines) > minRemaining {
		lines = append(lines, red(fmt.Sprintf(i18n.M.SubagentErrorFmt, oneLine(d.Err, max(18, w-12)))))
	}
	return lines
}

func (m chatTUI) subagentDetailTranscriptLines(d control.SubagentDetail) []string {
	replay := newSubagentDetailReplay(m.renderer)
	for _, ev := range d.Events {
		replay.Append(ev)
	}
	replay.Close()
	lines := replay.Lines()
	if len(lines) == 0 {
		if answer := strings.TrimSpace(d.Answer); answer != "" {
			lines = append(lines, answer)
		}
	}
	return lines
}

func subagentSummaryLabel(s control.SubagentSummary, w int) string {
	alias := strings.TrimSpace(s.Alias)
	if alias == "" {
		alias = s.Skill
	}
	if alias == "" {
		alias = i18n.M.SubagentTitle
	}
	age := ""
	if !s.StartedAt.IsZero() {
		age = " · " + compactDuration(time.Since(s.StartedAt))
	}
	state := control.SubagentStateText(s.State)
	prompt := ansi.Truncate(strings.TrimSpace(s.PromptPreview), max(16, w-44), "…")
	return fmt.Sprintf("%s  /%s  %s%s  %s", alias, s.Skill, state, age, dim(prompt))
}

type subagentDetailReplay struct {
	renderer  *mdRenderer
	lines     []string
	reasoning strings.Builder
	answer    strings.Builder
	lastText  string
}

func newSubagentDetailReplay(renderer *mdRenderer) *subagentDetailReplay {
	return &subagentDetailReplay{renderer: renderer}
}

func (r *subagentDetailReplay) Append(ev control.SubagentEvent) {
	switch ev.Kind {
	case event.Reasoning:
		if r.answer.Len() > 0 {
			r.flushAnswer(false)
		}
		r.reasoning.WriteString(ev.Text)
	case event.Text:
		if r.reasoning.Len() > 0 {
			r.flushReasoning()
		}
		r.answer.WriteString(ev.Text)
		r.lastText = strings.TrimSpace(r.answer.String())
	case event.Message:
		if r.reasoning.Len() > 0 {
			r.flushReasoning()
		}
		text := strings.TrimSpace(ev.Text)
		pending := strings.TrimSpace(r.answer.String())
		if pending != "" && text != "" && text != pending {
			r.flushAnswer(false)
			r.answer.WriteString(text)
		} else if text != "" && pending == "" && text != r.lastText {
			r.answer.WriteString(text)
		}
		r.flushAnswer(true)
	case event.ToolDispatch:
		r.flush()
		r.lines = append(r.lines, formatSubagentDispatchLine(ev.Tool, ev.Text))
	case event.ToolResult:
		r.flush()
		if ev.Error != "" {
			r.lines = append(r.lines, formatSubagentBlockedLine(ev.Tool, ev.Error))
		} else if out := strings.TrimSpace(ev.Text); out != "" {
			r.appendToolResult(ev.Tool, out)
		}
	case event.Notice:
		r.flush()
		if strings.TrimSpace(ev.Text) != "" {
			r.lines = append(r.lines, "  "+dim("· "+ev.Text))
		}
	case event.Phase:
		r.flush()
		if strings.TrimSpace(ev.Text) != "" {
			r.lines = append(r.lines, "["+ev.Text+"]")
		}
	case event.Usage:
		r.flush()
		if strings.TrimSpace(ev.Text) != "" {
			r.lines = append(r.lines, dim(strings.TrimSpace(ev.Text)))
		}
	case event.TurnDone:
		r.flush()
		if ev.Error != "" {
			r.lines = append(r.lines, red("  ⊘ "+ev.Error))
		}
	}
}

func (r *subagentDetailReplay) Close() {
	r.flush()
}

func (r *subagentDetailReplay) Lines() []string {
	return append([]string(nil), r.lines...)
}

func (r *subagentDetailReplay) flush() {
	if r.reasoning.Len() > 0 {
		r.flushReasoning()
	}
	if r.answer.Len() > 0 {
		r.flushAnswer(false)
	}
}

func (r *subagentDetailReplay) flushReasoning() {
	text := strings.TrimSpace(r.reasoning.String())
	r.reasoning.Reset()
	if text == "" {
		return
	}
	r.lines = append(r.lines, dim("  ▎ "+i18n.M.SubagentThinkingLabel))
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r.lines = append(r.lines, dim("  ▎ "+line))
	}
}

func (r *subagentDetailReplay) flushAnswer(final bool) {
	text := strings.TrimSpace(r.answer.String())
	r.answer.Reset()
	if text == "" {
		return
	}
	if final {
		r.lines = append(r.lines, green(i18n.M.SubagentAnswerLabel))
	}
	if r.renderer != nil {
		if rendered := strings.TrimRight(r.renderer.Render(text), "\n"); rendered != "" {
			r.lines = append(r.lines, strings.Split(rendered, "\n")...)
			return
		}
	}
	r.lines = append(r.lines, strings.Split(text, "\n")...)
}

func (r *subagentDetailReplay) appendToolResult(name, output string) {
	prefix := "  " + dim("⎿") + " " + accent(name)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		r.lines = append(r.lines, prefix)
		return
	}
	r.lines = append(r.lines, prefix+" "+dim(lines[0]))
	for _, line := range lines[1:] {
		r.lines = append(r.lines, "     "+dim(line))
	}
}

func formatSubagentDispatchLine(name, args string) string {
	line := "  " + accent("●") + " " + bold(name)
	if args = strings.TrimSpace(args); args != "" {
		line += " " + dim(oneLine(args, 72))
	}
	return line
}

func formatSubagentBlockedLine(name, errText string) string {
	line := "  " + red("●") + " " + bold(name)
	if errText = strings.TrimSpace(errText); errText != "" {
		line += " " + red("⊘ "+oneLine(errText, 72))
	}
	return line
}

func compactDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
