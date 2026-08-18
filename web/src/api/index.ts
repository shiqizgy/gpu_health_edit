import axios from "axios";

const http = axios.create({ baseURL: "/api/v1", timeout: 15000 });

// AI 助手 SSE 流式对话(axios 不支持流式,用原生 fetch + ReadableStream)
// onEvent(eventType, data) 会在每收到一个 SSE 事件时被调用
export async function assistantChatStream(
    payload: { conversation_id: number; uuid: string; message: string },
    onEvent: (eventType: string, data: string) => void,
    signal?: AbortSignal
): Promise<void> {
  const resp = await fetch("/api/v1/assistant/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal
  });

  if (!resp.ok || !resp.body) {
    const text = await resp.text().catch(() => "");
    throw new Error(`助手接口错误 ${resp.status}: ${text}`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // SSE 事件之间以空行(\n\n)分隔,逐个解析
    let sepIndex: number;
    while ((sepIndex = buffer.indexOf("\n\n")) !== -1) {
      const rawEvent = buffer.slice(0, sepIndex);
      buffer = buffer.slice(sepIndex + 2);
      parseSSEBlock(rawEvent, onEvent);
    }
  }
  // 处理残留
  if (buffer.trim()) parseSSEBlock(buffer, onEvent);
}

// 解析单个 SSE 块: "event: xxx\ndata: yyy"
function parseSSEBlock(block: string, onEvent: (t: string, d: string) => void) {
  let eventType = "message";
  const dataLines: string[] = [];
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      eventType = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).replace(/^ /, "")); // 去掉 "data:" 后的一个空格
    }
  }
  onEvent(eventType, dataLines.join("\n"));
}

http.interceptors.response.use(
  (r) => {
    // 后端统一响应是 { code, msg, data }，这里直接返回业务 data
    const body = r.data;
    if (body && typeof body === "object" && "code" in body) {
      return body.data;
    }
    return body;
  },
  (err) => {
    console.error("API 错误", err?.response?.data || err.message);
    return Promise.reject(err);
  }
);

export const api = {
  // 健康大盘
  dashboard: () => http.get<any, any>("/dashboard/overview"),

  // 指标系统
  metrics: (params?: any) => http.get<any, any>("/metrics", { params }),
  createMetric: (data: any) => http.post<any, any>("/metrics", data),
  updateMetric: (id: number, data: any) => http.put<any, any>(`/metrics/${id}`, data),
  deleteMetric: (id: number) => http.delete<any, any>(`/metrics/${id}`),

  // 评分策略
  strategies: () => http.get<any, any>("/strategies"),
  strategy: (id: number) => http.get<any, any>(`/strategies/${id}`),
  createStrategy: (data: any) => http.post<any, any>("/strategies", data),
  updateStrategyMeta: (id: number, data: any) => http.put<any, any>(`/strategies/${id}`, data),
  updateStrategyRules: (id: number, rules: any[]) => http.put<any, any>(`/strategies/${id}/rules`, rules),
  deleteStrategy: (id: number) => http.delete<any, any>(`/strategies/${id}`),

  // 策略绑定
  bindClusterStrategy: (clusterId: number, strategyId: number | null) =>
      http.put<any, any>(`/clusters/${clusterId}/strategy`, { strategy_id: strategyId }),
  bindGPUStrategy: (uuid: string, strategyId: number | null) =>
      http.put<any, any>(`/gpus/${uuid}/strategy`, { strategy_id: strategyId }),


  // 集群拓扑（三级树）
  topoClusters: () => http.get<any, any>("/topology/clusters"),
  topoNodes: (clusterId: number) => http.get<any, any>(`/topology/clusters/${clusterId}/nodes`),
  topoGPUs: (nodeId: number) => http.get<any, any>(`/topology/nodes/${nodeId}/gpus`),
  addGPU: (data: any) => http.post<any, any>("/topology/gpus", data),
  setGPUStatus: (uuid: string, status: string) => http.put<any, any>(`/topology/gpus/${uuid}/status`, { status }),
  topoSearch: (q: string) => http.get<any, any>("/topology/search", { params: { q } }),

  // 健康值
  healthClusters: () => http.get<any, any>("/health/clusters"),
  healthClusterGPUs: (clusterId: number, limit = 50, offset = 0) =>
    http.get<any, any>(`/health/clusters/${clusterId}/gpus`, { params: { limit, offset } }),
  healthGPUDetail: (uuid: string) => http.get<any, any>(`/health/gpus/${uuid}`),
  healthSearch: (q: string) => http.get<any, any>("/health/search", { params: { q } }),
  healthScoreTrend: (uuid: string, params: { from: string; to: string; max_points?: number }) =>
      http.get<any, any>(`/health/gpus/${uuid}/score-trend`, { params }),
  healthGPUMetrics: (uuid: string, params: { metrics: string; from: string; to: string; max_points?: number }) =>
      http.get<any, any>(`/health/gpus/${uuid}/metrics`, { params }),


    // 故障知识图谱
  faultKnowledge: (params?: any) => http.get<any, any>("/faults/knowledge", { params }),
  createFault: (data: any) => http.post<any, any>("/faults/knowledge", data),
  updateFault: (id: number, data: any) => http.put<any, any>(`/faults/knowledge/${id}`, data),
  deleteFault: (id: number) => http.delete<any, any>(`/faults/knowledge/${id}`),

  // 故障注入（演示）
  injectFault: (uuid: string, mode: string) => http.post<any, any>("/faults/inject", { uuid, mode }),
  listFaults: () => http.get<any, any>("/faults/inject"),

  //故障池
  faultPool: (params?: any) => http.get<any, any>("/faults/pool", { params }),
  faultPoolStats: () => http.get<any, any>("/faults/pool/stats"),
  resolveFault: (id: number) => http.put<any, any>(`/faults/pool/${id}/resolve`),
  
  //AI助手会话
  listConversations: () => http.get<any, any>("/assistant/conversations"),
  createConversation: (data: any) => http.post<any, any>("/assistant/conversations", data),
  getConversation: (id: number) => http.get<any, any>(`/assistant/conversations/${id}`),
  updateConversation: (id: number, data: any) => http.put<any, any>(`/assistant/conversations/${id}`, data),
  deleteConversation: (id: number) => http.delete<any, any>(`/assistant/conversations/${id}`),
};
