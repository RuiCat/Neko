/**
 * `qq_login` 工具：把「扫码登录」这件事交给后端（NapCat WebUI），插件只转述。
 *
 * 为什么要这个工具而不是让用户自己去 WebUI：模型可以在一个会话里完成
 * 「取二维码 → 等用户扫 → 查状态 → 继续测试」这条链，不必人肉切界面。
 *
 * 它**不做**的事：不实现 QQ 协议、不生成二维码图像、不保存 QQ 密码。
 * 二维码内容由后端给出，用任何扫码客户端/二维码工具都能渲染。
 */
import { makeTool } from "./toolDef.mjs";

const PARAMETERS = {
  type: "object",
  properties: {
    action: {
      type: "string",
      description: "qrcode=取登录二维码内容，refresh=刷新二维码，status=查登录状态，help=看使用说明",
    },
  },
  required: ["action"],
  additionalProperties: false,
};

/** 登录阶段的中文解释。 */
const PHASE_TEXT = {
  "" : "未知",
  qrcode: "等待扫码",
  qrcode_scanned: "已扫码，等待手机确认",
  initializing: "登录成功，正在初始化",
  online: "已登录并在线",
  offline: "已登录但掉线",
};

/**
 * 构造 `qq_login` 工具。
 * @param {object} webui createNapcatWebui 的返回值
 * @returns {object}
 */
export function buildQqLoginTool(webui) {
  return makeTool({
    name: "qq_login",
    description:
      "通过后端（NapCat WebUI）做 QQ 扫码登录：取二维码内容、刷新、查登录状态。" +
      "只对接已有后端，不实现 QQ 协议；二维码内容需要你用扫码客户端渲染后扫描。",
    parameters: PARAMETERS,
    async run(args) {
      const action = String(args.action || "").toLowerCase();
      const endpoint = webui.endpoint();

      if (action === "help") {
        return [
          "QQ 扫码登录走的是「已有后端」：",
          `1. 后端需已启动且 WebUI 可达（当前配置：${endpoint}）`,
          "2. 配置 WebUI 密码：插件 config.webuiToken 或环境变量 DSH_NAPCAT_TOKEN",
          "3. 调用 qq_login action=qrcode 拿到二维码内容 URL",
          "4. 用任意二维码工具/手机扫码器把它渲染成二维码，用手机 QQ 扫",
          "5. 调用 qq_login action=status 直到 isLogin=true",
          "6. 之后 qq_send / qq_inbox 走 OneBot HTTP（默认 http://127.0.0.1:3000）",
          "",
          "如果后端还没起：先启动 NapCat（或在别的机器上起，把 webuiBase 指过去），" +
            "WebUI 默认 http://127.0.0.1:6099。",
        ].join("\n");
      }

      if (!webui.hasToken()) {
        return `未配置 WebUI 密码（config.webuiToken / DSH_NAPCAT_TOKEN），无法登录后端。后端：${endpoint}`;
      }

      if (action === "qrcode" || action === "refresh") {
        const url = action === "refresh" ? await webui.refreshQrcode() : await webui.qrcode();
        return [
          "扫码登录二维码内容（用二维码工具渲染后，手机 QQ 扫描）：",
          url,
          "",
          "扫完用 qq_login action=status 查状态。",
        ].join("\n");
      }

      if (action === "status") {
        const state = await webui.status();
        const lines = [
          `登录状态：${state.isLogin ? "已登录" : "未登录"}${state.isOffline ? "（掉线）" : ""}`,
          `阶段：${PHASE_TEXT[state.loginPhase] || state.loginPhase || "未知"}`,
          `内核就绪：${state.coreReady ? "是" : "否"}`,
        ];
        if (!state.isLogin && state.qrcodeUrl) lines.push(`当前二维码内容：${state.qrcodeUrl}`);
        if (state.loginError) lines.push(`错误：${state.loginError}`);
        return lines.join("\n");
      }

      return `不认识的 action：${action}（支持 qrcode / refresh / status / help）`;
    },
  });
}
