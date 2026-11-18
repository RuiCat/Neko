/**
 * 工作区挂载重试 — 刚创建的会话在持久化头文件落盘前 attach 会被宿主拒绝
 * （`cannot attach session ...: its stored header ...`），表现为主/子会话
 * 掉进「未分组」。这里先轮询 `sessionPersistence.stat` 直到会话可见，再 attach。
 *
 * 全部失败只记日志：挂载只影响侧边栏归属，不应影响消息处理。
 */
import type { HostAdapter } from '../host/adapter.ts';
import type { Logger } from '../types.ts';

/** 轮询次数与间隔（总计约 1s，覆盖新建会话的落盘窗口）。 */
const ATTACH_ATTEMPTS = 10;
const ATTACH_INTERVAL_MS = 100;

/**
 * 等会话头落盘后挂到工作区；宿主无法判定存在性时直接尝试。
 * @param host - 宿主适配器
 * @param sessionId - 会话 id
 * @param workspaceDir - 工作区路径（惰性取，避免缓存漂移）
 * @param workspaceTitle - 工作区显示名（惰性取）
 * @param logger - 日志器
 * @returns 完成信号（永不 reject）
 */
export async function attachWorkspaceWhenPersisted(
  host: HostAdapter,
  sessionId: string,
  workspaceDir: () => string,
  workspaceTitle: () => string,
  logger: Logger,
): Promise<void> {
  try {
    for (let attempt = 0; attempt < ATTACH_ATTEMPTS; attempt++) {
      const exists = await host.sessionExists(sessionId).catch(() => undefined);
      // true = 已落盘；undefined = 宿主没有判定手段，直接尝试 attach。
      if (exists !== false) break;
      await new Promise<void>((resolve) => { setTimeout(resolve, ATTACH_INTERVAL_MS); });
    }
    await host.attachSessionToWorkspace(sessionId, workspaceDir(), workspaceTitle());
  } catch (error) {
    logger.warn(`attach workspace failed for ${sessionId}: ${error instanceof Error ? error.message : String(error)}`);
  }
}
