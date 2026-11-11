import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // REST 统一走 Go 后端 /api/**（internal/app/router.go 挂载面：/api/v1/** 业务路由
      // + /healthz、/readyz 探针；configs/config.yaml server.port 默认 8080）
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
  test: {
    // 组件测试默认 jsdom；纯函数测试文件头 /** @vitest-environment node */ 按文件覆盖提速
    environment: 'jsdom',
    include: ['src/**/*.test.{js,jsx}'],
    // jsdom 缺失的浏览器 API（matchMedia/ResizeObserver——antd 组件依赖）全局补齐
    setupFiles: ['src/test/setup.js'],
  },
})
