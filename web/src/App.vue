<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="overrides">
    <n-message-provider>
      <n-dialog-provider>
        <div class="shell">
          <aside class="sidebar">
            <div class="brand">
              <div class="brand-mark">◇</div>
              <div class="brand-text">
                <div class="brand-name">GPU HEALTH</div>
                <div class="brand-sub">监测平台</div>
              </div>
            </div>
            <n-menu
              :options="menuOptions"
              :value="activeKey"
              :expanded-keys="expandedKeys"
              @update:value="onNav"
              @update:expanded-keys="(k) => (expandedKeys = k)"
            />
          </aside>
          <main class="main">
            <header class="topbar">
              <div class="crumb">{{ currentTitle }}</div>
              <div class="clock mono">{{ clock }}</div>
            </header>
            <div class="content">
              <router-view />
            </div>
          </main>
        </div>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { h, ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { darkTheme } from "naive-ui";

const router = useRouter();
const route = useRoute();

const activeKey = computed(() => String(route.name || "dashboard"));
const expandedKeys = ref<string[]>(["assess", "fault"]);

const titleMap: Record<string, string> = {
  dashboard: "健康大盘",
  metrics: "指标系统",
  topology: "集群拓扑",
  health: "健康值",
  "fault-knowledge": "故障知识图谱",
  "fault-predict": "故障预测",
  "fault-rca": "故障根因分析",
  inject: "故障注入"
};
const currentTitle = computed(() => titleMap[activeKey.value] || "");

const menuOptions = [
  { label: "健康大盘", key: "dashboard" },
  { label: "指标系统", key: "metrics" },
  {
    label: "健康评估", key: "assess",
    children: [
      { label: "集群拓扑", key: "topology" },
      { label: "健康值", key: "health" }
    ]
  },
  {
    label: "GPU 故障", key: "fault",
    children: [
      { label: "故障知识图谱", key: "fault-knowledge" },
      { label: "故障预测", key: "fault-predict" },
      { label: "故障根因分析", key: "fault-rca" }
    ]
  },
  { label: "故障注入(演示)", key: "inject" }
];

function onNav(key: string) {
  // 只有叶子节点才跳转
  if (titleMap[key]) router.push({ name: key });
}

const overrides = {
  common: {
    primaryColor: "#38bdf8",
    primaryColorHover: "#7dd3fc",
    bodyColor: "#0a0e14",
    cardColor: "#0f141c",
    borderColor: "#243040"
  }
};

// 顶栏时钟
const clock = ref("");
let timer: any;
function tick() {
  const d = new Date();
  clock.value = d.toLocaleString("zh-CN", { hour12: false });
}
onMounted(() => {
  tick();
  timer = setInterval(tick, 1000);
});
onUnmounted(() => clearInterval(timer));
</script>

<style scoped>
.shell { display: flex; height: 100vh; }
.sidebar {
  width: 224px;
  background: var(--bg-1);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 18px;
  border-bottom: 1px solid var(--border);
}
.brand-mark {
  font-size: 24px;
  color: var(--accent);
  text-shadow: 0 0 12px var(--accent);
}
.brand-name {
  font-family: var(--font-mono);
  font-weight: 600;
  font-size: 15px;
  letter-spacing: 0.08em;
  color: var(--text-0);
}
.brand-sub { font-size: 11px; color: var(--text-2); letter-spacing: 0.1em; }
.main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  border-bottom: 1px solid var(--border);
  background: var(--bg-1);
}
.crumb { font-size: 14px; font-weight: 600; letter-spacing: 0.04em; }
.clock { font-size: 12px; color: var(--text-2); }
.content { flex: 1; overflow: auto; padding: 20px 24px; }
</style>
