/**
 * DSH B 站插件入口：注册七个工具。
 *
 * - 只读：视频详情、字幕、评论区、弹幕、播放地址；
 * - 登录：`bilibili_login`（扫码；凭据只落本机 0600 文件）；
 * - 写：`bilibili_comment_post`（三道闸：登录 + allowPost/dryRun 配置 + 本次 confirm）。
 *
 * 纯运行时：不监听事件；`DSH_BILIBILI=off` 可整体关闭。
 */
import { buildBilibiliTools } from "./bilibiliTools.mjs";
import { createBilibiliRuntime } from "./bilibiliRuntime.mjs";
import { buildSocialTools } from "./bilibiliSocialTools.mjs";

export const name = "bilibili";

/** 需要工具注册表。 */
export const inject = ["tools"];

/**
 * 挂载插件。
 * @param {object} ctx Cordis 上下文
 * @param {{cookie?:string, statePath?:string, allowPost?:boolean, dryRun?:boolean,
 *   timeoutMs?:number, maxChars?:number}} [config] 行配置
 */
export function apply(ctx, config = {}) {
  if (process.env.DSH_BILIBILI === "off") return;
  const runtime = createBilibiliRuntime(config);
  const readConfig = {
    fetchImpl: runtime.fetchImpl,
    userAgent: config.userAgent,
    timeoutMs: config.timeoutMs,
    maxChars: config.maxChars,
    // 用函数形式：扫码登录成功后，字幕/详情工具立刻用上新 cookie。
    cookie: () => runtime.holder.cookie,
  };
  for (const tool of buildBilibiliTools(readConfig)) ctx.tools.register(tool);
  for (const tool of buildSocialTools(runtime)) ctx.tools.register(tool);
}
