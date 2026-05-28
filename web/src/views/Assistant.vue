<template>
  <div class="assistant">
    <!-- 顶部:指定要分析的 GPU -->
    <div class="topbar">
      <n-input
        v-model:value="uuid"
        placeholder="输入要分析的 GPU UUID,如 GPU-000000000042"
        style="max-width: 420px"
      />
      <n-button type="primary" :disabled="!uuid || streaming" @click="quickDiagnose">
        诊断这张卡
      </n-button>
      <span class="hint">先填 UUID,可点快捷诊断,或在下方直接提问</span>
    </div>

    <!-- 对话区 -->
    <div class="chat" ref="chatRef">
      <div v-if="messages.length === 0" class="welcome">
        <div class="welcome-icon">🩺</div>
        <div class="welcome-title">GPU 故障分析助手</div>
        <div class="welcome-desc">
          填入一个 GPU UUID,我会查询它的实时指标、健康评分和匹配的故障知识,<br />
          为你分析这张卡的状态、风险和处置建议。
        </div>
      </div>

      <div
        v-for="(m, i) in messages"
        :key="i"
        :class="['bubble-row', m.role === 'user' ? 'row-user' : 'row-ai']"
      >
        <div :class="['bubble', m.role === 'user' ? 'bubble-user' : 'bubble-ai']">
          <div v-if="m.status" class="status-line">{{ m.status }}</div>
          <div class="bubble-content" v-html="render(m.content)"></div>
        </div>
      </div>
    </div>

    <!-- 输入区 -->
    <div class="inputbar">
      <n-input
        v-model:value="draft"
        type="textarea"
        :autosize="{ minRows: 1, maxRows: 4 }"
        placeholder="输入问题,如:这张卡为什么分数低?要不要换? (Enter 发送)"
        :disabled="streaming"
        @keydown="onKeydown"
      />
      <n-button type="primary" :loading="streaming" :disabled="!draft || streaming" @click="send">
        发送
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useMessage } from "naive-ui";
import { assistantChatStream } from "@/api";

const route = useRoute();
const msg = useMessage();

const uuid = ref("");
const draft = ref("");
const streaming = ref(false);
const chatRef = ref<HTMLElement | null>(null);

// 对话消息:role=user/assistant,content=文本,status=临时状态行
interface ChatMsg {
  role: "user" | "assistant";
  content: string;
  status?: string;
}
const messages = ref<ChatMsg[]>([]);

// 极简 markdown 渲染(加粗、换行、列表),避免引第三方库
function render(text: string): string {
  if (!text) return "";
  let html = text
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    .replace(/^- (.+)$/gm, "• $1")
    .replace(/\n/g, "<br/>");
  return html;
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

function quickDiagnose() {
  if (!uuid.value) { msg.warning("请先填入 GPU UUID"); return; }
  draft.value = "请诊断这张 GPU 卡当前的健康状况、存在的风险,并给出处置建议。";
  send();
}

async function send() {
  if (!uuid.value) { msg.warning("请先填入要分析的 GPU UUID"); return; }
  if (!draft.value || streaming.value) return;

  const userText = draft.value;
  draft.value = "";

  // 推入用户消息
  messages.value.push({ role: "user", content: userText });
  // 推入一个空的 AI 消息(待流式填充)
  const ai: ChatMsg = { role: "assistant", content: "", status: "正在连接助手..." };
  messages.value.push(ai);
  await scrollToBottom();

  // 组装 history(不含刚推入的这两条,且只取 user/assistant 的纯对话)
  const history = messages.value
    .slice(0, -2)
    .map((m) => ({ role: m.role, content: m.content }));

  streaming.value = true;
  try {
    await assistantChatStream(
      { uuid: uuid.value, message: userText, history },
      (eventType, data) => {
        if (eventType === "status") {
          ai.status = data;
        } else if (eventType === "message") {
          ai.status = ""; // 收到正文就清掉状态行
          ai.content += data;
          scrollToBottom();
        } else if (eventType === "error") {
          ai.status = "";
          ai.content += `\n\n⚠️ 出错了:${data}`;
        } else if (eventType === "done") {
          ai.status = "";
        }
      }
    );
  } catch (e: any) {
    ai.status = "";
    ai.content += `\n\n⚠️ 请求失败:${e?.message || e}`;
  } finally {
    streaming.value = false;
    scrollToBottom();
  }
}

// 支持从其他页面跳转时带 uuid(?uuid=xxx),并自动诊断一次
onMounted(() => {
  const q = route.query.uuid as string;
  if (q) {
    uuid.value = q;
    quickDiagnose();
  }
});
</script>

<style scoped>
.assistant {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 52px - 40px); /* 减顶栏和内容 padding */
}
.topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.hint { font-size: 12px; color: var(--text-2); }

.chat {
  flex: 1;
  overflow-y: auto;
  padding: 18px 4px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.welcome {
  margin: auto;
  text-align: center;
  color: var(--text-2);
}
.welcome-icon { font-size: 40px; margin-bottom: 12px; }
.welcome-title { font-size: 18px; font-weight: 700; color: var(--text-0); margin-bottom: 10px; }
.welcome-desc { font-size: 13px; line-height: 1.8; }

.bubble-row { display: flex; }
.row-user { justify-content: flex-end; }
.row-ai { justify-content: flex-start; }

.bubble {
  max-width: 76%;
  padding: 12px 16px;
  border-radius: 10px;
  font-size: 14px;
  line-height: 1.7;
}
.bubble-user {
  background: var(--accent-dim);
  color: #e6f6ff;
  border: 1px solid var(--accent);
}
.bubble-ai {
  background: var(--bg-2);
  color: var(--text-0);
  border: 1px solid var(--border);
}
.status-line {
  font-size: 12px;
  color: var(--text-2);
  font-style: italic;
  margin-bottom: 4px;
}
.bubble-content :deep(strong) { color: var(--accent); }

.inputbar {
  display: flex;
  gap: 10px;
  align-items: flex-end;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}
</style>
