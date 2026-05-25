<template>
  <div class="dash">
    <!-- 顶部统计卡 -->
    <div class="stats">
      <div class="stat-card">
        <div class="stat-label">GPU 总数</div>
        <div class="big-stat stat-val">{{ ov.total_gpu ?? 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">平均健康分</div>
        <div class="big-stat stat-val" :style="{ color: scoreColor(ov.avg_score) }">
          {{ (ov.avg_score ?? 0).toFixed(1) }}
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">故障数(critical+failed)</div>
        <div class="big-stat stat-val" :style="{ color: 'var(--lv-failed)' }">
          {{ ov.fault_count ?? 0 }}
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">健康率</div>
        <div class="big-stat stat-val" :style="{ color: 'var(--lv-healthy)' }">
          {{ healthyPct }}%
        </div>
      </div>
    </div>

    <div class="grid">
      <!-- 等级分布环形图 -->
      <div class="panel">
        <div class="panel-title">健康等级分布</div>
        <div style="padding: 16px">
          <v-chart :option="pieOption" autoresize style="height: 300px" />
        </div>
      </div>

      <!-- 风险最高的卡 -->
      <div class="panel">
        <div class="panel-title">风险最高的 GPU</div>
        <div class="risk-list">
          <div v-for="g in ov.riskiest || []" :key="g.gpu_uuid" class="risk-row">
            <span class="mono risk-uuid">{{ g.gpu_uuid }}</span>
            <span :class="['level-badge', 'lv-' + g.level]">{{ levelName(g.level) }}</span>
            <span class="mono risk-score" :style="{ color: scoreColor(g.score) }">
              {{ g.score.toFixed(1) }}
            </span>
          </div>
          <div v-if="!(ov.riskiest || []).length" class="empty">暂无数据，请确认仿真与评分服务已运行</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { api } from "@/api";
import VChart from "vue-echarts";
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { PieChart } from "echarts/charts";
import { TooltipComponent, LegendComponent } from "echarts/components";

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent]);

const ov = ref<any>({ total_gpu: 0, avg_score: 0, fault_count: 0, level_dist: {}, riskiest: [] });
let timer: any;

const levelColors: Record<string, string> = {
  healthy: "#22c55e", sub_healthy: "#84cc16", warning: "#eab308",
  critical: "#f97316", failed: "#ef4444"
};
const levelNames: Record<string, string> = {
  healthy: "健康", sub_healthy: "亚健康", warning: "警告",
  critical: "严重", failed: "故障"
};
function levelName(l: string) { return levelNames[l] || l; }
function scoreColor(s: number) {
  if (s >= 90) return "#22c55e";
  if (s >= 75) return "#84cc16";
  if (s >= 60) return "#eab308";
  if (s >= 30) return "#f97316";
  return "#ef4444";
}

const healthyPct = computed(() => {
  const total = ov.value.total_gpu || 0;
  if (!total) return 0;
  const h = ov.value.level_dist?.healthy || 0;
  return ((h / total) * 100).toFixed(0);
});

const pieOption = computed(() => {
  const dist = ov.value.level_dist || {};
  const order = ["healthy", "sub_healthy", "warning", "critical", "failed"];
  return {
    tooltip: { trigger: "item" },
    legend: { bottom: 0, textStyle: { color: "#9aa7b4" } },
    series: [{
      type: "pie", radius: ["45%", "72%"],
      itemStyle: { borderColor: "#0f141c", borderWidth: 2 },
      label: { color: "#e6edf3" },
      data: order.filter(k => dist[k]).map(k => ({
        name: levelNames[k], value: dist[k],
        itemStyle: { color: levelColors[k] }
      }))
    }]
  };
});

async function refresh() {
  try { ov.value = await api.dashboard(); } catch {}
}
onMounted(() => { refresh(); timer = setInterval(refresh, 15000); });
onUnmounted(() => clearInterval(timer));
</script>

<style scoped>
.dash { display: flex; flex-direction: column; gap: 20px; }
.stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; }
.stat-card {
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
}
.stat-label { font-size: 12px; color: var(--text-2); letter-spacing: 0.05em; margin-bottom: 12px; }
.stat-val { font-size: 38px; color: var(--text-0); }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.risk-list { padding: 8px 0; max-height: 300px; overflow: auto; }
.risk-row {
  display: grid;
  grid-template-columns: 1fr auto auto;
  align-items: center;
  gap: 12px;
  padding: 9px 16px;
  border-bottom: 1px solid var(--bg-2);
}
.risk-uuid { font-size: 12px; color: var(--text-1); }
.risk-score { font-size: 15px; font-weight: 600; min-width: 48px; text-align: right; }
.empty { padding: 40px; text-align: center; color: var(--text-2); font-size: 13px; }
</style>
