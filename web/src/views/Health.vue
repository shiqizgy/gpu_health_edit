<template>
  <div>
    <n-tabs type="line" v-model:value="tab">
      <!-- 集群 → 单卡 -->
      <n-tab-pane name="scores" tab="集群健康度">
        <div class="toolbar">
          <n-button size="small" @click="loadClusters">刷新</n-button>
          <span class="hint">点击集群行查看该集群内每张 GPU 的评分</span>
          <n-input-group style="width: 360px; margin-left: auto;">
            <n-input v-model:value="healthSearchKeyword" placeholder="搜索 GPU（UUID 模糊匹配）" size="small" clearable @clear="clearHealthSearch" />
            <n-button size="small" type="primary" @click="doHealthSearch">搜索</n-button>
          </n-input-group>
        </div>

        <!-- 集群表格(预聚合,毫秒级) -->
        <div class="panel">
          <div class="panel-title">集群汇总</div>
          <n-data-table
            :columns="clusterCols"
            :data="clusters"
            :bordered="false"
            size="small"
            :row-props="clusterRowProps"
          />
        </div>

        <!-- 搜索结果(独立面板,搜索时显示) -->
        <div class="panel" v-if="healthSearchResults.length" style="margin-top: 16px">
          <div class="panel-title">
            搜索结果（{{ healthSearchResults.length }} 张卡）
            <n-button size="tiny" style="margin-left:12px" @click="clearHealthSearch">清除</n-button>
          </div>
          <n-data-table
            :columns="gpuCols"
            :data="healthSearchResults"
            :bordered="false"
            size="small"
            :max-height="440"
          />
        </div>

        <!-- 单卡明细(点击集群后展开,分页) -->
        <div class="panel" v-if="activeCluster" style="margin-top: 16px">
          <div class="panel-title">
            集群 {{ activeCluster.cluster_name }} — GPU 评分明细(坏卡置顶)
          </div>
          <n-data-table
            :columns="gpuCols"
            :data="gpus"
            :bordered="false"
            size="small"
            :max-height="440"
          />
          <div class="pager">
            <n-pagination
              v-model:page="page"
              :page-count="pageCount"
              :page-size="pageSize"
              @update:page="loadGPUs"
            />
          </div>
        </div>
      </n-tab-pane>

      <!-- 策略管理 -->
      <n-tab-pane name="strategy" tab="评分策略">
        <div class="toolbar">
          <span class="hint">不同任务可配置不同的指标权重与维度权重，评分时按集群/卡选择对应策略</span>
          <n-button type="primary" size="small" @click="openCreateStrategy">+ 新建策略</n-button>
        </div>
        <div class="panel">
          <div class="panel-title">策略列表</div>
          <n-data-table :columns="strategyCols" :data="strategies" :bordered="false" size="small" />
        </div>

        <div class="panel" v-if="editStrategy" style="margin-top: 16px">
          <div class="panel-title">编辑策略：{{ editStrategy.name }}</div>
          <div style="padding: 16px">
               <!-- 基本信息：名称 / 说明 可修改（代码为标识，只读）-->
               <div class="basic-info">
                 <div class="section-title">基本信息</div>
                 <div class="weights-grid">
                    <div class="weight-item">
                       <label>策略代码（只读）</label>
                       <n-input :value="editStrategy.code" disabled />
                    </div>
                    <div class="weight-item">
                       <label>策略名称</label>
                          <n-input v-model:value="editStrategy.name" placeholder="如 推理宽松" />
                    </div>
                  </div>
                  <div class="weight-item" style="margin-top:12px">
                     <label>说明</label>
                        <n-input v-model:value="editStrategy.description" type="textarea":autosize="{ minRows: 1, maxRows: 3 }" placeholder="策略说明" />
                  </div>
               </div>

                <!-- 维度权重输入区域 -->
                <div class="dimension-weights">
                  <div class="section-title">维度权重设置</div>
                  <div class="weights-grid">
                    <div class="weight-item">
                      <label>硬件健康 (hardware)</label>
                      <n-input-number
                        v-model:value="dimensionWeights.hardware"
                        :min="0"
                        :max="1"
                        :step="0.05"
                        :precision="2"
                        placeholder="0.00-1.00"
                      />
                    </div>
                    <div class="weight-item">
                      <label>运行稳定性 (stability)</label>
                      <n-input-number
                        v-model:value="dimensionWeights.stability"
                        :min="0"
                        :max="1"
                        :step="0.05"
                        :precision="2"
                        placeholder="0.00-1.00"
                      />
                    </div>
                    <div class="weight-item">
                      <label>性能表现 (performance)</label>
                      <n-input-number
                        v-model:value="dimensionWeights.performance"
                        :min="0"
                        :max="1"
                        :step="0.05"
                        :precision="2"
                        placeholder="0.00-1.00"
                      />
                    </div>
                    <div class="weight-item">
                      <label>运行环境 (environment)</label>
                      <n-input-number
                        v-model:value="dimensionWeights.environment"
                        :min="0"
                        :max="1"
                        :step="0.05"
                        :precision="2"
                        placeholder="0.00-1.00"
                      />
                    </div>
                  </div>

                  <!-- 权重和验证显示 -->
                  <div class="weight-summary">
                    <div class="weight-total">
                      权重总和: <span :class="{'valid': isWeightValid, 'invalid': !isWeightValid}">{{ weightTotal.toFixed(2) }}</span>
                      <span v-if="!isWeightValid" class="error-text"> (必须为 1.00)</span>
                    </div>
                    <div class="weight-hint">
                      提示：四个维度权重之和必须等于 1.00
                    </div>
                  </div>
                </div>
            <div class="rules-title" style="display:flex;align-items:center;justify-content:space-between;">
              <span>指标权重 / 曲线 / 一票否决</span>
              <n-button size="tiny" type="primary" ghost @click="showAddMetric = true">+ 添加指标</n-button>
            </div>
            <n-data-table :columns="ruleCols" :data="editRules" :bordered="false" size="small" :max-height="380" />
            <n-space justify="end" style="margin-top: 16px">
              <n-button @click="editStrategy = null">取消</n-button>
              <n-button type="primary" @click="saveStrategy">保存（5秒内热加载生效）</n-button>
            </n-space>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>
  </div>

  <n-modal v-model:show="showCreateStrategy" preset="card" title="新建评分策略" style="width: 760px">
    <n-form label-placement="left" label-width="110">
      <!-- 基本信息 -->
      <n-grid :cols="2" :x-gap="16">
        <n-gi>
          <n-form-item label="策略代码">
            <n-input v-model:value="newStrategy.code" placeholder="如 inference_loose" />
          </n-form-item>
        </n-gi>
        <n-gi>
          <n-form-item label="策略名称">
            <n-input v-model:value="newStrategy.name" placeholder="如 推理宽松" />
          </n-form-item>
        </n-gi>
      </n-grid>
      <n-form-item label="说明">
        <n-input v-model:value="newStrategy.description" />
      </n-form-item>

      <!-- 维度权重(四个输入框) -->
      <div class="section-title">
        维度权重 <span :class="weightSumOK ? 'sum-ok' : 'sum-bad'">(当前合计: {{ weightSum }},需为 1.0)</span>
      </div>
      <n-grid :cols="4" :x-gap="12" style="margin-bottom: 18px">
        <n-gi>
          <div class="weight-label">硬件健康</div>
          <n-input-number v-model:value="newStrategy.weight_hardware"
            :min="0" :max="1" :step="0.05" :precision="2" style="width: 100%" />
        </n-gi>
        <n-gi>
          <div class="weight-label">运行稳定性</div>
          <n-input-number v-model:value="newStrategy.weight_stability"
            :min="0" :max="1" :step="0.05" :precision="2" style="width: 100%" />
        </n-gi>
        <n-gi>
          <div class="weight-label">性能表现</div>
          <n-input-number v-model:value="newStrategy.weight_performance"
            :min="0" :max="1" :step="0.05" :precision="2" style="width: 100%" />
        </n-gi>
        <n-gi>
          <div class="weight-label">运行环境</div>
          <n-input-number v-model:value="newStrategy.weight_environment"
            :min="0" :max="1" :step="0.05" :precision="2" style="width: 100%" />
        </n-gi>
      </n-grid>

      <!-- 参与计算的指标(按维度分组,可勾选,可调权重) -->
      <div class="section-title">参与计算的指标 (默认已加载默认策略的规则,可勾选/调权重)</div>
      <div class="metric-groups">
        <div v-for="(rules, dim) in groupedRules" :key="dim" class="metric-group">
          <div class="group-title">{{ dimNameMap[dim] }} ({{ rules.filter(r => r.enabled).length }}/{{ rules.length }})</div>
          <div v-for="r in rules" :key="r.metric_key" class="metric-row">
            <n-checkbox
              :checked="r.enabled"
              @update:checked="(val) => onRuleEnabledChange(r.metric_key, val)">
              <span class="metric-name">{{ r._displayName }}</span>
              <span class="metric-key">{{ r.metric_key }}</span>
            </n-checkbox>
            <n-input-number
               :value="r.weight"
               @update:value="(val) => onRuleWeightChange(r.metric_key, val)"
               :min="0" :step="0.5" :precision="2" size="small"
               style="width: 100px" :disabled="!r.enabled" />
            <span class="curve-tag">{{ r.curve_type }}<span v-if="r.is_veto" class="veto-tag">否决</span></span>
          </div>
        </div>
      </div>
    </n-form>
    <template #footer>
      <n-space justify="end">
        <n-button @click="showCreateStrategy = false">取消</n-button>
        <n-button type="primary" :disabled="!weightSumOK" @click="doCreateStrategy">创建</n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal v-model:show="showAssign" preset="card" :title="assignType === 'cluster' ? '为集群分配评分策略' : '为单卡分配评分策略'" style="width: 480px">
    <div style="margin-bottom: 12px; color: var(--text-1); font-size: 13px;">
      <span v-if="assignType === 'cluster'">
        目标集群: <strong>{{ assignTarget?.cluster_name }}</strong>
      </span>
      <span v-else>
        目标 GPU: <strong class="mono">{{ assignTarget?.gpu_uuid }}</strong>
      </span>
    </div>
    <n-select v-model:value="assignStrategyId"
      :options="strategyOptions"
      placeholder="选择策略(清空 = 恢复默认/解绑)"
      clearable />
    <div style="margin-top: 10px; font-size: 12px; color: var(--text-2)">
      优先级:卡级 &gt; 集群级 &gt; 全局默认
    </div>
    <template #footer>
      <n-space justify="end">
        <n-button @click="showAssign = false">取消</n-button>
        <n-button type="primary" @click="doAssign">确定</n-button>
      </n-space>
    </template>
  </n-modal>

  <!-- 添加指标到当前策略 -->
  <n-modal v-model:show="showAddMetric" preset="card" title="添加指标到当前策略" style="width: 480px">
    <div style="margin-bottom:12px;font-size:13px;color:var(--text-2)">
      选择要加入本策略的指标，初始权重为 1.0，曲线为 none，可加入后再编辑。
    </div>
    <n-select
      v-model:value="addMetricKey"
      :options="addableMetricOptions"
      filterable
      placeholder="搜索或选择指标..."
      style="margin-bottom:16px"
    />
    <template #footer>
      <n-space justify="end">
        <n-button @click="showAddMetric = false">取消</n-button>
        <n-button type="primary" :disabled="!addMetricKey" @click="doAddMetric">添加</n-button>
      </n-space>
    </template>
  </n-modal>
  <!-- 单卡异常详情抽屉(需求1) -->
  <n-drawer v-model:show="detailShow" :width="520">
    <n-drawer-content :title="detailSnap ? ('单卡详情 · ' + (levelNames[detailSnap.level] || detailSnap.level)) : '单卡详情'" closable>
      <n-spin :show="detailLoading">
        <div v-if="detailSnap">
          <div class="gd-head">
            <div class="gd-uuid mono">{{ detailSnap.gpu_uuid }}</div>
            <div class="gd-score">
              总分 <b :style="`color:${scoreColor(detailSnap.score)}`">{{ detailSnap.score.toFixed(1) }}</b>
              <span :class="`level-badge lv-${detailSnap.level}`" style="margin-left:8px">{{ levelNames[detailSnap.level] || detailSnap.level }}</span>
              <span v-if="detailSnap.veto" style="color:#ef4444;font-weight:600;margin-left:8px">VETO: {{ detailSnap.veto_reason }}</span>
            </div>
          </div>

          <template v-if="detailSnap.level === 'healthy'">
            <div class="gd-empty">该卡处于健康状态，各项指标正常。</div>
          </template>
          <template v-else>
            <div class="gd-section">不正常的指标（{{ detailAbnormal.length }}）</div>
            <div v-if="detailAbnormal.length === 0" class="gd-empty">未解析到异常指标明细。</div>
            <table v-else class="gd-table">
              <thead>
                <tr><th>指标</th><th>当前值</th><th>阈值/正常区间</th><th>得分</th><th>严重度</th></tr>
              </thead>
              <tbody>
                <tr v-for="m in detailAbnormal" :key="m.metric_key">
                  <td>
                    <div>{{ m.display_name }}</div>
                    <div class="mono gd-key">{{ m.metric_key }}</div>
                  </td>
                  <td class="mono">{{ fmtNum(m.value) }}<span v-if="m.unit"> {{ m.unit }}</span></td>
                  <td class="gd-thr">{{ thresholdText(m) }}</td>
                  <td class="mono" :style="`color:${abnSevColor(m.severity)};font-weight:600`">{{ m.score.toFixed(1) }}</td>
                  <td :style="`color:${abnSevColor(m.severity)}`">{{ m.severity === 'critical' ? '严重' : '告警' }}</td>
                </tr>
              </tbody>
            </table>

            <div class="gd-section" style="margin-top:18px">当前故障（{{ detailFaults.length }}）</div>
            <div v-if="detailFaults.length === 0" class="gd-empty">暂无进行中的故障事件。</div>
            <div v-else class="gd-faults">
              <div v-for="f in detailFaults" :key="f.id" class="gd-fault">
                <span class="gd-fname" :style="`color:${f.severity === 'warning' ? '#eab308' : '#ef4444'}`">{{ f.fault_name }}</span>
                <span class="mono gd-key">{{ f.metric_display || f.metric_key || '' }}</span>
              </div>
            </div>
          </template>
        </div>
      </n-spin>
    </n-drawer-content>
  </n-drawer>

</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, useDialog, NButton, NInput, NInputNumber, NSwitch, NSelect } from "naive-ui";
const dialog = useDialog();

const message = useMessage();
const tab = ref("scores");

// ---- 集群 → 单卡 ----
const clusters = ref<any[]>([]);
const activeCluster = ref<any>(null);
const gpus = ref<any[]>([]);
const page = ref(1);
const pageSize = 50;
const total = ref(0);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

function scoreColor(s: number) {
  if (s >= 90) return "#22c55e";
  if (s >= 75) return "#84cc16";
  if (s >= 60) return "#eab308";
  if (s >= 30) return "#f97316";
  return "#ef4444";
}
const levelNames: Record<string, string> = {
  healthy: "健康", sub_healthy: "亚健康", warning: "警告", critical: "严重", failed: "故障"
};

// ---- 搜索 ----
const healthSearchKeyword = ref("");
const healthSearchResults = ref<any[]>([]);

async function doHealthSearch() {
  const q = healthSearchKeyword.value.trim();
  if (!q) { message.warning("请输入搜索关键词"); return; }
  try {
    const list = await api.healthSearch(q);
    healthSearchResults.value = list || [];
    if (!healthSearchResults.value.length) {
      message.info("未找到匹配的 GPU 评分数据");
    }
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "搜索失败");
  }
}

function clearHealthSearch() {
  healthSearchKeyword.value = "";
  healthSearchResults.value = [];
}

const clusterCols = [
  { title: "集群", key: "cluster_name", width: 140 },
  { title: "编号", key: "cluster_code", width: 130,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.cluster_code) },
  { title: "GPU 数", key: "total_gpu", width: 90,
    render: (r: any) => h("span", { class: "mono" }, r.total_gpu) },
  { title: "平均分", key: "avg_score", width: 100,
    render: (r: any) => h("span", { class: "mono", style: `color:${scoreColor(r.avg_score)};font-weight:600` }, r.avg_score.toFixed(1)) },
  { title: "健康", key: "healthy_cnt", width: 70, render: (r: any) => h("span", { style: "color:#22c55e" }, r.healthy_cnt) },
  { title: "亚健康", key: "sub_healthy_cnt", width: 70, render: (r: any) => h("span", { style: "color:#84cc16" }, r.sub_healthy_cnt) },
  { title: "警告", key: "warning_cnt", width: 70, render: (r: any) => h("span", { style: "color:#eab308" }, r.warning_cnt) },
  { title: "严重", key: "critical_cnt", width: 70, render: (r: any) => h("span", { style: "color:#f97316" }, r.critical_cnt) },
  { title: "故障", key: "failed_cnt", width: 70, render: (r: any) => h("span", { style: "color:#ef4444" }, r.failed_cnt) },
  //{ title: "当前评分策略", key: "bound_strategy_id", width: 140,
  //  render: (r: any) => {
  //    const sid = r.bound_strategy_id;
  //    if (!sid) return h("span", { style: "color:var(--text-2);font-size:12px" }, "默认");
  //    const s = strategies.value.find((x: any) => x.id === sid);
  //    return h("span", { style: "font-size:12px;color:var(--accent)" }, s ? s.name : `策略#${sid}`);
  //  }
  //},
  //{ title: "评分策略", key: "strategy", width: 160,render: (r: any) => h(NButton, { size: "tiny", onClick: (e: any) => { e.stopPropagation(); openAssignCluster(r); } },() => "分配策略") }
];

function clusterRowProps(row: any) {
  return {
    style: "cursor: pointer",
    onClick: () => {
          if (activeCluster.value && activeCluster.value.cluster_id === row.cluster_id) {
            // 如果点击的是当前已展开的集群，则收起
            activeCluster.value = null;
          } else {
            // 否则展开新的集群
            activeCluster.value = row;
            page.value = 1;
            loadGPUs();
          }
        }
  };
}

const gpuCols = [
  { title: "GPU UUID", key: "gpu_uuid",
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px;color:#9aa7b4" }, r.gpu_uuid) },
  { title: "等级", key: "level", width: 110,
    render: (r: any) => h("span", { class: `level-badge lv-${r.level}` }, levelNames[r.level] || r.level) },
  { title: "总分", key: "score", width: 90,
    render: (r: any) => h("span", { class: "mono", style: `color:${scoreColor(r.score)};font-weight:600` }, r.score.toFixed(1)) },
  { title: "否决", key: "veto", width: 80,
    render: (r: any) => r.veto ? h("span", { style: "color:#ef4444;font-weight:600" }, "VETO") : h("span", { style: "color:#5e6b78" }, "—") },
  { title: "否决原因", key: "veto_reason", width: 200,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.veto_reason || "") },
  { title: "评分策略", key: "strategy_id", width: 120,
    render: (r: any) => {
      const sid = r.strategy_id;
      const s = strategies.value.find((x: any) => x.id === sid);
      const label = s ? s.name : "默认";
      return h("span", { style: "font-size:12px;color:var(--text-1)" }, label);
    }
  },
  { title: "操作", key: "ops", width: 170,
    render: (r: any) => h("div", { style: "display:flex;gap:6px" }, [
      h(NButton, { size: "tiny", type: r.level === "healthy" ? "default" : "warning",
        onClick: () => openGPUDetail(r) }, () => "异常详情"),
      h(NButton, { size: "tiny", onClick: () => openAssignGPU(r) }, () => "分配策略"),
    ]) }
];

async function loadClusters() {
  clusters.value = await api.healthClusters();
}
async function loadGPUs() {
  if (!activeCluster.value) return;
  const res = await api.healthClusterGPUs(activeCluster.value.cluster_id, pageSize, (page.value - 1) * pageSize);
  gpus.value = res.items || [];
  total.value = res.total || 0;
}

// ---- 单卡异常详情(需求1) ----
const detailShow = ref(false);
const detailLoading = ref(false);
const detailSnap = ref<any>(null);
const detailAbnormal = ref<any[]>([]);
const detailFaults = ref<any[]>([]);

async function openGPUDetail(row: any) {
  detailShow.value = true;
  detailLoading.value = true;
  detailSnap.value = null;
  detailAbnormal.value = [];
  detailFaults.value = [];
  try {
    const res = await api.healthGPUDetail(row.gpu_uuid);
    detailSnap.value = res.snapshot;
    detailAbnormal.value = res.abnormal || [];
    detailFaults.value = res.faults || [];
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "加载详情失败");
  } finally {
    detailLoading.value = false;
  }
}

function abnSevColor(s: string) {
  return s === "critical" ? "#ef4444" : "#eab308";
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

// ---- 策略管理 ----
const strategies = ref<any[]>([]);
const editStrategy = ref<any>(null);
const editRules = ref<any[]>([]);
const dimWeightsText = ref("");

// 从数据库拉取的全量指标（用于新增规则时选择）
const allMetrics = ref<any[]>([]);

// 新增指标弹窗
const showAddMetric = ref(false);
const addMetricKey = ref<string | null>(null);

// 添加维度权重状态
const dimensionWeights = ref({
  hardware: 0.45,
  stability: 0.25,
  performance: 0.20,
  environment: 0.10
});

//集群分配
const showAssign = ref(false);
const assignType = ref<"cluster" | "gpu">("cluster");
const assignTarget = ref<any>(null);
const assignStrategyId = ref<number | null>(null);

const strategyOptions = computed(() =>
  strategies.value.map((s: any) => ({
    label: `${s.name} (${s.code})${s.is_default ? ' [默认]' : ''}`,
    value: s.id
  }))
);

function openAssignCluster(cluster: any) {
  assignType.value = "cluster";
  assignTarget.value = cluster;
  // 显示当前已绑的策略(如果有)
  assignStrategyId.value = cluster.bound_strategy_id || null;
  showAssign.value = true;
}

function openAssignGPU(gpu: any) {
  assignType.value = "gpu";
  assignTarget.value = gpu;
  assignStrategyId.value = gpu.strategy_id || null;
  showAssign.value = true;
}

async function doAssign() {
  try {
    if (assignType.value === "cluster") {
      await api.bindClusterStrategy(assignTarget.value.cluster_id, assignStrategyId.value);
    } else {
      // 单卡绑定:用 gpu_uuid
      await api.bindGPUStrategy(assignTarget.value.gpu_uuid, assignStrategyId.value);
    }
    message.success(assignStrategyId.value
      ? "已分配,下个评分周期(≤1分钟)生效"
      : "已解绑,将恢复默认策略");
    showAssign.value = false;
    // 刷新数据
    if (assignType.value === "cluster") {
      await loadClusters();
    } else {
      await loadGPUs();
    }
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "操作失败");
  }
}

// 计算权重总和
const weightTotal = computed(() => {
  return Object.values(dimensionWeights.value).reduce((sum, weight) => sum + weight, 0);
});

// 验证权重是否有效
const isWeightValid = computed(() => {
  return Math.abs(weightTotal.value - 1.0) < 0.001; // 允许0.001的浮点误差
});

const strategyCols = [
  { title: "代码", key: "code", width: 160,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.code) },
  { title: "名称", key: "name", width: 180 },
  { title: "默认", key: "is_default", width: 70,
    render: (r: any) => r.is_default ? h("span", { style: "color:#38bdf8" }, "✓") : "" },
  { title: "说明", key: "description" },
  { title: "操作", key: "ops", width: 90,
    render: (r: any) => h(NButton, { size: "tiny", onClick: () => openEditStrategy(r) }, () => "编辑") }
];

const curveOptions = [
  { label: "none", value: "none" }, { label: "piecewise", value: "piecewise" },
  { label: "log", value: "log" }, { label: "xid_table", value: "xid_table" }, { label: "veto", value: "veto" }
];

const ruleCols = [
  { title: "指标", key: "metric_key",
    render: (r: any) => h("span", { class: "mono", style: "font-size:11px;color:#9aa7b4" }, r.metric_key) },
  { title: "权重", key: "weight", width: 110,
    render: (r: any) => h(NInputNumber, {
      value: r.weight, size: "tiny", step: 0.5, min: 0,
      "onUpdate:value": (v: number) => (r.weight = v ?? 0)
    }) },
  { title: "曲线", key: "curve_type", width: 130,
    render: (r: any) => h(NSelect, {
      value: r.curve_type, size: "tiny", options: curveOptions,
      "onUpdate:value": (v: string) => (r.curve_type = v)
    }) },
  { title: "一票否决", key: "is_veto", width: 90,
    render: (r: any) => h(NSwitch, {
      value: r.is_veto, size: "small",
      "onUpdate:value": (v: boolean) => (r.is_veto = v)
    }) },
  { title: "否决阈值", key: "veto_threshold", width: 110,
    render: (r: any) => h(NInputNumber, {
      value: r.veto_threshold, size: "tiny", min: 0,
      "onUpdate:value": (v: number) => (r.veto_threshold = v ?? 0)
    }) },
  { title: "移除", key: "remove", width: 70,
      render: (r: any) => h(NButton, {
        size: "tiny", type: "error", ghost: true,
        onClick: () => { editRules.value = editRules.value.filter((x: any) => x.metric_key !== r.metric_key); }
      }, () => "移除") }

];

async function loadStrategies() {
  strategies.value = await api.strategies();
}

// 可添加的指标 = 全量指标里，当前策略还没有的那些
const addableMetricOptions = computed(() => {
  const existingKeys = new Set(editRules.value.map((r: any) => r.metric_key));
  return allMetrics.value
    .filter((m: any) => !existingKeys.has(m.metric_key))
    .map((m: any) => ({
      label: `${m.display_name}（${m.metric_key}）`,
      value: m.metric_key,
    }));
});

function doAddMetric() {
  if (!addMetricKey.value) return;
  const metric = allMetrics.value.find((m: any) => m.metric_key === addMetricKey.value);
  if (!metric) return;
  editRules.value.push({
    id: 0,
    strategy_id: editStrategy.value.id,
    metric_key: metric.metric_key,
    weight: 1.0,
    curve_type: "none",
    curve_params: null,
    is_veto: false,
    veto_threshold: 0,
  });
  addMetricKey.value = null;
  showAddMetric.value = false;
}

async function openEditStrategy(r: any) {
  const full = await api.strategy(r.id);
  editStrategy.value = full;
  editRules.value = (full.rules || []).map((x: any) => ({ ...x }));

  // 解析维度权重到独立字段
  try {
    const parsedWeights = JSON.parse(full.dimension_weights);
    dimensionWeights.value = {
      hardware: parsedWeights.hardware || 0,
      stability: parsedWeights.stability || 0,
      performance: parsedWeights.performance || 0,
      environment: parsedWeights.environment || 0
    };
  } catch (e) {
    dimensionWeights.value = {
      hardware: 0.45,
      stability: 0.25,
      performance: 0.20,
      environment: 0.10
    };
  }

  // 新增：拉取全量指标，用于"添加指标"下拉
  const res = await api.metrics({ is_health_key: true, limit: 200 });
  allMetrics.value = res.items || [];
}

async function saveStrategy() {
  try {
    // 构建维度权重JSON
    const dimensionWeightsJson = JSON.stringify({
      hardware: parseFloat(dimensionWeights.value.hardware.toFixed(2)),
      stability: parseFloat(dimensionWeights.value.stability.toFixed(2)),
      performance: parseFloat(dimensionWeights.value.performance.toFixed(2)),
      environment: parseFloat(dimensionWeights.value.environment.toFixed(2))
    });

    await api.updateStrategyMeta(editStrategy.value.id, {
      name: editStrategy.value.name,
      description: editStrategy.value.description,
      dimension_weights: dimensionWeightsJson
    });

    // 2. 指标规则
    const processedRules = editRules.value.map(rule => {
      // 手动构建每个字段，确保格式正确
      const newRule = {
        id: rule.id || 0,
        strategy_id: editStrategy.value.id,
        metric_key: rule.metric_key || rule.metricKey,
        weight: parseFloat(rule.weight) || 0,
        curve_type: rule.curve_type || rule.curveType || 'none',
        is_veto: Boolean(rule.is_veto || rule.isVeto),
        veto_threshold: parseFloat(rule.veto_threshold || rule.vetoThreshold) || 0
      };

      // 关键修复：确保curve_params永远不会是空字符串
      const originalParams = rule.curve_params || rule.curveParams;
      if (!originalParams || originalParams === "" || originalParams === "null") {
        newRule.curve_params = null;
      } else {
        newRule.curve_params = originalParams;
      }

      return newRule;
    });

    // 添加前端调试
    console.log('即将发送的规则数据:', JSON.stringify(processedRules, null, 2));

    await api.updateStrategyRules(editStrategy.value.id, processedRules);
    message.success("策略已保存，评分服务将在 5 秒内热加载");
    editStrategy.value = null;
    await loadStrategies();
  } catch (e: any) {
    console.error('保存失败:', e);
    message.error("保存失败：" + (e.message || "未知错误"));
  }
}

onMounted(() => { loadClusters(); loadStrategies(); });

const showCreateStrategy = ref(false);
const newStrategy = ref<any>({
  code: "",
  name: "",
  description: "",
  // 维度权重用四个数字字段
  weight_hardware: 0.45,
  weight_stability: 0.25,
  weight_performance: 0.20,
  weight_environment: 0.10,
  // 指标规则列表(从默认策略加载并允许编辑)
  metricRules: [] as any[]
});

// 维度的中文名
const dimNameMap: Record<string, string> = {
  hardware: "硬件健康",
  stability: "运行稳定性",
  performance: "性能表现",
  environment: "运行环境"
};

async function openCreateStrategy() {
  console.log('1. openCreateStrategy 开始执行');
  try {
    if (allMetrics.value.length === 0) {
      console.log('2. 开始加载指标定义');
      const res = await api.metrics({ is_health_key: true, limit: 200 });
      allMetrics.value = res.items || [];
      console.log('3. 指标定义加载完成:', allMetrics.value.length);
    }
    const defStrategy = strategies.value.find((s: any) => s.is_default);
    console.log('4. 默认策略:', defStrategy);
    let defaultRules: any[] = [];
    if (defStrategy) {
      console.log('5. 开始加载策略详情');
      const full = await api.strategy(defStrategy.id);
      console.log('6. 策略详情加载完成:', full);
      defaultRules = (full.rules || []).map((r: any) => ({
        metric_key: r.metric_key,
        weight: r.weight,
        curve_type: r.curve_type,
        curve_params: r.curve_params,
        is_veto: r.is_veto,
        veto_threshold: r.veto_threshold,
        enabled: true
      }));
    }
    newStrategy.value = {
      code: "", name: "", description: "",
      weight_hardware: 0.45, weight_stability: 0.25,
      weight_performance: 0.20, weight_environment: 0.10,
      metricRules: defaultRules
    };
    console.log('7. 准备显示弹窗, showCreateStrategy 当前值:', showCreateStrategy.value);
    showCreateStrategy.value = true;
    console.log('8. showCreateStrategy 已设置为 true');
  } catch (e: any) {
    console.error('打开创建策略失败:', e);
    message.error('加载数据失败，请刷新后重试');
  }
}


// 添加事件处理方法
function onRuleEnabledChange(ruleKey: string, enabled: boolean) {
  const ruleIndex = newStrategy.value.metricRules.findIndex(r => r.metric_key === ruleKey);
  if (ruleIndex !== -1) {
    // 直接修改源数据
    newStrategy.value.metricRules[ruleIndex].enabled = enabled;
    // 强制触发响应式更新
    newStrategy.value.metricRules = [...newStrategy.value.metricRules];
  }
}

function onRuleWeightChange(ruleKey: string, weight: number) {
  const ruleIndex = newStrategy.value.metricRules.findIndex(r => r.metric_key === ruleKey);
  if (ruleIndex !== -1) {
    // 直接修改源数据
    newStrategy.value.metricRules[ruleIndex].weight = weight;
    // 强制触发响应式更新
    newStrategy.value.metricRules = [...newStrategy.value.metricRules];
  }
}

// 维度权重和(用 computed 实时显示)
const weightSum = computed(() => {
  const w = newStrategy.value;
  return Number(((w.weight_hardware || 0) + (w.weight_stability || 0)
    + (w.weight_performance || 0) + (w.weight_environment || 0)).toFixed(4));
});
const weightSumOK = computed(() => Math.abs(weightSum.value - 1) < 0.001);

async function doCreateStrategy() {
  const f = newStrategy.value;
  if (!f.code) { message.warning("策略代码必填"); return; }
  if (!f.name) { message.warning("策略名称必填"); return; }
  if (!weightSumOK.value) {
    message.error(`四个维度权重之和必须为 1.0,当前为 ${weightSum.value}`);
    return;
  }
  // 至少要勾一个指标
  const selected = f.metricRules.filter((r: any) => r.enabled);
  if (selected.length === 0) {
    message.warning("请至少选择一个参与计算的指标");
    return;
  }

  // 组装维度权重 JSON
  const dimWeights = JSON.stringify({
    hardware: f.weight_hardware,
    stability: f.weight_stability,
    performance: f.weight_performance,
    environment: f.weight_environment
  });

  // 组装规则(剥掉 enabled 字段)
  const rules = selected.map((r: any) => ({
    metric_key: r.metric_key,
    weight: r.weight,
    curve_type: r.curve_type,
    curve_params: r.curve_params,
    is_veto: r.is_veto,
    veto_threshold: r.veto_threshold
  }));

  try {
    await api.createStrategy({
      code: f.code,
      name: f.name,
      description: f.description,
      dimension_weights: dimWeights,
      rules: rules
    });
    message.success(`策略已创建,包含 ${rules.length} 个指标规则`);
    showCreateStrategy.value = false;
    await loadStrategies();
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "创建失败");
  }
}

const groupedRules = computed(() => {
  const groups: Record<string, any[]> = {
    hardware: [], stability: [], performance: [], environment: []
  };

  //确保数据存在
  if (!Array.isArray(allMetrics.value) || allMetrics.value.length === 0) {
    console.warn('指标定义未加载');
    return groups;
  }

  if (!newStrategy.value || !newStrategy.value.metricRules) {
      console.warn('策略规则未初始化');
      return groups;
  }

  // 用 allMetrics 拿到每个指标的维度和显示名
  const metricMeta: Record<string, any> = {};
  for (const m of allMetrics.value) {
    metricMeta[m.metric_key] = m;
  }

  for (const r of newStrategy.value.metricRules) {
    const meta = metricMeta[r.metric_key];
    if (!meta) {
      console.warn(`找不到指标 ${r.metric_key} 的元数据`);
      continue;
    }

    const dim = meta.dimension;
    if (groups[dim]) {
      // 创建深拷贝，确保响应式追踪
      const ruleCopy = {
        ...r,
        _displayName: meta.display_name || r.metric_key,
        _unit: meta.unit || ''
      };
      groups[dim].push(ruleCopy);
    }
  }

  return groups;
});

async function deleteStrategy(row: any) {
  dialog.warning({
    title: "确认删除", content: `删除策略「${row.name}」?`,
    positiveText: "删除", negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.deleteStrategy(row.id);
        message.success("已删除");
        await loadStrategies();
      } catch (e: any) {
        message.error(e?.response?.data?.msg || "删除失败");
      }
    }
  });
}

</script>

<style scoped>
.toolbar { display: flex; align-items: center; gap: 16px; margin: 12px 0 16px; }
.hint { font-size: 12px; color: var(--text-2); }
.pager { display: flex; justify-content: flex-end; padding: 14px 16px; }
.rules-title { font-size: 12px; color: var(--text-1); margin: 16px 0 8px; letter-spacing: 0.05em; }
.basic-info{
  margin-bottom: 18px;
  padding: 16px;
  background: var(--bg-2);
  border-radius: 6px;
}
.dimension-weights {
  margin-bottom: 24px;
  padding: 16px;
  background: var(--bg-2);
  border-radius: 6px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-1);
  margin-bottom: 12px;
}

.weights-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}

.weight-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.weight-item label {
  font-size: 12px;
  color: var(--text-2);
}

.weight-summary {
  padding: 12px;
  background: var(--bg-1);
  border-radius: 4px;
  border: 1px solid var(--border);
}

.weight-total {
  font-size: 13px;
  color: var(--text-1);
}

.weight-total .valid {
  color: var(--lv-healthy);
  font-weight: 600;
}

.weight-total .invalid {
  color: var(--lv-failed);
  font-weight: 600;
}

.error-text {
  color: var(--lv-failed);
  font-size: 12px;
}

.weight-hint {
  font-size: 11px;
  color: var(--text-2);
  margin-top: 4px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-1);
  margin: 16px 0 10px;
  letter-spacing: 0.04em;
}

.sum-ok { color: #22c55e; font-weight: 400; font-size: 12px; }

.sum-bad { color: #ef4444; font-weight: 400; font-size: 12px; }

.weight-label { font-size: 12px; color: var(--text-2); margin-bottom: 6px; }

.metric-groups {
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px 12px;
}

.metric-group { margin-bottom: 14px; }

.group-title {
  font-size: 12px; color: var(--accent); font-weight: 600;
  padding: 6px 0; border-bottom: 1px solid var(--bg-3); margin-bottom: 6px;
}

.metric-row {
  display: flex; align-items: center; gap: 10px;
  padding: 4px 0;
}

.metric-row .metric-name { font-size: 13px; margin-left: 4px; }

.metric-row .metric-key {
  font-family: var(--font-mono); font-size: 11px;
  color: var(--text-2); margin-left: 6px;
}

.curve-tag {
  font-size: 11px; color: var(--text-2); margin-left: auto;
  font-family: var(--font-mono);
}

.veto-tag {
  background: rgba(239,68,68,0.15); color: #ef4444;
  padding: 1px 6px; border-radius: 3px; margin-left: 6px;
}

/* 单卡异常详情抽屉 */
.gd-head { margin-bottom: 16px; }
.gd-uuid { font-size: 12px; color: #9aa7b4; word-break: break-all; }
.gd-score { font-size: 14px; margin-top: 6px; }
.gd-section { font-size: 13px; color: var(--text-1); font-weight: 600; margin: 8px 0; letter-spacing: 0.04em; }
.gd-empty { font-size: 13px; color: var(--text-2); padding: 10px 0; }
.gd-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.gd-table th, .gd-table td { text-align: left; padding: 7px 8px; border-bottom: 1px solid var(--border); vertical-align: top; }
.gd-table th { color: var(--text-2); font-weight: 500; }
.gd-key { font-size: 11px; color: var(--text-2); }
.gd-thr { color: var(--text-2); }
.gd-faults { display: flex; flex-direction: column; gap: 6px; }
.gd-fault { display: flex; align-items: center; gap: 10px; padding: 6px 8px; background: var(--bg-2); border-radius: 4px; }
.gd-fname { font-size: 13px; font-weight: 600; }

</style>
