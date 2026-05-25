<template>
  <div>
    <n-tabs type="line" v-model:value="tab">
      <!-- 集群 → 单卡 -->
      <n-tab-pane name="scores" tab="集群健康度">
        <div class="toolbar">
          <n-button size="small" @click="loadClusters">刷新</n-button>
          <span class="hint">点击集群行查看该集群内每张 GPU 的评分</span>
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
        </div>
        <div class="panel">
          <div class="panel-title">策略列表</div>
          <n-data-table :columns="strategyCols" :data="strategies" :bordered="false" size="small" />
        </div>

        <div class="panel" v-if="editStrategy" style="margin-top: 16px">
          <div class="panel-title">编辑策略：{{ editStrategy.name }}（版本 {{ editStrategy.version }}）</div>
          <div style="padding: 16px">
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
            <div class="rules-title">指标权重 / 曲线 / 一票否决</div>
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
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, NButton, NInput, NInputNumber, NSwitch, NSelect } from "naive-ui";

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
  { title: "故障", key: "failed_cnt", width: 70, render: (r: any) => h("span", { style: "color:#ef4444" }, r.failed_cnt) }
];

function clusterRowProps(row: any) {
  return {
    style: "cursor: pointer",
    onClick: () => { activeCluster.value = row; page.value = 1; loadGPUs(); }
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
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.veto_reason || "") }
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

// ---- 策略管理 ----
const strategies = ref<any[]>([]);
const editStrategy = ref<any>(null);
const editRules = ref<any[]>([]);
const dimWeightsText = ref("");

// 添加维度权重状态
const dimensionWeights = ref({
  hardware: 0.45,
  stability: 0.25,
  performance: 0.20,
  environment: 0.10
});

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
  { title: "版本", key: "version", width: 70, render: (r: any) => h("span", { class: "mono" }, r.version) },
  { title: "说明", key: "description" },
  { title: "操作", key: "ops", width: 90,
    render: (r: any) => h(NButton, { size: "tiny", onClick: () => openEditStrategy(r) }, () => "编辑权重") }
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
    }) }
];

async function loadStrategies() {
  strategies.value = await api.strategies();
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
    // 如果解析失败，使用默认值
    dimensionWeights.value = {
      hardware: 0.45,
      stability: 0.25,
      performance: 0.20,
      environment: 0.10
    };
  }
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
    // 创建新对象，确保curve_params正确处理
    const processedRule = {
      ...rule,
      strategy_id: editStrategy.value.id,
      // 关键修复：将空字符串转换为null
      curve_params: (!rule.curve_params || rule.curve_params === "") ? null : rule.curve_params
    };

    return processedRule;
  });

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
</script>

<style scoped>
.toolbar { display: flex; align-items: center; gap: 16px; margin: 12px 0 16px; }
.hint { font-size: 12px; color: var(--text-2); }
.pager { display: flex; justify-content: flex-end; padding: 14px 16px; }
.rules-title { font-size: 12px; color: var(--text-1); margin: 16px 0 8px; letter-spacing: 0.05em; }
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

</style>
