<template>
  <div>
    <!-- 工具栏：关键字 + 集群 + 严重度 + 状态 + 时间范围 -->
    <div class="toolbar">
      <n-space>
        <n-input v-model:value="keyword" placeholder="搜索故障名/设备/卡号/指标"
          clearable style="width: 240px"
          @keydown.enter="reload" @clear="reload" />
        <n-select v-model:value="clusterId" :options="clusterOptions" placeholder="按集群筛选"
          clearable style="width: 170px" @update:value="reload" />
        <n-select v-model:value="severity" :options="severityOptions" placeholder="严重度"
          clearable style="width: 130px" @update:value="reload" />
        <n-select v-model:value="status" :options="statusOptions"
          style="width: 130px" @update:value="reload" />
        <n-button @click="reload">搜索</n-button>
      </n-space>
      <div class="stat" v-if="stats">
        进行中故障：<b style="color:#ef4444">{{ stats.total || 0 }}</b>
        <span class="sev fatal">致命 {{ stats.fatal || 0 }}</span>
        <span class="sev critical">严重 {{ stats.critical || 0 }}</span>
        <span class="sev warning">告警 {{ stats.warning || 0 }}</span>
      </div>
    </div>

    <div class="panel">
      <div class="panel-title">故障池</div>
      <n-data-table
        :columns="cols"
        :data="rows"
        :bordered="false"
        size="small"
        :max-height="560"
      />
      <div class="pager">
        <n-pagination
          v-model:page="page"
          :page-count="pageCount"
          :page-size="pageSize"
          @update:page="load"
        />
      </div>
    </div>

    <!-- 详情抽屉 -->
    <n-drawer v-model:show="detailShow" :width="460">
      <n-drawer-content :title="detailRow?.fault_name || '故障详情'" closable>
        <div v-if="detailRow" class="detail">
          <div class="kv"><span>故障名称</span><b>{{ detailRow.fault_name }}</b></div>
          <div class="kv"><span>严重度</span>
            <n-tag size="small" :type="sevType(detailRow.severity)">{{ sevName(detailRow.severity) }}</n-tag>
          </div>
          <div class="kv"><span>状态</span>
            <n-tag size="small" :type="detailRow.status === 'open' ? 'error' : 'success'">
              {{ detailRow.status === 'open' ? '进行中' : '已恢复' }}
            </n-tag>
          </div>
          <div class="kv"><span>涉及设备</span><b>{{ detailRow.node_host || '—' }}</b></div>
          <div class="kv"><span>故障集群</span><b>{{ detailRow.cluster_name || '—' }}</b></div>
          <div class="kv"><span>故障卡号</span><b class="mono">{{ detailRow.gpu_uuid }}</b></div>
          <div class="kv" v-if="detailRow.metric_key"><span>异常指标</span>
            <b>{{ detailRow.metric_display || detailRow.metric_key }}</b>
          </div>
          <div class="kv" v-if="detailRow.metric_key"><span>触发值 / 门限</span>
            <b>{{ fmt(detailRow.trigger_value) }} / {{ fmt(detailRow.threshold) }}</b>
          </div>
          <div class="kv"><span>开始时间</span><b>{{ fmtTime(detailRow.started_at) }}</b></div>
          <div class="kv"><span>持续时长</span><b>{{ duration(detailRow) }}</b></div>
          <div class="kv" v-if="detailExtra?.card_score !== undefined">
            <span>当时卡总分</span><b>{{ fmt(detailExtra.card_score) }}</b>
          </div>

          <n-space style="margin-top:18px">
            <n-button size="small" v-if="detailRow.status === 'open'" type="warning" @click="resolve(detailRow)">
              人工标记已恢复
            </n-button>
          </n-space>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, NButton, NTag } from "naive-ui";

const message = useMessage();

const rows = ref<any[]>([]);
const stats = ref<any>(null);

// 过滤条件
const keyword = ref("");
const clusterId = ref<number | null>(null);
const severity = ref<string | null>(null);
const status = ref<string>("open");

// 分页
const page = ref(1);
const pageSize = 20;
const total = ref(0);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

const clusters = ref<any[]>([]);
const clusterOptions = computed(() =>
  clusters.value.map((c: any) => ({ label: c.cluster_name, value: c.cluster_id }))
);
const severityOptions = [
  { label: "致命", value: "fatal" },
  { label: "严重", value: "critical" },
  { label: "告警", value: "warning" },
];
const statusOptions = [
  { label: "进行中", value: "open" },
  { label: "已恢复", value: "resolved" },
  { label: "全部", value: "all" },
];

function sevName(s: string) {
  return ({ fatal: "致命", critical: "严重", warning: "告警" } as any)[s] || s;
}
function sevType(s: string) {
  return ({ fatal: "error", critical: "error", warning: "warning" } as any)[s] || "default";
}
function fmt(v: any) {
  if (v === null || v === undefined) return "—";
  return typeof v === "number" ? Number(v.toFixed(2)) : v;
}
function fmtTime(t: string) {
  if (!t) return "—";
  return new Date(t).toLocaleString("zh-CN", { hour12: false });
}
function duration(r: any) {
  if (!r?.started_at) return "—";
  const end = r.status === "open" ? Date.now() : new Date(r.last_seen_at || r.resolved_at).getTime();
  const sec = Math.max(0, Math.floor((end - new Date(r.started_at).getTime()) / 1000));
  const h2 = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60);
  return h2 > 0 ? `${h2}小时${m}分` : `${m}分钟`;
}

const cols = [
  { title: "故障名称", key: "fault_name", width: 170,
    render: (r: any) => h("span", { style: `color:${r.severity === 'warning' ? '#eab308' : '#ef4444'};font-weight:600` }, r.fault_name) },
  { title: "涉及设备", key: "node_host", width: 140,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.node_host || "—") },
  { title: "故障集群", key: "cluster_name", width: 130 },
  { title: "故障卡号", key: "gpu_uuid",
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px;color:#9aa7b4" }, r.gpu_uuid) },
  { title: "异常指标", key: "metric_display", width: 200,
    render: (r: any) => {
      if (!r.metric_key) return h("span", { style: "color:var(--text-2)" }, "—");
      const txt = `${r.metric_display || r.metric_key}（${fmt(r.trigger_value)} / 门限 ${fmt(r.threshold)}）`;
      return h("span", { style: "font-size:12px" }, txt);
    } },
  { title: "故障开始时间", key: "started_at", width: 170,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, fmtTime(r.started_at)) },
  { title: "详情", key: "ops", width: 90,
    render: (r: any) => h(NButton, { size: "tiny", onClick: () => openDetail(r) }, () => "详情") },
];

const detailShow = ref(false);
const detailRow = ref<any>(null);
const detailExtra = ref<any>(null);
function openDetail(r: any) {
  detailRow.value = r;
  detailExtra.value = null;
  try { detailExtra.value = r.detail ? JSON.parse(r.detail) : null; } catch (e) { /* ignore */ }
  detailShow.value = true;
}

async function resolve(r: any) {
  try {
    await api.resolveFault(r.id);
    message.success("已标记为恢复");
    detailShow.value = false;
    await load();
    await loadStats();
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "操作失败");
  }
}

async function load() {
  const params: any = { limit: pageSize, offset: (page.value - 1) * pageSize, status: status.value };
  if (keyword.value.trim()) params.keyword = keyword.value.trim();
  if (clusterId.value) params.cluster_id = clusterId.value;
  if (severity.value) params.severity = severity.value;
  const res = await api.faultPool(params);
  rows.value = res.items || [];
  total.value = res.total || 0;
}
function reload() { page.value = 1; load(); }

async function loadStats() {
  try { stats.value = await api.faultPoolStats(); } catch (e) { /* ignore */ }
}

onMounted(async () => {
  try { clusters.value = await api.healthClusters(); } catch (e) { /* ignore */ }
  await load();
  await loadStats();
});
</script>

<style scoped>
.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin: 12px 0 16px; flex-wrap: wrap; }
.stat { font-size: 12px; color: var(--text-2); }
.stat .sev { margin-left: 12px; }
.stat .fatal { color: #ef4444; }
.stat .critical { color: #f97316; }
.stat .warning { color: #eab308; }
.panel-title { font-size: 13px; color: var(--text-1); padding: 12px 16px; letter-spacing: 0.05em; }
.pager { display: flex; justify-content: flex-end; padding: 14px 16px; }
.detail .kv { display: flex; justify-content: space-between; gap: 12px; padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
.detail .kv span { color: var(--text-2); }
</style>
