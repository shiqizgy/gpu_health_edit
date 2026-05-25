<template>
  <div>
    <div class="toolbar">
      <n-space>
        <n-input v-model:value="keyword" placeholder="搜索故障类型/表现/原因" clearable
          style="width: 260px" @keydown.enter="load" />
        <n-button @click="load">搜索</n-button>
        <n-button type="primary" @click="openCreate">+ 新建故障知识</n-button>
      </n-space>
    </div>

    <div class="cards">
      <div v-for="f in rows" :key="f.id" class="fault-card">
        <div class="fc-head">
          <span :class="['level-badge', 'lv-' + sevLevel(f.severity)]">{{ sevName(f.severity) }}</span>
          <span class="fc-type">{{ f.fault_type }}</span>
          <span v-if="f.xid_code" class="mono fc-xid">XID {{ f.xid_code }}</span>
          <div class="fc-ops">
            <n-button size="tiny" @click="openEdit(f)">编辑</n-button>
            <n-button size="tiny" type="error" ghost @click="confirmDelete(f)">删除</n-button>
          </div>
        </div>
        <div class="fc-body">
          <div class="fc-field"><span class="fc-label">表现</span>{{ f.symptom }}</div>
          <div class="fc-field" v-if="f.possible_cause"><span class="fc-label">原因</span>{{ f.possible_cause }}</div>
          <div class="fc-field" v-if="f.metric_changes"><span class="fc-label">指标变化</span>{{ f.metric_changes }}</div>
          <div class="fc-field" v-if="f.suggestion"><span class="fc-label">建议</span>{{ f.suggestion }}</div>
          <a v-if="f.reference" :href="f.reference" target="_blank" class="fc-ref">参考资料 ↗</a>
        </div>
      </div>
      <div v-if="!rows.length" class="empty">暂无故障知识，点击右上角新建</div>
    </div>

    <n-modal v-model:show="showModal" preset="card" :title="editing ? '编辑故障知识' : '新建故障知识'" style="width: 640px">
      <n-form :model="form" label-placement="left" label-width="90">
        <n-grid :cols="2" :x-gap="16">
          <n-gi><n-form-item label="故障类型"><n-input v-model:value="form.fault_type" /></n-form-item></n-gi>
          <n-gi><n-form-item label="XID 码"><n-input v-model:value="form.xid_code" placeholder="48 (可空)" /></n-form-item></n-gi>
        </n-grid>
        <n-form-item label="严重等级">
          <n-select v-model:value="form.severity" :options="sevOptions" />
        </n-form-item>
        <n-form-item label="故障表现"><n-input v-model:value="form.symptom" type="textarea" :autosize="{ minRows: 2 }" /></n-form-item>
        <n-form-item label="可能原因"><n-input v-model:value="form.possible_cause" type="textarea" :autosize="{ minRows: 2 }" /></n-form-item>
        <n-form-item label="指标变化"><n-input v-model:value="form.metric_changes" type="textarea" :autosize="{ minRows: 2 }" /></n-form-item>
        <n-form-item label="处置建议"><n-input v-model:value="form.suggestion" type="textarea" :autosize="{ minRows: 2 }" /></n-form-item>
        <n-form-item label="参考链接"><n-input v-model:value="form.reference" /></n-form-item>
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
import { ref, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, useDialog } from "naive-ui";

const message = useMessage();
const dialog = useDialog();

const sevOptions = [
  { label: "warning 警告", value: "warning" },
  { label: "critical 严重", value: "critical" },
  { label: "fatal 致命", value: "fatal" }
];
function sevLevel(s: string) { return s === "fatal" ? "failed" : s === "critical" ? "critical" : "warning"; }
function sevName(s: string) { return { warning: "警告", critical: "严重", fatal: "致命" }[s] || s; }

const rows = ref<any[]>([]);
const keyword = ref("");
const showModal = ref(false);
const editing = ref(false);
const form = ref<any>(emptyForm());

function emptyForm() {
  return { id: 0, fault_type: "", xid_code: "", symptom: "", possible_cause: "",
    metric_changes: "", suggestion: "", reference: "", severity: "warning", related_metrics: "[]" };
}

async function load() {
  const res = await api.faultKnowledge({ keyword: keyword.value, limit: 100 });
  rows.value = res.items || [];
}
function openCreate() { editing.value = false; form.value = emptyForm(); showModal.value = true; }
function openEdit(f: any) { editing.value = true; form.value = { ...f }; showModal.value = true; }
async function save() {
  if (!form.value.fault_type || !form.value.symptom) {
    message.warning("故障类型和表现必填"); return;
  }
  try {
    if (editing.value) await api.updateFault(form.value.id, form.value);
    else await api.createFault(form.value);
    message.success("已保存");
    showModal.value = false;
    await load();
  } catch { message.error("保存失败"); }
}
function confirmDelete(f: any) {
  dialog.warning({
    title: "确认删除", content: `删除「${f.fault_type}」？`,
    positiveText: "删除", negativeText: "取消",
    onPositiveClick: async () => { await api.deleteFault(f.id); message.success("已删除"); await load(); }
  });
}

onMounted(load);
</script>

<style scoped>
.toolbar { margin-bottom: 16px; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(420px, 1fr)); gap: 16px; }
.fault-card { background: var(--bg-1); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
.fc-head {
  display: flex; align-items: center; gap: 10px;
  padding: 12px 16px; border-bottom: 1px solid var(--border); background: var(--bg-2);
}
.fc-type { font-weight: 600; font-size: 14px; }
.fc-xid { font-size: 12px; color: var(--accent); }
.fc-ops { margin-left: auto; display: flex; gap: 6px; }
.fc-body { padding: 14px 16px; display: flex; flex-direction: column; gap: 8px; }
.fc-field { font-size: 13px; color: var(--text-1); line-height: 1.5; }
.fc-label {
  display: inline-block; min-width: 64px; color: var(--text-2);
  font-size: 11px; letter-spacing: 0.05em; margin-right: 8px;
}
.fc-ref { font-size: 12px; color: var(--accent); text-decoration: none; margin-top: 4px; }
.empty { grid-column: 1/-1; padding: 60px; text-align: center; color: var(--text-2); }
</style>
