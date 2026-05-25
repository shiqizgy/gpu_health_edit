<template>
  <div class="topo">
    <div class="panel tree-panel">
      <div class="panel-title">集群 — 节点 — GPU 三级拓扑</div>
      <div style="padding: 12px 8px">
        <n-tree
          block-line
          :data="treeData"
          :on-load="onLoad"
          expand-on-click
          :node-props="nodeProps"
        />
        <div v-if="!treeData.length" class="empty">暂无拓扑，请先运行仿真服务初始化(make sim-init)</div>
      </div>
    </div>

    <div class="panel info-panel">
      <div class="panel-title">操作 / 详情</div>
      <div style="padding: 16px">
        <n-space vertical size="large">
          <div>
            <div class="info-label">动态扩缩容</div>
            <n-space vertical>
              <n-button size="small" @click="showAdd = true" block>+ 新增 GPU 卡</n-button>
              <n-input-group>
                <n-input v-model:value="opUUID" placeholder="GPU UUID" size="small" />
                <n-button size="small" @click="setStatus('maintenance')">维护</n-button>
                <n-button size="small" @click="setStatus('offline')">下线</n-button>
                <n-button size="small" type="primary" ghost @click="setStatus('online')">上线</n-button>
              </n-input-group>
            </n-space>
          </div>

          <div v-if="selected">
            <div class="info-label">选中节点</div>
            <pre class="mono detail">{{ JSON.stringify(selected, null, 2) }}</pre>
          </div>
        </n-space>
      </div>
    </div>

    <n-modal v-model:show="showAdd" preset="card" title="新增 GPU 卡(扩容)" style="width: 480px">
      <n-form :model="addForm" label-placement="left" label-width="100">
        <n-form-item label="UUID"><n-input v-model:value="addForm.uuid" placeholder="GPU-000000099999" /></n-form-item>
        <n-form-item label="集群ID"><n-input-number v-model:value="addForm.cluster_id" style="width:100%" /></n-form-item>
        <n-form-item label="节点ID"><n-input-number v-model:value="addForm.node_id" style="width:100%" /></n-form-item>
        <n-form-item label="卡序号"><n-input-number v-model:value="addForm.gpu_index" style="width:100%" /></n-form-item>
        <n-form-item label="型号"><n-input v-model:value="addForm.model" placeholder="H100-SXM5-80GB" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAdd = false">取消</n-button>
          <n-button type="primary" @click="doAdd">添加</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { api } from "@/api";
import { useMessage } from "naive-ui";

const message = useMessage();
const treeData = ref<any[]>([]);
const selected = ref<any>(null);
const showAdd = ref(false);
const opUUID = ref("");
const addForm = ref<any>({ uuid: "", cluster_id: 1, node_id: 1, gpu_index: 0, model: "H100-SXM5-80GB" });

// 顶层：加载集群
async function loadClusters() {
  const clusters = await api.topoClusters();
  treeData.value = (clusters || []).map((c: any) => ({
    key: "c-" + c.id,
    label: `${c.name} (${c.code})`,
    raw: c, type: "cluster",
    isLeaf: false
  }));
}

// 懒加载：点击集群→节点，点击节点→GPU
async function onLoad(node: any) {
  if (node.type === "cluster") {
    const nodes = await api.topoNodes(node.raw.id);
    node.children = (nodes || []).map((n: any) => ({
      key: "n-" + n.id,
      label: `${n.hostname} · ${n.gpu_count}卡`,
      raw: n, type: "node", isLeaf: false
    }));
  } else if (node.type === "node") {
    const gpus = await api.topoGPUs(node.raw.id);
    node.children = (gpus || []).map((g: any) => ({
      key: "g-" + g.uuid,
      label: `GPU${g.gpu_index} · ${g.uuid}`,
      raw: g, type: "gpu", isLeaf: true
    }));
  }
}

function nodeProps(info: any) {
  return {
    onClick: () => { selected.value = info.option.raw; }
  };
}

async function doAdd() {
  try {
    await api.addGPU(addForm.value);
    message.success("已添加 GPU");
    showAdd.value = false;
    await loadClusters();
  } catch (e: any) {
    message.error(e?.response?.data?.msg || "添加失败");
  }
}

async function setStatus(status: string) {
  if (!opUUID.value) { message.warning("请输入 UUID"); return; }
  try {
    await api.setGPUStatus(opUUID.value, status);
    message.success(`已设为 ${status}`);
  } catch (e: any) {
    message.error("操作失败");
  }
}

onMounted(loadClusters);
</script>

<style scoped>
.topo { display: grid; grid-template-columns: 1fr 360px; gap: 16px; }
.tree-panel { min-height: 600px; }
.info-panel { align-self: start; }
.info-label { font-size: 12px; color: var(--text-2); letter-spacing: 0.05em; margin-bottom: 8px; }
.detail {
  background: var(--bg-0);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 12px;
  font-size: 11px;
  color: var(--text-1);
  overflow: auto;
  max-height: 280px;
}
.empty { padding: 40px; text-align: center; color: var(--text-2); font-size: 13px; }
</style>
