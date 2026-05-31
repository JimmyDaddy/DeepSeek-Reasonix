/** Markdown command & agent tests — .reasonix/commands/, .claude/commands/,
 *  .reasonix/agents/, .claude/agents/ loading, priority, and integration. */

import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { handleSlash } from "../src/cli/ui/slash.js";
import {
  loadMarkdownAgents,
  loadMarkdownCommands,
  substituteArguments,
} from "../src/cli/ui/slash/md-commands.js";
import { DeepSeekClient, ImmutablePrefix } from "../src/index.js";
import { CacheFirstLoop } from "../src/loop.js";
import { SkillStore } from "../src/skills.js";

function makeLoop() {
  const client = new DeepSeekClient({
    apiKey: "sk-test",
    fetch: vi.fn() as unknown as typeof fetch,
  });
  return new CacheFirstLoop({
    client,
    prefix: new ImmutablePrefix({ system: "s" }),
  });
}

// MarkdownCommandLoader
describe("loadMarkdownCommands", () => {
  let dir: string;

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "reasonix-mdc-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("loads .md files as commands", () => {
    writeFileSync(
      join(dir, "deploy.md"),
      "---\ndescription: Deploy to prod\nargument-hint: [env]\n---\n\nRun deploy for $ARGUMENTS",
      "utf8",
    );
    writeFileSync(join(dir, "lint.md"), "---\ndescription: Lint\n---\n\nRun linter", "utf8");
    const cmds = loadMarkdownCommands([dir], "test");
    expect(cmds).toHaveLength(2);
    expect(cmds[0]!.name).toBe("deploy");
    expect(cmds[0]!.description).toBe("Deploy to prod");
    expect(cmds[0]!.argumentHint).toBe("[env]");
    expect(cmds[0]!.body).toContain("$ARGUMENTS");
    expect(cmds[0]!.source).toBe("test/deploy.md");
    expect(cmds[1]!.name).toBe("lint");
  });

  it("skips non-.md files", () => {
    writeFileSync(join(dir, "readme.txt"), "hello", "utf8");
    writeFileSync(join(dir, "note"), "hello", "utf8");
    const cmds = loadMarkdownCommands([dir], "test");
    expect(cmds).toHaveLength(0);
  });

  it("first dir wins on name conflict", () => {
    const dir2 = mkdtempSync(join(tmpdir(), "reasonix-mdc2-"));
    writeFileSync(join(dir, "cmd.md"), "---\ndescription: V1\n---\n\nbody1", "utf8");
    writeFileSync(join(dir2, "cmd.md"), "---\ndescription: V2\n---\n\nbody2", "utf8");
    const cmds = loadMarkdownCommands([dir, dir2], "test");
    expect(cmds).toHaveLength(1);
    expect(cmds[0]!.description).toBe("V1");
    rmSync(dir2, { recursive: true, force: true });
  });

  it("handles missing directories gracefully", () => {
    const cmds = loadMarkdownCommands([join(dir, "nonexistent")], "test");
    expect(cmds).toHaveLength(0);
  });

  it("handles CJK filenames", () => {
    writeFileSync(
      join(dir, "起草场景.md"),
      "---\ndescription: 起草正文场景\n---\n\n起草一个场景：$ARGUMENTS",
      "utf8",
    );
    const cmds = loadMarkdownCommands([dir], "test");
    expect(cmds).toHaveLength(1);
    expect(cmds[0]!.name).toBe("起草场景");
    expect(cmds[0]!.description).toBe("起草正文场景");
  });

  it("parses model frontmatter field", () => {
    writeFileSync(
      join(dir, "heavy.md"),
      "---\ndescription: Heavy task\nmodel: deepseek-v4-pro\n---\n\nbody",
      "utf8",
    );
    const cmds = loadMarkdownCommands([dir], "test");
    expect(cmds[0]!.model).toBe("deepseek-v4-pro");
  });
});

// substituteArguments
describe("substituteArguments", () => {
  it("replaces $ARGUMENTS with joined args", () => {
    expect(substituteArguments("Process: $ARGUMENTS", ["file1", "file2"])).toBe(
      "Process: file1 file2",
    );
  });

  it("replaces $ARGUMENTS with empty string when no args", () => {
    expect(substituteArguments("Process: $ARGUMENTS end", [])).toBe("Process:  end");
  });

  it("replaces multiple occurrences", () => {
    expect(substituteArguments("Start $ARGUMENTS mid $ARGUMENTS end", ["x"])).toBe(
      "Start x mid x end",
    );
  });

  it("leaves body unchanged when no $ARGUMENTS present", () => {
    expect(substituteArguments("Just do it", ["ignored"])).toBe("Just do it");
  });
});

// loadMarkdownAgents
describe("loadMarkdownAgents", () => {
  let dir: string;

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "reasonix-mda-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("loads agent .md files", () => {
    writeFileSync(
      join(dir, "规划师.md"),
      "---\nname: 规划师\ndescription: 规划章节\ntools: Read, Write, Grep\n---\n\n你是规划专员。",
      "utf8",
    );
    const agents = loadMarkdownAgents([dir], "test");
    expect(agents).toHaveLength(1);
    expect(agents[0]!.name).toBe("规划师");
    expect(agents[0]!.description).toBe("规划章节");
    expect(agents[0]!.tools).toBe("Read, Write, Grep");
    expect(agents[0]!.source).toBe("test/规划师.md");
  });

  it("falls back to filename stem when name missing", () => {
    writeFileSync(
      join(dir, "reviewer.md"),
      "---\ndescription: Review code\n---\n\nYou are a reviewer.",
      "utf8",
    );
    const agents = loadMarkdownAgents([dir], "test");
    expect(agents).toHaveLength(1);
    expect(agents[0]!.name).toBe("reviewer");
  });

  it("supports allowed-tools as alias for tools", () => {
    writeFileSync(
      join(dir, "agent.md"),
      "---\nname: agent\ndescription: Test\nallowed-tools: Read, Edit\n---\n\nbody",
      "utf8",
    );
    const agents = loadMarkdownAgents([dir], "test");
    expect(agents[0]!.tools).toBe("Read, Edit");
  });
});

// SkillStore loads agents as skills
describe("SkillStore agents", () => {
  let project: string;

  beforeEach(() => {
    project = mkdtempSync(join(tmpdir(), "reasonix-sa-"));
  });
  afterEach(() => {
    rmSync(project, { recursive: true, force: true });
  });

  it("loads .claude/agents/*.md as subagent skills", () => {
    const agentsDir = join(project, ".claude", "agents");
    mkdirSync(agentsDir, { recursive: true });
    writeFileSync(
      join(agentsDir, "规划师.md"),
      "---\nname: 规划师\ndescription: 规划章节\ntools: Read, Write\n---\n\n你是规划专员。",
      "utf8",
    );
    const store = new SkillStore({ projectRoot: project, homeDir: project, disableBuiltins: true });
    const skills = store.list();
    expect(skills).toHaveLength(1);
    expect(skills[0]!.name).toBe("规划师");
    expect(skills[0]!.description).toBe("规划章节");
    expect(skills[0]!.allowedTools).toEqual(["Read", "Write"]);
    expect(skills[0]!.runAs).toBe("subagent"); // agents force subagent mode
  });

  it("loads .reasonix/agents/*.md as subagent skills", () => {
    const agentsDir = join(project, ".reasonix", "agents");
    mkdirSync(agentsDir, { recursive: true });
    writeFileSync(
      join(agentsDir, "reviewer.md"),
      "---\nname: reviewer\ndescription: Review changes\n---\n\nYou review diffs.",
      "utf8",
    );
    const store = new SkillStore({ projectRoot: project, homeDir: project, disableBuiltins: true });
    const skills = store.list();
    expect(skills).toHaveLength(1);
    expect(skills[0]!.name).toBe("reviewer");
    expect(skills[0]!.description).toBe("Review changes");
  });
});

// dispatch integration — markdown commands as slash commands
describe("handleSlash with markdown commands", () => {
  let loop: CacheFirstLoop;
  let dir: string;

  beforeEach(() => {
    loop = makeLoop();
    dir = mkdtempSync(join(tmpdir(), "reasonix-slash-md-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("executes a markdown command via extraHandlers", () => {
    writeFileSync(
      join(dir, "greet.md"),
      "---\ndescription: Greeting\n---\n\nSay hello to $ARGUMENTS",
      "utf8",
    );
    const cmds = loadMarkdownCommands([dir], "test");
    const handler = (args: string[]) => {
      const cmd = cmds[0]!;
      const body = substituteArguments(cmd.body, args);
      return {
        info: `▸ running "${cmd.name}"`,
        resubmit: `# Command: ${cmd.name}\n\n${body}`,
      };
    };

    const result = handleSlash("greet", ["world"], loop, {
      extraHandlers: { greet: handler },
    });
    expect(result.info).toContain("greet");
    expect(result.resubmit).toContain("Say hello to world");
  });

  it("replaces $ARGUMENTS with empty when no args provided", () => {
    writeFileSync(
      join(dir, "task.md"),
      "---\ndescription: Task\n---\n\nDo: $ARGUMENTS end",
      "utf8",
    );
    const cmds = loadMarkdownCommands([dir], "test");
    const handler = (args: string[]) => {
      const cmd = cmds[0]!;
      const body = substituteArguments(cmd.body, args);
      return { info: "ok", resubmit: body };
    };

    const result = handleSlash("task", [], loop, {
      extraHandlers: { task: handler },
    });
    expect(result.resubmit).toContain("Do:  end");
  });
});

// /agents handler — list, show, new, run
describe("/agents handler", () => {
  let project: string;
  let loop: CacheFirstLoop;

  beforeEach(() => {
    project = mkdtempSync(join(tmpdir(), "reasonix-agents-cmd-"));
    loop = makeLoop();
  });
  afterEach(() => {
    rmSync(project, { recursive: true, force: true });
  });

  function writeAgent(name: string, frontmatter: string, body: string): string {
    const agentsDir = join(project, ".reasonix", "agents");
    mkdirSync(agentsDir, { recursive: true });
    const path = join(agentsDir, `${name}.md`);
    writeFileSync(path, `---\n${frontmatter}\n---\n\n${body}`, "utf8");
    return path;
  }

  it("/agents lists available agents", () => {
    writeAgent("planner", "name: planner\ndescription: Plans", "Plan things.");
    writeAgent("reviewer", "name: reviewer\ndescription: Reviews", "Review things.");

    const result = handleSlash("agents", [], loop, { codeRoot: project });
    expect(result.info).toContain("2 available");
    expect(result.info).toContain("/planner");
    expect(result.info).toContain("/reviewer");
  });

  it("/agents reports none when no agents exist", () => {
    const result = handleSlash("agents", [], loop, { codeRoot: project });
    expect(result.info).toContain("no agents");
  });

  it("/agents requires project root", () => {
    const result = handleSlash("agents", [], loop, {});
    expect(result.info).toContain("requires a project root");
  });

  it("/agents show <name> displays agent details", () => {
    writeAgent(
      "planner",
      "name: planner\ndescription: Plans chapters\ntools: Read, Write",
      "You are a planner.",
    );

    const result = handleSlash("agents", ["show", "planner"], loop, { codeRoot: project });
    expect(result.info).toContain("planner");
    expect(result.info).toContain("Plans chapters");
    expect(result.info).toContain("Read, Write");
    expect(result.info).toContain("You are a planner.");
  });

  it("/agents show <name> reports not-found", () => {
    const result = handleSlash("agents", ["show", "nonexistent"], loop, { codeRoot: project });
    expect(result.info).toContain("not found");
  });

  it("/agents show without name shows usage", () => {
    const result = handleSlash("agents", ["show"], loop, { codeRoot: project });
    expect(result.info).toContain("usage");
  });

  it("/agents new <name> creates a stub file", () => {
    const result = handleSlash("agents", ["new", "myagent"], loop, { codeRoot: project });
    expect(result.info).toContain("created");

    // Verify file exists
    expect(existsSync(join(project, ".reasonix", "agents", "myagent.md"))).toBe(true);
  });

  it("/agents new without name shows usage", () => {
    const result = handleSlash("agents", ["new"], loop, { codeRoot: project });
    expect(result.info).toContain("usage");
  });

  it("/agents new refuses overwrite", () => {
    writeAgent("existing", "name: existing\ndescription: Exists", "body");
    const result = handleSlash("agents", ["new", "existing"], loop, { codeRoot: project });
    expect(result.info).toContain("already exists");
  });

  it("/agents new requires project root", () => {
    const result = handleSlash("agents", ["new", "test"], loop, {});
    expect(result.info).toContain("requires a project root");
  });

  it("/agents run <name> invokes agent with resubmit", () => {
    writeAgent("runner", "name: runner\ndescription: Runner", "Execute: $ARGUMENTS");

    const result = handleSlash("agents", ["run", "runner", "task1"], loop, { codeRoot: project });
    expect(result.resubmit).toBeDefined();
    expect(result.resubmit).toContain("Execute: task1");
    expect(result.info).toContain("runner");
    expect(result.info).toContain("task1");
  });

  it("/agents run without name shows usage", () => {
    const result = handleSlash("agents", ["run"], loop, { codeRoot: project });
    expect(result.info).toContain("usage");
  });

  it("/agents run <name> reports not-found", () => {
    const result = handleSlash("agents", ["run", "ghost"], loop, { codeRoot: project });
    expect(result.info).toContain("not found");
  });

  it("/agents run without args keeps $ARGUMENTS placeholder", () => {
    writeAgent("noargs", "name: noargs\ndescription: No args", "Do: $ARGUMENTS done");

    const result = handleSlash("agents", ["run", "noargs"], loop, { codeRoot: project });
    // No extra args → $ARGUMENTS stays verbatim in the body
    expect(result.resubmit).toContain("$ARGUMENTS");
  });
});
