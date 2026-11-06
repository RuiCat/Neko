/**
 * B 站只读接口封装：视频详情、分 P 列表、播放器信息（含字幕清单）。
 *
 * 播放器信息走「WBI 签名优先、明文兜底」两条路：签名版对已登录会话更稳，
 * 明文版在未登录/签名失败时仍可能返回公开字幕。
 */
import { createBilibiliHttp } from "./bilibiliHttp.mjs";
import { parseVideoRef, videoQuery } from "./bilibiliInput.mjs";
import { createWbiSigner, toQuery } from "./wbiSign.mjs";

const VIEW_API = "https://api.bilibili.com/x/web-interface/view";
const PAGELIST_API = "https://api.bilibili.com/x/player/pagelist";
const PLAYER_WBI_API = "https://api.bilibili.com/x/player/wbi/v2";
const PLAYER_API = "https://api.bilibili.com/x/player/v2";
const PLAYURL_API = "https://api.bilibili.com/x/player/wbi/playurl";

/**
 * 建 API 门面。
 * @param {object} [config] cookie / userAgent / timeoutMs / fetchImpl
 */
export function createBilibiliApi(config = {}) {
  const http = createBilibiliHttp(config);
  const signer = createWbiSigner(http);

  return {
    /** 只读接口用的底层能力（评论区/弹幕等模块复用；不是稳定对外契约）。 */
    raw: { http, signer },

    /** 视频详情（标题/UP/时长/简介/分P）。 */
    async videoInfo(input) {
      const ref = parseVideoRef(input);
      return http.getJson(`${VIEW_API}?${videoQuery(ref)}`);
    },

    /** 分 P 列表；view 不可用时也能拿到 cid。 */
    async pageList(input) {
      const ref = parseVideoRef(input);
      return http.getJson(`${PAGELIST_API}?${videoQuery(ref)}`);
    },

    /**
     * 播放器信息：先 WBI 签名，失败再明文。
     * @param {string} bvid
     * @param {number} cid
     */
    async playerInfo(bvid, cid) {
      const signed = await signer.sign({ bvid, cid });
      if (signed) {
        try {
          return await http.getJson(`${PLAYER_WBI_API}?${toQuery(signed)}`);
        } catch {
          /* 签名版失败时落到明文版，由下面这行给出最终结论 */
        }
      }
      return http.getJson(`${PLAYER_API}?bvid=${encodeURIComponent(bvid)}&cid=${cid}`);
    },

    /**
     * 取播放地址（dash）。
     * @param {string} bvid
     * @param {number} cid
     * @param {{qn?:number, fnval?:number}} [options]
     */
    async playUrl(bvid, cid, options = {}) {
      const params = {
        bvid,
        cid,
        qn: Number.isFinite(options.qn) ? options.qn : 80,
        fnval: Number.isFinite(options.fnval) ? options.fnval : 16,
        fourk: 1,
      };
      const signed = await signer.sign(params);
      if (!signed) throw new Error("拿不到 WBI 密钥（nav 接口不可用），无法请求播放地址");
      return http.getJson(`${PLAYURL_API}?${toQuery(signed)}`);
    },

    /**
     * 取某个分 P 的 cid：优先用详情里的 pages，详情失败时退回 pagelist。
     * @param {string} input 视频引用
     * @param {number} page 分 P 号（1 起）
     */
    async resolveCid(input, page = 1) {
      try {
        const info = await this.videoInfo(input);
        const wanted = (info.pages || []).find((item) => item.page === page) || (info.pages || [])[0];
        if (wanted?.cid) return { bvid: info.bvid, cid: wanted.cid, title: wanted.part || info.title };
      } catch (error) {
        if (!/HTTP 412|code=-404/.test(String(error?.message))) throw error;
      }
      const pages = await this.pageList(input);
      const wanted = pages.find((item) => item.page === page) || pages[0];
      if (!wanted?.cid) throw new Error(`拿不到 cid（分 P ${page} 不存在？）`);
      return { bvid: parseVideoRef(input).bvid, cid: wanted.cid, title: wanted.part || "" };
    },
  };
}
