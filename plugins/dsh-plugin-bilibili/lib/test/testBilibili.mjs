/**
 * B 站插件自测：引用解析、cookie 折叠、412 风控重试、接口映射、字幕选轨与渲染、工具与挂载。
 *
 * 全部用注入的假 fetch 跑，不依赖网络；真机联调另有 `DSH_BILIBILI_LIVE=1` 的可选手工步骤（见 README）。
 */
import assert from "node:assert/strict";
import { createCookieJar, setCookieHeaders, toCookieHeader } from "../cookieJar.mjs";
import { parseVideoRef, videoQuery } from "../bilibiliInput.mjs";
import { createBilibiliHttp } from "../bilibiliHttp.mjs";
import { createBilibiliApi } from "../bilibiliApi.mjs";
import { formatTime, normalizeUrl, pickSubtitle, renderSubtitleLines } from "../bilibiliSubtitle.mjs";
import { keyFromUrl, mixinKey, signParams } from "../wbiSign.mjs";
import { buildBilibiliTools } from "../bilibiliTools.mjs";
import { apply } from "../plugin.mjs";

let passed = 0;
const failures = [];

/** 跑一条用例，失败继续。 */
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

const VIEW = {
  bvid: "BV1xx411c7mD",
  aid: 12345,
  title: "测试视频",
  owner: { name: "UP" },
  duration: 125,
  pubdate: 1700000000,
  desc: "简介",
  stat: { view: 10, danmaku: 2, like: 3 },
  pages: [
    { cid: 111, page: 1, part: "P1", duration: 60 },
    { cid: 222, page: 2, part: "P2", duration: 65 },
  ],
};

/** 造一个按 URL 路由的假 fetch，并记录每次请求。 */
function makeFetch(routes) {
  const calls = [];
  const impl = async (url, init = {}) => {
    calls.push({ url: String(url), headers: init.headers || {} });
    for (const route of routes) {
      if (String(url).includes(route.match)) {
        const result = typeof route.reply === "function" ? route.reply(url, init, calls) : route.reply;
        return result;
      }
    }
    return new Response("not found", { status: 404 });
  };
  impl.calls = calls;
  return impl;
}

await check("引用解析：BV / av / 链接 / 分P", () => {
  assert.deepEqual(parseVideoRef("BV1xx411c7mD"), { bvid: "BV1xx411c7mD", page: 1 });
  assert.equal(parseVideoRef("https://www.bilibili.com/video/BV1xx411c7mD/?p=2").page, 2);
  assert.equal(parseVideoRef("https://www.bilibili.com/video/av12345").aid, 12345);
  assert.equal(parseVideoRef("av12345").aid, 12345);
  assert.throws(() => parseVideoRef(""), /需要/);
  assert.throws(() => parseVideoRef("随便一段文字"), /无法/);
});

await check("查询串：bvid 优先", () => {
  assert.equal(videoQuery({ bvid: "BV1xx411c7mD", page: 1 }), "bvid=BV1xx411c7mD");
  assert.equal(videoQuery({ aid: 42, page: 1 }), "aid=42");
});

await check("cookie 折叠：只留 name=value", () => {
  const header = toCookieHeader([
    "buvid3=abc; Path=/; Domain=.bilibili.com",
    "b_nut=123; Path=/",
    "invalid",
  ]);
  assert.equal(header, "buvid3=abc; b_nut=123");
});

await check("cookie 罐：首页引导后缓存，不重复请求", async () => {
  const fetchImpl = makeFetch([
    {
      match: "www.bilibili.com",
      reply: () =>
        new Response("<html></html>", {
          status: 200,
          headers: { "set-cookie": "buvid3=xyz; Path=/" },
        }),
    },
  ]);
  const jar = createCookieJar({ fetchImpl });
  assert.equal(await jar.header(), "buvid3=xyz");
  assert.equal(await jar.header(), "buvid3=xyz");
  assert.equal(fetchImpl.calls.length, 1);
});

await check("412 风控：请求前先带好 cookie，且被风控时不重试只报错", async () => {
  let viewCalls = 0;
  const fetchImpl = makeFetch([
    {
      match: "www.bilibili.com/",
      reply: () => new Response("<html></html>", { status: 200, headers: { "set-cookie": "buvid3=ok; Path=/" } }),
    },
    {
      match: "web-interface/view",
      reply: () => {
        viewCalls += 1;
        return new Response("<html>出错啦</html>", { status: 412 });
      },
    },
  ]);
  const http = createBilibiliHttp({ fetchImpl });
  await assert.rejects(() => http.getJson("https://api.bilibili.com/x/web-interface/view?bvid=BV1xx411c7mD"), /风控/);
  assert.equal(viewCalls, 1, "被风控时不能重试加压");
  const first = fetchImpl.calls.find((call) => call.url.includes("web-interface/view"));
  assert.match(first.headers.cookie, /buvid3=ok/, "首次请求就要带 cookie");
});

await check("接口错误归一：code=-101 提示登录 cookie", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    {
      match: "web-interface/view",
      reply: () => new Response(JSON.stringify({ code: -101, message: "账号未登录" }), { status: 200 }),
    },
  ]);
  const http = createBilibiliHttp({ fetchImpl });
  await assert.rejects(() => http.getJson("https://api.bilibili.com/x/web-interface/view?bvid=BV1xx411c7mD"), /-101/);
});

await check("非 JSON 响应：报出风控/登录提示而不是崩溃", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response("<html>blocked</html>", { status: 200 }) },
  ]);
  const http = createBilibiliHttp({ fetchImpl });
  await assert.rejects(() => http.getJson("https://api.bilibili.com/x/web-interface/view?bvid=BV1xx411c7mD"), /不是 JSON/);
});

await check("resolveCid：按分 P 取 cid", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response(JSON.stringify({ code: 0, data: VIEW }), { status: 200 }) },
  ]);
  const api = createBilibiliApi({ fetchImpl });
  const second = await api.resolveCid("BV1xx411c7mD", 2);
  assert.equal(second.cid, 222);
  assert.equal(second.title, "P2");
});

await check("字幕选轨：语言优先，其次中文，最后第一条", () => {
  const subtitles = [
    { lan: "en-US", lan_doc: "English", subtitle_url: "//a/en.json" },
    { lan: "zh-CN", lan_doc: "中文", subtitle_url: "//a/zh.json" },
  ];
  assert.equal(pickSubtitle(subtitles, "en-US").lan, "en-US");
  assert.equal(pickSubtitle(subtitles).lan, "zh-CN");
  assert.equal(pickSubtitle([]), null);
});

await check("字幕渲染：时间戳与截断", () => {
  assert.equal(normalizeUrl("//x/y.json"), "https://x/y.json");
  assert.equal(formatTime(65), "01:05");
  const rendered = renderSubtitleLines([{ from: 5, content: "你好" }, { from: 65, content: "世界" }], 100);
  assert.match(rendered.text, /\[00:05\] 你好/);
  assert.match(rendered.text, /\[01:05\] 世界/);
  const truncated = renderSubtitleLines([{ from: 0, content: "x".repeat(50) }], 10);
  assert.equal(truncated.truncated, true);
  assert.equal(truncated.text.length, 10);
});

await check("工具：视频详情可读渲染", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response(JSON.stringify({ code: 0, data: VIEW }), { status: 200 }) },
  ]);
  const [infoTool] = buildBilibiliTools({ fetchImpl });
  const result = await infoTool.execute({ input: "BV1xx411c7mD" }, {});
  assert.equal(result.ok, true);
  assert.match(result.text, /标题：测试视频/);
  assert.match(result.text, /P2/);
  assert.match(result.text, /播放：10/);
});

await check("工具：无字幕时给出可执行说明（不报错）", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response(JSON.stringify({ code: 0, data: VIEW }), { status: 200 }) },
    {
      match: "player/v2",
      reply: () => new Response(JSON.stringify({ code: 0, data: { subtitle: { subtitles: [] } } }), { status: 200 }),
    },
  ]);
  const [, subtitleTool] = buildBilibiliTools({ fetchImpl });
  const result = await subtitleTool.execute({ input: "BV1xx411c7mD" }, {});
  assert.equal(result.ok, true);
  assert.match(result.text, /没有字幕轨/);
  assert.match(result.text, /DSH_BILIBILI_COOKIE/);
});

await check("工具：有字幕时下载并渲染", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response(JSON.stringify({ code: 0, data: VIEW }), { status: 200 }) },
    {
      match: "player/v2",
      reply: () =>
        new Response(
          JSON.stringify({
            code: 0,
            data: { subtitle: { subtitles: [{ lan: "zh-CN", lan_doc: "中文（自动）", subtitle_url: "//sub/x.json" }] } },
          }),
          { status: 200 },
        ),
    },
    {
      match: "sub/x.json",
      reply: () =>
        new Response(JSON.stringify({ body: [{ from: 1.5, to: 3, content: "第一句" }] }), { status: 200 }),
    },
  ]);
  const [, subtitleTool] = buildBilibiliTools({ fetchImpl });
  const result = await subtitleTool.execute({ input: "BV1xx411c7mD" }, {});
  assert.equal(result.ok, true);
  assert.match(result.text, /\[00:01\] 第一句/);
});

await check("工具定义合法：宿主校验器（不可用则降级断言）", async () => {
  const hostPath =
    "/usr/lib/node_modules/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-tools/lib/index.js";
  let checker = null;
  try {
    const host = await import(hostPath);
    checker = host.assertSupportedJsonSchema;
  } catch {
    console.log("     （宿主 dsh-tools 不可用，降级为最小断言）");
  }
  for (const tool of buildBilibiliTools({})) {
    assert.equal(typeof tool.output.render, "function");
    assert.equal(typeof tool.execute, "function");
    if (checker) {
      checker(tool.parameters);
      checker(tool.output.schema);
    }
  }
});

await check("挂载：注册 7 个工具，DSH_BILIBILI=off 时不注册", () => {
  const registered = [];
  apply({ tools: { register: (definition) => registered.push(definition.name) } }, { statePath: "/tmp/none-state.json" });
  assert.deepEqual(registered, [
    "bilibili_video_info",
    "bilibili_subtitles",
    "bilibili_login",
    "bilibili_comments",
    "bilibili_danmaku",
    "bilibili_play_urls",
    "bilibili_comment_post",
  ]);

  const off = [];
  process.env.DSH_BILIBILI = "off";
  apply({ tools: { register: (definition) => off.push(definition.name) } }, {});
  delete process.env.DSH_BILIBILI;
  assert.deepEqual(off, []);
});

await check("setCookieHeaders：兼容带 getSetCookie 的 Response", () => {
  const response = new Response("", { status: 200, headers: { "set-cookie": "a=1; Path=/" } });
  assert.ok(Array.isArray(setCookieHeaders(response)));
});


await check("WBI：密钥提取、mixinKey 与签名形状", () => {
  assert.equal(keyFromUrl("https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png"), "7cd084941338484aae1ad9425b84077c");
  const mixin = mixinKey("7cd084941338484aae1ad9425b84077c", "4932caff0ff746eab6f01bf08b70ac45");
  assert.equal(mixin.length, 32);
  const signed = signParams({ bvid: "BV1xx411c7mD", cid: 111 }, mixin, 1700000000);
  assert.equal(signed.wts, 1700000000);
  assert.equal(typeof signed.w_rid, "string");
  assert.equal(signed.w_rid.length, 32);
  // 同一输入必须稳定（可复算），换 mixin 必须变
  assert.equal(signParams({ bvid: "BV1xx411c7mD", cid: 111 }, mixin, 1700000000).w_rid, signed.w_rid);
  assert.notEqual(signParams({ bvid: "BV1xx411c7mD", cid: 111 }, "x".repeat(32), 1700000000).w_rid, signed.w_rid);
});

await check("WBI：签名版接口优先，拿到密钥后带上 w_rid", async () => {
  const seen = [];
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    {
      match: "web-interface/nav",
      reply: () =>
        new Response(
          JSON.stringify({
            code: 0,
            data: {
              wbi_img: {
                img_url: "https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png",
                sub_url: "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png",
              },
            },
          }),
          { status: 200 },
        ),
    },
    {
      match: "player/wbi/v2",
      reply: (url) => {
        seen.push(url);
        return new Response(JSON.stringify({ code: 0, data: { subtitle: { subtitles: [] } } }), { status: 200 });
      },
    },
  ]);
  const api = createBilibiliApi({ fetchImpl });
  await api.playerInfo("BV1xx411c7mD", 111);
  assert.equal(seen.length, 1);
  assert.match(seen[0], /wts=\d+/);
  assert.match(seen[0], /w_rid=[0-9a-f]{32}/);
});

await check("WBI：签名版失败时降级到明文版", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/nav", reply: () => new Response(JSON.stringify({ code: -101, message: "未登录" }), { status: 200 }) },
    { match: "player/v2", reply: () => new Response(JSON.stringify({ code: 0, data: { subtitle: { subtitles: [] } } }), { status: 200 }) },
  ]);
  const api = createBilibiliApi({ fetchImpl });
  const data = await api.playerInfo("BV1xx411c7mD", 111);
  assert.deepEqual(data.subtitle.subtitles, []);
});

await check("字幕：need_login_subtitle 时提示登录而不是「无字幕」", async () => {
  const fetchImpl = makeFetch([
    { match: "www.bilibili.com/", reply: () => new Response("", { status: 200 }) },
    { match: "web-interface/view", reply: () => new Response(JSON.stringify({ code: 0, data: VIEW }), { status: 200 }) },
    { match: "web-interface/nav", reply: () => new Response(JSON.stringify({ code: -101 }), { status: 200 }) },
    {
      match: "player/v2",
      reply: () =>
        new Response(
          JSON.stringify({ code: 0, data: { need_login_subtitle: true, subtitle: { subtitles: [{ lan: "zh-CN", subtitle_url: "" }] } } }),
          { status: 200 },
        ),
    },
  ]);
  const [, subtitleTool] = buildBilibiliTools({ fetchImpl });
  const result = await subtitleTool.execute({ input: "BV1xx411c7mD" }, {});
  assert.equal(result.ok, true);
  assert.match(result.text, /need_login_subtitle/);
});

console.log(`\n${passed} 项通过，${failures.length} 项失败`);
if (failures.length) {
  for (const failure of failures) console.error(`- ${failure.name}: ${failure.error.stack}`);
  process.exit(1);
}
