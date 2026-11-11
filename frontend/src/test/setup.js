/**
 * vitest 全局 setup（jsdom 环境补 jsdom 缺失的浏览器 API）。
 * node 环境纯函数测试无 window，全部判空跳过。
 */
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// vitest globals 默认关闭时 RTL 的 auto-cleanup 不生效——在此统一注册
afterEach(() => {
  cleanup()
})

// antd Grid 响应式 hook（useBreakpoint → responsiveObserver）依赖 matchMedia，jsdom 未实现
if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false
    },
  })
}

// antd 弹层/Tooltip 及后续 ECharts 依赖 ResizeObserver，jsdom 未实现
if (typeof window !== 'undefined' && !window.ResizeObserver) {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  window.ResizeObserver = ResizeObserverStub
  if (typeof globalThis !== 'undefined' && !globalThis.ResizeObserver) {
    globalThis.ResizeObserver = ResizeObserverStub
  }
}
