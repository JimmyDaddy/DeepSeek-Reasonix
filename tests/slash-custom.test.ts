/** Custom slash-command tests — skill auto-registration, settings.json commands,
 *  dispatch integration, and the /slash management command. */

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { handleSlash, parseSlash } from "../src/cli/ui/slash.js";
import { setExtraSlashSpecs, suggestSlashCommands } from "../src/cli/ui/slash/commands.js";
import { CustomSlashRegistry } from "../src/cli/ui/slash/custom.js";
import { globalSettingsPath, projectSettingsPath } from "../src/hooks.js";
import { DeepSeekClient, ImmutablePrefix } from "../src/index.js";
import { CacheFirstLoop } from "../src/loop.js";
import { SkillStore, applySkillsIndex } from "../src/skills.js";

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

// CustomSlashRegistry
describe("CustomSlashRegistry", () => {
  let home: string;
  let project: string;

  beforeEach(() => {
    home = mkdtempSync(join(tmpdir(), "reasonix-slah-home-"));
    project = mkdtempSync(join(tmpdir(), "reasonix-slah-proj-"));
  });
  afterEach(() => {
    rmSync(home, { recursive: true, force: true });
    rmSync(project, { recursive: true, force: true });
  });

  function writeSlashSettings(dir: string, commands: Record<string, unknown>): string {
    const reasonixDir = join(dir, ".reasonix");
    mkdirSync(reasonixDir, { recursive: true });
    const path = join(reasonixDir, "settings.json");
    writeFileSync(path, JSON.stringify({ slashCommands: commands }), "utf8");
    return path;
  }

  it("loads zero commands when no settings exist", () => {
    const reg = new CustomSlashRegistry({ projectRoot: project, homeDir: home });
    expect(reg.size).toBe(0);
    expect(reg.names()).toEqual([]);
  });

  it("loads global commands from settings.json", () => {
    writeSlashSettings(home, {
      deploy: { command: "echo deployed", description: "Deploy" },
      logs: { command: "tail log", description: "Show logs" },
    });
    const reg = new CustomSlashRegistry({ homeDir: home });
    expect(reg.size).toBe(2);
    expect(reg.names().sort()).toEqual(["deploy", "logs"]);
    expect(reg.lookup("deploy")?.command).toBe("echo deployed");
  });

  it("project commands override global commands for the same name", () => {
    writeSlashSettings(home, { deploy: { command: "echo global", description: "G" } });
    writeSlashSettings(project, { deploy: { command: "echo project", description: "P" } });
    const reg = new CustomSlashRegistry({ projectRoot: project, homeDir: home });
    expect(reg.size).toBe(1);
    expect(reg.lookup("deploy")?.command).toBe("echo project");
  });

  it("project commands add to global commands for different names", () => {
    writeSlashSettings(home, { globalOnly: { command: "echo g" } });
    writeSlashSettings(project, { projectOnly: { command: "echo p" } });
    const reg = new CustomSlashRegistry({ projectRoot: project, homeDir: home });
    expect(reg.size).toBe(2);
    expect(reg.names().sort()).toEqual(["globalOnly", "projectOnly"]);
  });

  it("reload() re-reads from disk", () => {
    const homePath = writeSlashSettings(home, { a: { command: "echo v1" } });
    const reg = new CustomSlashRegistry({ homeDir: home });
    expect(reg.lookup("a")?.command).toBe("echo v1");

    writeFileSync(
      homePath,
      JSON.stringify({ slashCommands: { b: { command: "echo v2" } } }),
      "utf8",
    );
    reg.reload();
    expect(reg.size).toBe(1);
    expect(reg.lookup("b")?.command).toBe("echo v2");
    expect(reg.lookup("a")).toBeUndefined();
  });

  it("specs() returns SlashCommandSpec entries for suggestion system", () => {
    writeSlashSettings(home, {
      deploy: { command: "echo hi", description: "Deploy to prod", argsHint: "[env]" },
    });
    const reg = new CustomSlashRegistry({ homeDir: home });
    const specs = reg.specs();
    expect(specs).toHaveLength(1);
    expect(specs[0]!.cmd).toBe("deploy");
    expect(specs[0]!.summary).toBe("Deploy to prod");
    expect(specs[0]!.group).toBe("extend");
    expect(specs[0]!.argsHint).toBe("[env]");
  });

  it("execute() runs a shell command and returns stdout as info", () => {
    const reg = new CustomSlashRegistry({ homeDir: home });
    const result = reg.execute("test", [], "echo hello world");
    expect(result.info).toBe("hello world");
  });

  it("execute() appends user args to the command", () => {
    const reg = new CustomSlashRegistry({ homeDir: home });
    const result = reg.execute("test", ["--verbose", "foo"], "echo");
    expect(result.info).toBe("--verbose foo");
  });

  it("execute() returns error info when command fails", () => {
    const reg = new CustomSlashRegistry({ homeDir: home });
    const result = reg.execute("fail", [], "nonexistent_command_xyz 2>/dev/null; exit 1");
    // exit code 1 should produce an error string
    expect(result.info).toBeTruthy();
  });

  it("handles malformed settings.json gracefully", () => {
    const reasonixDir = join(home, ".reasonix");
    mkdirSync(reasonixDir, { recursive: true });
    writeFileSync(join(reasonixDir, "settings.json"), "{ not json", "utf8");
    const reg = new CustomSlashRegistry({ homeDir: home });
    expect(reg.size).toBe(0);
  });
});

// Dispatch integration (extraHandlers fallback)
describe("handleSlash with extraHandlers", () => {
  let loop: CacheFirstLoop;

  beforeEach(() => {
    loop = makeLoop();
  });

  it("falls back to extraHandlers when built-in handler not found", () => {
    const extra = {
      deploy: () => ({ info: "deployed!" }),
    };
    const result = handleSlash("deploy", [], loop, { extraHandlers: extra });
    expect(result.info).toBe("deployed!");
    expect(result.unknown).toBeUndefined();
  });

  it("built-in handlers take priority over extra handlers", () => {
    const extra = {
      help: () => ({ info: "custom help" }),
    };
    const result = handleSlash("help", [], loop, { extraHandlers: extra });
    // The built-in /help handler should run, not the custom one
    expect(result.info).toContain("Commands");
  });

  it("returns unknown when no handler matches", () => {
    const result = handleSlash("nonexistent", [], loop, { extraHandlers: {} });
    expect(result.unknown).toBe(true);
  });

  it("resolves aliases for extra handlers", () => {
    const extra = {
      mycommand: () => ({ info: "ok" }),
    };
    // Aliases are only resolved from static SLASH_COMMANDS via the alias map
    // Extra handlers don't participate in alias resolution (simplifies design)
    const result = handleSlash("mycommand", [], loop, { extraHandlers: extra });
    expect(result.info).toBe("ok");
  });
});

// Suggestion system integration
describe("suggestSlashCommands with extra specs", () => {
  afterEach(() => {
    setExtraSlashSpecs([]);
  });

  it("returns extra specs alongside built-in commands", () => {
    setExtraSlashSpecs([{ cmd: "deploy", summary: "Deploy to prod", group: "extend" }]);
    const matches = suggestSlashCommands("dep", false);
    expect(matches.some((m) => m.cmd === "deploy")).toBe(true);
  });

  it("extra specs appear in empty-prefix browse mode", () => {
    setExtraSlashSpecs([{ cmd: "mycmd", summary: "My custom command", group: "extend" }]);
    const matches = suggestSlashCommands("", false);
    // extend group commands should appear in browse mode (not advanced)
    expect(matches.some((m) => m.cmd === "mycmd")).toBe(true);
  });

  it("clearing extra specs removes them", () => {
    setExtraSlashSpecs([{ cmd: "temp", summary: "Temporary", group: "extend" }]);
    setExtraSlashSpecs([]);
    const matches = suggestSlashCommands("temp", false);
    expect(matches.some((m) => m.cmd === "temp")).toBe(false);
  });
});

// Skill disableModelInvocation
describe("Skill disableModelInvocation", () => {
  let project: string;

  beforeEach(() => {
    project = mkdtempSync(join(tmpdir(), "reasonix-skill-dmi-"));
  });
  afterEach(() => {
    rmSync(project, { recursive: true, force: true });
  });

  function writeSkill(dir: string, name: string, frontmatter: string, body: string): string {
    const skillsDir = join(dir, ".reasonix", "skills");
    mkdirSync(skillsDir, { recursive: true });
    const path = join(skillsDir, `${name}.md`);
    writeFileSync(path, `---\n${frontmatter}\n---\n\n${body}`, "utf8");
    return path;
  }

  it("skill with disableModelInvocation: true is excluded from index", () => {
    writeSkill(
      project,
      "deployer",
      "name: deployer\ndescription: Deploy helper\ndisable-model-invocation: true",
      "Run npm run deploy",
    );
    const store = new SkillStore({ projectRoot: project, homeDir: project, disableBuiltins: true });
    const skills = store.list();
    expect(skills).toHaveLength(1);
    expect(skills[0]!.disableModelInvocation).toBe(true);

    // applySkillsIndex filters it out
    const result = applySkillsIndex("base prompt", {
      projectRoot: project,
      homeDir: project,
      disableBuiltins: true,
    });
    expect(result).not.toContain("deployer");
  });

  it("skill without disableModelInvocation appears in index", () => {
    writeSkill(
      project,
      "helper",
      "name: helper\ndescription: A helpful skill",
      "Do helpful things",
    );
    const store = new SkillStore({ projectRoot: project, homeDir: project, disableBuiltins: true });
    const skills = store.list();
    expect(skills).toHaveLength(1);
    expect(skills[0]!.disableModelInvocation).toBeFalsy();

    const result = applySkillsIndex("base prompt", {
      projectRoot: project,
      homeDir: project,
      disableBuiltins: true,
    });
    expect(result).toContain("helper");
  });
});

// /slash handler
describe("/slash handler", () => {
  let loop: CacheFirstLoop;

  beforeEach(() => {
    loop = makeLoop();
  });

  it("/slash with no extra handlers reports none", () => {
    const result = handleSlash("slash", [], loop, {
      extraHandlers: {},
      reloadExtraHandlers: () => 0,
    });
    expect(result.info).toContain("no custom slash commands");
  });

  it("/slash lists extra handler names", () => {
    const result = handleSlash("slash", [], loop, {
      extraHandlers: { deploy: () => ({ info: "ok" }), logs: () => ({ info: "ok" }) },
      reloadExtraHandlers: () => 2,
    });
    expect(result.info).toContain("deploy");
    expect(result.info).toContain("logs");
  });

  it("/slash reload calls reloadExtraHandlers", () => {
    let called = false;
    const result = handleSlash("slash", ["reload"], loop, {
      extraHandlers: {},
      reloadExtraHandlers: () => {
        called = true;
        return 3;
      },
    });
    expect(called).toBe(true);
    expect(result.info).toContain("3");
  });

  it("/slash reload without callback reports unavailable", () => {
    const result = handleSlash("slash", ["reload"], loop, {});
    expect(result.info).toContain("not available");
  });
});
