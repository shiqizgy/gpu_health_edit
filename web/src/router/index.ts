import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";

// 懒加载兜底：动态 import 失败（多因发布新版后旧 chunk 失效/dev 缓存过期）时，
// 自动整页刷新一次去拉最新资源；用 sessionStorage 防止无限刷新循环。
function lazy(loader: () => Promise<any>) {
  return () =>
      loader().catch((err) => {
        const key = "chunk_reloaded_at";
        const last = Number(sessionStorage.getItem(key) || 0);
        // 10 秒内只自动刷新一次，避免死循环
        if (Date.now() - last > 10000) {
          sessionStorage.setItem(key, String(Date.now()));
          window.location.reload();
          // 返回一个 pending 的 promise，等待页面刷新
          return new Promise(() => {});
        }
        throw err;
      });
}

const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/dashboard" },
  { path: "/dashboard", name: "dashboard", component: lazy(() => import("@/views/Dashboard.vue")) },
  { path: "/metrics/dcgm", name: "metrics-dcgm", component: lazy(() => import("@/views/Metrics.vue")), meta: { deviceType: "gpu" } },
  { path: "/metrics/npu",  name: "metrics-npu",  component: lazy(() => import("@/views/Metrics.vue")), meta: { deviceType: "npu" } },
  // 健康评估
  { path: "/topology", name: "topology", component: lazy(() => import("@/views/Topology.vue")) },
  { path: "/health", name: "health", component: lazy(() => import("@/views/Health.vue")) },
  { path: "/health/gpu/:uuid", name: "gpu-detail", component: lazy(() => import("@/views/GpuHealthDetail.vue")) },
  // GPU 故障
  { path: "/fault/knowledge", name: "fault-knowledge", component: lazy(() => import("@/views/FaultKnowledge.vue")) },
  { path: "/fault/pool", name: "fault-pool", component: lazy(() => import("@/views/FaultPool.vue")) },
  { path: "/fault/predict", name: "fault-predict", component: lazy(() => import("@/views/FaultPredict.vue")) },
  { path: "/fault/rca", name: "fault-rca", component: lazy(() => import("@/views/FaultRCA.vue")) },
  { path: "/fault/assistant", name: "fault-assistant", component: lazy(() => import("@/views/Assistant.vue")) },
  // 演示工具
  { path: "/inject", name: "inject", component: lazy(() => import("@/views/FaultInject.vue")) }
];

const router = createRouter({ history: createWebHistory(), routes });

// 双保险：全局捕获导航中的动态导入失败
router.onError((error) => {
  const msg = String(error?.message || "");
  if (msg.includes("Failed to fetch dynamically imported module") || msg.includes("Outdated Optimize Dep")) {
    const key = "chunk_reloaded_at";
    const last = Number(sessionStorage.getItem(key) || 0);
    if (Date.now() - last > 10000) {
      sessionStorage.setItem(key, String(Date.now()));
      window.location.reload();
    }
  }
});

export default router;
