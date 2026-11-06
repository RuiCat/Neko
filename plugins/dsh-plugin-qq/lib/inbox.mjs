/**
 * 入站收件箱：按 `message_id` 去重（含空消息），有界排队。
 *
 * 上游实测教训：轮询/重放模型下 `message_id` 是唯一幂等键；「没内容」的消息也必须记账，
 * 否则每一轮都会把同一个空包再处理一遍。
 */

const DEFAULT_MAX_QUEUE = 200;

/**
 * 建一个收件箱。
 * @param {{maxQueue?:number}} [options]
 */
export function createInbox(options = {}) {
  const maxQueue = Number.isFinite(options.maxQueue) ? options.maxQueue : DEFAULT_MAX_QUEUE;
  const seen = new Set();
  const queue = [];
  let dropped = 0;
  let received = 0;

  return {
    /**
     * 认领一个 message_id（去重记账，不入队）。空消息也要认领，避免每轮重处理同一个空包。
     * @param {object} message
     * @returns {boolean} 是否第一次见到
     */
    claim(message) {
      const id = message?.id;
      if (!id) return false;
      if (seen.has(id)) return false;
      seen.add(id);
      received += 1;
      return true;
    },

    /**
     * 入队（不做去重；调用方应先 claim）。
     * @param {object} message
     */
    enqueue(message) {
      if (!message?.text) return;
      if (queue.length >= maxQueue) {
        dropped += 1;
        return;
      }
      queue.push(message);
    },

    /**
     * 收一条消息：认领并入队；重复 id 返回 false。
     * @param {object} message normalizeMessage 的结果
     * @returns {boolean} 是否第一次见到
     */
    push(message) {
      if (!this.claim(message)) return false;
      this.enqueue(message);
      return true;
    },

    /**
     * 取走并清空前若干条。
     * @param {number} [limit]
     * @returns {object[]}
     */
    drain(limit = 50) {
      return queue.splice(0, Math.max(1, Math.floor(limit)));
    },

    /** 只看不移除。 */
    peek(limit = 50) {
      return queue.slice(0, Math.max(1, Math.floor(limit)));
    },

    /** 统计。 */
    stats() {
      return { queued: queue.length, received, dropped, seen: seen.size };
    },

    /** 清空（含去重表；测试与重启用）。 */
    reset() {
      seen.clear();
      queue.length = 0;
      dropped = 0;
      received = 0;
    },
  };
}
