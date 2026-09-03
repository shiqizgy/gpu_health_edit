<template>
  <!-- ══ 高度链说明（画布能不能显示出来的关键）══
       .kg-page  flex 纵向铺满 App.vue 的 .content
       .kg-body  flex:1 + min-height:0  吃掉剩余高度
       .kg-stage position:relative
       .kg-canvas position:absolute; inset:0   ← 尺寸完全由父容器决定
       不要用 height:100% 逐层继承，也不要把画布包进 n-spin。 -->
  <div class="kg-page">
    <!-- ── 工具栏 ─────────────────────────────────────────────── -->
    <div class="kg-toolbar">
      <div class="tb-left">
        <n-input
          v-model:value="filters.keyword"
          placeholder="搜索节点名称 / 摘要"
          clearable
          size="small"
          style="width: 196px"
          @keydown.enter="reloadGraph"
          @clear="reloadGraph"
        />
        <n-select
          v-model:value="filters.nodeType"
          :options="nodeTypeOptions"
          placeholder="全部类型"
          clearable
          size="small"
          style="width: 124px"
          @update:value="reloadGraph"
        />
        <n-button size="small" @click="reloadGraph">查询</n-button>

        <n-divider vertical />

        <n-button size="small" type="primary" @click="openCreateNode()">新建节点</n-button>
        <n-button
          size="small"
          :type="linkMode ? 'warning' : 'default'"
          :disabled="!ehReady"
          @click="toggleLinkMode"
        >
          {{ linkMode ? "退出连线" : "拖拽连线" }}
        </n-button>
        <n-button size="small" :disabled="!selected.node" @click="openCreateEdge()">
          连线…
        </n-button>

        <n-divider vertical />

        <n-select
          v-model:value="layoutName"
          :options="layoutOptions"
          size="small"
          style="width: 120px"
        />
        <n-button size="small" @click="relayoutAll">重新布局</n-button>
      </div>

      <div class="tb-right">
        <span class="tb-stat mono">{{ stats.nodes }} 节点 / {{ stats.edges }} 关系</span>
        <n-tag v-if="truncated" size="small" type="warning" :bordered="false">已截断</n-tag>
        <n-tag v-if="posSaving" size="small" type="info" :bordered="false">保存布局…</n-tag>
        <n-button size="small" quaternary @click="zoomBy(1.3)">＋</n-button>
        <n-button size="small" quaternary @click="zoomBy(0.77)">－</n-button>
        <n-button size="small" quaternary @click="fitView">适应</n-button>
        <n-button size="small" quaternary @click="importFromKnowledge">导入</n-button>
      </div>
    </div>

    <!-- ── 主体 ───────────────────────────────────────────────── -->
    <div class="kg-body">
      <div class="kg-stage">
        <div ref="cyEl" class="kg-canvas"></div>

        <!-- 图例：点击可按类型隐藏/显示 -->
        <div class="kg-legend">
          <span
            v-for="t in meta.node_types"
            :key="t.type"
            class="lg-item"
            :class="{ off: hiddenTypes.has(t.type) }"
            @click="toggleType(t.type)"
          >
            <i class="lg-dot" :style="{ background: t.color }"></i>{{ t.label }}
            <em>{{ meta.node_counts?.[t.type] || 0 }}</em>
          </span>
        </div>

        <!-- 加载遮罩用绝对定位盖住，不能包裹画布 -->
        <div v-if="loading" class="kg-mask"><n-spin size="medium" /></div>

        <div v-if="!loading && stats.nodes === 0" class="kg-empty">
          <p class="ke-title">图谱还没有内容</p>
          <p class="ke-hint">先把已有的故障知识条目导入成节点，再手工补充概念和影响范围。</p>
          <n-space justify="center" size="small">
            <n-button size="small" type="primary" @click="openCreateNode()">新建节点</n-button>
            <n-button size="small" @click="importFromKnowledge">从故障知识条目导入</n-button>
          </n-space>
        </div>

        <div class="kg-hint">
          <template v-if="linkMode">
            连线模式：从节点边缘拖到目标节点即可建立关系 · 再次点击「退出连线」恢复拖动
          </template>
          <template v-else>
            左键选中 · 双击展开关联 · 右键菜单 · 拖拽移动（位置自动保存） · 滚轮缩放
          </template>
        </div>

        <!-- 右键菜单 -->
        <div
          v-if="ctxMenu.show"
          class="kg-ctx"
          :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
          @click.stop
        >
          <div
            v-for="it in ctxMenu.items"
            :key="it.key"
            class="ctx-item"
            :class="{ danger: it.danger }"
            @click="runCtx(it)"
          >
            {{ it.label }}
          </div>
        </div>
      </div>

      <!-- ── 右侧详情 ─────────────────────────────────────────── -->
      <div class="kg-side">
        <div v-if="!detail" class="side-empty">
          在左侧选中一个节点，这里显示它的属性和全部关系。
        </div>

        <template v-else>
          <div class="sd-head">
            <span class="sd-badge" :style="badgeStyle(detail.node.node_type)">
              {{ typeLabel(detail.node.node_type) }}
            </span>
            <span class="sd-name">{{ detail.node.name }}</span>
          </div>

          <div class="sd-ops">
            <n-button size="tiny" @click="openEditNode()">编辑</n-button>
            <n-button size="tiny" @click="expandNode(detail.node.id)">展开关联</n-button>
            <n-button size="tiny" @click="openCreateEdge()">连线</n-button>
            <n-button size="tiny" type="error" ghost @click="confirmDeleteNode">删除</n-button>
          </div>

          <div class="sd-scroll">
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
                  style="margin-left: 6px"
                >
                  {{ detail.metric_ref.exists ? "已匹配" : "指标定义中未找到" }}
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
              <div v-if="!detail.edges.length" class="sd-sub">
                还没有关系，点上方「连线」添加。
              </div>
              <div v-for="ew in detail.edges" :key="ew.edge.id" class="rel-row">
                <span class="rel-dir">{{ ew.direction === "out" ? "→" : "←" }}</span>
                <span class="rel-type">{{ relLabel(ew.edge.rel_type) }}</span>
                <a class="rel-peer" @click="focusNode(ew.peer_id)">{{ ew.peer_name }}</a>
                <span class="rel-ops">
                  <n-button size="tiny" quaternary @click="openEditEdge(ew.edge)">改</n-button>
                  <n-button size="tiny" quaternary type="error" @click="confirmDeleteEdge(ew)">
                    删
                  </n-button>
                </span>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- ── 节点弹窗 ───────────────────────────────────────────── -->
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
          <template #feedback v-if="nodeModal.editing">
            <span class="form-hint">类型创建后不可修改，改类型会让已有关系违反约束。</span>
          </template>
        </n-form-item>

        <n-form-item v-if="nodeForm.node_type === 'metric'" label="关联指标">
          <n-select
            v-model:value="nodeForm.name"
            filterable
            remote
            clearable
            tag
            placeholder="输入关键字搜索指标定义"
            :options="metricOptions"
            :loading="metricLoading"
            @search="searchMetrics"
          />
        </n-form-item>
        <n-form-item v-else label="名称" required>
          <n-input v-model:value="nodeForm.name" placeholder="例如：HBM 双比特错误" />
        </n-form-item>

        <n-form-item v-if="nodeModal.editing" label="唯一键">
          <n-input :value="nodeForm.node_key" readonly />
          <template #feedback>
            <span class="form-hint">创建时生成，改名不会改变它。用于导入去重，不建议修改。</span>
          </template>
        </n-form-item>

        <n-form-item v-if="nodeForm.node_type === 'fault'" label="严重等级">
          <n-select v-model:value="nodeForm.severity" :options="sevOptions" clearable />
        </n-form-item>
        <n-form-item label="摘要">
          <n-input v-model:value="nodeForm.summary" placeholder="一句话说明，悬停时显示" />
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

    <!-- ── 关系弹窗 ───────────────────────────────────────────── -->
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
          <template #feedback><span class="form-hint">{{ relHint }}</span></template>
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
          <n-input v-model:value="edgeForm.label" placeholder="留空则显示关系类型默认名称" />
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
import { ref, reactive, computed, onMounted, onBeforeUnmount, nextTick } from "vue";
import { api } from "@/api";
import { useMessage, useDialog } from "naive-ui";
import cytoscape from "cytoscape";
import edgehandles from "cytoscape-edgehandles";

cytoscape.use(edgehandles);

const message = useMessage();
const dialog = useDialog();

/* ══════════════════════════════════════════════════════════════
   状态
   ══════════════════════════════════════════════════════════════ */

const cyEl = ref<HTMLDivElement | null>(null);
let cy: any = null;
let eh: any = null;               // edgehandles 实例
let runningLayout: any = null;    // 当前正在跑的布局，用于打断前一个

const ehReady = ref(false);
const linkMode = ref(false);
const loading = ref(false);
const saving = ref(false);
const posSaving = ref(false);
const truncated = ref(false);
const stats = reactive({ nodes: 0, edges: 0 });

const meta = ref<any>({ node_types: [], rel_types: [], node_counts: {}, edge_count: 0 });
const detail = ref<any>(null);
const selected = reactive<{ node: any }>({ node: null });

/* 当前画布上的原始数据。Cytoscape 里只存渲染必需字段，
   业务对象放这两个 Map，供下拉、去重、重建使用。 */
const rawNodes = ref<Map<number, any>>(new Map());
const rawEdges = ref<Map<number, any>>(new Map());

const filters = reactive({ keyword: "", nodeType: null as string | null });
const hiddenTypes = ref<Set<string>>(new Set());
const layoutName = ref("cose");

const layoutOptions = [
  { label: "力导向布局", value: "cose" },
  { label: "层次布局", value: "breadthfirst" },
  { label: "同心圆布局", value: "concentric" },
  { label: "环形布局", value: "circle" },
  { label: "网格布局", value: "grid" }
];

const sevOptions = [
  { label: "警告", value: "warning" },
  { label: "严重", value: "critical" },
  { label: "致命", value: "fatal" }
];

/* ══════════════════════════════════════════════════════════════
   元数据派生
   ══════════════════════════════════════════════════════════════ */

const nodeTypeOptions = computed(() =>
  (meta.value.node_types || []).map((t: any) => ({ label: t.label, value: t.type }))
);

const nodeOptions = computed(() =>
  [...rawNodes.value.values()].map((n: any) => ({
    label: `[${typeLabel(n.node_type)}] ${n.name}`,
    value: n.id
  }))
);

function typeLabel(t: string) {
  return (meta.value.node_types || []).find((x: any) => x.type === t)?.label || t;
}
function typeColor(t: string) {
  return (meta.value.node_types || []).find((x: any) => x.type === t)?.color || "#9aa7b4";
}
function relLabel(t: string) {
  return (meta.value.rel_types || []).find((x: any) => x.type === t)?.label || t;
}
function sevName(s: string) {
  return ({ warning: "警告", critical: "严重", fatal: "致命" } as any)[s] || s;
}
function badgeStyle(t: string) {
  const c = typeColor(t);
  return { background: c + "22", color: c, border: `1px solid ${c}55` };
}

/* 判断一对节点类型之间是否存在合法关系，供 edgehandles 和下拉过滤共用 */
function allowedRelTypes(fromType: string, toType: string) {
  return (meta.value.rel_types || []).filter(
    (r: any) =>
      !r.pairs || !r.pairs.length || r.pairs.some((p: string[]) => p[0] === fromType && p[1] === toType)
  );
}

const relOptionsForPair = computed(() => {
  const from = rawNodes.value.get(edgeForm.from_id as any);
  const to = rawNodes.value.get(edgeForm.to_id as any);
  if (!from || !to) {
    return (meta.value.rel_types || []).map((r: any) => ({ label: r.label, value: r.type }));
  }
  return allowedRelTypes(from.node_type, to.node_type).map((r: any) => ({
    label: r.label,
    value: r.type
  }));
});

const relHint = computed(() => {
  const r = (meta.value.rel_types || []).find((x: any) => x.type === edgeForm.rel_type);
  return r?.desc || "先选好起点和终点，可选的关系类型会自动过滤";
});

/* ══════════════════════════════════════════════════════════════
   Cytoscape 样式与初始化
   ══════════════════════════════════════════════════════════════ */

function nodeSize(n: any) {
  if (n.node_type !== "fault") return 34;
  return n.severity === "fatal" ? 52 : n.severity === "critical" ? 44 : 38;
}

function buildStyle() {
  return [
    {
      selector: "node",
      style: {
        "background-color": "data(color)",
        "background-opacity": 0.9,
        "border-width": 2,
        "border-color": "data(color)",
        "border-opacity": 0.45,
        width: "data(size)",
        height: "data(size)",
        label: "data(name)",
        color: "#e6edf3",
        "font-size": 11,
        "font-family": "IBM Plex Sans, PingFang SC, sans-serif",
        "text-valign": "bottom",
        "text-halign": "center",
        "text-margin-y": 6,
        "text-wrap": "ellipsis",
        "text-max-width": "110px",
        "text-outline-width": 3,
        "text-outline-color": "#0a0e14",
        "text-outline-opacity": 0.9,
        "transition-property": "border-width, border-color, opacity",
        "transition-duration": "150ms"
      }
    },
    {
      selector: "edge",
      style: {
        width: "data(w)",
        "line-color": "#33465c",
        "target-arrow-color": "#33465c",
        "target-arrow-shape": "triangle",
        "arrow-scale": 0.85,
        "curve-style": "bezier",
        "control-point-step-size": 44,
        label: "data(label)",
        "font-size": 9,
        color: "#8895a3",
        "text-rotation": "autorotate",
        "text-background-color": "#0f141c",
        "text-background-opacity": 0.88,
        "text-background-padding": "2px",
        "text-background-shape": "roundrectangle",
        "transition-property": "line-color, target-arrow-color, opacity, width",
        "transition-duration": "150ms"
      }
    },
    { selector: "node.sel", style: { "border-width": 4, "border-color": "#e6edf3", "border-opacity": 1 } },
    { selector: "node.nb", style: { "border-width": 3, "border-color": "#38bdf8", "border-opacity": 0.9 } },
    { selector: "edge.nb", style: { "line-color": "#38bdf8", "target-arrow-color": "#38bdf8", width: 2.4 } },
    { selector: ".faded", style: { opacity: 0.12 } },
    /* 已固定位置的节点：双线边框，提示它不参与自动布局 */
    { selector: "node.pinned", style: { "border-style": "double", "border-width": 5, "border-opacity": 0.75 } },
    { selector: ".hidden", style: { display: "none" } },
    /* edgehandles 拖拽连线的临时样式 */
    { selector: ".eh-handle", style: { "background-color": "#38bdf8", width: 12, height: 12, "border-width": 0 } },
    { selector: ".eh-source, .eh-target", style: { "border-width": 3, "border-color": "#38bdf8", "border-opacity": 1 } },
    { selector: ".eh-preview, .eh-ghost-edge", style: { "line-color": "#38bdf8", "target-arrow-color": "#38bdf8", "line-style": "dashed" } },
    { selector: ".eh-ghost-edge.eh-preview-active", style: { opacity: 0 } }
  ];
}

/* 双击判定的状态。合并进唯一的 tap handler，
   避免双击时 selectNode 被触发两次、发出两次详情请求。 */
let lastTapTime = 0;
let lastTapId = "";

function initCy() {
  if (!cyEl.value) return;

  cy = cytoscape({
    container: cyEl.value,
    elements: [],
    style: buildStyle(),
    layout: { name: "preset" },
    minZoom: 0.1,
    maxZoom: 4,
    wheelSensitivity: 0.25,
    boxSelectionEnabled: false,
    autounselectify: false
  });

  /* 单击 / 双击节点：统一在这一个 handler 里判定 */
  cy.on("tap", "node", (evt: any) => {
    hideCtx();
    const idStr = evt.target.id();
    const now = Date.now();

    if (idStr === lastTapId && now - lastTapTime < 320) {
      lastTapTime = 0;
      lastTapId = "";
      expandNode(Number(idStr));
      return;
    }
    lastTapTime = now;
    lastTapId = idStr;

    highlight(evt.target);
    selectNode(Number(idStr));
  });

  cy.on("tap", "edge", (evt: any) => {
    hideCtx();
    const raw = rawEdges.value.get(edgeIdOf(evt.target));
    if (raw) openEditEdge(raw);
  });

  cy.on("tap", (evt: any) => {
    if (evt.target === cy) {
      hideCtx();
      clearHighlight();
      detail.value = null;
      selected.node = null;
    }
  });

  /* 拖拽结束：固定位置 + 标记待保存。
     这是和 ECharts 力导向体验差异最大的一点——拖到哪停在哪。 */
  cy.on("dragfree", "node", (evt: any) => {
    evt.target.addClass("pinned");
    evt.target.lock();
    markDirty(Number(evt.target.id()));
  });

  cy.on("cxttap", "node", (evt: any) => showCtxForNode(evt));
  cy.on("cxttap", "edge", (evt: any) => showCtxForEdge(evt));
  cy.on("cxttap", (evt: any) => {
    if (evt.target === cy) showCtxForCanvas(evt);
  });

  initEdgeHandles();
}

/* 拖拽连线 */
function initEdgeHandles() {
  try {
    eh = (cy as any).edgehandles({
      snap: true,
      snapThreshold: 24,
      noEdgeEventsInDraw: true,
      disableBrowserGestures: true,
      /* 交互层就拦住 schema 不允许的组合，避免拖完才被后端打回 */
      canConnect: (src: any, tgt: any) => {
        if (src.id() === tgt.id()) return false;
        const from = rawNodes.value.get(Number(src.id()));
        const to = rawNodes.value.get(Number(tgt.id()));
        if (!from || !to) return false;
        return allowedRelTypes(from.node_type, to.node_type).length > 0;
      },
      edgeParams: () => ({ data: { label: "", w: 2 } })
    });
    eh.disableDrawMode();
    ehReady.value = true;

    /* 拖出线之后不直接落库，先移除预览边、弹窗让用户选关系类型 */
    cy.on("ehcomplete", (_e: any, src: any, tgt: any, added: any) => {
      try { added.remove(); } catch { /* 预览边可能已被移除 */ }
      const fromId = Number(src.id());
      const toId = Number(tgt.id());
      Object.assign(edgeForm, emptyEdgeForm());
      edgeForm.from_id = fromId;
      edgeForm.to_id = toId;
      const opts = relOptionsForPair.value;
      if (opts.length === 1) edgeForm.rel_type = opts[0].value;
      Object.assign(edgeModal, { show: true, editing: false, id: 0, version: 0 });
    });
  } catch (e) {
    /* 扩展没装成功也不影响其余功能，只是禁用「拖拽连线」按钮 */
    console.warn("cytoscape-edgehandles 初始化失败，拖拽连线不可用", e);
    ehReady.value = false;
  }
}

function toggleLinkMode() {
  if (!eh) return;
  linkMode.value = !linkMode.value;
  if (linkMode.value) {
    eh.enableDrawMode();
    message.info("连线模式：从节点边缘拖到目标节点");
  } else {
    eh.disableDrawMode();
  }
}

/* Cytoscape 边 id 形如 "e123"，这里统一解析回业务 id */
function edgeIdOf(ele: any): number {
  return Number(String(ele.id()).replace(/^e/, ""));
}

/* ══════════════════════════════════════════════════════════════
   高亮与视图
   ══════════════════════════════════════════════════════════════ */

function highlight(node: any) {
  clearHighlight();
  const nb = node.closedNeighborhood();
  cy.elements().addClass("faded");
  nb.removeClass("faded");
  nb.nodes().addClass("nb");
  nb.edges().addClass("nb");
  node.removeClass("nb").addClass("sel");
}

function clearHighlight() {
  if (!cy) return;
  cy.elements().removeClass("faded nb sel");
}

function zoomBy(f: number) {
  if (!cy) return;
  cy.zoom({ level: cy.zoom() * f, renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 } });
}

function fitView() {
  if (!cy || cy.elements().length === 0) return;
  cy.fit(cy.elements().not(".hidden"), 60);
}

/* 定位到某个节点。retry 参数防止无限递归——
   目标可能被类型筛选隐藏，也可能根本不在展开范围内。 */
async function focusNode(id: number, retry = 0) {
  if (!cy) return;
  let n = cy.getElementById(String(id));

  /* 在画布上但被筛选隐藏：先放开该类型 */
  if (n.length && n.hasClass("hidden")) {
    const t = n.data("ntype");
    const s = new Set(hiddenTypes.value);
    s.delete(t);
    hiddenTypes.value = s;
    applyTypeFilter();
  }

  if (n.length === 0) {
    if (retry >= 1) {
      message.warning("该节点不在当前视图中，请调整筛选条件后重试");
      return;
    }
    await expandNode(id);
    await nextTick();
    return focusNode(id, retry + 1);
  }

  cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 1) }, { duration: 300 });
  highlight(n);
  selectNode(id);
}

/* ══════════════════════════════════════════════════════════════
   布局
   ══════════════════════════════════════════════════════════════ */

function layoutConfig(name: string, randomize: boolean) {
  const base: any = { name, animate: true, animationDuration: 500, fit: true, padding: 60 };
  if (name === "cose") {
    return {
      ...base,
      randomize,
      nodeRepulsion: 12000,
      idealEdgeLength: 130,
      edgeElasticity: 100,
      nestingFactor: 1.2,
      gravity: 0.25,
      numIter: 1200,
      initialTemp: 200,
      coolingFactor: 0.95,
      nodeOverlap: 24
    };
  }
  if (name === "breadthfirst") return { ...base, directed: true, spacingFactor: 1.4, grid: true };
  if (name === "concentric") {
    return { ...base, concentric: (n: any) => n.degree(), levelWidth: () => 2, minNodeSpacing: 40 };
  }
  if (name === "circle") return { ...base, spacingFactor: 1.2 };
  return { ...base, avoidOverlap: true, spacingFactor: 1.1 };
}

function stopRunningLayout() {
  if (runningLayout) {
    try { runningLayout.stop(); } catch { /* 已结束 */ }
    runningLayout = null;
  }
}

/* 全量重排：解开所有固定，重新算位置，并把结果写回数据库，
   这样「重新布局」这个动作本身也是可持久化的。 */
function relayoutAll() {
  if (!cy || cy.elements().length === 0) return;
  stopRunningLayout();
  cy.nodes().unlock().removeClass("pinned");

  runningLayout = cy.layout(layoutConfig(layoutName.value, true));
  runningLayout.one("layoutstop", () => {
    cy.nodes().forEach((n: any) => markDirty(Number(n.id())));
    runningLayout = null;
  });
  runningLayout.run();
}

/* 增量布局：只给没有坐标的新节点找位置，已摆好的节点保持原位。
   带 4 秒超时兜底——布局被打断时 layoutstop 不会触发，
   如果不恢复，老节点会永久 locked，用户再也拖不动。 */
function layoutFreshOnly() {
  if (!cy) return;
  const fresh = cy.nodes(".fresh");
  if (fresh.length === 0) {
    fitView();
    return;
  }
  stopRunningLayout();

  const old = cy.nodes().not(fresh);
  const wasLocked = old.filter((n: any) => n.locked());
  old.lock();

  let restored = false;
  const restore = () => {
    if (restored) return;
    restored = true;
    old.unlock();
    wasLocked.lock();
    cy.nodes().removeClass("fresh");
    runningLayout = null;
  };

  runningLayout = cy.layout({ ...layoutConfig("cose", false), fit: false });
  runningLayout.one("layoutstop", () => {
    /* 新节点的位置也要落库，否则刷新后又要重算 */
    fresh.forEach((n: any) => markDirty(Number(n.id())));
    restore();
  });
  setTimeout(restore, 4000);
  runningLayout.run();
}

/* ══════════════════════════════════════════════════════════════
   位置持久化
   ══════════════════════════════════════════════════════════════ */

const dirtyPos = new Set<number>();
let posTimer: any = null;

function markDirty(id: number) {
  dirtyPos.add(id);
  clearTimeout(posTimer);
  posTimer = setTimeout(flushPositions, 800);
}

async function flushPositions() {
  if (!cy || dirtyPos.size === 0) return;
  const positions: any[] = [];
  dirtyPos.forEach((id) => {
    const n = cy.getElementById(String(id));
    if (n.length) {
      const p = n.position();
      positions.push({ id, x: p.x, y: p.y });
      /* 同步更新本地缓存，避免下一次 rebuild 时又当成无坐标节点 */
      const raw = rawNodes.value.get(id);
      if (raw) { raw.pos_x = p.x; raw.pos_y = p.y; }
    }
  });
  dirtyPos.clear();
  if (!positions.length) return;

  posSaving.value = true;
  try {
    await api.kgSaveLayout(positions);
  } catch (e: any) {
    message.warning("布局保存失败，位置在刷新后会丢失：" + apiMsg(e, ""));
  } finally {
    posSaving.value = false;
  }
}

/* ══════════════════════════════════════════════════════════════
   数据 → 画布
   ══════════════════════════════════════════════════════════════ */

function toCyElements(nodes: any[], edges: any[]) {
  const els: any[] = [];
  for (const n of nodes) {
    const el: any = {
      group: "nodes",
      data: {
        id: String(n.id),
        name: n.name,
        color: typeColor(n.node_type),
        size: nodeSize(n),
        ntype: n.node_type
      }
    };
    /* 存过坐标的节点直接摆回原位并标记固定 */
    if (n.pos_x != null && n.pos_y != null) {
      el.position = { x: Number(n.pos_x), y: Number(n.pos_y) };
      el.classes = "pinned";
    }
    els.push(el);
  }

  const idSet = new Set(nodes.map((n: any) => n.id));
  for (const e of edges) {
    /* 双保险：Cytoscape 遇到端点不存在的边会抛异常导致整个 add() 失败，
       画布会整个变空。后端已保证，这里再挡一次。 */
    if (!idSet.has(e.from_id) || !idSet.has(e.to_id)) continue;
    els.push({
      group: "edges",
      data: {
        id: "e" + e.id,
        source: String(e.from_id),
        target: String(e.to_id),
        label: e.label || relLabel(e.rel_type),
        w: 1 + (Number(e.weight) || 1) * 1.6,
        rtype: e.rel_type
      }
    });
  }
  return els;
}

function rebuildCanvas(nodes: any[], edges: any[]) {
  if (!cy) return;
  stopRunningLayout();

  rawNodes.value = new Map(nodes.map((n: any) => [n.id, n]));
  rawEdges.value = new Map(edges.map((e: any) => [e.id, e]));

  cy.elements().remove();
  cy.add(toCyElements(nodes, edges));
  cy.nodes(".pinned").lock();

  applyTypeFilter();
  syncStats();

  const total = cy.nodes().length;
  const unplaced = cy.nodes().not(".pinned");

  if (total === 0) return;
  if (unplaced.length === total) {
    /* 全是没坐标的节点：正常全量布局，并把结果存下来 */
    stopRunningLayout();
    runningLayout = cy.layout(layoutConfig(layoutName.value, true));
    runningLayout.one("layoutstop", () => {
      cy.nodes().forEach((n: any) => markDirty(Number(n.id())));
      runningLayout = null;
    });
    runningLayout.run();
  } else if (unplaced.length > 0) {
    unplaced.addClass("fresh");
    layoutFreshOnly();
  } else {
    fitView();
  }
}

function mergeIntoCanvas(nodes: any[], edges: any[]) {
  if (!cy) return;

  const newNodes: any[] = [];
  for (const n of nodes) {
    if (!rawNodes.value.has(n.id)) {
      rawNodes.value.set(n.id, n);
      newNodes.push(n);
    }
  }

  const allIds = new Set(rawNodes.value.keys());
  const newEdges: any[] = [];
  for (const e of edges) {
    if (rawEdges.value.has(e.id)) continue;
    if (!allIds.has(e.from_id) || !allIds.has(e.to_id)) continue;
    rawEdges.value.set(e.id, e);
    newEdges.push(e);
  }

  if (!newNodes.length && !newEdges.length) {
    message.info("没有发现新的关联节点");
    return;
  }

  /* 只把新元素加进画布，已有节点位置完全不动 */
  const els = toCyElements(newNodes, []).concat(
    toCyElements([...rawNodes.value.values()], newEdges).filter((el: any) => el.group === "edges")
  );
  const added = cy.add(els);
  added.nodes().filter((n: any) => !n.hasClass("pinned")).addClass("fresh");
  added.nodes(".pinned").lock();

  applyTypeFilter();
  syncStats();
  layoutFreshOnly();
}

function toggleType(t: string) {
  const s = new Set(hiddenTypes.value);
  s.has(t) ? s.delete(t) : s.add(t);
  hiddenTypes.value = s;
  applyTypeFilter();
}

function applyTypeFilter() {
  if (!cy) return;
  cy.nodes().forEach((n: any) => {
    n.toggleClass("hidden", hiddenTypes.value.has(n.data("ntype")));
  });
  /* 端点被隐藏的边也要隐藏，否则画布上会出现悬空的线 */
  cy.edges().forEach((e: any) => {
    e.toggleClass("hidden", e.source().hasClass("hidden") || e.target().hasClass("hidden"));
  });
}

function syncStats() {
  if (!cy) return;
  stats.nodes = cy.nodes().length;
  stats.edges = cy.edges().length;
}

/* ══════════════════════════════════════════════════════════════
   接口调用
   ══════════════════════════════════════════════════════════════ */

async function loadMeta() {
  try {
    const m = await api.kgMeta();
    /* axios 拦截器已把 {code,msg,data} 剥到 data，
       这里兼容拦截器被改动的情况 */
    meta.value = m?.node_types ? m : m?.data || { node_types: [], rel_types: [], node_counts: {} };
  } catch (e: any) {
    message.error("加载图谱元数据失败：" + apiMsg(e, "请检查 /kg/meta 接口"));
  }
}

async function reloadGraph() {
  loading.value = true;
  try {
    const g = await api.kgGraph({
      keyword: filters.keyword || undefined,
      node_type: filters.nodeType || undefined,
      limit: 300
    });
    truncated.value = !!g?.truncated;
    rebuildCanvas(g?.nodes || [], g?.edges || []);
    if (detail.value && !rawNodes.value.has(detail.value.node.id)) {
      detail.value = null;
      selected.node = null;
    }
    await loadMeta();
  } catch (e: any) {
    message.error("加载图谱失败：" + apiMsg(e, "请检查 /kg/graph 接口"));
  } finally {
    loading.value = false;
  }
}

async function selectNode(id: number) {
  try {
    const d = await api.kgNodeDetail(id);
    detail.value = d;
    selected.node = d.node;
  } catch (e: any) {
    if (e?.response?.status === 404) {
      message.warning("该节点已被删除，正在刷新");
      detail.value = null;
      selected.node = null;
      await reloadGraph();
      return;
    }
    message.error("加载节点详情失败：" + apiMsg(e, ""));
  }
}

async function expandNode(id: number) {
  loading.value = true;
  try {
    const g = await api.kgNeighbors(id, { depth: 1, limit: 200 });
    mergeIntoCanvas(g?.nodes || [], g?.edges || []);
    if (g?.truncated) message.warning("关联节点较多，本次只展开了前 200 个");
  } catch (e: any) {
    message.error("展开关联失败：" + apiMsg(e, ""));
  } finally {
    loading.value = false;
  }
}

/* ══════════════════════════════════════════════════════════════
   右键菜单
   ══════════════════════════════════════════════════════════════ */

const ctxMenu = reactive<{ show: boolean; x: number; y: number; items: any[] }>({
  show: false, x: 0, y: 0, items: []
});

function showCtxAt(evt: any, items: any[]) {
  const p = evt.renderedPosition || evt.position;
  const box = cyEl.value!.getBoundingClientRect();
  ctxMenu.x = Math.max(4, Math.min(p.x, box.width - 168));
  ctxMenu.y = Math.max(4, Math.min(p.y, box.height - items.length * 32 - 12));
  ctxMenu.items = items;
  ctxMenu.show = true;
}
function hideCtx() { ctxMenu.show = false; }
function runCtx(item: any) { hideCtx(); item.fn?.(); }

function showCtxForNode(evt: any) {
  const id = Number(evt.target.id());
  const pinned = evt.target.hasClass("pinned");
  highlight(evt.target);
  selectNode(id);

  showCtxAt(evt, [
    { key: "expand", label: "展开关联", fn: () => expandNode(id) },
    { key: "edge", label: "从此节点连线", fn: () => openCreateEdge() },
    { key: "edit", label: "编辑节点", fn: () => openEditNode() },
    {
      key: "pin",
      label: pinned ? "取消固定" : "固定位置",
      fn: () => {
        const n = cy.getElementById(String(id));
        if (!n.length) return;
        if (pinned) {
          n.unlock();
          n.removeClass("pinned");
        } else {
          n.lock();
          n.addClass("pinned");
          markDirty(id);
        }
      }
    },
    { key: "hide", label: "从画布移除", fn: () => removeFromCanvas(id) },
    { key: "del", label: "删除节点", danger: true, fn: () => confirmDeleteNode() }
  ]);
}

/* 从画布移除必须同步清理 rawNodes / rawEdges，
   否则「连线」下拉里还能选到画布上不存在的节点，
   建完关系又看不见，用户会以为没保存成功。 */
function removeFromCanvas(id: number) {
  const ele = cy.getElementById(String(id));
  if (!ele.length) return;
  ele.connectedEdges().forEach((e: any) => rawEdges.value.delete(edgeIdOf(e)));
  ele.remove();
  rawNodes.value.delete(id);
  dirtyPos.delete(id);
  if (selected.node?.id === id) {
    detail.value = null;
    selected.node = null;
  }
  syncStats();
}

function showCtxForEdge(evt: any) {
  const raw = rawEdges.value.get(edgeIdOf(evt.target));
  if (!raw) return;
  showCtxAt(evt, [
    { key: "edit", label: "编辑关系", fn: () => openEditEdge(raw) },
    {
      key: "del",
      label: "删除关系",
      danger: true,
      fn: () =>
        dialog.warning({
          title: "删除关系",
          content: "删除后不可恢复。",
          positiveText: "删除",
          negativeText: "取消",
          onPositiveClick: async () => {
            try {
              await api.kgDeleteEdge(raw.id);
              message.success("关系已删除");
              await reloadGraph();
            } catch (e: any) {
              handleApiError(e, "删除关系失败");
            }
          }
        })
    }
  ]);
}

function showCtxForCanvas(evt: any) {
  showCtxAt(evt, [
    { key: "new", label: "新建节点", fn: () => openCreateNode() },
    { key: "relayout", label: "重新布局", fn: () => relayoutAll() },
    { key: "fit", label: "适应画布", fn: () => fitView() },
    {
      key: "unpin",
      label: "解除全部固定",
      fn: () => {
        cy.nodes().unlock().removeClass("pinned");
        message.info("已解除固定，下次重新布局会重新计算位置");
      }
    }
  ]);
}

/* ══════════════════════════════════════════════════════════════
   节点增删改
   ══════════════════════════════════════════════════════════════ */

const nodeModal = reactive({ show: false, editing: false, id: 0, version: 0 });
const nodeForm = reactive<any>(emptyNodeForm());

function emptyNodeForm() {
  return {
    node_type: "fault", name: "", node_key: "",
    summary: "", description: "", severity: "", props: "{}"
  };
}

function openCreateNode() {
  Object.assign(nodeForm, emptyNodeForm());
  Object.assign(nodeModal, { show: true, editing: false, id: 0, version: 0 });
}

function openEditNode() {
  const n = selected.node;
  if (!n) return;
  Object.assign(nodeForm, {
    node_type: n.node_type,
    name: n.name,
    node_key: n.node_key,
    summary: n.summary || "",
    description: n.description || "",
    severity: n.severity || "",
    props: n.props || "{}"
  });
  Object.assign(nodeModal, { show: true, editing: true, id: n.id, version: n.version });
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

  const payload: any = {
    node_type: nodeForm.node_type,
    name: nodeForm.name.trim(),
    summary: nodeForm.summary,
    description: nodeForm.description,
    severity: nodeForm.node_type === "fault" ? nodeForm.severity : "",
    props: nodeForm.props?.trim() || "{}"
  };
  /* 指标节点把指标名写进 props，后端据此做引用有效性校验 */
  if (payload.node_type === "metric") {
    const p = safeParse(payload.props);
    p.metric_name = payload.name;
    payload.props = JSON.stringify(p);
  }

  saving.value = true;
  try {
    if (nodeModal.editing) {
      payload.version = nodeModal.version;
      await api.kgUpdateNode(nodeModal.id, payload);
      message.success("节点已保存");
      nodeModal.show = false;
      await reloadGraph();
      await selectNode(nodeModal.id);
    } else {
      const created = await api.kgCreateNode(payload);
      message.success("节点已创建");
      nodeModal.show = false;
      await reloadGraph();
      if (created?.id) await focusNode(created.id);
    }
  } catch (e: any) {
    handleApiError(e, "保存节点失败");
  } finally {
    saving.value = false;
  }
}

function confirmDeleteNode() {
  const n = selected.node;
  if (!n) return;
  const cnt = detail.value?.edges?.length || 0;
  dialog.warning({
    title: "删除节点",
    content: cnt
      ? `删除「${n.name}」会同时删除它的 ${cnt} 条关系，此操作不可撤销。`
      : `删除「${n.name}」，此操作不可撤销。`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.kgDeleteNode(n.id);
        message.success("节点已删除");
        detail.value = null;
        selected.node = null;
        await reloadGraph();
      } catch (e: any) {
        handleApiError(e, "删除节点失败");
      }
    }
  });
}

/* ══════════════════════════════════════════════════════════════
   关系增删改
   ══════════════════════════════════════════════════════════════ */

const edgeModal = reactive({ show: false, editing: false, id: 0, version: 0 });
const edgeForm = reactive<any>(emptyEdgeForm());

function emptyEdgeForm() {
  return { from_id: null, to_id: null, rel_type: null, label: "", weight: 1 };
}

function openCreateEdge() {
  Object.assign(edgeForm, emptyEdgeForm());
  if (selected.node) edgeForm.from_id = selected.node.id;
  Object.assign(edgeModal, { show: true, editing: false, id: 0, version: 0 });
}

function openEditEdge(e: any) {
  Object.assign(edgeForm, {
    from_id: e.from_id,
    to_id: e.to_id,
    rel_type: e.rel_type,
    label: e.label || "",
    weight: Number(e.weight) || 1
  });
  Object.assign(edgeModal, { show: true, editing: true, id: e.id, version: e.version });
}

async function saveEdge() {
  if (!edgeModal.editing) {
    if (!edgeForm.from_id || !edgeForm.to_id) { message.warning("请选择起点和终点"); return; }
    if (edgeForm.from_id === edgeForm.to_id) { message.warning("起点和终点不能相同"); return; }
    if (!edgeForm.rel_type) { message.warning("请选择关系类型"); return; }
  }

  saving.value = true;
  try {
    if (edgeModal.editing) {
      await api.kgUpdateEdge(edgeModal.id, {
        label: edgeForm.label,
        weight: edgeForm.weight,
        props: "{}",
        version: edgeModal.version
      });
      message.success("关系已保存");
    } else {
      await api.kgCreateEdge({
        from_id: edgeForm.from_id,
        to_id: edgeForm.to_id,
        rel_type: edgeForm.rel_type,
        label: edgeForm.label,
        weight: edgeForm.weight,
        props: "{}"
      });
      message.success("关系已创建");
    }
    edgeModal.show = false;
    await reloadGraph();
    if (selected.node) await selectNode(selected.node.id);
  } catch (e: any) {
    handleApiError(e, "保存关系失败");
  } finally {
    saving.value = false;
  }
}

function confirmDeleteEdge(ew: any) {
  dialog.warning({
    title: "删除关系",
    content: `删除「${selected.node?.name}」与「${ew.peer_name}」之间的关系？`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.kgDeleteEdge(ew.edge.id);
        message.success("关系已删除");
        await reloadGraph();
        if (selected.node) await selectNode(selected.node.id);
      } catch (e: any) {
        handleApiError(e, "删除关系失败");
      }
    }
  });
}

/* ══════════════════════════════════════════════════════════════
   导入与指标搜索
   ══════════════════════════════════════════════════════════════ */

function importFromKnowledge() {
  dialog.info({
    title: "从故障知识条目导入",
    content:
      "把「故障知识」里的条目转成故障节点和指标节点。已存在的节点会被跳过，不会覆盖你手工修改过的内容。",
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
          message.warning(`有 ${r.warnings.length} 条需要确认：${r.warnings[0]}`, { duration: 8000 });
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
      metricOptions.value = (list || []).map((m: any) => ({
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

/* ══════════════════════════════════════════════════════════════
   工具
   ══════════════════════════════════════════════════════════════ */

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
  return Object.keys(safeParse(s)).length > 0;
}
function prettyJSON(s: string) {
  try { return JSON.stringify(JSON.parse(s || "{}"), null, 2); } catch { return s; }
}
function apiMsg(e: any, fallback: string) {
  return e?.response?.data?.msg || e?.message || fallback;
}
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

/* ══════════════════════════════════════════════════════════════
   生命周期
   ══════════════════════════════════════════════════════════════ */

let ro: ResizeObserver | null = null;

function onDocClick() { hideCtx(); }
function onCtxMenu(e: Event) { e.preventDefault(); }
function onBeforeUnload() { flushPositions(); }

onMounted(async () => {
  await loadMeta();
  await nextTick();          // 等容器完成布局，否则 Cytoscape 读到的尺寸是 0
  initCy();
  await reloadGraph();

  if (cyEl.value && "ResizeObserver" in window) {
    ro = new ResizeObserver(() => cy && cy.resize());
    ro.observe(cyEl.value);
  }
  document.addEventListener("click", onDocClick);
  window.addEventListener("beforeunload", onBeforeUnload);
  cyEl.value?.addEventListener("contextmenu", onCtxMenu);
});

onBeforeUnmount(() => {
  flushPositions();          // 防抖窗口内的最后一次拖动别丢
  document.removeEventListener("click", onDocClick);
  window.removeEventListener("beforeunload", onBeforeUnload);
  cyEl.value?.removeEventListener("contextmenu", onCtxMenu);
  ro?.disconnect();
  clearTimeout(metricTimer);
  clearTimeout(posTimer);
  stopRunningLayout();
  try { eh?.destroy(); } catch { /* 未初始化成功 */ }
  if (cy) { cy.destroy(); cy = null; }
});
</script>

<style scoped>
.kg-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 92px);
  min-height: 460px;
}

.kg-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 12px; margin-bottom: 10px; flex-wrap: wrap; flex-shrink: 0;
}
.tb-left, .tb-right { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.tb-stat { color: var(--text-1); font-size: 12px; }

.kg-body { display: flex; gap: 12px; flex: 1; min-height: 0; }

.kg-stage {
  position: relative; flex: 1; min-width: 0;
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
}
.kg-canvas { position: absolute; inset: 0; }

.kg-legend {
  position: absolute; top: 12px; left: 14px; z-index: 3;
  display: flex; gap: 14px; flex-wrap: wrap;
}
.lg-item {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 12px; color: var(--text-1);
  cursor: pointer; user-select: none;
  padding: 2px 6px; border-radius: 4px;
  transition: background .15s, opacity .15s;
}
.lg-item:hover { background: var(--bg-2); }
.lg-item.off { opacity: .35; text-decoration: line-through; }
.lg-dot { width: 9px; height: 9px; border-radius: 50%; display: inline-block; }
.lg-item em { font-style: normal; color: var(--text-2); font-family: var(--font-mono); }

.kg-mask {
  position: absolute; inset: 0; z-index: 5;
  display: flex; align-items: center; justify-content: center;
  background: rgba(10, 14, 20, .45);
}

.kg-empty {
  position: absolute; inset: 0; z-index: 4;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 4px; color: var(--text-1);
}
.ke-title { font-size: 15px; margin: 0; }
.ke-hint { color: var(--text-2); font-size: 13px; margin: 4px 0 14px; }

.kg-hint {
  position: absolute; bottom: 10px; left: 14px; z-index: 3;
  font-size: 11px; color: var(--text-2); pointer-events: none;
}

.kg-ctx {
  position: absolute; z-index: 20; min-width: 152px;
  background: var(--bg-2); border: 1px solid var(--border);
  border-radius: 6px; padding: 4px; box-shadow: 0 8px 24px rgba(0,0,0,.5);
}
.ctx-item {
  padding: 7px 12px; font-size: 13px; color: var(--text-0);
  border-radius: 4px; cursor: pointer; white-space: nowrap;
}
.ctx-item:hover { background: var(--bg-3); }
.ctx-item.danger { color: var(--lv-failed); }

.kg-side {
  width: 336px; flex-shrink: 0; padding: 14px;
  background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px;
  display: flex; flex-direction: column; min-height: 0;
}
.side-empty {
  color: var(--text-2); font-size: 13px; line-height: 1.7;
  padding-top: 40px; text-align: center;
}

.sd-head { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; flex-shrink: 0; }
.sd-badge { font-size: 11px; padding: 2px 8px; border-radius: 4px; }
.sd-name { font-size: 15px; color: var(--text-0); font-weight: 600; word-break: break-all; }
.sd-ops { display: flex; gap: 6px; margin-bottom: 12px; flex-wrap: wrap; flex-shrink: 0; }
.sd-scroll { flex: 1; overflow-y: auto; min-height: 0; padding-right: 4px; }

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
.rel-ops { margin-left: auto; display: flex; gap: 2px; flex-shrink: 0; }

.form-hint { font-size: 11px; color: var(--text-2); }
.mono { font-family: var(--font-mono); font-size: 12px; }
</style>
