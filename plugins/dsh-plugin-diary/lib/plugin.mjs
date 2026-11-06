/**
 * DSH 日记插件入口：向 `ctx.tools` 注册三个日记工具。
 *
 * 纯运行时：不监听事件、不改 profile、不动记忆索引；关掉它的方式有两个——
 * 环境变量 `DSH_DIARY=off`，或从 profile 的 bundles 里移除本包。
 */
import { buildDiaryTools } from "./diaryTools.mjs";

export const name = "diary";

/** 需要工具注册表；没有它时插件不该被加载。 */
export const inject = ["tools"];

/**
 * 挂载插件。
 * @param {object} ctx Cordis 上下文
 * @param {object} [config] 行配置（当前只接受空对象，保留给未来的限额项）
 */
export function apply(ctx, config = {}) {
  if (process.env.DSH_DIARY === "off") return;
  void config;
  for (const tool of buildDiaryTools()) ctx.tools.register(tool);
}
