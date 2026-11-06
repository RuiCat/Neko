/**
 * 登录凭据：cookie 的落盘、读取与安全描述。
 *
 * 三条硬规则：
 * 1. 凭据只写 `~/.dsh/bilibili-state.json`（默认，0600），**不写进项目仓库**；
 * 2. 任何返回值/日志都**不回显 cookie 值**，只描述"有哪些字段"；
 * 3. 写盘走临时文件 + rename，避免半截 JSON。
 */
import { chmodSync, existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

const DEFAULT_STATE_PATH = join(homedir(), ".dsh", "bilibili-state.json");

/** 解析后的 cookie 映射（name → value）。 */
export function parseCookie(cookie) {
  const map = {};
  for (const part of String(cookie || "").split(";")) {
    const text = part.trim();
    if (!text || !text.includes("=")) continue;
    const index = text.indexOf("=");
    map[text.slice(0, index).trim()] = text.slice(index + 1).trim();
  }
  return map;
}

/** 从 cookie 里取 CSRF（`bili_jct`），发写请求必须带。 */
export function csrfOf(cookie) {
  return parseCookie(cookie).bili_jct || "";
}

/**
 * 只描述凭据形态，不泄露值。
 * @param {string} cookie
 * @returns {{present:boolean, hasSessData:boolean, hasCsrf:boolean, userId:string}}
 */
export function describeCookie(cookie) {
  const map = parseCookie(cookie);
  return {
    present: Boolean(cookie),
    hasSessData: Boolean(map.SESSDATA),
    hasCsrf: Boolean(map.bili_jct),
    userId: map.DedeUserID || "",
  };
}

/** 解析状态文件路径（配置优先，其次环境变量，最后默认）。 */
export function statePathOf(config = {}) {
  if (typeof config.statePath === "string" && config.statePath) return config.statePath;
  if (process.env.DSH_BILIBILI_STATE) return process.env.DSH_BILIBILI_STATE;
  return DEFAULT_STATE_PATH;
}

/**
 * 读凭据文件；不存在或坏掉都返回 null（不抛错，登录工具会提示重新扫码）。
 * @param {string} path
 * @returns {{cookie:string, savedAt:string, userId:string}|null}
 */
export function loadCredential(path) {
  if (!existsSync(path)) return null;
  try {
    const data = JSON.parse(readFileSync(path, "utf8"));
    if (!data || typeof data.cookie !== "string" || !data.cookie) return null;
    return { cookie: data.cookie, savedAt: String(data.savedAt || ""), userId: String(data.userId || "") };
  } catch {
    return null;
  }
}

/**
 * 保存凭据（0600，临时文件 + rename）。
 * @param {string} path
 * @param {{cookie:string}} input
 * @returns {{ok:boolean, path:string, userId:string}}
 */
export function saveCredential(path, input) {
  const cookie = String(input.cookie || "");
  if (!parseCookie(cookie).SESSDATA) throw new Error("没有 SESSDATA，这不算登录成功");
  mkdirSync(dirname(path), { recursive: true });
  const payload = {
    cookie,
    savedAt: new Date().toISOString(),
    userId: parseCookie(cookie).DedeUserID || "",
  };
  const temporary = `${path}.tmp-${process.pid}-${Date.now()}`;
  writeFileSync(temporary, `${JSON.stringify(payload, null, 2)}\n`, { encoding: "utf8", mode: 0o600 });
  renameSync(temporary, path);
  try {
    chmodSync(path, 0o600);
  } catch {
    /* 权限位在某些文件系统上不可改，不影响功能 */
  }
  return { ok: true, path, userId: payload.userId };
}
