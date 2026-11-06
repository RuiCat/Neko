/**
 * 运行时上下文：把「cookie 从哪来、登录后怎么立即生效、写操作闸门」集中到一处。
 *
 * cookie 来源优先级：行配置 `cookie` → 环境变量 `DSH_BILIBILI_COOKIE` → 凭据文件。
 * 扫码登录成功后写凭据文件并**原地更新 holder**，后续请求立刻用新 cookie（不必重启）。
 */
import { createBilibiliApi } from "./bilibiliApi.mjs";
import { describeCookie, loadCredential, saveCredential, statePathOf } from "./bilibiliCredential.mjs";
import { generateQr, pollQr } from "./bilibiliLogin.mjs";

/**
 * 建运行时。
 * @param {object} [config] cookie / statePath / allowPost / dryRun / fetchImpl / userAgent / timeoutMs
 */
export function createBilibiliRuntime(config = {}) {
  const statePath = statePathOf(config);
  const saved = loadCredential(statePath);
  const holder = {
    cookie: String(config.cookie || process.env.DSH_BILIBILI_COOKIE || saved?.cookie || ""),
    userId: saved?.userId || "",
    savedAt: saved?.savedAt || "",
    pendingKey: "",
  };
  const fetchImpl = config.fetchImpl || globalThis.fetch;
  const api = createBilibiliApi({
    fetchImpl,
    userAgent: config.userAgent,
    timeoutMs: config.timeoutMs,
    cookie: () => holder.cookie,
  });

  const infoCache = new Map();
  const INFO_TTL_MS = 60 * 1000;

  return {
    api,
    fetchImpl,

    /**
     * 取视频详情（带 60s 缓存）：多个工具都要 aid/cid，重复请求很容易触发 B 站风控。
     * @param {string} input 视频引用
     */
    async info(input) {
      const key = String(input || "");
      const hit = infoCache.get(key);
      if (hit && Date.now() - hit.at < INFO_TTL_MS) return hit.value;
      const value = await api.videoInfo(input);
      infoCache.set(key, { at: Date.now(), value });
      return value;
    },
    userAgent: config.userAgent,
    timeoutMs: config.timeoutMs,
    holder,
    statePath,
    allowPost: config.allowPost === true,
    dryRun: config.dryRun !== false,

    /** 凭据形态（不回显值）。 */
    describe() {
      return {
        ...describeCookie(holder.cookie),
        source: holder.cookie ? (saved && config.cookie ? "config+file" : config.cookie ? "config" : "file") : "none",
        savedAt: holder.savedAt,
        statePath,
      };
    },

    /** 生成二维码（仅供工具层调用）。 */
    async loginGenerate() {
      const qr = await generateQr({ fetchImpl, userAgent: config.userAgent });
      holder.pendingKey = qr.key;
      return qr;
    },

    /**
     * 轮询二维码；成功后落盘并更新内存 cookie。
     * @param {string} [key] 不传则用最近一次生成的 key
     */
    async loginPoll(key) {
      const result = await pollQr({ fetchImpl, userAgent: config.userAgent }, key || holder.pendingKey);
      if (result.cookie) {
        holder.cookie = result.cookie;
        holder.savedAt = new Date().toISOString();
        holder.userId = describeCookie(result.cookie).userId;
        saveCredential(statePath, { cookie: result.cookie });
      }
      return result;
    },
  };
}
