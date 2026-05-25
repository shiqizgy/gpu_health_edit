<template>
  <div>
    <div class="toolbar">
      <n-space>
        <n-select v-model:value="dimFilter" :options="dimOptions" placeholder="按维度筛选"
          clearable style="width: 180px" @update:value="load" />
        <n-button type="primary" @click="openCreate">+ 新建指标</n-button>
      </n-space>
    </div>

    <div class="panel">
      <n-data-table
        :columns="columns"
        :data="rows"
        :bordered="false"
        :max-height="620"
        size="small"
      />
    </div>

    <!-- 新建/编辑弹窗 -->
    <n-modal v-model:show="showModal" preset="card" :title="editing ? '编辑指标' : '新建指标'" style="width: 680px">
      <n-form :model="form" label-placement="left" label-width="100">
        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="指标名称">
              <n-input v-model:value="form.metric_key" placeholder="DCGM_FI_DEV_GPU_TEMP" :disabled="editing" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="显示名">
              <n-input v-model:value="form.display_name" placeholder="GPU 核心温度" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="单位">
              <n-input v-model:value="form.unit" placeholder="C / W / ratio" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="指标类型">
              <n-select v-model:value="form.metric_type" :options="typeOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="指标维度">
              <n-select v-model:value="form.dimension" :options="dimOptions" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="所属设备">
              <n-input v-model:value="form.device_type" placeholder="gpu" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="正常范围">
              <n-input v-model:value="form.normal_range" placeholder="<80" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="异常范围">
              <n-input v-model:value="form.abnormal_range" placeholder=">=87" />
            </n-form-item>
          </n-gi>
        </n-grid>
        <n-form-item label="概念说明">
          <n-input v-model:value="form.concept" type="textarea" :autosize="{ minRows: 2 }" />
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
import { ref, h, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, useDialog, NButton, NSpace, NTag } from "naive-ui";

const message = useMessage();
const dialog = useDialog();

const dimOptions = [
  { label: "运行环境 environment", value: "environment" },
  { label: "性能表现 performance", value: "performance" },
  { label: "硬件健康 hardware", value: "hardware" },
  { label: "运行稳定性 stability", value: "stability" }
];
const typeOptions = [
  { label: "gauge 瞬时值", value: "gauge" },
  { label: "counter 计数器", value: "counter" }
];

const dimLabels: Record<string, string> = {
  environment: "环境", performance: "性能", hardware: "硬件", stability: "稳定性"
};

const rows = ref<any[]>([]);
const dimFilter = ref<string | null>(null);
const showModal = ref(false);
const editing = ref(false);
const form = ref<any>(emptyForm());

function emptyForm() {
  return {
    id: 0, metric_key: "", display_name: "", unit: "", metric_type: "gauge",
    dimension: "performance", concept: "", device_type: "gpu",
    normal_range: "", abnormal_range: "", remark: "", is_health_key: true
  };
}

const columns = [
  { title: "指标名称", key: "metric_key", width: 280,
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px;color:#9aa7b4" }, r.metric_key) },
  { title: "显示名", key: "display_name", width: 140 },
  { title: "维度", key: "dimension", width: 90,
    render: (r: any) => h(NTag, { size: "small", bordered: false }, () => dimLabels[r.dimension] || r.dimension) },
  { title: "类型", key: "metric_type", width: 90 },
  { title: "单位", key: "unit", width: 70 },
  { title: "正常范围", key: "normal_range", width: 100 },
  { title: "异常范围", key: "abnormal_range", width: 100 },
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

async function load() {
  const params: any = {};
  if (dimFilter.value) params.dimension = dimFilter.value;
  rows.value = await api.metrics(params);
}

function openCreate() {
  editing.value = false;
  form.value = emptyForm();
  showModal.value = true;
}
function openEdit(r: any) {
  editing.value = true;
  form.value = { ...r };
  showModal.value = true;
}
async function save() {
  if (!form.value.metric_key || !form.value.dimension) {
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
    content: `删除指标 ${r.metric_key}？`,
    positiveText: "删除", negativeText: "取消",
    onPositiveClick: async () => {
      await api.deleteMetric(r.id);
      message.success("已删除");
      await load();
    }
  });
}

onMounted(load);
</script>

<style scoped>
.toolbar { margin-bottom: 16px; }
</style>
