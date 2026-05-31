// settings.json "slashCommands" → /name shell-command executor. Pure TUI-side, never injected into model context.

import { execSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { globalSettingsPath, projectSettingsPath } from "../../../hooks.js";
import { t } from "../../../i18n/index.js";
import type { SlashCommandSpec, SlashResult } from "./types.js";

/** Per-command config shape inside settings.json["slashCommands"]. */
export interface CustomSlashCommandConfig {
  /** Shell command to execute. */
  command: string;
  /** One-line description shown in /help and suggestion picker. */
  description?: string;
  /** Optional argument hint shown after the command name. */
  argsHint?: string;
}

/** Parsed form of settings.json — only the slashCommands key we care about. */
interface SlashSettings {
  slashCommands?: Record<string, CustomSlashCommandConfig>;
}

const DEFAULT_TIMEOUT_MS = 10_000;

function readSlashSettings(path: string): SlashSettings | null {
  if (!existsSync(path)) return null;
  try {
    const raw = readFileSync(path, "utf8");
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === "object") return parsed as SlashSettings;
  } catch {
    /* malformed JSON → treat as no custom commands; do NOT throw */
  }
  return null;
}

export interface CustomSlashRegistryOptions {
  /** Absolute project root, if any. Without it, only global commands load. */
  projectRoot?: string;
  /** Override `~` for tests. */
  homeDir?: string;
}

export class CustomSlashRegistry {
  private commands: Record<string, CustomSlashCommandConfig> = {};
  private readonly projectRoot: string | undefined;
  private readonly homeDir: string;

  constructor(opts: CustomSlashRegistryOptions = {}) {
    this.projectRoot = opts.projectRoot;
    this.homeDir = opts.homeDir ?? homedir();
    this.reload();
  }

  /** Re-read settings.json files and rebuild the command map. */
  reload(): number {
    const merged: Record<string, CustomSlashCommandConfig> = {};

    // Global scope loads first
    const globalPath = globalSettingsPath(this.homeDir);
    const globalSettings = readSlashSettings(globalPath);
    if (globalSettings?.slashCommands) {
      Object.assign(merged, globalSettings.slashCommands);
    }

    // Project scope overrides global for same keys
    if (this.projectRoot) {
      const projPath = projectSettingsPath(this.projectRoot);
      // Only load if the file exists, to avoid a confusing "no project" warning
      if (existsSync(projPath)) {
        const projSettings = readSlashSettings(projPath);
        if (projSettings?.slashCommands) {
          Object.assign(merged, projSettings.slashCommands);
        }
      }
    }

    this.commands = merged;
    return Object.keys(this.commands).length;
  }

  /** Look up a command by name. Returns undefined if not found. */
  lookup(name: string): CustomSlashCommandConfig | undefined {
    return this.commands[name];
  }

  /** List all command names (for /help and suggestions). */
  names(): string[] {
    return Object.keys(this.commands);
  }

  /** Build SlashCommandSpec entries for integration into the suggestion system. */
  specs(): SlashCommandSpec[] {
    return Object.entries(this.commands).map(([cmd, cfg]) => ({
      cmd,
      summary: cfg.description ?? `/${cmd}`,
      group: "extend" as const,
      argsHint: cfg.argsHint,
    }));
  }

  /** Count of loaded commands. */
  get size(): number {
    return Object.keys(this.commands).length;
  }

  /** Execute a custom command's shell command and return a SlashResult.
   *  This is the fallback handler — it runs the shell command synchronously,
   *  captures stdout/stderr, and returns the output as `info`. */
  execute(name: string, args: string[], command: string): SlashResult {
    try {
      // Append user-passed args to the command string
      const cmd = args.length > 0 ? `${command} ${args.join(" ")}` : command;
      const stdout = execSync(cmd, {
        encoding: "utf8" as const,
        timeout: DEFAULT_TIMEOUT_MS,
        maxBuffer: 1024 * 1024,
        // @types/node ExecSyncOptions.shell is `string | undefined` but
        // runtime Node accepts `boolean` since v6.
        shell: true as unknown as string,
      }).trim();
      if (stdout) {
        return { info: stdout };
      }
      return { info: t("handlers.admin.customExecOk", { name }) };
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return { info: t("handlers.admin.customExecFailed", { name, reason: message }) };
    }
  }
}
