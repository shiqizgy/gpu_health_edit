<template>
  <div>
    <div class="toolbar">
      <n-space>
        <n-input v-model:value="keyword" placeholder="搜索指标名称/官方编号" clearable
          style="width: 220px" @keydown.enter="reload" @clear="reload" />
        <n-select v-model:value="dimFilter" :options="dimOptions" placeholder="按维度筛选"
          clearable style="width: 170px" @update:value="reload" />
        <n-select v-model:value="vtFilter" :options="vtOptions" placeholder="按数值类型筛选"
          clearable style="width: 160px" @update:value="reload" />
        <n-select v-model:value="ownerFilter" :options="ownerOptions" placeholder="按归属主体筛选"
          clearable style="width: 150px" @update:value="reload" />
        <n-select v-model:value="purposeFilter" :options="purposeOptions" placeholder="按健康度用途筛选"
          clearable style="width: 150px" @update:value="reload" />
        <n-checkbox v-model:checked="healthKeyOnly" @update:checked="reload">仅参与评分</n-checkbox>
        <n-button @click="reload">搜索</n-button>
        <n-button type="primary" @click="openCreate">+ 新建指标</n-button>
      </n-space>
    </div>

    <div class="panel">
      <n-data-table :columns="columns" :data="rows" :bordered="false" :max-height="620" size="small" />
      <div class="pager">
        <n-pagination v-model:page="page" :page-count="pageCount" :page-size="pageSize" @update:page="load" />
      </div>
    </div>

    <n-modal v-model:show="showModal" preset="card" :title="editing ? '编辑指标' : '新建指标'" style="width: 760px">
      <n-form :model="form" label-placement="left" label-width="90">
        <n-grid :cols="3" :x-gap="12">
          <n-gi>
            <n-form-item label="指标名称">
              <n-input v-model:value="form.metric_name" placeholder="DCGM_FI_DEV_GPU_TEMP" :disabled="editing" />
            </n-form-item>
          </n-gi>
          <n-gi :span="2">
            <n-form-item label="官方编号">
              <n-input v-model:value="form.official_num" placeholder="DCGM_FI_DEV_GPU_TEMP (150)" />
            </n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="卡的类型">
              <n-select v-model:value="form.card_type" :options="[
                { label: 'GPU', value: 'GPU' }, { label: 'NPU', value: 'NPU' }]" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="数值类型">
              <n-select v-model:value="form.value_type" :options="vtOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="维度">
              <n-select v-model:value="form.dimension" :options="dimOptions" filterable tag />
            </n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="归属主体">
              <n-select v-model:value="form.owner_subject" :options="ownerOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="健康度用途">
              <n-select v-model:value="form.health_purpose" :options="purposeOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="一票否决">
              <n-select v-model:value="form.is_veto" :options="[
                { label: '是', value: 1 }, { label: '否', value: 0 }]" />
            </n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="单位"><n-input v-model:value="form.unit" placeholder="℃ / W / MB" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="工作范围"><n-input v-model:value="form.work_range" placeholder="20~85" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="厂商"><n-input v-model:value="form.vendor" placeholder="NVIDIA / 华为昇腾" /></n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="正常下界"><n-input-number v-model:value="form.lower_bound" style="width:100%" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="正常上界"><n-input-number v-model:value="form.upper_bond" style="width:100%" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="告警下界"><n-input-number v-model:value="form.warn_lowbound" style="width:100%" /></n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="告警上界"><n-input-number v-model:value="form.warn_upbound" style="width:100%" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="正常速率"><n-input-number v-model:value="form.normal_rate" style="width:100%" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="告警速率"><n-input-number v-model:value="form.warn_rate" style="width:100%" /></n-form-item>
          </n-gi>

          <n-gi>
            <n-form-item label="速率单位"><n-input v-model:value="form.normal_rate_unit" placeholder="次/天、μs/s" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="布尔正常"><n-input v-model:value="form.bool_normal" placeholder="0 / OK" /></n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="布尔异常"><n-input v-model:value="form.bool_abnormal" placeholder="1 / Fail" /></n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="概念说明">
          <n-input v-model:value="form.concept" type="textarea" :autosize="{ minRows: 2 }" />
        </n-form-item>
        <n-form-item label="枚举结果">
          <n-input v-model:value="form.enum_result" type="textarea" :autosize="{ minRows: 2 }" />
        </n-form-item>
        <n-form-item label="降频阈值">
          <n-input v-model:value="form.derate_threshold" />
        </n-form-item>
        <n-form-item label="来源依据">
          <n-input v-model:value="form.source_ref" />
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="form.remark" />
        </n-form-item>
        <n-form-item label="参与评分">
          <n-switch v-model:value="form.is_health_key" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" @click="save">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, computed, onMounted, watch } from "vue";
import { useRoute } from "vue-router";
import { api } from "@/api";
import { useMessage, useDialog, NButton, NSpace, NTag } from "naive-ui";

const message = useMessage();
const dialog = useDialog();
const route = useRoute();
const pageCardType = computed(() => (route.meta.cardType as string) || "GPU");

// —— 维度（新表 dimension 实际值，来自种子数据）——
const gpuDims = [
  { label: "显存可靠性",     value: "memory显存可靠性" },
  { label: "算力性能",       value: "compute算力性能" },
  { label: "NVLink片间互连", value: "nvlink片间互连（DCGM）" },
  { label: "温度散热",       value: "thermal温度散热" },
  { label: "功耗电源",       value: "power功耗电源" },
  { label: "驱动",           value: "driver驱动（DCGM）" },
  { label: "PCIe总线",       value: "pcie总线" },
  { label: "运行稳定",       value: "stability运行稳定" },
];
const npuDims = [
  { label: "昇腾互连通信",     value: "interconnect昇腾互连通信" },
  { label: "显存可靠性",       value: "memory显存可靠性" },
  { label: "可靠性与运行状态", value: "reliability昇腾可靠性与运行状态" },
  { label: "PCIe总线",         value: "pcie总线" },
  { label: "算力性能",         value: "compute算力性能" },
  { label: "辅助与效率指标",   value: "auxiliary辅助与效率指标" },
  { label: "温度散热",         value: "thermal温度散热" },
  { label: "功耗电源",         value: "power功耗电源" },
];

const dimOptions = computed(() => (pageCardType.value === "NPU" ? npuDims : gpuDims));
const dimLabels: Record<string, string> = Object.fromEntries(
  [...gpuDims, ...npuDims].map((d) => [d.value, d.label])
);

// —— 数值类型码 1~8 ——
const vtOptions = [
  { label: "Gauge 连续数值", value: 1 },
  { label: "Gauge_Rate 比率", value: 2 },
  { label: "Counter 累计计数", value: 3 },
  { label: "Counter_Duration 累计时长", value: 4 },
  { label: "Level_Count 水位计数", value: 5 },
  { label: "Bool 布尔", value: 6 },
  { label: "Ordinal 枚举", value: 7 },
  { label: "Other 其他", value: 8 },
];
const vtLabels: Record<number, string> = Object.fromEntries(
  vtOptions.map((v) => [v.value, v.label])
);

// —— 归属主体 1~6 ——
const ownerOptions = [
  { label: "A 单卡自身", value: 1 },
  { label: "B 链路", value: 2 },
  { label: "C 共享基础设施", value: 3 },
  { label: "D 环境与配置", value: 4 },
  { label: "E 跨节点网络", value: 5 },
  { label: "（空）", value: 6 },
];
const ownerLabels: Record<number, string> = Object.fromEntries(
  ownerOptions.map((v) => [v.value, v.label])
);

// —— 健康度用途 1~4 ——
const purposeOptions = [
  { label: "核心", value: 1 },
  { label: "归因", value: 2 },
  { label: "配置合规", value: 3 },
  { label: "性能容量", value: 4 },
];
const purposeLabels: Record<number, string> = Object.fromEntries(
  purposeOptions.map((v) => [v.value, v.label])
);

const rows = ref<any[]>([]);
const dimFilter = ref<string | null>(null);
const vtFilter = ref<number | null>(null);
const ownerFilter = ref<number | null>(null);
const purposeFilter = ref<number | null>(null);
const showModal = ref(false);
const editing = ref(false);
const form = ref<any>(emptyForm());
const keyword = ref("");
const healthKeyOnly = ref(false);

const page = ref(1);
const pageSize = 20;
const total = ref(0);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

function emptyForm() {
  const isNpu = pageCardType.value === "NPU";
  return {
    id: 0, seq_no: null, metric_name: "", official_num: "",
    card_type: isNpu ? "NPU" : "GPU",
    dimension: isNpu ? "reliability昇腾可靠性与运行状态" : "compute算力性能",
    value_type: 1, owner_subject: 1, health_purpose: 1,
    unit: "", work_range: "",
    lower_bound: null, upper_bond: null, warn_lowbound: null, warn_upbound: null,
    normal_rate: null, warn_rate: null, normal_rate_unit: "",
    bool_normal: "", bool_abnormal: "", enum_result: "",enum_score: "",
    concept: "", is_veto: 0, derate_threshold: "", source_ref: "",
    vendor: isNpu ? "华为昇腾" : "NVIDIA",
    remark: "", is_health_key: true,
  };
}

const columns = [
  { type: "expand", renderExpand: (r: any) => h("div", { class: "metric-detail" }, [
      h("p", null, [h("b", null, "概念："), r.concept || "—"]),
      h("p", null, [h("b", null, "枚举结果："), r.enum_result || "—"]),
      h("p", null, [h("b", null, "降频/关机阈值："), r.derate_threshold || "—"]),
      h("p", null, [h("b", null, "来源依据："), r.source_ref || "—"]),
      h("p", null, [h("b", null, "备注："), r.remark || "—"]),
    ]) },
  { title: "序号", key: "seq_no", width: 60 },
  { title: "指标名称", key: "metric_name", width: 280, ellipsis: { tooltip: true },
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px;color:#9aa7b4" }, r.metric_name) },
  { title: "官方编号", key: "official_num", width: 170, ellipsis: { tooltip: true } },
  { title: "卡类型", key: "card_type", width: 70 },
  { title: "厂商", key: "vendor", width: 90 },
  { title: "维度", key: "dimension", width: 110,
    render: (r: any) => h(NTag, { size: "small", bordered: false }, () => dimLabels[r.dimension] || r.dimension) },
  { title: "数值类型", key: "value_type", width: 130,
    render: (r: any) => vtLabels[r.value_type] || r.value_type },
  { title: "归属主体", key: "owner_subject", width: 110,
    render: (r: any) => ownerLabels[r.owner_subject] || r.owner_subject },
  { title: "用途", key: "health_purpose", width: 90,
    render: (r: any) => purposeLabels[r.health_purpose] || r.health_purpose },
  { title: "单位", key: "unit", width: 70 },
  { title: "工作范围", key: "work_range", width: 110, ellipsis: { tooltip: true } },
  { title: "正常区间", key: "_normal", width: 110,render: (r: any) => fmtRange(r.lower_bound, r.upper_bond) },
  { title: "告警区间", key: "_warn", width: 110,render: (r: any) => fmtRange(r.warn_lowbound, r.warn_upbound) },
  { title: "速率(正常/告警)", key: "_rate", width: 130,render: (r: any) => {
  if (r.normal_rate == null && r.warn_rate == null) return "—";
  return `${r.normal_rate ?? "—"} / ${r.warn_rate ?? "—"} ${r.normal_rate_unit || ""}`;
      } },
  { title: "布尔(正常/异常)", key: "_bool", width: 120,render: (r: any) => (r.bool_normal || r.bool_abnormal)
        ? `${r.bool_normal || "—"} / ${r.bool_abnormal || "—"}` : "—" },

  { title: "一票否决", key: "is_veto", width: 80,
    render: (r: any) => r.is_veto === 1
      ? h(NTag, { size: "small", type: "error", bordered: false }, () => "否决")
      : h("span", { style: "color:#5e6b78" }, "—") },
  { title: "评分", key: "is_health_key", width: 60,
    render: (r: any) => r.is_health_key ? h("span", { style: "color:#22c55e" }, "是") : h("span", { style: "color:#5e6b78" }, "否") },
  {
    title: "操作", key: "ops", width: 130,
    render: (r: any) => h(NSpace, { size: 6 }, () => [
      h(NButton, { size: "tiny", onClick: () => openEdit(r) }, () => "编辑"),
      h(NButton, { size: "tiny", type: "error", ghost: true, onClick: () => confirmDelete(r) }, () => "删除")
    ])
  }
];

function fmtRange(lo: any, up: any) {
  if (lo == null && up == null) return "—";
  return `${lo ?? "-∞"} ~ ${up ?? "+∞"}`;
}

async function load() {
  const params: any = {
    limit: pageSize,
    offset: (page.value - 1) * pageSize,
    card_type: pageCardType.value,
  };
  if (keyword.value.trim()) params.keyword = keyword.value.trim();
  if (dimFilter.value) params.dimension = dimFilter.value;
  if (vtFilter.value) params.value_type = vtFilter.value;
  if (ownerFilter.value) params.owner_subject = ownerFilter.value;
  if (purposeFilter.value) params.health_purpose = purposeFilter.value;
  if (healthKeyOnly.value) params.is_health_key = "true";

  const res = await api.metrics(params);
  rows.value = res.items || [];
  total.value = res.total || 0;
}

function reload() { page.value = 1; load(); }

function openCreate() { editing.value = false; form.value = emptyForm(); showModal.value = true; }
function openEdit(r: any) {
  editing.value = true;
  form.value = { ...emptyForm(), ...r,
    lower_bound: r.lower_bound ?? null, upper_bond: r.upper_bond ?? null,
    warn_lowbound: r.warn_lowbound ?? null, warn_upbound: r.warn_upbound ?? null,
    normal_rate: r.normal_rate ?? null, warn_rate: r.warn_rate ?? null };
  showModal.value = true;
}
async function save() {
  if (!form.value.metric_name || !form.value.dimension) {
    message.warning("指标名称和维度必填");
    return;
  }
  try {
    if (editing.value) await api.updateMetric(form.value.id, form.value);
    else await api.createMetric(form.value);
    message.success("已保存");
    showModal.value = false;
    await load();
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "保存失败");
  }
}
function confirmDelete(r: any) {
  dialog.warning({
    title: "确认删除",
    content: `删除指标 ${r.metric_name}？`,
    positiveText: "删除", negativeText: "取消",
    onPositiveClick: async () => {
      await api.deleteMetric(r.id);
      message.success("已删除");
      await load();
    }
  });
}

watch(pageCardType, () => { page.value = 1; load(); });
onMounted(load);
</script>

<style scoped>
.toolbar { margin-bottom: 16px; }
</style>
