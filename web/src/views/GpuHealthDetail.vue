<template>
  <div class="gd">
    <!-- 顶栏 -->
    <div class="gd-topbar">
      <div class="gd-title">
        <n-button quaternary size="small" @click="goBack">←</n-button>
        <span class="gd-crumb">
          {{ meta?.cluster_name || "集群" }} / 节点 {{ meta?.node_ip || "-" }}
          / GPU {{ meta?.gpu_index ?? "-" }}
          <span v-if="meta?.model"> · {{ meta.model }}</span>
        </span>
      </div>
      <div class="gd-ctrl">
        <n-select v-model:value="rangePreset" size="small" :options="rangeOptions"
          style="width: 120px" @update:value="reloadTimeSeries" />
        <n-button size="small" @click="reloadAll" :loading="loading">刷新</n-button>
      </div>
    </div>

    <n-spin :show="loading">
      <div class="gd-body">
        <!-- 左列 -->
        <div class="gd-col-left">
          <!-- 健康画像 -->
          <div class="panel gd-panel">
            <div class="panel-title">健康画像</div>
            <div class="gd-radar-wrap">
              <v-chart v-if="hasDimensions" :option="radarOption" autoresize style="height: 260px" />
              <div v-else class="gd-radar-empty">
                <div class="gd-score-big" :style="{ color: scoreColor(score) }">{{ score.toFixed(0) }}</div>
                <div class="gd-empty-hint">该卡健康，暂无维度明细</div>
              </div>
              <div class="gd-actions">
                <n-button size="small" secondary @click="goAssistant">✦ AI 诊断</n-button>
                <n-button size="small" secondary @click="goAssistant">◈ 诊断报告</n-button>
              </div>
            </div>
          </div>

          <!-- 异常指标 -->
          <div class="panel gd-panel">
            <div class="panel-title">异常指标 {{ abnormal.length }} 项</div>
            <div class="gd-abn-list">
              <div v-if="!abnormal.length" class="gd-empty-hint" style="padding:20px">各项指标正常。</div>
              <div v-for="m in abnormal" :key="m.metric_key" class="gd-abn-card"
                :class="m.severity === 'critical' ? 'sev-crit' : 'sev-warn'">
                <div class="gd-abn-head">
                  <span class="gd-abn-name">{{ m.display_name }}</span>
                  <span class="mono gd-abn-val">{{ fmtNum(m.value) }}<span v-if="m.unit"> {{ m.unit }}</span></span>
                </div>
                <div class="gd-abn-sub mono">
                  {{ dimName(m.dimension) }} · {{ thresholdText(m) }} · 得分 {{ m.score.toFixed(0) }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 右列 -->
        <div class="gd-col-right">
          <!-- 健康分趋势 -->
          <div class="panel gd-panel">
            <div class="panel-title">健康分趋势 <span class="gd-sub">历史回溯 · 事件标注</span></div>
            <div style="padding: 12px 16px">
              <v-chart v-if="trendPoints.length" :option="trendOption" autoresize style="height: 220px" />
              <div v-else class="gd-empty-hint" style="padding:40px">暂无趋势数据</div>
            </div>
          </div>

          <!-- 指标明细小多图 -->
          <div class="panel gd-panel">
            <div class="panel-title">指标明细 <span class="gd-sub">时间轴与上图联动</span></div>
            <div class="gd-metric-grid">
              <div v-for="s in series" :key="s.metric" class="gd-metric-cell">
                <div class="gd-metric-name">{{ s.display_name || s.metric }}
                  <span class="gd-unit" v-if="s.unit">{{ s.unit }}</span></div>
                <v-chart :option="miniOption(s)" autoresize style="height: 120px" />
              </div>
              <div v-if="!series.length" class="gd-empty-hint" style="grid-column:1/-1;padding:30px">暂无指标数据</div>
            </div>
          </div>
        </div>
      </div>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useMessage } from "naive-ui";
import { api } from "@/api";
import VChart from "vue-echarts";
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { LineChart, RadarChart } from "echarts/charts";
import {
  GridComponent, TooltipComponent, RadarComponent,
  MarkPointComponent, MarkLineComponent, DataZoomComponent, GraphicComponent
} from "echarts/components";

use([
  CanvasRenderer, LineChart, RadarChart,
  GridComponent, TooltipComponent, RadarComponent,
  MarkPointComponent, MarkLineComponent, DataZoomComponent, GraphicComponent
]);

const route = useRoute();
const router = useRouter();
const message = useMessage();
const uuid = String(route.params.uuid || "");

const loading = ref(false);

// ---- 详情数据 ----
const snapshot = ref<any>(null);
const dimensions = ref<any[]>([]);
const abnormal = ref<any[]>([]);
const faults = ref<any[]>([]);
const meta = ref<any>(null);

// ---- 趋势 / 指标时序 ----
const trendPoints = ref<any[]>([]);
const trendEvents = ref<any[]>([]);
const series = ref<any[]>([]);

const rangePreset = ref<"1h" | "6h" | "24h" | "7d">("6h");
const rangeOptions = [
  { label: "近 1 小时", value: "1h" },
  { label: "近 6 小时", value: "6h" },
  { label: "近 24 小时", value: "24h" },
  { label: "近 7 天", value: "7d" },
];

const dimNameMap: Record<string, string> = {
  // GPU (DCGM)
  "thermal温度散热": "温度散热",
  "power功耗电源": "功耗电源",
  "memory显存可靠性": "显存可靠性",
  "pcie总线": "PCIe总线",
  "nvlink片间互连（DCGM）": "NVLink互连",
  "driver驱动（DCGM）": "驱动",
  "compute算力性能": "算力性能",
  // NPU（昇腾）
  "interconnect昇腾互连通信": "昇腾互连",
  "reliability昇腾可靠性与运行状态": "可靠性",
  "auxiliary辅助与效率指标": "辅助效率",
};

function dimName(d: string) { return dimNameMap[d] || d; }

const levelNames: Record<string, string> = {
  healthy: "健康", sub_healthy: "亚健康", warning: "警告", critical: "严重", failed: "故障",
};

function scoreColor(s: number) {
  if (s >= 90) return "#22c55e";
  if (s >= 75) return "#84cc16";
  if (s >= 60) return "#eab308";
  if (s >= 30) return "#f97316";
  return "#ef4444";
}
function fmtNum(v: any) {
  if (v === null || v === undefined) return "—";
  return typeof v === "number" ? Number(v.toFixed(2)) : v;
}
function thresholdText(m: any) {
  const parts: string[] = [];
  if (m.warn_threshold !== null && m.warn_threshold !== undefined) parts.push(`告警 ${m.warn_threshold}`);
  if (m.crit_threshold !== null && m.crit_threshold !== undefined) parts.push(`严重 ${m.crit_threshold}`);
  if (parts.length === 0 && m.normal_range) return `正常 ${m.normal_range}`;
  return parts.join(" / ") || "—";
}

const score = computed(() => Number(snapshot.value?.score ?? 0));
const hasDimensions = computed(() => dimensions.value && dimensions.value.length > 0);

// ---- 雷达图 ----
const radarOption = computed(() => {
  const dims = dimensions.value || [];
  return {
    tooltip: { trigger: "item" },
    radar: {
      radius: "62%",
      splitNumber: 4,
      axisName: { color: "#9aa7b4", fontSize: 12 },
      splitLine: { lineStyle: { color: "#243040" } },
      splitArea: { areaStyle: { color: ["transparent"] } },
      axisLine: { lineStyle: { color: "#243040" } },
      indicator: dims.map((d: any) => ({ name: dimName(d.dimension), max: 100 })),
    },
    graphic: [{
      type: "text", left: "center", top: "center",
      style: { text: score.value.toFixed(0), fontSize: 34, fontWeight: 600, fill: scoreColor(score.value) },
    }],
    series: [{
      type: "radar", symbol: "circle", symbolSize: 4,
      areaStyle: { color: "rgba(56,189,248,0.18)" },
      lineStyle: { color: "#38bdf8" },
      itemStyle: { color: "#38bdf8" },
      data: [{ value: dims.map((d: any) => Number(d.score.toFixed(1))) }],
    }],
  };
});

// ---- 趋势图 ----
const trendOption = computed(() => {
  const pts = trendPoints.value.map((p: any) => [new Date(p.ts).getTime(), Number(p.score.toFixed(1))]);
  const marks = trendEvents.value.map((e: any) => ({
    coord: nearestPoint(e.ts, pts),
    value: e.label,
    itemStyle: { color: e.type === "xid" ? "#ef4444" : "#f97316" },
  }));
  return {
    grid: { left: 40, right: 16, top: 20, bottom: 28 },
    tooltip: { trigger: "axis" },
    xAxis: { type: "time", axisLabel: { color: "#5e6b78" }, axisLine: { lineStyle: { color: "#243040" } } },
    yAxis: {
      type: "value", min: 0, max: 100,
      alignTicks: false,
      axisLabel: { color: "#5e6b78" }, splitLine: { lineStyle: { color: "#1d2733" } },
    },
    dataZoom: [{ type: "inside" }],
    series: [{
      type: "line", showSymbol: false, smooth: true,
      lineStyle: { color: "#8b5cf6", width: 2 },
      areaStyle: { color: "rgba(139,92,246,0.12)" },
      data: pts,
      markPoint: marks.length ? { symbolSize: 42, label: { fontSize: 10, color: "#fff" }, data: marks } : undefined,
    }],
  };
});

function nearestPoint(ts: string, pts: number[][]) {
  const t = new Date(ts).getTime();
  if (!pts.length) return [t, 0];
  let best = pts[0];
  let bestDiff = Math.abs(pts[0][0] - t);
  for (const p of pts) {
    const d = Math.abs(p[0] - t);
    if (d < bestDiff) { bestDiff = d; best = p; }
  }
  return best;
}

// ---- 指标小多图 ----
function miniOption(s: any) {
  const data = (s.points || []).map((p: any) => [new Date(p.ts).getTime(), p.v]);
  const color = s.type === "counter" ? "#ef4444" : "#38bdf8";
  return {
    grid: { left: 40, right: 10, top: 10, bottom: 20 },
    tooltip: { trigger: "axis" },
    xAxis: { type: "time", axisLabel: { color: "#5e6b78", fontSize: 10 }, axisLine: { lineStyle: { color: "#243040" } } },
    yAxis: { type: "value", scale: true, axisLabel: { color: "#5e6b78", fontSize: 10 }, splitLine: { lineStyle: { color: "#1d2733" } } },
    series: [{
      type: "line", showSymbol: false, smooth: s.type !== "counter",
      step: s.type === "counter" ? "end" : false,
      lineStyle: { color, width: 1.5 }, data,
    }],
  };
}

// ---- 时间范围 ----
function rangeFromTo() {
  const to = new Date();
  const map: Record<string, number> = { "1h": 1, "6h": 6, "24h": 24, "7d": 24 * 7 };
  const from = new Date(to.getTime() - (map[rangePreset.value] || 6) * 3600 * 1000);
  return { from: from.toISOString(), to: to.toISOString() };
}

// ---- 加载 ----
async function loadDetail() {
  const res = await api.healthGPUDetail(uuid);
  snapshot.value = res.snapshot || null;
  dimensions.value = res.dimensions || [];
  abnormal.value = res.abnormal || [];
  faults.value = res.faults || [];
  meta.value = res.meta || null;
}

async function loadTrend() {
  const { from, to } = rangeFromTo();
  const res = await api.healthScoreTrend(uuid, { from, to, max_points: 300 });
  trendPoints.value = res.points || [];
  trendEvents.value = res.events || [];
}

async function loadSeries() {
  const { from, to } = rangeFromTo();
  const res = await api.healthGPUMetrics(uuid, {
    from, to, max_points: 500,
  });
  series.value = res.series || [];
}

async function reloadTimeSeries() {
  loading.value = true;
  try {
    await Promise.all([loadTrend(), loadSeries()]);
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "加载时序失败");
  } finally {
    loading.value = false;
  }
}

async function reloadAll() {
  loading.value = true;
  try {
    await Promise.all([loadDetail(), loadTrend(), loadSeries()]);
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "加载详情失败");
  } finally {
    loading.value = false;
  }
}

function goBack() { router.back(); }
function goAssistant() {
  router.push({ name: "fault-assistant", query: { uuid } });
}

onMounted(reloadAll);
</script>

<style scoped>
.gd { display: flex; flex-direction: column; gap: 16px; }
.gd-topbar {
  display: flex; align-items: center; justify-content: space-between;
  background: var(--bg-1); border: 1px solid var(--border);
  border-radius: 8px; padding: 12px 16px;
}
.gd-title { display: flex; align-items: center; gap: 10px; }
.gd-crumb { font-size: 14px; font-weight: 600; color: var(--text-0); }
.gd-ctrl { display: flex; align-items: center; gap: 10px; }
.gd-body { display: grid; grid-template-columns: 360px 1fr; gap: 16px; align-items: start; }
.gd-col-left, .gd-col-right { display: flex; flex-direction: column; gap: 16px; }
.gd-panel { overflow: hidden; }
.gd-sub { font-size: 11px; color: var(--text-2); font-weight: 400; margin-left: 8px; text-transform: none; }

.gd-radar-wrap { padding: 12px 16px; }
.gd-radar-empty { padding: 40px 0; text-align: center; }
.gd-score-big { font-family: var(--font-mono); font-size: 48px; font-weight: 600; }
.gd-empty-hint { color: var(--text-2); font-size: 13px; text-align: center; }
.gd-actions { display: flex; gap: 10px; margin-top: 12px; }
.gd-actions .n-button { flex: 1; }

.gd-abn-list { padding: 12px 16px; display: flex; flex-direction: column; gap: 10px; }
.gd-abn-card { border-radius: 6px; padding: 10px 12px; border-left: 3px solid; }
.gd-abn-card.sev-crit { background: rgba(239,68,68,0.10); border-color: #ef4444; }
.gd-abn-card.sev-warn { background: rgba(234,179,8,0.10); border-color: #eab308; }
.gd-abn-head { display: flex; align-items: center; justify-content: space-between; }
.gd-abn-name { font-size: 13px; font-weight: 600; color: var(--text-0); }
.gd-abn-val { font-size: 13px; color: var(--text-0); }
.gd-abn-sub { font-size: 11px; color: var(--text-1); margin-top: 4px; }

.gd-metric-grid {
  display: grid; grid-template-columns: repeat(3, 1fr);
  gap: 12px; padding: 12px 16px;
}
.gd-metric-cell {
  background: var(--bg-2); border: 1px solid var(--border);
  border-radius: 6px; padding: 8px 10px;
}
.gd-metric-name { font-size: 12px; color: var(--text-1); margin-bottom: 4px; }
.gd-unit { color: var(--text-2); font-size: 11px; margin-left: 4px; }
</style>
