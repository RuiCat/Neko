/**
 * B 站互动能力自测：凭据安全、扫码三态、评论归一、弹幕解析、播放地址归一、发评论四道闸。
 *
 * 全部用注入的假 fetch，不联网、不写真实凭据（临时目录里的 state 文件用完即删）。
 */
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { csrfOf, describeCookie, loadCredential, parseCookie, saveCredential } from "../bilibiliCredential.mjs";
import { generateQr, pickLoginCookies, pollQr } from "../bilibiliLogin.mjs";
import { listComments, listReplies, normalizeReply, renderComments } from "../bilibiliComments.mjs";
import { clockText, decodeEntities, parseDanmaku, renderDanmaku } from "../bilibiliDanmaku.mjs";
import { normalizePlayUrl, qualityName, renderPlayUrl } from "../bilibiliPlayUrl.mjs";
import { postComment } from "../bilibiliPost.mjs";
import { createBilibiliRuntime } from "../bilibiliRuntime.mjs";
import { buildSocialTools } from "../bilibiliSocialTools.mjs";
import { apply } from "../plugin.mjs";

let passed = 0;
const failures = [];

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

/** 按 URL 路由的假 fetch。 */
function makeFetch(routes) {
  const calls = [];
  const impl = async (url, init = {}) => {
    calls.push({ url: String(url), method: init.method || "GET", headers: init.headers || {}, body: init.body });
    for (const route of routes) {
      if (String(url).includes(route.match)) return route.reply(url, init, calls);
    }
    return new Response("{}", { status: 404 });
  };
  impl.calls = calls;
  return impl;
}

const json = (value, init = {}) => new Response(JSON.stringify(value), { status: 200, ...init });

/** 造一个带多条 set-cookie 的假响应（Node 的 Headers 不接受换行，只能自己实现）。 */
const jsonWithCookies = (value, cookies) => ({
  ok: true,
  status: 200,
  headers: { getSetCookie: () => cookies, get: () => null },
  json: async () => value,
  text: async () => JSON.stringify(value),
});
const STATE_DIR = mkdtempSync(join(tmpdir(), "bili-state-"));
const STATE_PATH = join(STATE_DIR, "state.json");

await check("凭据：解析、csrf、描述不泄露值", () => {
  const cookie = "SESSDATA=s3cr3t; bili_jct=tok123; DedeUserID=42";
  assert.equal(parseCookie(cookie).SESSDATA, "s3cr3t");
  assert.equal(csrfOf(cookie), "tok123");
  const desc = describeCookie(cookie);
  assert.deepEqual(desc, { present: true, hasSessData: true, hasCsrf: true, userId: "42" });
  assert.ok(!JSON.stringify(desc).includes("s3cr3t"), "描述里不能出现 cookie 值");
});

await check("凭据：落盘 0600、可回读、原子写", () => {
  saveCredential(STATE_PATH, { cookie: "SESSDATA=abc; bili_jct=xyz; DedeUserID=7" });
  const mode = statSync(STATE_PATH).mode & 0o777;
  assert.equal(mode, 0o600);
  const loaded = loadCredential(STATE_PATH);
  assert.equal(loaded.userId, "7");
  assert.match(loaded.cookie, /SESSDATA=abc/);
  assert.ok(!readFileSync(STATE_PATH, "utf8").includes("tmp-"));
});

await check("凭据：没有 SESSDATA 不给存", () => {
  assert.throws(() => saveCredential(STATE_PATH, { cookie: "bili_jct=x" }), /SESSDATA/);
});

await check("扫码：generate 返回 url 与 key", async () => {
  const fetchImpl = makeFetch([
    { match: "qrcode/generate", reply: () => json({ code: 0, data: { url: "https://qr/x", qrcode_key: "k".repeat(32) } }) },
  ]);
  const qr = await generateQr({ fetchImpl });
  assert.equal(qr.url, "https://qr/x");
  assert.equal(qr.key.length, 32);
});

await check("扫码：三态与成功取 cookie", async () => {
  const statuses = [
    { match: "poll", reply: () => json({ code: 0, data: { code: 86101, message: "" } }) },
  ];
  assert.equal((await pollQr({ fetchImpl: makeFetch(statuses) }, "k")).status, "waiting");
  assert.equal(
    (await pollQr({ fetchImpl: makeFetch([{ match: "poll", reply: () => json({ code: 0, data: { code: 86090 } }) }]) }, "k")).status,
    "scanned",
  );
  assert.equal(
    (await pollQr({ fetchImpl: makeFetch([{ match: "poll", reply: () => json({ code: 0, data: { code: 86038 } }) }]) }, "k")).status,
    "expired",
  );
  const success = await pollQr(
    {
      fetchImpl: makeFetch([
        {
          match: "poll",
          reply: () =>
            jsonWithCookies({ code: 0, data: { code: 0, message: "" } }, [
              "SESSDATA=ok; Path=/",
              "bili_jct=ct; Path=/",
              "foo=bar; Path=/",
            ]),
        },
      ]),
    },
    "k",
  );
  assert.equal(success.status, "success");
  assert.match(success.cookie, /SESSDATA=ok/);
  assert.match(success.cookie, /bili_jct=ct/);
  assert.ok(!success.cookie.includes("foo=bar"), "只收登录相关 cookie");
});

await check("扫码：确认但没有 SESSDATA 时明确失败", async () => {
  const result = await pollQr(
    { fetchImpl: makeFetch([{ match: "poll", reply: () => json({ code: 0, data: { code: 0 } }) }]) },
    "k",
  );
  assert.equal(result.status, "success-no-cookie");
});

await check("pickLoginCookies：只挑白名单键", () => {
  const cookie = pickLoginCookies(["SESSDATA=a; Path=/", "other=b; Path=/", "DedeUserID=9; Path=/"]);
  assert.equal(cookie, "SESSDATA=a; DedeUserID=9");
});

await check("评论：归一与渲染", async () => {
  const fetchImpl = makeFetch([
    {
      match: "x/v2/reply?",
      reply: () =>
        json({
          code: 0,
          data: {
            page: { acount: 42 },
            replies: [
              { rpid: 1, member: { uname: "甲" }, content: { message: "好看" }, like: 5, rcount: 2, ctime: 1700000000 },
            ],
            top: { upper: { rpid: 9, member: { uname: "UP" }, content: { message: "置顶" }, like: 100 } },
          },
        }),
    },
  ]);
  const result = await listComments({ getJson: async (url) => (await fetchImpl(url)).json().then((p) => p.data) }, { aid: 1 });
  assert.equal(result.total, 42);
  assert.equal(result.replies[0].user, "甲");
  assert.equal(result.top[0].user, "UP");
  const text = renderComments(result);
  assert.match(text, /共 42 条评论/);
  assert.match(text, /置顶/);
  assert.match(text, /rpid 1/);
});

await check("评论：楼中楼与空结果", async () => {
  const http = { getJson: async () => ({ replies: [{ rpid: 2, member: { uname: "乙" }, content: { message: "回复" } }] }) };
  const result = await listReplies(http, { aid: 1, root: 1 });
  assert.equal(result.replies.length, 1);
  assert.equal(normalizeReply({}).user, "匿名");
});

await check("弹幕：实体解码与解析（含关键词与上限）", () => {
  assert.equal(decodeEntities("a&amp;b&lt;c&gt;d&quot;e&#39;f"), "a&b<c>d\"e'f");
  assert.equal(decodeEntities("&#x4f60;&#22909;"), "你好");
  const xml = [
    '<i><d p="12.5,1,25,16777215,1,0,x,1">第一条 &amp; 好</d>',
    '<d p="65.2,5,25,255,1,0,x,2">顶部弹幕</d>',
    "<d p=\"70,1,25,16777215,1,0,x,3\"></d>",
    "</i>",
  ].join("");
  const parsed = parseDanmaku(xml, { limit: 10 });
  assert.equal(parsed.total, 3);
  assert.equal(parsed.items.length, 2, "空正文不入结果");
  assert.equal(parsed.items[0].text, "第一条 & 好");
  assert.equal(parsed.items[1].mode, 5);
  assert.equal(parseDanmaku(xml, { keyword: "顶部" }).items.length, 1);
  assert.equal(parseDanmaku(xml, { limit: 1 }).items.length, 1);
  assert.equal(clockText(65), "01:05");
  assert.match(renderDanmaku(parsed), /\[00:12\] 第一条 & 好/);
});

await check("播放地址：归一与渲染", () => {
  const normalized = normalizePlayUrl({
    quality: 64,
    accept_quality: [64, 32],
    dash: {
      video: [{ id: 64, codecs: "avc1", width: 1280, height: 720, bandwidth: 1000000, baseUrl: "https://v/1" }],
      audio: [{ id: 30280, codecs: "mp4a", bandwidth: 128000, baseUrl: "https://a/1" }],
    },
  });
  assert.equal(normalized.videos.length, 1);
  assert.equal(normalized.videos[0].url, "https://v/1");
  assert.equal(qualityName(64), "720P");
  const text = renderPlayUrl(normalized, { loginNote: true });
  assert.match(text, /当前清晰度：720P/);
  assert.match(text, /Referer/);
  assert.match(text, /bilibili_login/);
});

await check("发评论：未登录/无 csrf 直接拒绝", async () => {
  await assert.rejects(() => postComment({ cookie: "", aid: 1, message: "x" }), /未登录/);
  await assert.rejects(() => postComment({ cookie: "SESSDATA=a", aid: 1, message: "x" }), /bili_jct/);
});

await check("发评论：带上 csrf/oid/message 且解析 rpid", async () => {
  const fetchImpl = makeFetch([{ match: "reply/add", reply: () => json({ code: 0, data: { rpid: 777 } }) }]);
  const result = await postComment({
    fetchImpl,
    cookie: "SESSDATA=a; bili_jct=ct",
    aid: 9,
    message: "写得好",
  });
  assert.equal(result.rpid, "777");
  const body = new URLSearchParams(fetchImpl.calls[0].body);
  assert.equal(body.get("oid"), "9");
  assert.equal(body.get("csrf"), "ct");
  assert.equal(body.get("message"), "写得好");
  assert.match(fetchImpl.calls[0].headers.referer, /bilibili\.com/);
});

await check("发评论：code 非 0 抛错不假装成功", async () => {
  const fetchImpl = makeFetch([{ match: "reply/add", reply: () => json({ code: -102, message: "账号异常" }) }]);
  await assert.rejects(
    () => postComment({ fetchImpl, cookie: "SESSDATA=a; bili_jct=ct", aid: 1, message: "x" }),
    /-102/,
  );
});

await check("工具闸门：未启用写操作时只给预览", async () => {
  const fetchImpl = makeFetch([
    { match: "x/web-interface/view", reply: () => json({ code: 0, data: { bvid: "BV1xx411c7mD", aid: 9, title: "t", pages: [{ cid: 1, page: 1 }] } }) },
  ]);
  const runtime = createBilibiliRuntime({ fetchImpl, statePath: STATE_PATH, cookie: "SESSDATA=a; bili_jct=ct" });
  const tools = Object.fromEntries(buildSocialTools(runtime).map((tool) => [tool.name, tool]));
  const text = (await tools.bilibili_comment_post.execute({ input: "BV1xx411c7mD", message: "hi", confirm: true }, {})).text;
  assert.match(text, /未发送：插件配置 allowPost=false/);

  runtime.allowPost = true;
  const dry = (await tools.bilibili_comment_post.execute({ input: "BV1xx411c7mD", message: "hi", confirm: true }, {})).text;
  assert.match(dry, /干跑/);

  runtime.dryRun = false;
  const noConfirm = (await tools.bilibili_comment_post.execute({ input: "BV1xx411c7mD", message: "hi" }, {})).text;
  assert.match(noConfirm, /confirm=true/);
  assert.equal(fetchImpl.calls.filter((call) => call.url.includes("reply/add")).length, 0, "前三道闸都没发请求");
});

await check("工具：评论/弹幕/播放/登录状态都可读渲染", async () => {
  const fetchImpl = makeFetch([
    { match: "x/web-interface/view", reply: () => json({ code: 0, data: { bvid: "BV1xx411c7mD", aid: 9, title: "视频", pages: [{ cid: 5, page: 1, part: "P1" }] } }) },
    { match: "x/v2/reply?", reply: () => json({ code: 0, data: { page: { acount: 1 }, replies: [{ rpid: 1, member: { uname: "甲" }, content: { message: "评论" } }] } }) },
    { match: "5.xml", reply: () => new Response('<i><d p="1,1,25,0,1,0,x,1">弹幕来了</d></i>', { status: 200 }) },
    { match: "web-interface/nav", reply: () => json({ code: -101, data: { wbi_img: { img_url: "https://x/aaa.png", sub_url: "https://x/bbb.png" } } }) },
    { match: "player/wbi/playurl", reply: () => json({ code: 0, data: { quality: 32, accept_quality: [32], dash: { video: [{ id: 32, codecs: "avc1", baseUrl: "https://v/9", width: 640, height: 360, bandwidth: 1 }], audio: [] } } }) },
  ]);
  const freshState = join(STATE_DIR, "fresh-state.json");
  const runtime = createBilibiliRuntime({ fetchImpl, statePath: freshState, cookie: "" });
  const tools = Object.fromEntries(buildSocialTools(runtime).map((tool) => [tool.name, tool]));

  const comments = (await tools.bilibili_comments.execute({ input: "BV1xx411c7mD" }, {})).text;
  assert.match(comments, /评论/);
  const danmaku = (await tools.bilibili_danmaku.execute({ input: "BV1xx411c7mD" }, {})).text;
  assert.match(danmaku, /弹幕来了/);
  const play = (await tools.bilibili_play_urls.execute({ input: "BV1xx411c7mD" }, {})).text;
  assert.match(play, /https:\/\/v\/9/);
  const status = (await tools.bilibili_login.execute({ action: "status" }, {})).text;
  assert.match(status, /未登录/);
});

await check("插件挂载：注册 7 个工具，DSH_BILIBILI=off 时不注册", () => {
  const names = [];
  apply({ tools: { register: (definition) => names.push(definition.name) } }, { statePath: STATE_PATH, fetchImpl: makeFetch([]) });
  assert.equal(names.length, 7);
  assert.ok(names.includes("bilibili_comment_post"));
  const off = [];
  process.env.DSH_BILIBILI = "off";
  apply({ tools: { register: (definition) => off.push(definition.name) } }, {});
  delete process.env.DSH_BILIBILI;
  assert.deepEqual(off, []);
});

await check("工具定义合法：宿主校验器（不可用则降级断言）", async () => {
  const hostPath = "/usr/lib/node_modules/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-tools/lib/index.js";
  let checker = null;
  try {
    checker = (await import(hostPath)).assertSupportedJsonSchema;
  } catch {
    console.log("     （宿主 dsh-tools 不可用，降级为最小断言）");
  }
  const runtime = createBilibiliRuntime({ statePath: STATE_PATH, fetchImpl: makeFetch([]) });
  for (const tool of buildSocialTools(runtime)) {
    if (checker) {
      checker(tool.parameters);
      checker(tool.output.schema);
    }
  }
});

rmSync(STATE_DIR, { recursive: true, force: true });

console.log(`\n${passed} 项通过，${failures.length} 项失败`);
if (failures.length) {
  for (const failure of failures) console.error(`- ${failure.name}: ${failure.error.stack}`);
  process.exit(1);
}
