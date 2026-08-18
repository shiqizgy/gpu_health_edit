<template>
  <div class="assistant-layout">
    <!-- 左侧:会话列表 -->
    <aside class="conv-sidebar">
      <div class="sidebar-header">
        <n-button type="primary" size="small" block @click="newConv">
          + 新建对话
        </n-button>
      </div>
      <div class="conv-list">
        <div v-for="c in store.conversations" :key="c.id"
          :class="['conv-item', store.currentConvId === c.id ? 'active' : '']"
          @click="store.switchConversation(c.id)">
          <div class="conv-title">{{ c.title }}</div>
          <div class="conv-meta">
            <span class="conv-uuid mono">{{ c.gpu_uuid ? c.gpu_uuid.slice(-12) : '—' }}</span>
            <span class="conv-time">{{ relativeTime(c.updated_at) }}</span>
          </div>
          <n-button text size="tiny" class="conv-del"
            @click.stop="confirmDelete(c)">×</n-button>
        </div>
        <div v-if="store.conversations.length === 0" class="empty">
          点击上方"新建对话"开始
        </div>
      </div>
    </aside>

    <!-- 右侧:对话区 -->
    <main class="conv-main">
      <div v-if="!store.currentConvId" class="welcome">
        <div class="welcome-icon">🩺</div>
        <div class="welcome-title">GPU 故障分析助手</div>
        <div class="welcome-desc">
          请从左侧选择一个对话,或点击"新建对话"开始。<br/>
          助手会根据 GPU 当前实时指标、健康评分、故障知识为你分析。
        </div>
      </div>

      <template v-else>
        <!-- 顶部:GPU UUID -->
        <div class="topbar">
          <span class="gpu-label">GPU UUID:</span>
          <span class="mono gpu-uuid">{{ store.currentGpuUuid || '未指定' }}</span>
        </div>

        <!-- 消息区 -->
        <div class="chat" ref="chatRef">
          <div v-for="(m, i) in store.messages" :key="i"
            :class="['bubble-row', m.role === 'user' ? 'row-user' : 'row-ai']">
            <div :class="['bubble', m.role === 'user' ? 'bubble-user' : 'bubble-ai']">
              <div v-if="m.status" class="status-line">{{ m.status }}</div>
              <div class="bubble-content" v-html="render(m.content)"></div>
            </div>
          </div>
        </div>

        <!-- 输入区 -->
        <div class="inputbar">
          <n-input v-model:value="draft" type="textarea"
            :autosize="{ minRows: 1, maxRows: 4 }"
            placeholder="输入问题 (Enter 发送,Shift+Enter 换行)"
            :disabled="store.streaming" @keydown="onKeydown" />
          <n-button type="primary" :loading="store.streaming"
            :disabled="!draft || store.streaming" @click="send">
            发送
          </n-button>
        </div>
      </template>
    </main>

    <!-- 新建对话弹窗 -->
    <n-modal v-model:show="showNew" preset="card" title="新建对话" style="width: 440px">
      <n-form label-placement="left" label-width="100">
        <n-form-item label="GPU UUID">
          <n-input v-model:value="newUuid" placeholder="如 GPU-000000000042" />
        </n-form-item>
        <n-form-item label="对话标题">
          <n-input v-model:value="newTitle" placeholder="留空自动用 UUID" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showNew = false">取消</n-button>
          <n-button type="primary" @click="doNew">创建</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, watch, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useMessage, useDialog } from "naive-ui";
import { useAssistantStore } from "@/stores/assistant";

const store = useAssistantStore();
const route = useRoute();
const msg = useMessage();
const dialog = useDialog();

const draft = ref("");
const chatRef = ref<HTMLElement | null>(null);
const showNew = ref(false);
const newUuid = ref("");
const newTitle = ref("");

function render(text: string): string {
  if (!text) return "";
  return text
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    .replace(/^- (.+)$/gm, "• $1")
    .replace(/\n/g, "<br/>");
}

function relativeTime(s: string): string {
  if (!s) return "";
  const t = new Date(s).getTime();
  const diff = Date.now() - t;
  if (diff < 60_000) return "刚刚";
  if (diff < 3600_000) return `${Math.floor(diff / 60_000)}分钟前`;
  if (diff < 86400_000) return `${Math.floor(diff / 3600_000)}小时前`;
  return new Date(t).toLocaleDateString();
}

async function scrollToBottom() {
  await nextTick();
  if (chatRef.value) chatRef.value.scrollTop = chatRef.value.scrollHeight;
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    send();
  }
}

async function send() {
  if (!draft.value || store.streaming) return;
  const text = draft.value;
  draft.value = "";
  try {
    await store.sendMessage(text);
  } catch (e: any) {
    msg.error(e?.message || "发送失败");
  }
}

function newConv() {
  newUuid.value = "";
  newTitle.value = "";
  showNew.value = true;
}

async function doNew() {
  if (!newUuid.value) {
    msg.warning("请填入 GPU UUID");
    return;
  }
  try {
    await store.newConversation(newUuid.value, newTitle.value);
    showNew.value = false;
  } catch (e: any) {
    msg.error("创建失败:" + (e?.message || e));
  }
}

function confirmDelete(c: any) {
  dialog.warning({
    title: "删除对话",
    content: `确认删除 "${c.title}"?对话内容不可恢复。`,
    positiveText: "删除", negativeText: "取消",
    onPositiveClick: async () => {
      await store.deleteConversation(c.id);
      msg.success("已删除");
    }
  });
}

// 消息变化时自动滚到底
watch(() => store.messages.length, scrollToBottom);
watch(() => store.streaming, () => scrollToBottom());

// 首次进入:加载会话列表
onMounted(async () => {
  await store.loadConversations();

  // 支持从其他页面带 uuid 跳进来,自动新建会话
  const q = route.query.uuid as string;
  if (q) {
    await store.newConversation(q);
  }
});
</script>

<style scoped>
.assistant-layout {
  display: flex;
  height: calc(100vh - 52px - 40px);
  gap: 16px;
}

/* 左侧会话列表 */
.conv-sidebar {
  width: 240px;
  flex-shrink: 0;
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
}
.sidebar-header { padding: 12px; border-bottom: 1px solid var(--border); }
.conv-list { flex: 1; overflow-y: auto; padding: 4px; }
.conv-item {
  position: relative;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 2px;
  border: 1px solid transparent;
}
.conv-item:hover { background: var(--bg-2); }
.conv-item.active { background: var(--bg-2); border-color: var(--accent-dim); }
.conv-title {
  font-size: 13px; font-weight: 500; color: var(--text-0);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  padding-right: 20px;
}
.conv-meta {
  display: flex; justify-content: space-between;
  font-size: 11px; color: var(--text-2); margin-top: 4px;
}
.conv-uuid { color: var(--text-1); }
.conv-del {
  position: absolute; top: 8px; right: 8px;
  color: var(--text-2); font-size: 16px;
  opacity: 0; transition: opacity 0.2s;
}
.conv-item:hover .conv-del { opacity: 1; }
.conv-del:hover { color: var(--lv-failed); }
.empty { padding: 24px 12px; text-align: center; color: var(--text-2); font-size: 12px; }

/* 右侧对话区 */
.conv-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--bg-1);
  border: 1px solid var(--border);
  border-radius: 8px;
  min-width: 0;
}
.topbar {
  display: flex; align-items: center; gap: 12px;
  padding: 14px 20px; border-bottom: 1px solid var(--border);
}
.gpu-label { font-size: 12px; color: var(--text-2); }
.gpu-uuid { font-size: 13px; color: var(--accent); }

.chat {
  flex: 1; overflow-y: auto; padding: 20px;
  display: flex; flex-direction: column; gap: 14px;
}

.welcome {
  margin: auto; text-align: center; color: var(--text-2); padding: 60px 40px;
}
.welcome-icon { font-size: 48px; margin-bottom: 16px; }
.welcome-title { font-size: 18px; font-weight: 700; color: var(--text-0); margin-bottom: 10px; }
.welcome-desc { font-size: 13px; line-height: 1.8; }

.bubble-row { display: flex; }
.row-user { justify-content: flex-end; }
.row-ai { justify-content: flex-start; }
.bubble {
  max-width: 78%; padding: 12px 16px; border-radius: 10px;
  font-size: 14px; line-height: 1.7;
}
.bubble-user {
  background: var(--accent-dim); color: #e6f6ff;
  border: 1px solid var(--accent);
}
.bubble-ai {
  background: var(--bg-2); color: var(--text-0);
  border: 1px solid var(--border);
}
.status-line {
  font-size: 12px; color: var(--text-2); font-style: italic;
  margin-bottom: 4px;
}
.bubble-content :deep(strong) { color: var(--accent); }

.inputbar {
  display: flex; gap: 10px; align-items: flex-end;
  padding: 14px 20px; border-top: 1px solid var(--border);
}
</style>
