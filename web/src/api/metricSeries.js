import axios from 'axios'

const BASE = import.meta.env.VITE_API_BASE || '/api/v1'

// 拉单卡指标时序
export function fetchGpuMetrics(uuid, { metrics, from, to, maxPoints = 1500 }) {
    return axios.get(`${BASE}/health/gpus/${encodeURIComponent(uuid)}/metrics`, {
        params: { metrics: metrics.join(','), from, to, max_points: maxPoints },
    }).then(r => {
        const data = r.data && r.data.data
        if (!data || typeof data !== 'object') throw new Error('指标接口返回为空')
        return data
    })
}

// 指标目录：接口返回 { total, items }，字段为 metric_name / concept
export function fetchMetricCatalog() {
    return axios.get(`${BASE}/metrics`, { params: { is_health_key: true, limit: 200 } })
        .then(r => (r.data && r.data.data && r.data.data.items) || [])
}