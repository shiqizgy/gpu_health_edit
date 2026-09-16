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
                <div class="gd-score-big" :style="{ color: scoreColor(score) }">{{ Number(score || 0).toFixed(0) }}</div>
                <div class="gd-empty-hint">该卡健康，暂无维度明细</div>
              </div>
              <div class="gd-actions">
                <n-button size="small" secondary @click="goAssistant">✦ AI 诊断</n-button>
                <n-button size="small" secondary @click="goAssistant">◈ 诊断报告</n-button>
              </div>
            </div>
          </div>

          <div class="panel gd-panel">
            <div class="panel-title">基本信息</div>
            <div class="gd-info">
              <div><label>集群</label><span>{{ meta?.cluster_name || '—' }}</span></div>
              <div><label>节点 IP</label><span class="mono">{{ meta?.node_ip || '—' }}</span></div>
              <div><label>机器 SN</label><span class="mono">{{ meta?.sn || '—' }}</span></div>
              <div><label>卡序号</label><span class="mono">{{ meta?.gpu_index ?? '—' }}</span></div>
              <div><label>型号</label><span>{{ meta?.model || '—' }}</span></div>
              <div><label>健康等级</label>
                <span :class="`level-badge lv-${snapshot?.level}`">{{ levelNames[snapshot?.level] || snapshot?.level || '—' }}</span></div>
              <div><label>一票否决</label>
                <span :style="{ color: snapshot?.veto ? '#ef4444' : 'var(--text-2)' }">{{ snapshot?.veto ? (snapshot?.veto_reason || '是') : '否' }}</span></div>
              <div><label>评分时间</label><span class="mono">{{ fmtDateTime(snapshot?.scored_at) }}</span></div>
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

        <div class="panel gd-panel">
          <div class="panel-title">当前故障 {{ faults.length }} 项</div>
          <div class="gd-abn-list">
            <div v-if="!faults.length" class="gd-empty-hint" style="padding:12px">暂无进行中的故障事件。</div>
            <div v-for="f in faults" :key="f.id" class="gd-abn-card"
                 :class="f.severity === 'warning' ? 'sev-warn' : 'sev-crit'">
              <div class="gd-abn-head">
                <span class="gd-abn-name">{{ f.fault_name }}</span>
                <span class="mono gd-abn-val">{{ fmtNum(f.trigger_value) }}</span>
              </div>
              <div class="gd-abn-sub mono">{{ f.metric_display || f.metric_key || '—' }} · 开始于 {{ fmtDateTime(f.started_at) }}</div>
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
              <div v-if="trendEvents.length" class="gd-events">
                <span class="gd-events-title">事件 {{ trendEvents.length }} 段：</span>
                <span v-for="(e, i) in trendEvents.slice(0, 8)" :key="i" class="gd-event-chip"
                      :class="e.fatal ? 'chip-fatal' : 'chip-warn'">
                  {{ e.label }} · {{ fmtClock(e.ts) }}–{{ fmtClock(e.end || e.ts) }} · {{ e.count }} 次
                </span>
                <span v-if="trendEvents.length > 8" class="gd-sub">等 {{ trendEvents.length }} 段</span>
              </div>
              <div v-else class="gd-empty-hint" style="padding:40px">暂无趋势数据</div>
            </div>
          </div>

          <!-- 指标明细小多图 -->
          <div class="panel gd-panel">
            <div class="panel-title" style="display:flex;align-items:center;gap:12px">
              <span>指标明细 <span class="gd-sub">时间轴与上图联动</span></span>
              <div style="margin-left:auto;display:flex;gap:8px;align-items:center">
                <n-radio-group v-model:value="metricFilter" size="small">
                  <n-radio-button value="all">全部 {{ series.length }}</n-radio-button>
                  <n-radio-button value="abnormal">异常 {{ abnormalKeys.size }}</n-radio-button>
                  <n-radio-button value="nodata">未采集 {{ noDataCount }}</n-radio-button>
                </n-radio-group>
                <n-select v-model:value="dimFilter" :options="dimOptions" size="small" clearable
                          placeholder="按维度筛选" style="width: 150px" />
              </div>
            </div>
            <div class="gd-metric-grid">
              <div v-for="s in viewSeries" :key="s.metric" class="gd-metric-cell"
                   :class="{ 'cell-abn': abnormalKeys.has(s.metric), 'cell-empty': s.status !== 'ok' }">
                <div class="gd-metric-head">
                  <div class="gd-metric-key mono" :title="s.official_no ? `${s.metric}\n${s.official_no}` : s.metric">{{ s.metric }}</div>
                  <span v-if="s.status === 'ok'" class="mono gd-latest">
                    {{ fmtNum(s.latest) }}<span v-if="s.unit" class="gd-unit">{{ s.unit }}</span>
                  </span>
                </div>
                <div class="gd-metric-name" :title="s.display_name">
                  {{ s.display_name || '—' }}
                  <span v-if="abnormalKeys.has(s.metric)" class="gd-abn-tag">异常</span>
                </div>
                <v-chart :option="miniOption(s)" autoresize style="height: 120px" />
              </div>
              <div v-if="!viewSeries.length" class="gd-empty-hint" style="grid-column:1/-1;padding:30px">
                {{ series.length ? '当前筛选条件下没有指标' : '暂无指标数据' }}
              </div>
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
  GridComponent, TooltipComponent, RadarComponent, MarkPointComponent, MarkLineComponent,
  MarkAreaComponent, DataZoomComponent, GraphicComponent, VisualMapComponent
} from "echarts/components";

use([CanvasRenderer, LineChart, RadarChart, GridComponent, TooltipComponent, RadarComponent,
  MarkPointComponent, MarkLineComponent, MarkAreaComponent, DataZoomComponent, GraphicComponent, VisualMapComponent]);

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
const trendBucketSec = ref(60);
const curRange = ref<{ from: number; to: number }>({ from: Date.now() - 6 * 3600e3, to: Date.now() });
const metricFilter = ref<"all" | "abnormal" | "nodata">("all");
const dimFilter = ref<string | null>(null);

const abnormalKeys = computed(() => new Set(abnormal.value.map((m: any) => m.metric_key)));
const noDataCount = computed(() => series.value.filter((s: any) => s.status !== "ok").length);
const dimOptions = computed(() =>
  [...new Set(series.value.map((s: any) => s.dimension).filter(Boolean))]
    .map((d: any) => ({ label: dimName(d), value: d })));

// 排序：异常指标在前，有数据的次之，未采集的放最后；同组内按维度、字段名排序
const viewSeries = computed(() => {
  const rank = (s: any) => (abnormalKeys.value.has(s.metric) ? 0 : s.status === "ok" ? 1 : 2);
  return series.value
    .filter((s: any) => metricFilter.value === "all"
      || (metricFilter.value === "abnormal" ? abnormalKeys.value.has(s.metric) : s.status !== "ok"))
    .filter((s: any) => !dimFilter.value || s.dimension === dimFilter.value)
    .slice()
    .sort((a: any, b: any) => rank(a) - rank(b)
      || String(a.dimension).localeCompare(String(b.dimension))
      || String(a.metric).localeCompare(String(b.metric)));
});

const fmtClock = (t: any) => new Date(t).toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit" });
const fmtDateTime = (t: any) => t ? new Date(t).toLocaleString("zh-CN", { hour12: false }) : "—";

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
  healthy: "健康", sub_healthy: "亚健康", warning: "警告", critical: "严重", failed: "故障", unknown: "未知",
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
  const pts = trendPoints.value.map((p: any) => [new Date(p.ts).getTime(), Number(Number(p.score).toFixed(1))]);
  const segs = trendEvents.value.map((e: any) => ({
    ...e, t0: new Date(e.ts).getTime(), t1: new Date(e.end || e.ts).getTime(),
  }));
  const bucketMs = trendBucketSec.value * 1000;

  const areas = segs.map((e: any) => [
    { xAxis: e.t0, name: e.count > 1 ? `${e.label} ×${e.count}` : e.label,
      itemStyle: { color: e.fatal ? "rgba(239,68,68,0.14)" : "rgba(249,115,22,0.12)" } },
    { xAxis: Math.max(e.t1, e.t0 + bucketMs) },
  ]);
  const pins = segs.length <= 30
    ? segs.map((e: any) => ({
        coord: nearestPoint(e.ts, pts), value: String(e.code),
        itemStyle: { color: e.fatal ? "#ef4444" : "#f97316" },
      }))
    : [];

  return {
    grid: { left: 40, right: 48, top: 24, bottom: 52 },
    tooltip: {
      trigger: "axis",
      formatter: (ps: any[]) => {
        if (!ps?.length) return "";
        const [t, v] = ps[0].value;
        const hit = segs.filter((e: any) => t >= e.t0 - bucketMs && t <= e.t1 + bucketMs);
        let html = `${fmtDateTime(t)}<br/>健康分：<b>${v}</b>`;
        if (hit.length) {
          html += "<br/>" + hit.map((e: any) =>
            `<span style="color:${e.fatal ? "#ef4444" : "#f97316"}">●</span> ${e.label}` +
            `（${fmtClock(e.t0)} ~ ${fmtClock(e.t1)}，${e.count} 次${e.fatal ? "，致命" : ""}）`).join("<br/>");
        }
        return html;
      },
    },
    visualMap: {
      show: false, dimension: 1, seriesIndex: 0,
      pieces: [
        { lt: 30, color: "#ef4444" }, { gte: 30, lt: 60, color: "#f97316" },
        { gte: 60, lt: 75, color: "#eab308" }, { gte: 75, lt: 90, color: "#84cc16" },
        { gte: 90, color: "#22c55e" },
      ],
    },
    xAxis: { type: "time", axisLabel: { color: "#5e6b78" }, axisLine: { lineStyle: { color: "#243040" } } },
    yAxis: { type: "value", min: 0, max: 100, axisLabel: { color: "#5e6b78" }, splitLine: { lineStyle: { color: "#1d2733" } } },
    dataZoom: [
      { type: "inside" },
      { type: "slider", height: 14, bottom: 8, borderColor: "#243040",
        textStyle: { color: "#5e6b78" }, fillerColor: "rgba(56,189,248,0.15)" },
    ],
    series: [{
      type: "line", showSymbol: false, step: "end",   // 分数是离散档位，用阶梯线比平滑线更真实
      lineStyle: { width: 2 },
      data: pts,
      markArea: areas.length ? {
        label: { show: segs.length <= 8, color: "#fca5a5", fontSize: 10, position: "insideTop" },
        data: areas,
      } : undefined,
      markPoint: pins.length ? {
        symbol: "pin", symbolSize: 26, label: { fontSize: 9, color: "#fff" }, data: pins,
      } : undefined,
      markLine: {
        silent: true, symbol: "none",
        lineStyle: { type: "dashed", color: "#334155" },
        label: { color: "#5e6b78", fontSize: 9, formatter: "{b}", position: "end" },
        data: [{ yAxis: 90, name: "健康" }, { yAxis: 60, name: "警告" }, { yAxis: 30, name: "故障" }],
      },
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
  const empty = data.length === 0;
  const isAbn = abnormalKeys.value.has(s.metric);
  const color = isAbn ? "#ef4444" : (s.type === "counter" ? "#f97316" : "#38bdf8");
  const xAxis = {
    type: "time", min: curRange.value.from, max: curRange.value.to,
    axisLabel: { color: "#5e6b78", fontSize: 10 }, axisLine: { lineStyle: { color: "#243040" } },
  };
  const grid = { left: 40, right: 10, top: 10, bottom: 20 };

  if (empty) {
    const reason = s.status === "error"
      ? "查询失败，请点刷新重试"
      : (s.is_alive === false ? "CK 近期无该指标上报" : "所选时间段内无数据");
    return {
      grid, xAxis,
      yAxis: { type: "value", min: 0, max: 1, axisLabel: { show: false }, splitLine: { lineStyle: { color: "#1d2733" } } },
      series: [{ type: "line", data: [] }],
      graphic: [{
        type: "group", left: "center", top: "middle", silent: true,
        children: [
          { type: "text", style: { text: "指标未采集", fill: "#94a3b8", fontSize: 13, fontWeight: 600, align: "center" } },
          { type: "text", top: 20, style: { text: reason, fill: "#5e6b78", fontSize: 10, align: "center" } },
        ],
      }],
    };
  }

  const lines: any[] = [];
  const addLine = (v: any, name: string, c: string, type: string) => {
    if (v !== null && v !== undefined) {
      lines.push({ yAxis: v, name, lineStyle: { color: c, type },
        label: { formatter: name, color: c, fontSize: 9, position: "insideEndTop" } });
    }
  };
  addLine(s.alert_upper, "告警上限", "#ef4444", "dashed");
  if (s.upper_bound !== s.alert_upper) addLine(s.upper_bound, "正常上界", "#eab308", "dotted");
  addLine(s.alert_lower, "告警下限", "#ef4444", "dashed");
  if (s.lower_bound !== s.alert_lower) addLine(s.lower_bound, "正常下界", "#eab308", "dotted");

  return {
    grid, xAxis,
    tooltip: { trigger: "axis", valueFormatter: (v: any) => `${fmtNum(v)}${s.unit ? " " + s.unit : ""}` },
    yAxis: { type: "value", scale: true, axisLabel: { color: "#5e6b78", fontSize: 10 }, splitLine: { lineStyle: { color: "#1d2733" } } },
    series: [{
      type: "line", showSymbol: false, smooth: s.type !== "counter",
      step: s.type === "counter" ? "end" : false,
      lineStyle: { color, width: 1.5 },
      areaStyle: isAbn ? { color: "rgba(239,68,68,0.08)" } : undefined,
      data,
      markLine: lines.length ? { silent: true, symbol: "none", data: lines } : undefined,
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
  trendBucketSec.value = res.bucket_sec || 60;
}

async function loadSeries() {
  const { from, to } = rangeFromTo();
  curRange.value = { from: new Date(from).getTime(), to: new Date(to).getTime() };
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
.gd-metric-cell.cell-abn { border-color: rgba(239,68,68,0.55); }
.gd-metric-cell.cell-empty { opacity: .8; }
.gd-metric-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.gd-metric-key { font-size: 11px; color: var(--accent); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.gd-metric-name { font-size: 12px; color: var(--text-1); margin: 2px 0 4px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.gd-latest { font-size: 12px; color: var(--text-0); flex-shrink: 0; }
.gd-abn-tag { font-size: 10px; color: #ef4444; background: rgba(239,68,68,.12);
  padding: 0 5px; border-radius: 3px; margin-left: 4px; }
.gd-events { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-top: 8px; }
.gd-events-title { font-size: 12px; color: var(--text-2); }
.gd-event-chip { font-size: 11px; padding: 2px 8px; border-radius: 10px; font-family: var(--font-mono); }
.chip-fatal { color: #fca5a5; background: rgba(239,68,68,.12); }
.chip-warn  { color: #fdba74; background: rgba(249,115,22,.12); }
.gd-info { padding: 12px 16px; display: grid; grid-template-columns: 1fr; gap: 8px; font-size: 12px; }
.gd-info > div { display: flex; justify-content: space-between; gap: 12px; }
.gd-info label { color: var(--text-2); }
.gd-info span { color: var(--text-0); text-align: right; word-break: break-all; }

.gd-unit { color: var(--text-2); font-size: 11px; margin-left: 4px; }
</style>
