/**
 * QQ 插件自测：消息折文本、长文切段、去重收件箱、OneBot 鉴权头、干跑与白名单、轮询归一、工具与挂载。
 *
 * 全部用注入的假 fetch，不连任何 OneBot 后端；因此自测通过**不代表**真机能收发。
 * 用法：`node lib/test/testQq.mjs`（退出码 0 全过）。
 */
import assert from "node:assert/strict";

import { createInbox } from "../inbox.mjs";
import { messageToText, normalizeMessage, splitLongText } from "../messageText.mjs";
import { createOneBotClient } from "../oneBotClient.mjs";
import { createQqService } from "../qqService.mjs";
import { buildQqTools } from "../qqTools.mjs";
import { apply } from "../plugin.mjs";
import { createForwarder, toUserMessage } from "../forwarder.mjs";

let passed = 0;
const failures = [];

/** 跑一条用例。 */
async function check(name, fn) {
  try {
    await fn();
    passed += 1;
    console.log(`ok   ${name}`);
  } catch (error) {
    failures.push({ name, error });
    console.log(`FAIL ${name}: ${error.message}`);
  }
}

/** 假 OneBot 后端：记录请求，按 action 返回。 */
function makeFetch(handler) {
  const calls = [];
  const impl = async (url, init = {}) => {
    const body = init.body ? JSON.parse(init.body) : {};
    calls.push({ url: String(url), headers: init.headers || {}, body });
    const action = String(url).split("/").pop();
    const reply = handler(action, body, calls);
    return new Response(JSON.stringify(reply), { status: 200 });
  };
  impl.calls = calls;
  return impl;
}

const OK_SEND = { status: "ok", retcode: 0, data: { message_id: 999 } };

await check("messageToText：字符串与段数组", () => {
  assert.equal(messageToText("你好"), "你好");
  assert.equal(
    messageToText([
      { type: "text", data: { text: "看这个 " } },
      { type: "at", data: { qq: "123" } },
      { type: "image", data: {} },
    ]),
    "看这个 @123[图片]",
  );
  assert.equal(messageToText([]), "");
});

await check("splitLongText：短文本原样、长文本切段且不丢字", () => {
  assert.deepEqual(splitLongText("短"), ["短"]);
  assert.deepEqual(splitLongText(""), []);
  const text = "第一段。".repeat(200);
  const chunks = splitLongText(text, 30);
  assert.ok(chunks.length > 1);
  assert.ok(chunks.every((chunk) => chunk.length <= 30));
  assert.equal(chunks.join(""), text.replace(/\s/g, ""));
});

await check("normalizeMessage：群/私聊统一形态", () => {
  const group = normalizeMessage(
    { message_id: 1, group_id: 100, time: 1700000000, sender: { card: "群名片", user_id: 7 }, message: "hi" },
    "group",
  );
  assert.deepEqual(
    { id: group.id, kind: group.kind, conversation: group.conversation, sender: group.sender, text: group.text },
    { id: "1", kind: "group", conversation: "100", sender: "群名片", text: "hi" },
  );
  const priv = normalizeMessage({ message_id: 2, user_id: 7, sender: { nickname: "昵称" }, message: "yo" }, "private");
  assert.equal(priv.conversation, "7");
  assert.equal(priv.sender, "昵称");
});

await check("收件箱：按 message_id 去重，空消息也记账", () => {
  const inbox = createInbox({ maxQueue: 2 });
  assert.equal(inbox.push({ id: "a", text: "1" }), true);
  assert.equal(inbox.push({ id: "a", text: "1" }), false);
  assert.equal(inbox.push({ id: "b", text: "" }), true, "空消息也要标记已处理");
  assert.equal(inbox.push({ id: "b", text: "" }), false);
  assert.equal(inbox.stats().queued, 1, "空文本不占队列");
  inbox.push({ id: "c", text: "2" });
  inbox.push({ id: "d", text: "3" });
  assert.equal(inbox.stats().dropped, 1, "超出上限要记溢出");
  assert.deepEqual(inbox.drain(5).map((item) => item.id), ["a", "c"]);
  assert.equal(inbox.stats().queued, 0);
});

await check("OneBot 鉴权：令牌为空不带 Authorization，非空是 Bearer", async () => {
  const fetchImpl = makeFetch(() => OK_SEND);
  await createOneBotClient({ fetchImpl }).call("send_group_msg", { group_id: 1, message: "x" });
  await createOneBotClient({ fetchImpl, token: "tk" }).call("send_group_msg", { group_id: 1, message: "x" });
  assert.equal(fetchImpl.calls[0].headers.authorization, undefined);
  assert.equal(fetchImpl.calls[1].headers.authorization, "Bearer tk");
});

await check("OneBot 错误归一：retcode 非 0 抛带原因的错", async () => {
  const fetchImpl = makeFetch(() => ({ status: "failed", retcode: 1400, wording: "bad request" }));
  await assert.rejects(() => createOneBotClient({ fetchImpl }).call("send_group_msg", {}), /retcode=1400/);
});

await check("服务默认：未启用 + 干跑，发送被拒绝且不联网", async () => {
  const fetchImpl = makeFetch(() => OK_SEND);
  const service = createQqService({ fetchImpl, watchGroups: ["1"] });
  const status = service.status();
  assert.equal(status.enabled, false);
  assert.equal(status.dryRun, true);
  const result = await service.send("group", "1", "hi");
  assert.equal(result.ok, false);
  assert.equal(fetchImpl.calls.length, 0);
});

await check("白名单：目标不在 watch 列表时拒绝", async () => {
  const fetchImpl = makeFetch(() => OK_SEND);
  const service = createQqService({ fetchImpl, enabled: true, dryRun: false, watchGroups: ["1"] });
  const result = await service.send("group", "2", "hi");
  assert.equal(result.ok, false);
  assert.match(result.reason, /白名单/);
  assert.equal(fetchImpl.calls.length, 0);
});

await check("干跑：允许目标也不真正上网", async () => {
  const fetchImpl = makeFetch(() => OK_SEND);
  const service = createQqService({ fetchImpl, enabled: true, dryRun: true, watchGroups: ["1"] });
  const result = await service.send("group", "1", "hi");
  assert.equal(result.ok, true);
  assert.equal(result.dryRun, true);
  assert.equal(fetchImpl.calls.length, 0);
});

await check("真发：长文本切段，逐段调用 send_group_msg", async () => {
  const fetchImpl = makeFetch(() => OK_SEND);
  const service = createQqService({
    fetchImpl,
    enabled: true,
    dryRun: false,
    watchGroups: ["1"],
    maxTextChars: 10,
    segmentDelayMs: 0,
  });
  const result = await service.send("group", "1", "abcdefghijklmnopqrstuvwxyz");
  assert.equal(result.ok, true);
  assert.ok(result.chunks >= 3);
  assert.equal(fetchImpl.calls.length, result.chunks);
  assert.ok(fetchImpl.calls.every((call) => call.url.endsWith("/send_group_msg")));
});

await check("轮询：群/好友历史归一入队并去重", async () => {
  const fetchImpl = makeFetch((action) => {
    if (action === "get_group_msg_history") {
      return { status: "ok", retcode: 0, data: { messages: [{ message_id: 11, group_id: 1, message: "群消息" }] } };
    }
    return { status: "ok", retcode: 0, data: { messages: [{ message_id: 12, user_id: 2, message: "私聊消息" }] } };
  });
  const service = createQqService({ fetchImpl, watchGroups: ["1"], watchUsers: ["2"] });
  const first = await service.pollOnce();
  assert.equal(first.errors.length, 0);
  assert.equal(first.stats.received, 2);
  await service.pollOnce();
  assert.equal(service.status().received, 2, "同一批消息重复轮询不应重复计数");
  const drained = service.drain(10);
  assert.deepEqual(drained.map((item) => item.text), ["群消息", "私聊消息"]);
});

await check("轮询失败：错误被收集且不影响其他会话", async () => {
  const fetchImpl = async (url) => {
    if (String(url).includes("get_group_msg_history")) throw new Error("boom");
    return new Response(JSON.stringify({ status: "ok", retcode: 0, data: { messages: [] } }), { status: 200 });
  };
  const service = createQqService({ fetchImpl, watchGroups: ["1"], watchUsers: ["2"] });
  const result = await service.pollOnce();
  assert.equal(result.errors.length, 1);
  assert.match(service.status().lastError, /群 1/);
});

await check("工具：qq_send / qq_inbox / qq_status 都有可读返回", async () => {
  const fetchImpl = makeFetch((action) => {
    if (action === "get_group_msg_history") {
      return { status: "ok", retcode: 0, data: { messages: [{ message_id: 1, group_id: 9, sender: { card: "甲" }, message: "在吗" }] } };
    }
    return OK_SEND;
  });
  const service = createQqService({ fetchImpl, watchGroups: ["9"] });
  const tools = Object.fromEntries(buildQqTools(service).map((tool) => [tool.name, tool]));
  assert.deepEqual(Object.keys(tools), ["qq_send", "qq_inbox", "qq_status"]);

  const send = await tools.qq_send.execute({ target: "group", id: "9", text: "hi" }, {});
  assert.match(send.text, /未启用/);

  await service.pollOnce();
  const inbox = await tools.qq_inbox.execute({ limit: 10 }, {});
  assert.equal(inbox.ok, true);
  assert.match(inbox.text, /在吗/);

  const status = await tools.qq_status.execute({}, {});
  assert.match(status.text, /启用：否/);
  assert.match(status.text, /干跑：是/);
});

await check("工具定义合法：宿主校验器（不可用则降级断言）", async () => {
  const hostPath = "/usr/lib/node_modules/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-tools/lib/index.js";
  let checker = null;
  try {
    const host = await import(hostPath);
    checker = host.assertSupportedJsonSchema;
  } catch {
    console.log("     （宿主 dsh-tools 不可用，降级为最小断言）");
  }
  const service = createQqService({});
  for (const tool of buildQqTools(service)) {
    assert.equal(typeof tool.output.render, "function");
    if (checker) {
      checker(tool.parameters);
      checker(tool.output.schema);
    }
  }
});

await check("挂载：注册 4 个工具；未启用时不起轮询；DSH_QQ=off 时不注册", () => {
  const registered = [];
  const events = [];
  const ctx = {
    tools: { register: (definition) => registered.push(definition.name) },
    on: (event) => events.push(event),
  };
  apply(ctx, {});
  assert.deepEqual(registered, ["qq_send", "qq_inbox", "qq_status", "qq_login"]);
  assert.ok(events.includes("dispose"));

  const off = [];
  process.env.DSH_QQ = "off";
  apply({ tools: { register: (definition) => off.push(definition.name) }, on: () => {} }, {});
  delete process.env.DSH_QQ;
  assert.deepEqual(off, []);
});


await check("转发：toUserMessage 形状与来源标注", () => {
  const message = toUserMessage({ id: "1", kind: "group", conversation: "9", sender: "甲", text: "在吗" });
  assert.equal(message.role, "user");
  assert.equal(message.source.kind, "user");
  assert.match(message.content[0].text, /\[QQ 群 9\] 甲：在吗/);
  assert.equal(typeof message.id, "string");
});

await check("转发：配置了会话才投递，找不到会话时回退入队", async () => {
  const received = [];
  const agents = { get: () => ({ followup: (message) => received.push(message) }) };
  const forwarder = createForwarder({ agents, sessionId: "s-1", enabled: true });
  assert.equal(forwarder.active(), true);
  const result = forwarder.forward([{ id: "1", kind: "private", conversation: "2", sender: "乙", text: "hi" }]);
  assert.equal(result.forwarded, 1);
  assert.equal(received.length, 1);

  const missing = createForwarder({ agents: { get: () => undefined }, sessionId: "nope", enabled: true });
  assert.equal(missing.forward([{ id: "1", text: "hi" }]).forwarded, 0);

  const disabled = createForwarder({ agents, sessionId: "", enabled: true });
  assert.equal(disabled.active(), false);
});

await check("转发开启时：轮询的新消息进会话而不是本机队列", async () => {
  const fetchImpl = makeFetch((action) =>
    action === "get_group_msg_history"
      ? { status: "ok", retcode: 0, data: { messages: [{ message_id: 51, group_id: 9, sender: { card: "丙" }, message: "转发我" }] } }
      : { status: "ok", retcode: 0, data: { messages: [] } },
  );
  const delivered = [];
  const service = createQqService({
    fetchImpl,
    watchGroups: ["9"],
    forwardSessionId: "s-1",
    agents: { get: () => ({ followup: (message) => delivered.push(message) }) },
  });
  await service.pollOnce();
  assert.equal(delivered.length, 1);
  assert.match(delivered[0].content[0].text, /转发我/);
  assert.equal(service.status().queued, 0, "已转发的消息不该再占队列");
});

console.log(`\n${passed} 项通过，${failures.length} 项失败`);
if (failures.length) {
  for (const failure of failures) console.error(`- ${failure.name}: ${failure.error.stack}`);
  process.exit(1);
}
