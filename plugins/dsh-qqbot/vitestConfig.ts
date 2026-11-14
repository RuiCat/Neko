/**
 * Vitest 配置：在默认的 *.test.ts 之外纳入 camelCase 命名的 *Test.ts
 * （本项目新增测试文件遵循命名规范，不沿用上游的 dot 命名）。
 * 由 package.json 的 test 脚本以 `--config vitestConfig.ts` 显式加载。
 */
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['src/**/*.test.ts', 'src/**/*Test.ts'],
  },
});
