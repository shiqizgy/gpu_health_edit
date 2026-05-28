import axios from "axios";

const http = axios.create({ baseURL: "/api/v1", timeout: 15000 });

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

  // 健康值
  healthClusters: () => http.get<any, any>("/health/clusters"),
  healthClusterGPUs: (clusterId: number, limit = 50, offset = 0) =>
    http.get<any, any>(`/health/clusters/${clusterId}/gpus`, { params: { limit, offset } }),
  healthGPUDetail: (uuid: string) => http.get<any, any>(`/health/gpus/${uuid}`),

  // 故障知识图谱
  faultKnowledge: (params?: any) => http.get<any, any>("/faults/knowledge", { params }),
  createFault: (data: any) => http.post<any, any>("/faults/knowledge", data),
  updateFault: (id: number, data: any) => http.put<any, any>(`/faults/knowledge/${id}`, data),
  deleteFault: (id: number) => http.delete<any, any>(`/faults/knowledge/${id}`),

  // 故障注入（演示）
  injectFault: (uuid: string, mode: string) => http.post<any, any>("/faults/inject", { uuid, mode }),
  listFaults: () => http.get<any, any>("/faults/inject")
};
