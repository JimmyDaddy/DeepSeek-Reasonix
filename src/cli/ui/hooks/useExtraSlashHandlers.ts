// Combined extra slash-command handler map: .md commands > settings.json > skills/agents.

import { join } from "node:path";
import { useCallback, useMemo, useRef } from "react";
import { SkillStore } from "../../../skills.js";
import { setExtraSlashSpecs } from "../slash/commands.js";
import { CustomSlashRegistry } from "../slash/custom.js";
import type { SlashHandler } from "../slash/dispatch.js";
import { loadMarkdownCommands, substituteArguments } from "../slash/md-commands.js";
import type { SlashCommandSpec } from "../slash/types.js";

export interface ExtraSlashHandlers {
  /** Combined handler map. Keys are slash-command names. */
  handlers: Record<string, SlashHandler>;
  /** Reload all sources from disk. Returns total extra handler count. */
  reload: () => number;
}

function dirs(root: string | undefined, ...segments: string[]): string[] {
  if (!root) return [];
  return [join(root, ...segments)];
}

export function useExtraSlashHandlers(
  projectRoot: string | undefined,
  homeDir?: string,
): ExtraSlashHandlers {
  const projectRootRef = useRef(projectRoot);
  projectRootRef.current = projectRoot;

  const build = useCallback((): Record<string, SlashHandler> => {
    const root = projectRootRef.current;
    const handlers: Record<string, SlashHandler> = {};

    // Skill auto-registration (lowest priority)
    const store = new SkillStore({ projectRoot: root, homeDir });
    const skills = store.list();
    const skillStoreRef = { current: store };

    for (const skill of skills) {
      handlers[skill.name] = (_args, _loop, _ctx) => {
        const fresh = skillStoreRef.current.read(skill.name);
        const body = fresh?.body ?? skill.body;
        const desc = fresh?.description ?? skill.description;
        const header = `# Skill: ${skill.name}${desc ? `\n> ${desc}` : ""}`;
        const extraArgs = _args.join(" ").trim();
        const argsLine = extraArgs ? `\n\nArguments: ${extraArgs}` : "";
        return {
          info: `▸ running skill "${skill.name}"${extraArgs ? ` — ${extraArgs}` : ""}`,
          resubmit: `${header}\n\n${body}${argsLine}`,
        };
      };
    }

    // settings.json slashCommands
    const registry = new CustomSlashRegistry({ projectRoot: root, homeDir });
    for (const name of registry.names()) {
      const cfg = registry.lookup(name);
      if (!cfg) continue;
      handlers[name] = (_args, _loop, _ctx) => {
        return registry.execute(name, _args, cfg.command);
      };
    }

    // .claude/commands/*.md
    const claudeCommands = loadMarkdownCommands(
      dirs(root, ".claude", "commands"),
      ".claude/commands",
    );
    for (const cmd of claudeCommands) {
      handlers[cmd.name] = (args, _loop, _ctx) => {
        const body = substituteArguments(cmd.body, args);
        const argsLine = args.length > 0 ? ` — ${args.join(" ")}` : "";
        return {
          info: `▸ running command "${cmd.name}"${argsLine}`,
          resubmit: `# Command: ${cmd.name}${cmd.description ? `\n> ${cmd.description}` : ""}\n\n${body}`,
        };
      };
    }

    // .reasonix/commands/*.md (highest priority)
    const reasonixCommands = loadMarkdownCommands(
      dirs(root, ".reasonix", "commands"),
      ".reasonix/commands",
    );
    for (const cmd of reasonixCommands) {
      handlers[cmd.name] = (args, _loop, _ctx) => {
        const body = substituteArguments(cmd.body, args);
        const argsLine = args.length > 0 ? ` — ${args.join(" ")}` : "";
        return {
          info: `▸ running command "${cmd.name}"${argsLine}`,
          resubmit: `# Command: ${cmd.name}${cmd.description ? `\n> ${cmd.description}` : ""}\n\n${body}`,
        };
      };
    }

    // Build suggestion specs from all sources
    const skillSpecs: SlashCommandSpec[] = skills.map((s) => ({
      cmd: s.name,
      summary: s.description || s.name,
      group: "extend" as const,
    }));
    const cmdSpecs = (specs: ReturnType<typeof loadMarkdownCommands>): SlashCommandSpec[] =>
      specs.map((c) => ({
        cmd: c.name,
        summary: c.description || c.name,
        group: "extend" as const,
        argsHint: c.argumentHint,
      }));
    const extraSpecs: SlashCommandSpec[] = [
      ...skillSpecs,
      ...registry.specs(),
      ...cmdSpecs(claudeCommands),
      ...cmdSpecs(reasonixCommands),
    ];
    setExtraSlashSpecs(extraSpecs);

    return handlers;
  }, [homeDir]);

  const handlersRef = useRef<Record<string, SlashHandler>>(build());
  const reload = useCallback((): number => {
    handlersRef.current = build();
    return Object.keys(handlersRef.current).length;
  }, [build]);

  return useMemo(
    () => ({
      get handlers() {
        return handlersRef.current;
      },
      reload,
    }),
    [reload],
  );
}
