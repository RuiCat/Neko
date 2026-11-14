/**
 * detailGrid.mjs —— 实体详情网格的行高平衡：同行卡片等高，同时保留值的完整高度。
 *
 * 为什么不用纯 CSS：Chrome 在「grid + align-items:stretch」下算行高时不把被拉伸项的内容计入，
 * 行会锁在最小高度、长值被 clip 裁掉（实测锁在 102px，内容 324px）。改为渲染后用 JS 量一次
 * 每行最高的卡片，给同行其它卡片补 min-height：值仍完整显示，同行卡片上下边对齐。
 */

/**
 * 按行补齐 min-height（只改高度，不动内容）。
 * 先把上一轮补的高度清掉再量，避免行高被一轮轮垫高。
 * @param {HTMLElement} list 描述列表根节点
 * @returns {void}
 */
function applyRowHeights(list) {
  const items = [...list.querySelectorAll(".petui-description-list__item")];
  for (const item of items) item.style.minHeight = "";
  const measure = () => {
    const rows = new Map();
    for (const item of items) {
      const top = Math.round(item.offsetTop);
      const row = rows.get(top);
      if (row === undefined) rows.set(top, [item]);
      else row.push(item);
    }
    for (const row of rows.values()) {
      const height = Math.max(...row.map((item) => item.getBoundingClientRect().height));
      for (const item of row) item.style.minHeight = `${Math.ceil(height)}px`;
    }
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(() => requestAnimationFrame(measure));
  else measure();
}

/**
 * 建一个行高平衡器：每次列表重渲染后 schedule 一次，窗口尺寸变化时也会重新平衡。
 * @param {HTMLElement} list 描述列表根节点
 * @returns {{schedule: () => void, dispose: () => void}} 平衡器
 */
export function createRowBalancer(list) {
  let frame = 0;
  const schedule = () => {
    if (typeof requestAnimationFrame !== "function") {
      applyRowHeights(list);
      return;
    }
    if (frame !== 0) cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => {
      frame = 0;
      applyRowHeights(list);
    });
  };
  const onResize = () => schedule();
  window.addEventListener("resize", onResize);
  return {
    schedule,
    dispose() {
      window.removeEventListener("resize", onResize);
      if (frame !== 0) cancelAnimationFrame(frame);
    },
  };
}
