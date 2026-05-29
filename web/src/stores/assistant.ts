import { defineStore } from "pinia";
import { ref } from "vue";
import { api, assistantChatStream } from "@/api";

export interface ChatMsg {
    id?: number;
    role: "user" | "assistant";
    content: string;
    status?: string;  // 仅运行时用,流式状态提示
}

export interface Conversation {
    id: number;
    title: string;
    gpu_uuid: string;
    updated_at: string;
}

export const useAssistantStore = defineStore("assistant", () => {
    const conversations = ref<Conversation[]>([]);     // 左侧会话列表
    const currentConvId = ref<number | null>(null);    // 当前打开的会话
    const messages = ref<ChatMsg[]>([]);                // 当前会话的消息
    const currentGpuUuid = ref("");                    // 当前会话关联的 GPU
    const streaming = ref(false);                      // 流式中(禁止发送)

    // 加载会话列表
    async function loadConversations() {
        conversations.value = await api.listConversations();
    }

    // 新建会话
    async function newConversation(gpuUuid: string, title?: string) {
        const conv = await api.createConversation({
            title: title || (gpuUuid ? `GPU ${gpuUuid.slice(-8)} 诊断` : "新对话"),
            gpu_uuid: gpuUuid
        });
        await loadConversations();
        await switchConversation(conv.id);
        return conv;
    }

    // 切换到某个会话(从后端加载消息)
    async function switchConversation(id: number) {
        const data = await api.getConversation(id);
        currentConvId.value = id;
        currentGpuUuid.value = data.conversation.gpu_uuid || "";
        messages.value = (data.messages || []).map((m: any) => ({
            id: m.id, role: m.role, content: m.content
        }));
    }

    // 删除会话
    async function deleteConversation(id: number) {
        await api.deleteConversation(id);
        if (currentConvId.value === id) {
            currentConvId.value = null;
            messages.value = [];
            currentGpuUuid.value = "";
        }
        await loadConversations();
    }

    // 发送消息(流式)
    async function sendMessage(text: string) {
        if (!currentConvId.value) {
            throw new Error("请先新建或选择一个会话");
        }
        if (!currentGpuUuid.value) {
            throw new Error("当前会话未关联 GPU UUID");
        }
        if (streaming.value) return;

        // 1. 立刻推用户消息到 UI
        messages.value.push({ role: "user", content: text });
        // 2. 推一个空 AI 消息,等流式填充
        const ai: ChatMsg = { role: "assistant", content: "", status: "正在分析..." };
        messages.value.push(ai);

        streaming.value = true;
        try {
            await assistantChatStream(
                {
                    conversation_id: currentConvId.value,
                    uuid: currentGpuUuid.value,
                    message: text
                },
                (eventType, data) => {
                    if (eventType === "status") {
                        ai.status = data;
                    } else if (eventType === "message") {
                        ai.status = "";
                        // 关键:用替换数组方式触发 Vue 响应式(强制视图更新)
                        ai.content += data;
                        messages.value = [...messages.value];
                    } else if (eventType === "error") {
                        ai.status = "";
                        ai.content += `\n\n⚠️ ${data}`;
                    } else if (eventType === "done") {
                        ai.status = "";
                    }
                }
            );
        } finally {
            streaming.value = false;
            // 刷新会话列表(updated_at 变了,排序会变)
            loadConversations();
        }
    }

    return {
        conversations, currentConvId, messages, currentGpuUuid, streaming,
        loadConversations, newConversation, switchConversation, deleteConversation,
        sendMessage
    };
});
