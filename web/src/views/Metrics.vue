<template>
  <div>
    <div class="toolbar">
      <n-space>
        <n-input v-model:value="keyword" placeholder="搜索指标名/显示名/概念/备注"
          clearable style="width: 240px"
          @keydown.enter="reload" @clear="reload" />
        <n-select v-model:value="dimFilter" :options="dimOptions" placeholder="按维度筛选"
          clearable style="width: 170px" @update:value="reload" />
        <n-select v-model:value="deviceFilter" :options="deviceOptions" placeholder="按设备筛选"
          clearable style="width: 140px" @update:value="reload" />
        <n-select v-model:value="typeFilter" :options="typeOptions" placeholder="按类型筛选"
          clearable style="width: 150px" @update:value="reload" />
        <n-checkbox v-model:checked="healthKeyOnly" @update:checked="reload">仅参与评分</n-checkbox>
        <n-button @click="reload">搜索</n-button>
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
      <div class="pager">
            <n-pagination
              v-model:page="page"
              :page-count="pageCount"
              :page-size="pageSize"
              @update:page="load"
            />
          </div>
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
              <n-select v-model:value="form.device_type" :options="deviceOptions" />
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
import { ref, h, computed, onMounted } from "vue";
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

// 设备类型三类
const deviceOptions = [
  { label: "GPU", value: "gpu" },
  { label: "服务器", value: "server" },
  { label: "网络设备", value: "network" }
];
const deviceLabels: Record<string, string> = {
  gpu: "GPU", server: "服务器", network: "网络设备"
};

const typeOptions = [
  { label: "gauge 瞬时值", value: "gauge" },
  { label: "counter 计数器", value: "counter" },
  { label: "enum 枚举", value: "enum" },
  { label: "histogram 直方图", value: "histogram" },
  { label: "boolean 布尔", value: "boolean" },
  { label: "other 其他", value: "other" }
];

const dimLabels: Record<string, string> = {
  environment: "环境", performance: "性能", hardware: "硬件", stability: "稳定性"
};

const rows = ref<any[]>([]);
const dimFilter = ref<string | null>(null);
const showModal = ref(false);
const editing = ref(false);
const form = ref<any>(emptyForm());
const deviceFilter = ref<string | null>(null);
const typeFilter = ref<string | null>(null);
const keyword = ref("");
const healthKeyOnly = ref(false);

// 分页
const page = ref(1);
const pageSize = 20;
const total = ref(0);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

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
  { title: "所属设备", key: "device_type", width: 100,
    render: (r: any) => h(NTag, { size: "small", type: "info", bordered: false }, () => deviceLabels[r.device_type] || r.device_type) },
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
  const params: any = {
    limit: pageSize,
    offset: (page.value - 1) * pageSize,
  };
  if (keyword.value.trim()) params.keyword = keyword.value.trim();
  if (dimFilter.value) params.dimension = dimFilter.value;
  if (deviceFilter.value) params.device_type = deviceFilter.value;
  if (typeFilter.value) params.metric_type = typeFilter.value;
  if (healthKeyOnly.value) params.is_health_key = "true";

  const res = await api.metrics(params);
  rows.value = res.items || [];   // 后端现在返回 { total, items }
  total.value = res.total || 0;
}

// 任意搜索条件变化时，回到第1页再取数
function reload() {
  page.value = 1;
  load();
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
