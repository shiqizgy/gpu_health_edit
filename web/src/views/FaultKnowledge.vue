<template>
  <div class="kg-page">
    <!-- ── 顶部工具栏 ────────────────────────────────────────────── -->
    <div class="kg-toolbar">
      <n-space align="center" :size="10" :wrap="false">
        <n-input
          v-model:value="filters.keyword"
          placeholder="搜索节点名称或摘要"
          clearable
          style="width: 220px"
          @keydown.enter="reloadGraph"
          @clear="reloadGraph"
        />
        <n-select
          v-model:value="filters.nodeType"
          :options="nodeTypeOptions"
          placeholder="全部类型"
          clearable
          style="width: 150px"
          @update:value="reloadGraph"
        />
        <n-button @click="reloadGraph">查询</n-button>
        <n-divider vertical />
        <n-button type="primary" @click="openCreateNode()">新建节点</n-button>
        <n-button :disabled="!selectedNode" @click="openCreateEdge()">
          从选中节点连线
        </n-button>
        <n-divider vertical />
        <n-button quaternary @click="importFromKnowledge">从故障知识条目导入</n-button>
      </n-space>

      <n-space align="center" :size="8">
        <span class="kg-stat">{{ graph.nodes.length }} 个节点 / {{ graph.edges.length }} 条关系</span>
        <n-tag v-if="graph.truncated" size="small" type="warning" :bordered="false">
          结果已截断
        </n-tag>
        <n-button size="small" quaternary @click="resetView">重置视图</n-button>
      </n-space>
    </div>

    <!-- ── 主体：图 + 详情面板 ──────────────────────────────────── -->
    <div class="kg-body">
      <div class="kg-canvas">
        <n-spin :show="loading">
          <div class="kg-legend">
            <span v-for="t in meta.node_types" :key="t.type" class="lg-item">
              <i class="lg-dot" :style="{ background: t.color }"></i>{{ t.label }}
              <em>{{ meta.node_counts?.[t.type] || 0 }}</em>
            </span>
          </div>

          <v-chart
            v-if="graph.nodes.length"
            ref="chartRef"
            class="kg-chart"
            :option="chartOption"
            autoresize
            @click="onChartClick"
            @dblclick="onChartDblClick"
          />
          <div v-else-if="!loading" class="kg-empty">
            <p>图谱还没有内容。</p>
            <p class="kg-empty-hint">
              先点「从故障知识条目导入」把已有的故障条目转成节点，再手工补充概念和影响范围。
            </p>
            <n-space justify="center">
              <n-button type="primary" @click="openCreateNode()">新建节点</n-button>
              <n-button @click="importFromKnowledge">从故障知识条目导入</n-button>
            </n-space>
          </div>
        </n-spin>

        <div class="kg-tip">单击选中 · 双击展开该节点的关联 · 滚轮缩放 · 拖拽移动</div>
      </div>

      <!-- ── 右侧详情 ─────────────────────────────────────────── -->
      <div class="kg-side">
        <div v-if="!detail" class="kg-side-empty">
          在左侧选中一个节点，这里会显示它的属性和全部关系。
        </div>

        <template v-else>
          <div class="sd-head">
            <n-tag size="small" :bordered="false" :color="typeTagColor(detail.node.node_type)">
              {{ typeLabel(detail.node.node_type) }}
            </n-tag>
            <span class="sd-name">{{ detail.node.name }}</span>
          </div>

          <div class="sd-ops">
            <n-button size="tiny" @click="openEditNode()">编辑</n-button>
            <n-button size="tiny" @click="expandSelected">展开关联</n-button>
            <n-button size="tiny" @click="openCreateEdge()">连线</n-button>
            <n-button size="tiny" type="error" ghost @click="confirmDeleteNode">删除</n-button>
          </div>

          <n-scrollbar style="max-height: calc(100vh - 300px)">
            <div class="sd-block">
              <div class="sd-label">唯一键</div>
              <div class="sd-val mono">{{ detail.node.node_key }}</div>
            </div>
            <div class="sd-block" v-if="detail.node.severity">
              <div class="sd-label">严重等级</div>
              <div class="sd-val">{{ sevName(detail.node.severity) }}</div>
            </div>
            <div class="sd-block" v-if="detail.node.summary">
              <div class="sd-label">摘要</div>
              <div class="sd-val">{{ detail.node.summary }}</div>
            </div>
            <div class="sd-block" v-if="detail.node.description">
              <div class="sd-label">详细说明</div>
              <div class="sd-val pre">{{ detail.node.description }}</div>
            </div>

            <div class="sd-block" v-if="detail.metric_ref">
              <div class="sd-label">指标引用</div>
              <div class="sd-val">
                <span class="mono">{{ detail.metric_ref.metric_name || "未设置" }}</span>
                <n-tag
                  size="tiny"
                  :bordered="false"
                  :type="detail.metric_ref.exists ? 'success' : 'warning'"
                  style="margin-left: 8px"
                >
                  {{ detail.metric_ref.exists ? "已匹配指标定义" : "指标定义中未找到" }}
                </n-tag>
                <div v-if="detail.metric_ref.exists" class="sd-sub">
                  {{ detail.metric_ref.card_type }} · {{ detail.metric_ref.dimension }}
                </div>
              </div>
            </div>

            <div class="sd-block" v-if="hasProps(detail.node.props)">
              <div class="sd-label">扩展属性</div>
              <pre class="sd-json">{{ prettyJSON(detail.node.props) }}</pre>
            </div>

            <div class="sd-block">
              <div class="sd-label">关系（{{ detail.edges.length }}）</div>
              <div v-if="!detail.edges.length" class="sd-sub">还没有关系，点上方「连线」添加。</div>
              <div v-for="ew in detail.edges" :key="ew.edge.id" class="rel-row">
                <span class="rel-dir">{{ ew.direction === "out" ? "→" : "←" }}</span>
                <span class="rel-type">{{ relLabel(ew.edge.rel_type) }}</span>
                <a class="rel-peer" @click="selectNode(ew.peer_id)">{{ ew.peer_name }}</a>
                <span class="rel-peer-type">{{ typeLabel(ew.peer_type) }}</span>
                <span class="rel-ops">
                  <n-button size="tiny" quaternary @click="openEditEdge(ew.edge)">改</n-button>
                  <n-button size="tiny" quaternary type="error" @click="confirmDeleteEdge(ew)">删</n-button>
                </span>
              </div>
            </div>
          </n-scrollbar>
        </template>
      </div>
    </div>

    <!-- ── 节点编辑弹窗 ─────────────────────────────────────────── -->
    <n-modal
      v-model:show="nodeModal.show"
      preset="card"
      :title="nodeModal.editing ? '编辑节点' : '新建节点'"
      style="width: 620px"
    >
      <n-form :model="nodeForm" label-placement="left" label-width="90">
        <n-form-item label="节点类型" required>
          <n-select
            v-model:value="nodeForm.node_type"
            :options="nodeTypeOptions"
            :disabled="nodeModal.editing"
          />
        </n-form-item>
        <n-form-item v-if="nodeForm.node_type === 'metric'" label="关联指标">
          <n-select
            v-model:value="nodeForm.name"
            filterable
            remote
            clearable
            placeholder="输入关键字搜索指标定义"
            :options="metricOptions"
            :loading="metricLoading"
            @search="searchMetrics"
          />
        </n-form-item>
        <n-form-item v-else label="名称" required>
          <n-input v-model:value="nodeForm.name" placeholder="例如：HBM 双比特错误" />
        </n-form-item>
        <n-form-item v-if="nodeForm.node_type === 'fault'" label="严重等级">
          <n-select v-model:value="nodeForm.severity" :options="sevOptions" clearable />
        </n-form-item>
        <n-form-item label="摘要">
          <n-input v-model:value="nodeForm.summary" placeholder="一句话说明，鼠标悬停时显示" />
        </n-form-item>
        <n-form-item label="详细说明">
          <n-input
            v-model:value="nodeForm.description"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 10 }"
          />
        </n-form-item>
        <n-form-item label="扩展属性">
          <n-input
            v-model:value="nodeForm.props"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            placeholder='JSON 对象，例如 {"xid_code":"48"}'
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="nodeModal.show = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveNode">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- ── 关系编辑弹窗 ─────────────────────────────────────────── -->
    <n-modal
      v-model:show="edgeModal.show"
      preset="card"
      :title="edgeModal.editing ? '编辑关系' : '新建关系'"
      style="width: 560px"
    >
      <n-form :model="edgeForm" label-placement="left" label-width="90">
        <n-form-item label="起点">
          <n-select
            v-model:value="edgeForm.from_id"
            filterable
            :options="nodeOptions"
            :disabled="edgeModal.editing"
            placeholder="选择起点节点"
          />
        </n-form-item>
        <n-form-item label="关系" required>
          <n-select
            v-model:value="edgeForm.rel_type"
            :options="relOptionsForPair"
            :disabled="edgeModal.editing"
            placeholder="选择关系类型"
          />
          <template #feedback>
            <span class="form-hint">{{ relHint }}</span>
          </template>
        </n-form-item>
        <n-form-item label="终点">
          <n-select
            v-model:value="edgeForm.to_id"
            filterable
            :options="nodeOptions"
            :disabled="edgeModal.editing"
            placeholder="选择终点节点"
          />
        </n-form-item>
        <n-form-item label="说明">
          <n-input v-model:value="edgeForm.label" placeholder="留空则显示关系类型的默认名称" />
        </n-form-item>
        <n-form-item label="关联强度">
          <n-slider v-model:value="edgeForm.weight" :min="0.1" :max="1" :step="0.1" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="edgeModal.show = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveEdge">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, useDialog } from "naive-ui";
import VChart from "vue-echarts";
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { GraphChart } from "echarts/charts";
import { TooltipComponent, LegendComponent } from "echarts/components";

use([CanvasRenderer, GraphChart, TooltipComponent, LegendComponent]);

const message = useMessage();
const dialog = useDialog();

/* ── 状态 ─────────────────────────────────────────────────────── */

const loading = ref(false);
const saving = ref(false);
const chartRef = ref<any>(null);

const meta = ref<any>({ node_types: [], rel_types: [], node_counts: {}, edge_count: 0 });
const graph = ref<{ nodes: any[]; edges: any[]; truncated: boolean }>({
  nodes: [], edges: [], truncated: false
});
const detail = ref<any>(null);
const selectedNode = computed(() => detail.value?.node || null);

const filters = reactive({ keyword: "", nodeType: null as string | null });

const sevOptions = [
  { label: "警告", value: "warning" },
  { label: "严重", value: "critical" },
  { label: "致命", value: "fatal" }
];

/* ── 元数据派生 ────────────────────────────────────────────────── */

const nodeTypeOptions = computed(() =>
  meta.value.node_types.map((t: any) => ({ label: t.label, value: t.type }))
);

const nodeOptions = computed(() =>
  graph.value.nodes.map((n: any) => ({
    label: `[${typeLabel(n.node_type)}] ${n.name}`,
    value: n.id
  }))
);

function typeLabel(t: string) {
  return meta.value.node_types.find((x: any) => x.type === t)?.label || t;
}
function typeColor(t: string) {
  return meta.value.node_types.find((x: any) => x.type === t)?.color || "#9aa7b4";
}
function typeTagColor(t: string) {
  const c = typeColor(t);
  return { color: c + "22", textColor: c, borderColor: c + "55" };
}
function relLabel(t: string) {
  return meta.value.rel_types.find((x: any) => x.type === t)?.label || t;
}
function sevName(s: string) {
  return ({ warning: "警告", critical: "严重", fatal: "致命" } as any)[s] || s;
}

/* 只列出「当前起点/终点类型组合」允许的关系，从源头避免提交后被后端打回 */
const relOptionsForPair = computed(() => {
  const from = graph.value.nodes.find((n: any) => n.id === edgeForm.from_id);
  const to = graph.value.nodes.find((n: any) => n.id === edgeForm.to_id);
  return meta.value.rel_types
    .filter((r: any) => {
      if (!r.pairs || !r.pairs.length) return true;
      if (!from || !to) return true;
      return r.pairs.some((p: string[]) => p[0] === from.node_type && p[1] === to.node_type);
    })
    .map((r: any) => ({ label: r.label, value: r.type }));
});

const relHint = computed(() => {
  const r = meta.value.rel_types.find((x: any) => x.type === edgeForm.rel_type);
  return r?.desc || "先选好起点和终点，可选的关系类型会自动过滤";
});

/* ── ECharts 配置 ──────────────────────────────────────────────── */

const chartOption = computed(() => {
  const idSet = new Set(graph.value.nodes.map((n: any) => String(n.id)));
  const selectedId = selectedNode.value ? String(selectedNode.value.id) : "";

  const nodes = graph.value.nodes.map((n: any) => {
    const color = typeColor(n.node_type);
    const isSel = String(n.id) === selectedId;
    return {
      id: String(n.id),
      name: n.name,
      category: n.node_type,
      symbolSize: n.node_type === "fault" ? (n.severity === "fatal" ? 46 : 38) : 30,
      itemStyle: {
        color,
        borderColor: isSel ? "#e6edf3" : color,
        borderWidth: isSel ? 3 : 0
      },
      label: { show: true, color: "#e6edf3", fontSize: 11 },
      _raw: n
    };
  });

  /* 只画两端都在画布上的边。后端已经保证了这一点，
     这里再挡一次，防止任何情况下 ECharts 因为找不到端点而抛异常。 */
  const links = graph.value.edges
    .filter((e: any) => idSet.has(String(e.from_id)) && idSet.has(String(e.to_id)))
    .map((e: any) => ({
      id: String(e.id),
      source: String(e.from_id),
      target: String(e.to_id),
      value: e.rel_type,
      label: { show: true, formatter: e.label || relLabel(e.rel_type), fontSize: 10, color: "#9aa7b4" },
      lineStyle: { width: 1 + (e.weight || 1) * 1.5, opacity: 0.65, curveness: 0.12 },
      _raw: e
    }));

  return {
    backgroundColor: "transparent",
    tooltip: {
      confine: true,
      formatter: (p: any) => {
        if (p.dataType === "node") {
          const n = p.data._raw;
          return `<b>${escapeHTML(n.name)}</b><br/>${typeLabel(n.node_type)}` +
            (n.summary ? `<br/><span style="color:#9aa7b4">${escapeHTML(n.summary)}</span>` : "");
        }
        const e = p.data._raw;
        return `${relLabel(e.rel_type)}${e.label ? "：" + escapeHTML(e.label) : ""}`;
      }
    },
    legend: [{
      data: meta.value.node_types.map((t: any) => t.type),
      show: false
    }],
    series: [{
      type: "graph",
      layout: "force",
      roam: true,
      draggable: true,
      zoom: 1,
      categories: meta.value.node_types.map((t: any) => ({
        name: t.type, itemStyle: { color: t.color }
      })),
      edgeSymbol: ["none", "arrow"],
      edgeSymbolSize: 8,
      emphasis: { focus: "adjacency", label: { fontSize: 13 } },
      force: { repulsion: 320, edgeLength: [90, 170], gravity: 0.06, friction: 0.25 },
      data: nodes,
      links
    }]
  };
});

function escapeHTML(s: string) {
  return String(s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" } as any)[c]);
}

/* ── 数据加载 ──────────────────────────────────────────────────── */

async function loadMeta() {
  try {
    meta.value = await api.kgMeta();
  } catch {
    message.error("加载图谱元数据失败，请刷新页面重试");
  }
}

async function reloadGraph() {
  loading.value = true;
  try {
    graph.value = await api.kgGraph({
      keyword: filters.keyword || undefined,
      node_type: filters.nodeType || undefined,
      limit: 300
    });
    /* 当前选中的节点如果不在新结果里，清空详情，避免面板和画布对不上 */
    if (detail.value && !graph.value.nodes.some((n: any) => n.id === detail.value.node.id)) {
      detail.value = null;
    }
    await loadMeta();
  } catch {
    message.error("加载图谱失败，请稍后重试");
  } finally {
    loading.value = false;
  }
}

async function selectNode(id: number) {
  try {
    detail.value = await api.kgNodeDetail(id);
  } catch {
    message.error("该节点已不存在，正在刷新图谱");
    detail.value = null;
    await reloadGraph();
  }
}

/* 双击展开：把邻域结果并入当前画布，而不是整个替换，
   这样用户可以一层层往外探索而不丢失已有的上下文 */
async function expandNode(id: number) {
  loading.value = true;
  try {
    const g = await api.kgNeighbors(id, { depth: 1, limit: 200 });
    const nodeMap = new Map<number, any>();
    [...graph.value.nodes, ...g.nodes].forEach((n: any) => nodeMap.set(n.id, n));
    const edgeMap = new Map<number, any>();
    [...graph.value.edges, ...g.edges].forEach((e: any) => edgeMap.set(e.id, e));

    graph.value = {
      nodes: [...nodeMap.values()],
      edges: [...edgeMap.values()].filter(
        (e: any) => nodeMap.has(e.from_id) && nodeMap.has(e.to_id)
      ),
      truncated: g.truncated
    };
    if (g.truncated) message.warning("关联节点较多，已只展开前 200 个");
  } catch {
    message.error("展开关联失败，请稍后重试");
  } finally {
    loading.value = false;
  }
}

function expandSelected() {
  if (selectedNode.value) expandNode(selectedNode.value.id);
}

function onChartClick(p: any) {
  if (p.dataType === "node") selectNode(Number(p.data.id));
  else if (p.dataType === "edge") openEditEdge(p.data._raw);
}
function onChartDblClick(p: any) {
  if (p.dataType === "node") expandNode(Number(p.data.id));
}
function resetView() {
  filters.keyword = "";
  filters.nodeType = null;
  detail.value = null;
  reloadGraph();
}

/* ── 节点增删改 ────────────────────────────────────────────────── */

const nodeModal = reactive({ show: false, editing: false, id: 0, version: 0 });
const nodeForm = reactive<any>(emptyNodeForm());

function emptyNodeForm() {
  return { node_type: "fault", name: "", summary: "", description: "", severity: "", props: "{}" };
}

function openCreateNode() {
  Object.assign(nodeForm, emptyNodeForm());
  nodeModal.editing = false;
  nodeModal.id = 0;
  nodeModal.version = 0;
  nodeModal.show = true;
}

function openEditNode() {
  const n = selectedNode.value;
  if (!n) return;
  Object.assign(nodeForm, {
    node_type: n.node_type, name: n.name, summary: n.summary || "",
    description: n.description || "", severity: n.severity || "", props: n.props || "{}"
  });
  nodeModal.editing = true;
  nodeModal.id = n.id;
  nodeModal.version = n.version;
  nodeModal.show = true;
}

async function saveNode() {
  if (!nodeForm.name?.trim()) {
    message.warning("请填写节点名称");
    return;
  }
  if (!isValidJSONObject(nodeForm.props)) {
    message.warning("扩展属性必须是合法的 JSON 对象");
    return;
  }
  /* 指标节点自动把指标名写进 props，供后端做引用校验 */
  const payload: any = { ...nodeForm };
  if (payload.node_type === "metric") {
    const p = safeParse(payload.props);
    p.metric_name = payload.name;
    payload.props = JSON.stringify(p);
  }
  if (payload.node_type !== "fault") payload.severity = "";

  saving.value = true;
  try {
    if (nodeModal.editing) {
      payload.version = nodeModal.version;
      await api.kgUpdateNode(nodeModal.id, payload);
      message.success("节点已保存");
      await selectNode(nodeModal.id);
    } else {
      const created = await api.kgCreateNode(payload);
      message.success("节点已创建");
      await reloadGraph();
      await selectNode(created.id);
    }
    nodeModal.show = false;
    await reloadGraph();
  } catch (e: any) {
    handleApiError(e, "保存节点失败");
  } finally {
    saving.value = false;
  }
}

function confirmDeleteNode() {
  const n = selectedNode.value;
  if (!n) return;
  const edgeCount = detail.value?.edges?.length || 0;
  dialog.warning({
    title: "删除节点",
    content: edgeCount
      ? `删除「${n.name}」会同时删除它的 ${edgeCount} 条关系，此操作不可撤销。`
      : `删除「${n.name}」，此操作不可撤销。`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.kgDeleteNode(n.id);
        message.success("节点已删除");
        detail.value = null;
        await reloadGraph();
      } catch (e: any) {
        handleApiError(e, "删除节点失败");
      }
    }
  });
}

/* ── 关系增删改 ────────────────────────────────────────────────── */

const edgeModal = reactive({ show: false, editing: false, id: 0, version: 0 });
const edgeForm = reactive<any>(emptyEdgeForm());

function emptyEdgeForm() {
  return { from_id: null, to_id: null, rel_type: null, label: "", weight: 1 };
}

function openCreateEdge() {
  Object.assign(edgeForm, emptyEdgeForm());
  if (selectedNode.value) edgeForm.from_id = selectedNode.value.id;
  edgeModal.editing = false;
  edgeModal.id = 0;
  edgeModal.version = 0;
  edgeModal.show = true;
}

function openEditEdge(e: any) {
  Object.assign(edgeForm, {
    from_id: e.from_id, to_id: e.to_id, rel_type: e.rel_type,
    label: e.label || "", weight: e.weight || 1
  });
  edgeModal.editing = true;
  edgeModal.id = e.id;
  edgeModal.version = e.version;
  edgeModal.show = true;
}

async function saveEdge() {
  if (!edgeModal.editing) {
    if (!edgeForm.from_id || !edgeForm.to_id) {
      message.warning("请选择起点和终点");
      return;
    }
    if (edgeForm.from_id === edgeForm.to_id) {
      message.warning("起点和终点不能是同一个节点");
      return;
    }
    if (!edgeForm.rel_type) {
      message.warning("请选择关系类型");
      return;
    }
  }
  saving.value = true;
  try {
    if (edgeModal.editing) {
      await api.kgUpdateEdge(edgeModal.id, {
        label: edgeForm.label, weight: edgeForm.weight, version: edgeModal.version
      });
      message.success("关系已保存");
    } else {
      await api.kgCreateEdge({
        from_id: edgeForm.from_id, to_id: edgeForm.to_id,
        rel_type: edgeForm.rel_type, label: edgeForm.label, weight: edgeForm.weight
      });
      message.success("关系已创建");
    }
    edgeModal.show = false;
    await reloadGraph();
    if (selectedNode.value) await selectNode(selectedNode.value.id);
  } catch (e: any) {
    handleApiError(e, "保存关系失败");
  } finally {
    saving.value = false;
  }
}

function confirmDeleteEdge(ew: any) {
  dialog.warning({
    title: "删除关系",
    content: `删除「${selectedNode.value?.name}」与「${ew.peer_name}」之间的关系？`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.kgDeleteEdge(ew.edge.id);
        message.success("关系已删除");
        await reloadGraph();
        if (selectedNode.value) await selectNode(selectedNode.value.id);
      } catch (e: any) {
        handleApiError(e, "删除关系失败");
      }
    }
  });
}

/* ── 导入与指标搜索 ────────────────────────────────────────────── */

function importFromKnowledge() {
  dialog.info({
    title: "从故障知识条目导入",
    content: "把「故障知识」里的条目转成故障节点和指标节点。已存在的节点会被跳过，不会覆盖你手工修改过的内容。",
    positiveText: "开始导入",
    negativeText: "取消",
    onPositiveClick: async () => {
      loading.value = true;
      try {
        const r = await api.kgImportFaultKnowledge();
        message.success(
          `导入完成：新建故障 ${r.fault_created} 个、指标 ${r.metric_created} 个、关系 ${r.edge_created} 条，跳过 ${r.skipped} 个已存在条目`
        );
        if (r.warnings?.length) {
          message.warning(`有 ${r.warnings.length} 条数据需要人工确认：${r.warnings[0]}`, { duration: 8000 });
        }
        await reloadGraph();
      } catch (e: any) {
        handleApiError(e, "导入失败");
      } finally {
        loading.value = false;
      }
    }
  });
}

const metricOptions = ref<any[]>([]);
const metricLoading = ref(false);
let metricTimer: any = null;

function searchMetrics(kw: string) {
  clearTimeout(metricTimer);
  metricTimer = setTimeout(async () => {
    metricLoading.value = true;
    try {
      const list = await api.kgMetricOptions({ keyword: kw, limit: 50 });
      metricOptions.value = list.map((m: any) => ({
        label: `${m.metric_name}（${m.card_type}）`,
        value: m.metric_name
      }));
    } catch {
      metricOptions.value = [];
    } finally {
      metricLoading.value = false;
    }
  }, 250);
}

/* ── 工具 ─────────────────────────────────────────────────────── */

function safeParse(s: string) {
  try { return JSON.parse(s || "{}"); } catch { return {}; }
}
function isValidJSONObject(s: string) {
  if (!s || !s.trim()) return true;
  try {
    const v = JSON.parse(s);
    return v !== null && typeof v === "object" && !Array.isArray(v);
  } catch { return false; }
}
function hasProps(s: string) {
  const v = safeParse(s);
  return v && Object.keys(v).length > 0;
}
function prettyJSON(s: string) {
  try { return JSON.stringify(JSON.parse(s || "{}"), null, 2); } catch { return s; }
}

/* 后端已经把校验失败、冲突、不存在分别映射成 400/409/404，
   这里把 msg 直接展示给用户——这些文案是给人看的，不是内部错误 */
function handleApiError(e: any, fallback: string) {
  const status = e?.response?.status;
  const msg = e?.response?.data?.msg;
  if (status === 409) {
    message.error(msg || "数据已被他人修改，请刷新后重试");
    reloadGraph();
    return;
  }
  if (status === 404) {
    message.error(msg || "目标已不存在，正在刷新");
    reloadGraph();
    return;
  }
  message.error(msg || fallback);
}

onMounted(async () => {
  await loadMeta();
  await reloadGraph();
});
</script>

<style scoped>
.kg-page { display: flex; flex-direction: column; height: calc(100vh - 96px); }

.kg-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 16px; margin-bottom: 12px; flex-wrap: wrap;
}
.kg-stat { color: var(--text-1); font-size: 12px; font-family: var(--font-mono); }

.kg-body { display: flex; gap: 12px; flex: 1; min-height: 0; }

.kg-canvas {
  position: relative; flex: 1; min-width: 0;
  background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px;
}
.kg-chart { width: 100%; height: 100%; min-height: 420px; }

.kg-legend {
  position: absolute; top: 12px; left: 14px; z-index: 2;
  display: flex; gap: 14px; flex-wrap: wrap;
}
.lg-item { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; color: var(--text-1); }
.lg-dot { width: 9px; height: 9px; border-radius: 50%; display: inline-block; }
.lg-item em { font-style: normal; color: var(--text-2); font-family: var(--font-mono); }

.kg-tip {
  position: absolute; bottom: 10px; left: 14px;
  font-size: 11px; color: var(--text-2); pointer-events: none;
}
.kg-empty { padding: 80px 20px; text-align: center; color: var(--text-1); }
.kg-empty-hint { color: var(--text-2); font-size: 13px; margin: 6px 0 18px; }

.kg-side {
  width: 340px; flex-shrink: 0; padding: 14px;
  background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px;
  overflow: hidden;
}
.kg-side-empty { color: var(--text-2); font-size: 13px; line-height: 1.7; padding-top: 40px; text-align: center; }

.sd-head { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.sd-name { font-size: 15px; color: var(--text-0); font-weight: 600; word-break: break-all; }
.sd-ops { display: flex; gap: 6px; margin-bottom: 14px; flex-wrap: wrap; }

.sd-block { margin-bottom: 14px; }
.sd-label { font-size: 11px; color: var(--text-2); margin-bottom: 4px; letter-spacing: .04em; }
.sd-val { font-size: 13px; color: var(--text-0); line-height: 1.6; word-break: break-word; }
.sd-val.pre { white-space: pre-wrap; }
.sd-sub { font-size: 12px; color: var(--text-2); margin-top: 3px; }
.sd-json {
  background: var(--bg-2); border: 1px solid var(--border); border-radius: 5px;
  padding: 8px; font-size: 11px; color: var(--text-1);
  font-family: var(--font-mono); overflow-x: auto; margin: 0;
}

.rel-row {
  display: flex; align-items: center; gap: 6px;
  padding: 6px 0; border-bottom: 1px dashed var(--border); font-size: 12px;
}
.rel-dir { color: var(--accent); font-family: var(--font-mono); }
.rel-type { color: var(--text-1); flex-shrink: 0; }
.rel-peer { color: var(--accent); cursor: pointer; word-break: break-all; }
.rel-peer:hover { text-decoration: underline; }
.rel-peer-type { color: var(--text-2); font-size: 11px; flex-shrink: 0; }
.rel-ops { margin-left: auto; display: flex; gap: 2px; flex-shrink: 0; }

.form-hint { font-size: 11px; color: var(--text-2); }
.mono { font-family: var(--font-mono); font-size: 12px; }
</style>
