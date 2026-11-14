/**
 * bundle 相对入口：给 `mcp-astrbox` 这一行用，替代裸包名 'dsh-mcp-client'。
 *
 * 为什么需要它：DSH 的 loader 只在 **profile 目录**上解析裸包名，而 `link:` 安装的
 * bundle 不会被 pnpm 安装它自己的依赖，所以裸名在 link 安装下必然解析失败。
 * patch 里的相对名会被锚定到 bundle 目录（profile/node_modules/dsh-astrbox），
 * 再顺着符号链接落回本仓库；真实安装（拷贝）时同一条相对路径也成立。
 *
 * 本文件只做转发，实现仍在 plugins/dsh-mcp-client（保留组件可单独替换）。
 */
export * from 'dsh-mcp-client'
