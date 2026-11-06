/**
 * NapCat WebUI 桥接：**登录由后端完成**，插件只做三件事——拿 WebUI 凭证、要二维码、
 * 查登录状态。QQ 协议本身永远不在这里实现。
 *
 * 接口事实（来自 NapCatQQ 源码 `packages/napcat-webui-backend`）：
 * - `POST /api/auth/login`  body `{hash}`，`hash = sha256(token + ".napcat")` 十六进制；
 *   返回 `data.Credential`（base64 的 JSON，1 小时有效）；
 * - `POST /api/QQLogin/GetQQLoginQrcode`  → `data.qrcode`（二维码内容 URL）；
 * - `POST /api/QQLogin/CheckLoginStatus`  → `data.isLogin / loginPhase / qrcodeurl / loginError`；
 * - 鉴权头 `Authorization: Bearer <Credential>`；响应统一 `{code, message, data}`，`code!==0` 即失败。
 *
 * 凭证只在内存与 `~/.dsh/napcat-webui-credential.json`（0600）之间流转，不回显。
 */
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

const DEFAULT_BASE = "http://127.0.0.1:6099";
const DEFAULT_CREDENTIAL_PATH = join(homedir(), ".dsh", "napcat-webui-credential.json");
const CREDENTIAL_TTL_MS = 55 * 60 * 1000;

/** NapCat WebUI 的密码哈希：sha256(token + ".napcat")。 */
export function tokenHash(token) {
  return createHash("sha256").update(`${token}.napcat`).digest("hex");
}

/** 解析凭证缓存路径。 */
export function credentialPathOf(config = {}) {
  return config.webuiCredentialPath || process.env.DSH_NAPCAT_CREDENTIAL_PATH || DEFAULT_CREDENTIAL_PATH;
}

/** 读缓存凭证；过期或损坏返回 null。 */
export function loadCredential(path) {
  if (!existsSync(path)) return null;
  try {
    const data = JSON.parse(readFileSync(path, "utf8"));
    if (!data?.credential || !data?.at) return null;
    if (Date.now() - Number(data.at) > CREDENTIAL_TTL_MS) return null;
    return String(data.credential);
  } catch {
    return null;
  }
}

/** 写缓存凭证（0600，原子替换）。 */
export function saveCredential(path, credential) {
  mkdirSync(dirname(path), { recursive: true });
  const temporary = `${path}.tmp-${process.pid}-${Date.now()}`;
  writeFileSync(temporary, `${JSON.stringify({ credential, at: Date.now() }, null, 2)}\n`, {
    encoding: "utf8",
    mode: 0o600,
  });
  renameSync(temporary, path);
  try {
    chmodSync(path, 0o600);
  } catch {
    /* 某些文件系统不支持改权限位 */
  }
}

/**
 * 建 WebUI 客户端。
 * @param {{webuiBase?:string, webuiToken?:string, webuiCredentialPath?:string,
 *   fetchImpl?:Function, timeoutMs?:number}} [config]
 */
export function createNapcatWebui(config = {}) {
  const base = String(config.webuiBase || process.env.DSH_NAPCAT_WEBUI || DEFAULT_BASE).replace(/\/+$/, "");
  const token = String(config.webuiToken || process.env.DSH_NAPCAT_TOKEN || "");
  const credentialPath = credentialPathOf(config);
  const fetchImpl = config.fetchImpl || globalThis.fetch;
  const timeoutMs = Number.isFinite(config.timeoutMs) ? config.timeoutMs : 10000;
  let credential = loadCredential(credentialPath) || "";

  /** 发一个 POST，返回 data；code!==0 抛错。 */
  async function post(path, body, useAuth = true) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    let response;
    try {
      response = await fetchImpl(`${base}${path}`, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          ...(useAuth && credential ? { authorization: `Bearer ${credential}` } : {}),
        },
        body: JSON.stringify(body || {}),
        signal: controller.signal,
      });
    } catch (error) {
      if (error?.name === "AbortError") throw new Error(`NapCat WebUI 超时（${timeoutMs}ms）：${base}${path}`);
      throw new Error(
        `连不上 NapCat WebUI（${base}）：${error instanceof Error ? error.message : String(error)}。` +
          "确认 NapCat 已启动且 WebUI 端口可达。",
      );
    } finally {
      clearTimeout(timer);
    }
    const text = await response.text();
    let payload;
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`NapCat WebUI 返回的不是 JSON（HTTP ${response.status}）：${text.slice(0, 120)}`);
    }
    if (payload.code !== 0) throw new Error(`NapCat WebUI 失败：${payload.message || `code=${payload.code}`}`);
    return payload.data;
  }

  return {
    /** 当前配置的后端地址。 */
    endpoint() {
      return base;
    },

    /** 是否配了 WebUI 密码（没有就什么都做不了）。 */
    hasToken() {
      return Boolean(token);
    },

    /** 拿凭证：有缓存就用，否则用 token 换。 */
    async ensureCredential() {
      if (credential) return credential;
      if (!token) throw new Error("没有配置 WebUI 密码（config.webuiToken 或 DSH_NAPCAT_TOKEN）");
      const data = await post("/api/auth/login", { hash: tokenHash(token) }, false);
      if (!data?.Credential) throw new Error("登录响应里没有 Credential（密码错误或启用了 2FA）");
      credential = String(data.Credential);
      saveCredential(credentialPath, credential);
      return credential;
    },

    /** 取登录二维码（内容 URL）。 */
    async qrcode() {
      await this.ensureCredential();
      const data = await post("/api/QQLogin/GetQQLoginQrcode", {});
      if (!data?.qrcode) throw new Error("后端没有返回二维码（可能已经登录，或 QQ 内核尚未初始化登录）");
      return String(data.qrcode);
    },

    /** 刷新二维码。 */
    async refreshQrcode() {
      await this.ensureCredential();
      const data = await post("/api/QQLogin/RefreshQRcode", {});
      return this.qrcodeAfterRefresh(data);
    },

    /** 刷新后重新取一次（后端行为不完全一致，取不到就退回直接取）。 */
    async qrcodeAfterRefresh(data) {
      if (data?.qrcode) return String(data.qrcode);
      return this.qrcode();
    },

    /** 查登录状态。 */
    async status() {
      await this.ensureCredential();
      const data = await post("/api/QQLogin/CheckLoginStatus", {});
      return {
        isLogin: data?.isLogin === true,
        isOffline: data?.isOffline === true,
        loginPhase: String(data?.loginPhase || ""),
        coreReady: data?.coreReady === true,
        qrcodeUrl: String(data?.qrcodeurl || ""),
        loginError: String(data?.loginError || ""),
      };
    },
  };
}
