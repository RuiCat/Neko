/**
 * DSH QQ 插件入口：注册 `qq_send` / `qq_inbox` / `qq_status`，并按配置启动轮询。
 *
 * 安全默认：`enabled=false`、`dryRun=true`。没手动打开前既不联网也不起定时器；
 * 打开后也只监听 `watchGroups`/`watchUsers` 里显式列出的会话（白名单）。
 */
import { createQqService } from "./qqService.mjs";
import { buildQqTools } from "./qqTools.mjs";
import { createNapcatWebui } from "./napcatWebui.mjs";
import { buildQqLoginTool } from "./qqLoginTool.mjs";

export const name = "qq";

/** 需要工具注册表。 */
export const inject = ["tools"];

/**
 * 挂载插件。
 * @param {object} ctx Cordis 上下文
 * @param {object} [config] 见 README 配置表
 */
export function apply(ctx, config = {}) {
  if (process.env.DSH_QQ === "off") return;
  const merged = {
    ...config,
    token: config.token || process.env.DSH_QQ_TOKEN || "",
    apiBase: config.apiBase || process.env.DSH_QQ_API_BASE || undefined,
  };
  const service = createQqService(merged);
  for (const tool of buildQqTools(service)) ctx.tools.register(tool);

  // 扫码登录走「已有后端」的 WebUI，与收发用的 OneBot HTTP 是两个端口。
  const webui = createNapcatWebui({ ...merged, fetchImpl: config.fetchImpl });
  ctx.tools.register(buildQqLoginTool(webui));
  service.start();

  if (typeof ctx.on === "function") {
    ctx.on("dispose", () => service.stop());
  }
}
