package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/skill"
	"reasonix/internal/tool"
)

type desktopTestTool struct {
	name     string
	readOnly bool
}

func (t desktopTestTool) Name() string { return t.name }
func (t desktopTestTool) Description() string {
	return t.name
}
func (desktopTestTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (desktopTestTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}
func (t desktopTestTool) ReadOnly() bool { return t.readOnly }

func TestDesktopCommandsKeepSubagentSkillsButHideSubagentManagement(t *testing.T) {
	app := NewApp()
	ctrl := control.New(control.Options{Skills: []skill.Skill{
		{Name: "inline", Description: "inline", RunAs: skill.RunInline},
		{Name: "scout", Description: "scout", RunAs: skill.RunSubagent},
		{Name: "agent-one", Description: "agent", RunAs: skill.RunSubagent, Path: "/tmp/.claude/agents/agent-one.md"},
	}})
	app.setTestCtrl(ctrl, "")
	defer ctrl.Close()

	names := map[string]bool{}
	for _, cmd := range app.Commands() {
		names[cmd.Name] = true
	}
	for _, want := range []string{"inline", "scout"} {
		if !names[want] {
			t.Fatalf("desktop commands should keep %q visible, have %#v", want, names)
		}
	}
	for _, hidden := range []string{"agents", "subagents"} {
		if names[hidden] {
			t.Fatalf("desktop commands should hide /%s, have %#v", hidden, names)
		}
	}
}

func TestDesktopSlashArgsHideSubagentManagementCommands(t *testing.T) {
	app := NewApp()
	ctrl := control.New(control.Options{Skills: []skill.Skill{
		{Name: "inline", Scope: skill.ScopeProject, RunAs: skill.RunInline},
		{Name: "scout", Scope: skill.ScopeProject, RunAs: skill.RunSubagent},
		{Name: "agent-one", Scope: skill.ScopeProject, RunAs: skill.RunSubagent, Path: "/tmp/.claude/agents/agent-one.md"},
	}})
	app.setTestCtrl(ctrl, "")
	defer ctrl.Close()

	if got := app.SlashArgs("/subagents "); len(got.Items) != 0 {
		t.Fatalf("/subagents args should be hidden in desktop, got %#v", got.Items)
	}

	if got := app.SlashArgs("/agents "); len(got.Items) != 0 {
		t.Fatalf("/agents args should be absent once the command is removed, got %#v", got.Items)
	}

	got := app.SlashArgs("/skill show ")
	labels := map[string]bool{}
	for _, item := range got.Items {
		labels[item.Label] = true
	}
	if !labels["inline"] || !labels["scout"] {
		t.Fatalf("/skill show should keep slash skills, including subagent skills, got %#v", got.Items)
	}
}

func TestDesktopSubmitRoutesSubagentsToControllerButAllowsSkills(t *testing.T) {
	var notices []string
	done := make(chan struct{}, 1)
	reg := tool.NewRegistry()
	reg.Add(desktopTestTool{name: "read_file", readOnly: true})
	ctrl := control.New(control.Options{
		Registry: reg,
		Skills: []skill.Skill{
			{Name: "scout", Description: "scout", RunAs: skill.RunSubagent, AllowedTools: []string{"read_file"}},
		},
		SkillRunner: func(context.Context, skill.Skill, string, skill.SubagentRunContext) (string, error) {
			return "ok", nil
		},
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				notices = append(notices, e.Text)
			}
			if e.Kind == event.TurnDone && e.Subagent != nil {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		}),
	})
	app := NewApp()
	app.setTestCtrl(ctrl, "")
	defer ctrl.Close()

	app.Submit("/subagents")
	if len(notices) != 1 || notices[0] != i18n.M.SubagentsNone {
		t.Fatalf("/subagents should fall back to controller notice text in desktop, got %#v", notices)
	}

	app.Submit("/scout inspect")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("/scout should remain allowed in desktop and complete a subagent run")
	}
	if runs := ctrl.ListSubagents(); len(runs) != 1 || runs[0].Skill != "scout" {
		t.Fatalf("desktop should still allow subagent slash skills, runs=%#v", runs)
	}
}

func TestDesktopSubagentManagementPrefixHelper(t *testing.T) {
	if !isDesktopSubagentManagementPrefix("/subagents ") {
		t.Fatal("/subagents prefix should be hidden from desktop arg completion")
	}
}
