import { createRouter, createWebHistory } from "vue-router";

const routes = [
  { path: "/", redirect: "/dashboard" },
  { path: "/dashboard", name: "dashboard", component: () => import("@/views/Dashboard.vue") },
  { path: "/metrics", name: "metrics", component: () => import("@/views/Metrics.vue") },
  // 健康评估
  { path: "/topology", name: "topology", component: () => import("@/views/Topology.vue") },
  { path: "/health", name: "health", component: () => import("@/views/Health.vue") },
  // GPU 故障
  { path: "/fault/knowledge", name: "fault-knowledge", component: () => import("@/views/FaultKnowledge.vue") },
  { path: "/fault/predict", name: "fault-predict", component: () => import("@/views/FaultPredict.vue") },
  { path: "/fault/rca", name: "fault-rca", component: () => import("@/views/FaultRCA.vue") },
  { path: "/fault/assistant", name: "fault-assistant", component: () => import("@/views/Assistant.vue") },
  // 演示工具
  { path: "/inject", name: "inject", component: () => import("@/views/FaultInject.vue") }
];

export default createRouter({ history: createWebHistory(), routes });
