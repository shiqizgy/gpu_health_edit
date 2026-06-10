import axios from 'axios'

const BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080/api/v1'

// 拉单卡指标时序
export function fetchGpuMetrics(uuid, { metrics, from, to, maxPoints = 1500 }) {
    return axios.get(`${BASE}/health/gpus/${encodeURIComponent(uuid)}/metrics`, {
        params: { metrics: metrics.join(','), from, to, max_points: maxPoints },
    }).then(r => r.data.data) // pkg/response 包装：{code,msg,data}
}

// 指标目录（复用现有接口），用于选择器
export function fetchMetricCatalog() {
    return axios.get(`${BASE}/metrics`).then(r => r.data.data)
}