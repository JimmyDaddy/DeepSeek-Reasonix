import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { wrapToCells } from "@/cli/ui/text-width.js";
import { t, tObj } from "@/i18n/index.js";
import { VERSION } from "@/version.js";
import { formatDuration, formatLoopStatus, parseLoopCommand } from "../../loop.js";
import { SLASH_COMMANDS, SLASH_GROUP_ORDER, orderSlashCommandsByGroup } from "../commands.js";
import type { SlashHandler } from "../dispatch.js";
import { loadMarkdownAgents } from "../md-commands.js";
import type { SlashCommandSpec, SlashGroup } from "../types.js";

const ABOUT_WEBSITE = "https://esengine.github.io/DeepSeek-Reasonix/";
const ABOUT_REPO = "https://github.com/esengine/DeepSeek-Reasonix";
const ABOUT_LICENSE = "MIT";

const exit: SlashHandler = () => ({ exit: true });

const resetLog: SlashHandler = (_args, loop) => {
  const { dropped, archived, systemRebuilt } = loop.clearLog();
  const head = archived
    ? t("handlers.basic.newInfoArchived", { count: dropped, archived })
    : t("handlers.basic.newInfo", { count: dropped });
  const info = systemRebuilt ? head + t("handlers.basic.newInfoSystemReloaded") : head;
  return { clear: true, info };
};

function groupHeader(group: SlashGroup): string {
  const cap = group.charAt(0).toUpperCase() + group.slice(1);
  const label = t(`slashSuggestions.group${cap}`);
  const detail = t(`slashSuggestions.groupDetail${cap}`);
  return `${label}  ·  ${detail}`;
}

const HELP_NAME_COL = 28;
const HELP_HANGING_INDENT = 2 + HELP_NAME_COL + 2;

function renderRow(spec: SlashCommandSpec, cols: number): string {
  const name = `/${spec.cmd}${spec.argsHint ? ` ${spec.argsHint}` : ""}`;
  const desc = t(`slash.${spec.cmd}.description`);
  const summary = desc === `slash.${spec.cmd}.description` ? spec.summary : desc;
  const descWidth = Math.max(20, cols - HELP_HANGING_INDENT);
  const chunks = wrapToCells(summary, descWidth);
  const head = `  ${name.padEnd(HELP_NAME_COL)}  ${chunks[0] ?? ""}`;
  if (chunks.length <= 1) return head;
  const tail = chunks.slice(1).map((c) => `${" ".repeat(HELP_HANGING_INDENT)}${c}`);
  return [head, ...tail].join("\n");
}

const help: SlashHandler = () => {
  // Match the info-card chrome (`markdown.tsx` BODY_LEFT_CELLS=7) so wide
  // CJK descriptions wrap inside the card, not past its right edge.
  const cols = (process.stdout.columns ?? 80) - 7;
  const lines: string[] = [t("handlers.basic.helpTitle"), ""];
  const rowsByGroup = new Map<SlashGroup, SlashCommandSpec[]>();
  for (const group of SLASH_GROUP_ORDER) rowsByGroup.set(group, []);
  for (const command of orderSlashCommandsByGroup(SLASH_COMMANDS)) {
    rowsByGroup.get(command.group)!.push(command);
  }
  for (const group of SLASH_GROUP_ORDER) {
    const rows = rowsByGroup.get(group) ?? [];
    if (rows.length === 0) continue;
    lines.push(`  ${groupHeader(group)}`);
    for (const r of rows) lines.push(renderRow(r, cols));
    lines.push("");
  }
  lines.push(
    t("handlers.basic.helpShellTitle"),
    t("handlers.basic.helpShell"),
    t("handlers.basic.helpShellDetail"),
    t("handlers.basic.helpShellConsent"),
    t("handlers.basic.helpShellExample"),
    "",
    t("handlers.basic.helpShellGateTitle"),
    t("handlers.basic.helpShellGate"),
    t("handlers.basic.helpShellGateDetail"),
    t("handlers.basic.helpShellGatePolicy"),
    "",
    t("handlers.basic.helpMemoryTitle"),
    t("handlers.basic.helpMemoryPin"),
    t("handlers.basic.helpMemoryPinEx"),
    t("handlers.basic.helpMemoryGlobal"),
    t("handlers.basic.helpMemoryGlobalEx"),
    t("handlers.basic.helpMemoryPinBoth"),
    t("handlers.basic.helpMemoryEscape"),
    "",
    t("handlers.basic.helpFileTitle"),
    t("handlers.basic.helpFile"),
    t("handlers.basic.helpFilePicker"),
    "",
    t("handlers.basic.helpUrlTitle"),
    t("handlers.basic.helpUrl"),
    t("handlers.basic.helpUrlCache"),
    t("handlers.basic.helpUrlPunct"),
    "",
    t("handlers.basic.helpSessionsTitle"),
    t("handlers.basic.helpSessionCustom"),
    t("handlers.basic.helpSessionNone"),
  );
  return { info: lines.join("\n") };
};

const retry: SlashHandler = (_args, loop) => {
  const prev = loop.retryLastUser();
  if (!prev) {
    return { info: t("handlers.basic.retryNone") };
  }
  const preview = prev.length > 80 ? `${prev.slice(0, 80)}…` : prev;
  return {
    info: t("handlers.basic.retryInfo", { preview }),
    resubmit: prev,
  };
};

const loop: SlashHandler = (args, _loop, ctx) => {
  if (!ctx.startLoop || !ctx.stopLoop || !ctx.getLoopStatus) {
    return { info: t("handlers.basic.loopTuiOnly") };
  }
  const cmd = parseLoopCommand(args);
  if (cmd.kind === "error") return { info: cmd.message };
  if (cmd.kind === "stop") {
    const wasActive = ctx.getLoopStatus() !== null;
    ctx.stopLoop();
    return {
      info: wasActive ? t("handlers.basic.loopStopped") : t("handlers.basic.loopNoActive"),
    };
  }
  if (cmd.kind === "status") {
    const status = ctx.getLoopStatus();
    if (!status) {
      return { info: t("handlers.basic.loopNoActiveHint") };
    }
    return { info: `▸ ${formatLoopStatus(status.prompt, status.nextFireMs, status.iter)}` };
  }
  ctx.startLoop(cmd.intervalMs, cmd.prompt);
  return {
    info: t("handlers.basic.loopStarted", {
      prompt: cmd.prompt,
      duration: formatDuration(cmd.intervalMs),
    }),
  };
};

const keys: SlashHandler = (_args, _loop, ctx) => {
  if (!ctx.postKeys) return { info: t("handlers.basic.keysNeedsTui") };
  const ref = tObj<{
    topic: string;
    sections: ReadonlyArray<{
      title?: string;
      rows: ReadonlyArray<{ key: string; text: string }>;
    }>;
    footer: string;
  }>("ui.keysReference");
  ctx.postKeys({ topic: ref.topic, sections: ref.sections, footer: ref.footer });
  return {};
};

const about: SlashHandler = () => {
  const lines = [
    t("handlers.basic.aboutHeader", { version: VERSION }),
    "",
    `  ${t("handlers.basic.aboutWebsiteLabel")}  ${ABOUT_WEBSITE}`,
    `  ${t("handlers.basic.aboutRepoLabel")}   ${ABOUT_REPO}`,
    `  ${t("handlers.basic.aboutLicenseLabel")}   ${ABOUT_LICENSE}`,
  ];
  return { info: lines.join("\n") };
};

const slashCmd: SlashHandler = (args, _loop, ctx) => {
  const sub = (args[0] ?? "").toLowerCase();

  if (sub === "reload") {
    if (!ctx.reloadExtraHandlers) {
      return { info: t("handlers.admin.slashReloadUnavailable") };
    }
    const count = ctx.reloadExtraHandlers();
    return { info: t("handlers.admin.slashReloaded", { count }) };
  }

  // list (default)
  const extraKeys = ctx.extraHandlers ? Object.keys(ctx.extraHandlers) : [];
  if (extraKeys.length === 0) {
    return { info: t("handlers.admin.slashNone") };
  }
  const lines = [t("handlers.admin.slashHeader", { count: extraKeys.length })];
  for (const name of extraKeys.sort()) {
    lines.push(`  /${name}`);
  }
  lines.push("");
  lines.push(t("handlers.admin.slashFooter"));
  return { info: lines.join("\n") };
};

const agents: SlashHandler = (args, _loop, ctx) => {
  const projectRoot = ctx.codeRoot;
  const dirs = (root: string) => [
    join(root, ".reasonix", "agents"),
    join(root, ".claude", "agents"),
  ];

  const sub = (args[0] ?? "").toLowerCase();

  // /agents new <name>
  if (sub === "new" || sub === "init") {
    const name = args[1];
    if (!name) return { info: "usage: /agents new <name>" };
    if (!projectRoot)
      return { info: "agent creation requires a project root (run from reasonix code)." };
    const targetDir = join(projectRoot, ".reasonix", "agents");
    try {
      mkdirSync(targetDir, { recursive: true });
    } catch {
      return { info: `cannot create directory: ${targetDir}` };
    }
    const filePath = join(targetDir, `${name}.md`);
    if (existsSync(filePath)) return { info: `agent "${name}" already exists at ${filePath}` };
    const stub = `---
name: ${name}
description: What does this agent do?
tools: read_file, search_content, glob
---

# ${name}

Replace this body with the agent's system prompt.
The agent runs as an isolated subagent (subagent mode).
`;
    writeFileSync(filePath, stub, "utf8");
    return {
      info: `▸ agent "${name}" created at ${filePath}\n  edit it, then /slash reload to pick it up.`,
    };
  }

  // /agents run <name> [args]
  if (sub === "run") {
    const name = args[1];
    if (!name) return { info: "usage: /agents run <name> [args]" };
    if (!projectRoot) return { info: "agent invocation requires a project root." };
    const all = loadMarkdownAgents(dirs(projectRoot), "agents");
    const found = all.find((a) => a.name === name);
    if (!found) return { info: `agent "${name}" not found.` };
    const extraArgs = args.slice(2).join(" ").trim();
    const body = extraArgs ? found.body.replace(/\$ARGUMENTS/g, extraArgs) : found.body;
    return {
      info: `▸ running agent "${name}"${extraArgs ? ` — ${extraArgs}` : ""}`,
      resubmit: `# Agent: ${found.name}${found.description ? `\n> ${found.description}` : ""}\n\n${body}`,
    };
  }

  // /agents show <name>
  if (sub === "show" || sub === "cat") {
    const target = args[1];
    if (!target) return { info: "usage: /agents show <name>" };
    if (!projectRoot) return { info: "agent inspection requires a project root." };
    const all = loadMarkdownAgents(dirs(projectRoot), "agents");
    const found = all.find((a) => a.name === target);
    if (!found) return { info: `agent "${target}" not found.` };
    const lines = [
      `▸ ${found.name}  (${found.source})`,
      found.description ? `  ${found.description}` : "",
      found.tools ? `  tools: ${found.tools}` : "",
      found.model ? `  model: ${found.model}` : "",
      "",
      found.body,
    ].filter((l) => l !== "");
    return { info: lines.join("\n") };
  }

  // /agents list (default)
  if (!projectRoot) return { info: "agent listing requires a project root." };
  const all = loadMarkdownAgents(dirs(projectRoot), "agents");
  if (all.length === 0) {
    return { info: "no agents found in .reasonix/agents/ or .claude/agents/" };
  }
  const lines = [`▸ agents · ${all.length} available:`];
  for (const a of all) {
    const desc = a.description ? `  —  ${a.description}` : "";
    lines.push(`  /${a.name}${desc}`);
  }
  return { info: lines.join("\n") };
};

export const handlers: Record<string, SlashHandler> = {
  exit,
  new: resetLog,
  help,
  retry,
  loop,
  keys,
  about,
  slash: slashCmd,
  agents,
};
