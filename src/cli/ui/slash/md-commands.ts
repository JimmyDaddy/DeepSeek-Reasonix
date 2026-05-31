// Loads .md files with YAML frontmatter as slash commands / agent definitions.
// First-write-wins across dirs; callers stack dirs in priority order.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { parseFrontmatter } from "../../../frontmatter.js";

export interface MarkdownCommandDef {
  name: string;
  description: string;
  /** Hint shown after the command name in suggestions, e.g. "[场景文件]" */
  argumentHint?: string;
  /** Full markdown body (post-frontmatter), with `$ARGUMENTS` placeholder intact. */
  body: string;
  /** Which directory this was loaded from (for /slash listing). */
  source: string;
  /** Optional model override from frontmatter `model:` */
  model?: string;
}

export interface MarkdownAgentDef {
  name: string;
  description: string;
  /** Comma-separated tool names from frontmatter `tools:` or `allowed-tools:` */
  tools?: string;
  body: string;
  source: string;
  model?: string;
}

function isValidMdName(name: string): boolean {
  return /^[a-zA-Z0-9\u4e00-\u9fff\u3400-\u4dbf_./-]{1,64}$/.test(name);
}

/** Load commands from a single directory. .md files → commands; subdirectories with SKILL.md are treated as agent skills (loaded by SkillStore). */
export function loadMarkdownCommands(
  dirs: readonly string[],
  sourceName: string,
): MarkdownCommandDef[] {
  const byName = new Map<string, MarkdownCommandDef>();
  for (const dir of dirs) {
    if (!existsSync(dir)) continue;
    let entries: string[];
    try {
      entries = readdirSync(dir);
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (!entry.endsWith(".md")) continue;
      const stem = entry.slice(0, -3);
      if (!isValidMdName(stem)) continue;
      if (byName.has(stem)) continue; // first dir wins (priority handled by caller's dir order)
      const filePath = join(dir, entry);
      let raw: string;
      try {
        raw = readFileSync(filePath, "utf8");
      } catch {
        continue;
      }
      const { data, body } = parseFrontmatter(raw);
      byName.set(stem, {
        name: stem,
        description: (data.description ?? "").trim(),
        argumentHint: data["argument-hint"]?.trim(),
        body: body.trim(),
        source: `${sourceName}/${entry}`,
        model: data.model?.trim(),
      });
    }
  }
  return [...byName.values()].sort((a, b) => a.name.localeCompare(b.name));
}

/** Load agents from a directory of .md files. Each file must have `name` frontmatter. */
export function loadMarkdownAgents(
  dirs: readonly string[],
  sourceName: string,
): MarkdownAgentDef[] {
  const byName = new Map<string, MarkdownAgentDef>();
  for (const dir of dirs) {
    if (!existsSync(dir)) continue;
    let entries: string[];
    try {
      entries = readdirSync(dir);
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (!entry.endsWith(".md")) continue;
      const stem = entry.slice(0, -3);
      const filePath = join(dir, entry);
      let raw: string;
      try {
        raw = readFileSync(filePath, "utf8");
      } catch {
        continue;
      }
      const { data, body } = parseFrontmatter(raw);
      const name = (data.name ?? stem).trim();
      if (!isValidMdName(name)) continue;
      if (byName.has(name)) continue;
      byName.set(name, {
        name,
        description: (data.description ?? "").trim(),
        tools: data.tools?.trim() || data["allowed-tools"]?.trim(),
        body: body.trim(),
        source: `${sourceName}/${entry}`,
        model: data.model?.trim(),
      });
    }
  }
  return [...byName.values()].sort((a, b) => a.name.localeCompare(b.name));
}

/** Substitute `$ARGUMENTS` in a command body with user-supplied arguments. */
export function substituteArguments(body: string, args: string[]): string {
  if (args.length === 0) {
    return body.replace(/\$ARGUMENTS/g, "");
  }
  return body.replace(/\$ARGUMENTS/g, args.join(" "));
}
