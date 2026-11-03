// apiClient.js —— 监视器 REST 客户端：token 引导、统一错误体解析、日期/值工具。

const TOKEN_KEY = "neko.monitor.token";
const QUERY_TOKEN = new URLSearchParams(location.search).get("token") ?? "";
export const TOKEN = QUERY_TOKEN !== "" ? QUERY_TOKEN : (localStorage.getItem(TOKEN_KEY) ?? "");
if (QUERY_TOKEN !== "") localStorage.setItem(TOKEN_KEY, QUERY_TOKEN);

export const TOPICS = ["state", "diagnostics", "logs", "tick"];
const API_BASE = "/monitor/api";

// K1 错误体统一 {"ok":false,"error":{code,message}}；非 2xx 或 ok:false 一律抛 code/status。
export const api = async (path, init = {}) => {
  const headers = { Accept: "application/json", ...(init.headers ?? {}) };
  if (TOKEN !== "") headers["X-Auth-Token"] = TOKEN;
  const response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  const body = await response.json().catch(() => ({ ok: false, error: { code: "bad_response", message: "响应不是 JSON" } }));
  if (response.ok && body.ok !== false) return body;
  const error = body.error ?? { code: `http_${response.status}`, message: "请求失败" };
  throw Object.assign(new Error(error.message ?? error.code), { code: error.code, status: response.status });
};

export const today = () => {
  const now = new Date();
  const pad = (value) => String(value).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
};

export const parseValue = (raw) => {
  try {
    return JSON.parse(raw);
  } catch (error) {
    return raw;
  }
};
