package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/agent/testutil"
	"reasonix/internal/diff"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

type previewWriterTool struct{}

func (previewWriterTool) Name() string        { return "writer_tool" }
func (previewWriterTool) Description() string { return "" }
func (previewWriterTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
}
func (previewWriterTool) ReadOnly() bool { return false }
func (previewWriterTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "wrote file", nil
}
func (previewWriterTool) Preview(args json.RawMessage) (diff.Change, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return diff.Change{}, err
	}
	return diff.Build(p.Path, "before\n", "after\n", diff.Modify), nil
}

func toolMessageContent(req provider.Request, name string) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		m := req.Messages[i]
		if m.Role == provider.RoleTool && m.Name == name {
			return m.Content
		}
	}
	return ""
}

func TestRunSubAgentWithConfigUsesAskerAndPreEditHook(t *testing.T) {
	prov := testutil.NewMock("sub",
		testutil.Turn{ToolCalls: []provider.ToolCall{
			{
				ID:        "ask-1",
				Name:      "ask",
				Arguments: `{"questions":[{"header":"Direction","question":"Which path?","options":[{"label":"Keep going"},{"label":"Stop"}]}]}`,
			},
			{
				ID:        "write-1",
				Name:      "writer_tool",
				Arguments: `{"path":"/tmp/story.md"}`,
			},
		}},
		testutil.Turn{Text: "done"},
	)
	reg := tool.NewRegistry()
	reg.Add(NewAskTool())
	reg.Add(previewWriterTool{})

	asker := &recordingAsker{}
	var changes []diff.Change
	out, err := RunSubAgentWithConfig(context.Background(), prov, reg, "skill sys", "inspect and then edit", Options{}, SubagentRunConfig{
		Sink:  event.Discard,
		Asker: asker,
		PreEditHook: func(ch diff.Change) {
			changes = append(changes, ch)
		},
	})
	if err != nil {
		t.Fatalf("RunSubAgentWithConfig: %v", err)
	}
	if out != "done" {
		t.Fatalf("final answer = %q, want done", out)
	}
	if len(asker.questions) != 1 || asker.questions[0].Header != "Direction" {
		t.Fatalf("asker questions = %+v, want one propagated prompt", asker.questions)
	}
	if len(changes) != 1 || changes[0].Path != "/tmp/story.md" {
		t.Fatalf("pre-edit changes = %+v, want one preview for /tmp/story.md", changes)
	}
}

func TestRunSubAgentWithConfigAppliesPlanMode(t *testing.T) {
	prov := testutil.NewMock("sub",
		testutil.Turn{ToolCalls: []provider.ToolCall{{
			ID:        "write-1",
			Name:      "writer_tool",
			Arguments: `{"path":"/tmp/story.md"}`,
		}}},
		testutil.Turn{Text: "done"},
	)
	reg := tool.NewRegistry()
	reg.Add(previewWriterTool{})

	preEditCalled := false
	out, err := RunSubAgentWithConfig(context.Background(), prov, reg, "skill sys", "try to edit in plan mode", Options{}, SubagentRunConfig{
		Sink:     event.Discard,
		PlanMode: true,
		PreEditHook: func(diff.Change) {
			preEditCalled = true
		},
	})
	if err != nil {
		t.Fatalf("RunSubAgentWithConfig: %v", err)
	}
	if out != "done" {
		t.Fatalf("final answer = %q, want done", out)
	}
	if preEditCalled {
		t.Fatal("pre-edit hook should not fire when plan mode blocks the writer")
	}
	reqs := prov.Requests()
	if len(reqs) < 2 {
		t.Fatalf("provider requests = %d, want follow-up request carrying the blocked tool result", len(reqs))
	}
	if got := toolMessageContent(reqs[1], "writer_tool"); !strings.HasPrefix(got, "blocked:") {
		t.Fatalf("tool result = %q, want blocked plan-mode message", got)
	}
}
