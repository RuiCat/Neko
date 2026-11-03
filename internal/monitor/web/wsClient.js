// wsClient.js —— /monitor/ws 客户端：连接与重连、重连后等 snapshot、按帧 type 分发、订阅/退订与心跳。
const DEFAULT_TOPICS = ["state", "diagnostics", "logs", "tick"];
const TOKEN_PROTOCOL_PREFIX = "astrbox.token.";
const PING_INTERVAL_MS = 30000;
const PONG_TIMEOUT_MS = 90000;
const RECONNECT_MIN_MS = 1000;
const RECONNECT_MAX_MS = 30000;

/** 判断一个值是否是 K2 约定的帧（必须带字符串 type）。 */
const isFrame = (value) => value !== null && typeof value === "object" && typeof value.type === "string";

/**
 * 创建 /monitor/ws 客户端实例。
 * K2：每次连接（含重连）在 hello 后都会重新收到 snapshot；snapshot 之前的 delta 一律丢弃。
 * options: { url, token, topics, entity, paths, onFrame, onConnectionChange }
 */
export function createWsClient(options = {}) {
  const { url, token = "", topics = DEFAULT_TOPICS, entity = "", paths = [], onFrame, onConnectionChange } = options;
  const subscribed = new Set(topics);
  const filters = { entity, paths: [...paths] };
  let socket = null;
  let connection = "offline";
  let synced = false;
  let retryDelay = RECONNECT_MIN_MS;
  let reconnectTimer = null;
  let pingTimer = null;
  let lastPongAt = 0;
  let closedByUser = false;

  const setConnection = (next) => {
    if (next === connection) return;
    connection = next;
    onConnectionChange?.(next);
  };

  const send = (payload) => {
    if (socket !== null && socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(payload));
  };

  const stopPing = () => {
    if (pingTimer === null) return;
    clearInterval(pingTimer);
    pingTimer = null;
  };

  const startPing = () => {
    stopPing();
    lastPongAt = Date.now();
    pingTimer = setInterval(() => {
      if (socket === null) return;
      if (Date.now() - lastPongAt > PONG_TIMEOUT_MS) {
        socket.close();
        return;
      }
      send({ type: "ping" });
    }, PING_INTERVAL_MS);
  };

  const stopReconnect = () => {
    if (reconnectTimer === null) return;
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  };

  const scheduleReconnect = () => {
    if (closedByUser || reconnectTimer !== null) return;
    setConnection("reconnecting");
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      open();
    }, retryDelay);
    retryDelay = Math.min(RECONNECT_MAX_MS, retryDelay * 2);
  };

  const handleMessage = (event) => {
    let frame = null;
    try {
      frame = JSON.parse(event.data);
    } catch (error) {
      return;
    }
    if (!isFrame(frame)) return;
    if (frame.type === "hello") {
      retryDelay = RECONNECT_MIN_MS;
      startPing();
      setConnection("live");
    }
    if (frame.type === "pong") lastPongAt = Date.now();
    if (frame.type === "snapshot") synced = true;
    // K2：delta 只在 snapshot 之后；重连后未收到 snapshot 前不吞掉乱序增量。
    if (frame.type === "delta" && !synced) return;
    onFrame?.(frame);
  };

  const handleClose = (target) => {
    if (socket !== target) return;
    socket = null;
    synced = false;
    stopPing();
    if (closedByUser) {
      setConnection("offline");
      return;
    }
    scheduleReconnect();
  };

  const buildTarget = () => {
    const params = new URLSearchParams();
    if (subscribed.size > 0) params.set("topics", [...subscribed].join(","));
    if (filters.entity !== "") params.set("entity", filters.entity);
    if (filters.paths.length > 0) params.set("paths", filters.paths.join(","));
    const query = params.toString();
    return query === "" ? url : `${url}?${query}`;
  };

  const open = () => {
    if (socket !== null || closedByUser) return;
    synced = false;
    setConnection("reconnecting");
    const protocols = token === "" ? null : [`${TOKEN_PROTOCOL_PREFIX}${token}`];
    socket = protocols === null ? new WebSocket(buildTarget()) : new WebSocket(buildTarget(), protocols);
    const activeSocket = socket;
    socket.addEventListener("message", handleMessage);
    socket.addEventListener("error", () => activeSocket.close());
    socket.addEventListener("close", () => handleClose(activeSocket));
  };

  const connect = () => {
    closedByUser = false;
    open();
  };

  const close = () => {
    closedByUser = true;
    stopReconnect();
    stopPing();
    if (socket !== null) socket.close();
    socket = null;
    synced = false;
    setConnection("offline");
  };

  const restart = () => {
    closedByUser = false;
    stopReconnect();
    retryDelay = RECONNECT_MIN_MS;
    if (socket !== null) socket.close();
    socket = null;
    open();
  };

  const subscribe = (nextTopics = [], nextFilters = {}) => {
    for (const topic of nextTopics) subscribed.add(topic);
    if (typeof nextFilters.entity === "string") filters.entity = nextFilters.entity;
    if (Array.isArray(nextFilters.paths)) filters.paths = [...nextFilters.paths];
    send({ type: "subscribe", topics: [...subscribed], entity: filters.entity, paths: filters.paths });
  };

  const unsubscribe = (nextTopics = []) => {
    for (const topic of nextTopics) subscribed.delete(topic);
    send({ type: "unsubscribe", topics: [...nextTopics] });
  };

  return { connect, close, restart, subscribe, unsubscribe };
}
