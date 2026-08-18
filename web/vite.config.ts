import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) }
  },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true }
    }
  },
  // 预声明需要预构建的依赖：echarts 采用子路径按需导入，
  // 显式列出可避免 dev 运行时“临时发现新依赖 → 重新优化 → 旧 chunk 504”
  optimizeDeps: {
    include: [
      "echarts/core",
      "echarts/charts",
      "echarts/components",
      "echarts/renderers",
      "vue-echarts",
      "naive-ui",
      "axios",
      "pinia",
      "vue-router"
    ]
  },
  build: {
    // 生产打包时把 echarts 单独拆成一个 vendor chunk，
    // 便于长期缓存，也避免和业务代码混在一起频繁失效
    rollupOptions: {
      output: {
        manualChunks: {
          echarts: ["echarts/core", "echarts/charts", "echarts/components", "echarts/renderers", "vue-echarts"],
          "naive-ui": ["naive-ui"]
        }
      }
    }
  }
});
