package cli

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/i18n"
)

func TestSubagentsCommandOpensPickerWithoutNoticeOnEmptyList(t *testing.T) {
	m := newTestChatTUI()
	m.ctrl = control.New(control.Options{})

	if cmd := m.runSlashCommand("/subagents"); cmd != nil {
		t.Fatalf("/subagents should open the picker locally, got cmd=%v", cmd)
	}
	if m.subagentPicker == nil {
		t.Fatal("/subagents should open the picker even when the list is empty")
	}
	if joined := strings.Join(*m.pendingCommit, "\n"); strings.Contains(joined, "no active or recent subagents") {
		t.Fatalf("/subagents should not write an empty-list notice into scrollback, got %q", joined)
	}
}

func TestSubagentsCommandClearsCompletionFocus(t *testing.T) {
	m := newTestChatTUI()
	m.ctrl = control.New(control.Options{})
	m.completion = completion{active: true}

	m.openSubagents()
	if m.completion.active {
		t.Fatal("/subagents should clear autocomplete so modal keys are not shadowed")
	}
	if m.subagentPicker == nil {
		t.Fatal("/subagents should open the picker")
	}
}

func TestSubagentsFilterShowsMatchingRuns(t *testing.T) {
	m := newTestChatTUI()
	m.width = 100
	m.height = 24
	m.subagentPicker = &subagentPicker{}
	m.subagentPicker.setItems([]control.SubagentSummary{
		{ID: "run-1", Alias: "Ellis", Skill: "scout", State: event.SubagentRunning},
		{ID: "run-2", Alias: "Morgan", Skill: "review", State: event.SubagentCompleted},
	}, "")

	updated, _ := m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Text: "/"}))
	m = updated.(chatTUI)
	if m.subagentPicker == nil || !m.subagentPicker.filterEdit {
		t.Fatalf("/ should enter subagent filter mode, picker=%#v", m.subagentPicker)
	}

	for _, ch := range []string{"r", "u", "n"} {
		updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Text: ch}))
		m = updated.(chatTUI)
	}
	if got := m.subagentPicker.filter; got != "run" {
		t.Fatalf("filter query = %q, want run", got)
	}
	if len(m.subagentPicker.items) != 1 || m.subagentPicker.items[0].ID != "run-1" {
		t.Fatalf("filter should keep only the running scout item, got %#v", m.subagentPicker.items)
	}
	rendered := m.renderSubagents()
	for _, want := range []string{"showing 1 of 2", "filter> run", "Ellis"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("filtered browser should render %q, got %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "Morgan") {
		t.Fatalf("non-matching items should be hidden, got %q", rendered)
	}

	updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(chatTUI)
	if m.subagentPicker == nil || m.subagentPicker.filterEdit {
		t.Fatalf("Enter should leave filter mode, picker=%#v", m.subagentPicker)
	}
	if rendered = m.renderSubagents(); !strings.Contains(rendered, "filter: run") {
		t.Fatalf("committed filter should remain visible in the header, got %q", rendered)
	}
}

func TestSubagentsFilterBackspaceAndEsc(t *testing.T) {
	m := newTestChatTUI()
	m.width = 100
	m.height = 24
	m.subagentPicker = &subagentPicker{}
	m.subagentPicker.setItems([]control.SubagentSummary{
		{ID: "run-1", Alias: "Ellis", Skill: "scout", State: event.SubagentRunning},
		{ID: "run-2", Alias: "Morgan", Skill: "review", State: event.SubagentCompleted},
	}, "")

	updated, _ := m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Text: "/"}))
	m = updated.(chatTUI)
	if m.subagentPicker == nil || !m.subagentPicker.filterEdit {
		t.Fatalf("/ should enter filter mode, picker=%#v", m.subagentPicker)
	}
	for _, ch := range []string{"s", "c"} {
		updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Text: ch}))
		m = updated.(chatTUI)
	}
	if got := len(m.subagentPicker.items); got != 1 {
		t.Fatalf("typed filter should narrow to one item, got %d items %#v", got, m.subagentPicker.items)
	}

	updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	m = updated.(chatTUI)
	updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	m = updated.(chatTUI)
	if got := m.subagentPicker.filter; got != "" {
		t.Fatalf("backspacing to empty should clear the query, got %q", got)
	}
	if got := len(m.subagentPicker.items); got != 2 {
		t.Fatalf("empty filter should restore the full list, got %d items %#v", got, m.subagentPicker.items)
	}

	updated, _ = m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = updated.(chatTUI)
	if m.subagentPicker == nil || m.subagentPicker.filterEdit {
		t.Fatalf("Esc should leave filter mode without closing the browser, picker=%#v", m.subagentPicker)
	}
}

func TestSubagentsClearSubcommandUsesControllerNotice(t *testing.T) {
	m := newTestChatTUI()
	m.ctrl = control.New(control.Options{})

	m.runSubagentsSubcommand("/subagents clear all")
	if m.subagentPicker != nil {
		t.Fatalf("/subagents clear all should not open the picker, got %#v", m.subagentPicker)
	}
	got := strings.Join(m.transcript, "\n")
	if !strings.Contains(got, i18n.M.SubagentsClearedAll) {
		t.Fatalf("/subagents clear all should write the controller notice, got %q", got)
	}
}

func TestSubagentDetailUsesSharedRenderer(t *testing.T) {
	m := newTestChatTUI()
	m.width = 100
	m.height = 24
	detail := control.SubagentDetail{
		ID:    "sub-1",
		Skill: "scout",
		Alias: "One",
		State: event.SubagentRunning,
		Events: []control.SubagentEvent{
			{Kind: event.Reasoning, Text: "我"},
			{Kind: event.Reasoning, Text: "需要"},
			{Kind: event.ToolDispatch, Tool: "read_file", Text: `{"path":"x"}`},
			{Kind: event.Text, Text: "结"},
			{Kind: event.Text, Text: "论"},
			{Kind: event.Message, Text: "结论"},
			{Kind: event.Usage, Text: "  · 1200 tok · in 1000 (900 cached / 100 new) · out 200 (50 reasoning) · ¥0.0012"},
		},
	}

	rendered := m.renderSubagentDetail(detail, 0)
	if strings.Contains(rendered, "[subagent]") {
		t.Fatalf("detail should not render a nested subagent card header, got %q", rendered)
	}
	for _, want := range []string{"Subagent", "One", "/scout", "running", "read_file", "我需要", "结论", "1200 tok"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("detail should render %q, got %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "reasoning 我") || strings.Contains(rendered, "text 结") {
		t.Fatalf("detail should not expose raw per-event labels/chunks, got %q", rendered)
	}
}

func TestCompletedSubagentDetailKeepsFullTranscriptStyling(t *testing.T) {
	m := newTestChatTUI()
	m.width = 120
	m.height = 32
	detail := control.SubagentDetail{
		ID:    "sub-1",
		Skill: "scout",
		Alias: "Done",
		State: event.SubagentCompleted,
		Events: []control.SubagentEvent{
			{Kind: event.Reasoning, Text: "先检查索引"},
			{Kind: event.ToolDispatch, Tool: "read_file", Text: `{"path":"story.md"}`},
			{Kind: event.ToolResult, Tool: "read_file", Text: strings.Repeat("正文片段", 40)},
			{Kind: event.Text, Text: "正文分析"},
			{Kind: event.Message, Text: "完整结论\n\n- 保留第一条\n- 保留第二条"},
			{Kind: event.Usage, Text: "  · 6055 tok · in 5664 (4864 cached / 800 new) · out 391 (18 reasoning) · ¥0.0017"},
		},
	}

	lines := m.subagentDetailTranscriptLines(detail)
	fullTranscript := strings.Join(lines, "\n")
	for _, want := range []string{"thinking", "先检查索引", "read_file", "正文片段", "正文分析", "answer", "完整结论", "保留第二条", "6055 tok", "in 5664"} {
		if !strings.Contains(fullTranscript, want) {
			t.Fatalf("completed detail should keep full transcript element %q, got %#v", want, lines)
		}
	}
	rendered := m.renderSubagentDetail(detail, 0)
	if strings.Contains(rendered, "no recorded child events yet") {
		t.Fatalf("completed detail should not collapse into empty/fallback rendering, got %q", rendered)
	}
}

func TestSubagentDetailPageKeysScroll(t *testing.T) {
	m := newTestChatTUI()
	m.width = 72
	m.height = 18
	m.ctrl = control.New(control.Options{})
	detail := control.SubagentDetail{
		ID:     "sub-1",
		Skill:  "scout",
		Alias:  "One",
		State:  event.SubagentCompleted,
		Answer: strings.Repeat("长结论", 120),
	}
	for i := 0; i < 30; i++ {
		detail.Events = append(detail.Events, control.SubagentEvent{Kind: event.Text, Text: strings.Repeat("轨迹", 40)})
		detail.Events = append(detail.Events, control.SubagentEvent{Kind: event.Message, Text: strings.Repeat("结论", 40)})
	}
	m.subagentPicker = &subagentPicker{detail: &detail, detailID: detail.ID}

	before := m.renderSubagentDetail(detail, 0)
	updated, _ := m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	m2 := updated.(chatTUI)
	if m2.subagentPicker == nil || m2.subagentPicker.detailScroll == 0 {
		t.Fatalf("PgDown should advance detail scroll, picker=%#v", m2.subagentPicker)
	}
	after := m2.renderSubagentDetail(detail, m2.subagentPicker.detailScroll)
	if before == after {
		t.Fatal("detail rendering should change after PgDown scroll")
	}

	updated, _ = m2.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	m3 := updated.(chatTUI)
	if m3.subagentPicker == nil || m3.subagentPicker.detailScroll != 0 {
		t.Fatalf("PgUp should return to top, picker=%#v", m3.subagentPicker)
	}
}

func TestSubagentDetailMouseWheelScrolls(t *testing.T) {
	m := newTestChatTUI()
	m.width = 72
	m.height = 18
	m.ctrl = control.New(control.Options{})
	detail := control.SubagentDetail{
		ID:     "sub-1",
		Skill:  "scout",
		Alias:  "One",
		State:  event.SubagentCompleted,
		Answer: strings.Repeat("长结论", 120),
	}
	for i := 0; i < 30; i++ {
		detail.Events = append(detail.Events, control.SubagentEvent{Kind: event.Text, Text: strings.Repeat("轨迹", 40)})
	}
	m.subagentPicker = &subagentPicker{detail: &detail, detailID: detail.ID}

	updated, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	m2 := updated.(chatTUI)
	if m2.subagentPicker == nil || m2.subagentPicker.detailScroll == 0 {
		t.Fatalf("mouse wheel down should advance detail scroll, picker=%#v", m2.subagentPicker)
	}
	updated, _ = m2.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	m3 := updated.(chatTUI)
	if m3.subagentPicker == nil || m3.subagentPicker.detailScroll != 0 {
		t.Fatalf("mouse wheel up should return detail scroll to top, picker=%#v", m3.subagentPicker)
	}
}

func TestSubagentDetailEscClosesBrowser(t *testing.T) {
	m := newTestChatTUI()
	detail := control.SubagentDetail{ID: "sub-1", Skill: "scout", Alias: "One", State: event.SubagentCompleted}
	m.subagentPicker = &subagentPicker{
		items:    []control.SubagentSummary{{ID: detail.ID, Skill: detail.Skill, Alias: detail.Alias, State: detail.State}},
		detail:   &detail,
		detailID: detail.ID,
	}

	updated, _ := m.handleSubagentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m2 := updated.(chatTUI)
	if m2.subagentPicker != nil {
		t.Fatalf("Esc from subagent detail should close the browser, got %#v", m2.subagentPicker)
	}
}

func TestSubagentDetailViewIsIndependentFromMainComposer(t *testing.T) {
	m := newTestChatTUI()
	m.width = 100
	m.height = 24
	m.ctrl = control.New(control.Options{})
	m.input.SetValue("main draft")
	detail := control.SubagentDetail{
		ID:    "sub-1",
		Skill: "scout",
		Alias: "One",
		State: event.SubagentCompleted,
		Events: []control.SubagentEvent{
			{Kind: event.Reasoning, Text: "think"},
			{Kind: event.Text, Text: "answer"},
			{Kind: event.Message, Text: "answer"},
			{Kind: event.ToolDispatch, Tool: "read_file", Text: `{"path":"x"}`},
		},
	}
	m.subagentPicker = &subagentPicker{
		items:    []control.SubagentSummary{{ID: detail.ID, Skill: detail.Skill, Alias: detail.Alias, State: detail.State}},
		detail:   &detail,
		detailID: detail.ID,
	}

	rendered := m.View().Content
	if !strings.Contains(rendered, "Subagent") || !strings.Contains(rendered, "One") || !strings.Contains(rendered, "thinking") || !strings.Contains(rendered, "read_file") {
		t.Fatalf("detail view should render the selected subagent transcript, got %q", rendered)
	}
	if strings.Contains(rendered, "main draft") {
		t.Fatalf("detail view should not embed the main composer, got %q", rendered)
	}
}

func TestSubagentDetailEnterIsIgnored(t *testing.T) {
	m := newTestChatTUI()
	m.width = 100
	m.height = 24
	m.input.SetValue("/scout nested")
	detail := control.SubagentDetail{ID: "sub-1", Skill: "scout", Alias: "One", State: event.SubagentCompleted}
	m.subagentPicker = &subagentPicker{
		items:    []control.SubagentSummary{{ID: detail.ID, Skill: detail.Skill, Alias: detail.Alias, State: detail.State}},
		detail:   &detail,
		detailID: detail.ID,
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m2 := updated.(chatTUI)
	if len(*m2.pendingCommit) != 0 {
		t.Fatalf("subagent detail Enter must not write into main scrollback, committed=%v", *m2.pendingCommit)
	}
	if m2.subagentPicker == nil || m2.subagentPicker.detail == nil {
		t.Fatalf("Enter should keep the detail view open, got %#v", m2.subagentPicker)
	}
	if got := m2.input.Value(); got != "/scout nested" {
		t.Fatalf("detail Enter should not submit or edit the main composer, got %q", got)
	}
}

func TestSubagentsBrowserLetsCtrlDQuit(t *testing.T) {
	m := newTestChatTUI()
	m.ctrl = control.New(control.Options{})
	m.subagentPicker = &subagentPicker{}

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("Ctrl-D should return a quit command while subagents browser is open")
	}
}

func TestSubagentDetailIsHeightBoundedAndScrollable(t *testing.T) {
	m := newTestChatTUI()
	m.width = 72
	m.height = 18
	detail := control.SubagentDetail{
		ID:     "sub-1",
		Skill:  "scout",
		Alias:  "One",
		State:  event.SubagentCompleted,
		Answer: strings.Repeat("长结论", 120),
	}
	for i := 0; i < 30; i++ {
		detail.Events = append(detail.Events, control.SubagentEvent{Kind: event.Text, Text: strings.Repeat("轨迹", 40)})
		detail.Events = append(detail.Events, control.SubagentEvent{Kind: event.Message, Text: strings.Repeat("结论", 40)})
	}

	m.subagentPicker = &subagentPicker{detail: &detail, detailID: detail.ID}
	rendered := m.renderSubagentDetailScreen()
	rows := strings.Count(rendered, "\n") + 1
	if rows > m.height {
		t.Fatalf("detail screen should fit the terminal, rows=%d limit=%d\n%s", rows, m.height, rendered)
	}
	if m.subagentDetailScrollLimit(detail) == 0 {
		t.Fatalf("long detail should expose scroll range")
	}

	m.subagentPicker.detailScroll = m.subagentDetailScrollLimit(detail)
	atBottom := m.renderSubagentDetailScreen()
	if rows := strings.Count(atBottom, "\n") + 1; rows > m.height {
		t.Fatalf("scrolled detail screen should remain height-bounded, rows=%d limit=%d", rows, m.height)
	}
	for _, line := range strings.Split(atBottom, "\n") {
		if w := visibleWidth(line); w > m.width {
			t.Fatalf("detail line should not rely on terminal wrapping, width=%d limit=%d line=%q", w, m.width, line)
		}
	}
}
