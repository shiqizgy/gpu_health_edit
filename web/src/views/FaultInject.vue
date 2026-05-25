<template>
  <div>
    <div class="panel info">
      演示工具：向仿真服务注入故障意图。约下一个生成周期(≤1分钟)后该卡指标按故障特征变化，
      评分服务随后将其健康分拉低。注入后可在「健康值」页看到对应卡掉到 critical/failed。
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="panel-title">注入故障</div>
      <div style="padding: 20px">
        <n-form label-placement="left" label-width="100">
          <n-form-item label="GPU UUID">
            <n-input v-model:value="uuid" placeholder="GPU-000000000001" style="max-width: 320px" />
          </n-form-item>
          <n-form-item label="故障模式">
            <n-radio-group v-model:value="mode">
              <n-radio-button value="healthy">恢复正常</n-radio-button>
              <n-radio-button value="high_temp">高温降频</n-radio-button>
              <n-radio-button value="xid">XID 致命</n-radio-button>
              <n-radio-button value="ecc">ECC 双比特</n-radio-button>
              <n-radio-button value="link_down">互连异常</n-radio-button>
              <n-radio-button value="remap_fail">重映射失败</n-radio-button>
            </n-radio-group>
          </n-form-item>
          <n-space>
            <n-button type="primary" @click="inject">注入</n-button>
            <n-button @click="injectBatch">批量注入前 8 张卡</n-button>
          </n-space>
        </n-form>
      </div>
    </div>

    <div class="panel" style="margin-top: 16px">
      <div class="panel-title">当前注入的故障意图</div>
      <n-data-table :columns="cols" :data="rows" :bordered="false" size="small" :max-height="360" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted } from "vue";
import { api } from "@/api";
import { useMessage, NButton } from "naive-ui";

const message = useMessage();
const uuid = ref("GPU-000000000001");
const mode = ref("xid");
const rows = ref<any[]>([]);

const cols = [
  { title: "GPU UUID", key: "uuid",
    render: (r: any) => h("span", { class: "mono", style: "font-size:12px" }, r.uuid) },
  { title: "故障模式", key: "mode" },
  { title: "操作", key: "ops", width: 120,
    render: (r: any) => h(NButton, { size: "tiny", onClick: () => clearOne(r.uuid) }, () => "恢复正常") }
];

async function refresh() {
  const faults = await api.listFaults();
  rows.value = Object.entries(faults || {}).map(([uuid, mode]) => ({ uuid, mode }));
}
async function inject() {
  if (!uuid.value) { message.warning("请输入 UUID"); return; }
  await api.injectFault(uuid.value, mode.value);
  message.success(`已向 ${uuid.value} 注入 ${mode.value}`);
  await refresh();
}
async function injectBatch() {
  for (let i = 0; i < 8; i++) {
    const u = `GPU-${String(i).padStart(12, "0")}`;
    await api.injectFault(u, mode.value);
  }
  message.success("已批量注入前 8 张卡");
  await refresh();
}
async function clearOne(u: string) {
  await api.injectFault(u, "healthy");
  await refresh();
}

onMounted(refresh);
</script>

<style scoped>
.info { padding: 14px 16px; font-size: 13px; color: var(--text-1); line-height: 1.7; border-left: 3px solid var(--lv-warning); }
</style>
