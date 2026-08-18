<template>
  <n-drawer v-model:show="visible" :width="900" placement="right">
    <n-drawer-content :title="`GPU 指标曲线 · ${uuid}`" closable>
      <!-- 控制条 -->
      <n-space vertical :size="12">
        <n-space align="center">
          <n-radio-group v-model:value="preset" size="small" @update:value="onPreset">
            <n-radio-button value="1h">1小时</n-radio-button>
            <n-radio-button value="6h">6小时</n-radio-button>
            <n-radio-button value="24h">24小时</n-radio-button>
            <n-radio-button value="7d">7天</n-radio-button>
            <n-radio-button value="custom">自定义</n-radio-button>
          </n-radio-group>
          <n-date-picker
            v-if="preset === 'custom'"
            v-model:value="customRange" type="datetimerange" size="small"
            @update:value="load" />
        </n-space>

        <n-select
          v-model:value="selected" multiple filterable
          :options="metricOptions" placeholder="选择要查看的指标"
          @update:value="load" style="max-width: 700px" />
      </n-space>

      <!-- 图表区：小多图 -->
      <n-spin :show="loading">
        <div v-if="series.length === 0 && !loading" style="padding:40px;text-align:center;color:#999">
          暂无数据
        </div>
        <div v-for="s in series" :key="s.metric" style="margin-top:16px">
          <div style="font-size:13px;color:#666;margin-bottom:4px">
            {{ s.display_name || s.metric }}
            <span style="color:#aaa">（{{ s.dimension }} · {{ s.type }}{{ s.unit ? ' · ' + s.unit : '' }}）</span>
          </div>
          <v-chart :option="chartOption(s)" autoresize style="height:220px" />
        </div>
        <!-- XID 事件单独列出 -->
        <div v-if="events.length" style="margin-top:16px">
          <n-alert type="warning" title="XID 事件">
            <div v-for="(e,i) in events" :key="i">
              {{ fmtTime(e.ts) }} — XID {{ e.code }}
            </div>
          </n-alert>
        </div>
      </n-spin>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, MarkLineComponent, DataZoomComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import VChart from 'vue-echarts'
import { fetchGpuMetrics, fetchMetricCatalog } from '@/api/metricSeries'

use([LineChart, GridComponent, TooltipComponent, MarkLineComponent, DataZoomComponent, CanvasRenderer])

const props = defineProps({
  uuid: { type: String, default: '' },
  show: { type: Boolean, default: false },
})
const emit = defineEmits(['update:show'])
const visible = computed({
  get: () => props.show,
  set: v => emit('update:show', v),
})

const preset = ref('24h')
const customRange = ref(null)
const selected = ref(['DCGM_FI_DEV_GPU_TEMP', 'DCGM_FI_DEV_POWER_USAGE',
  'DCGM_FI_DEV_ECC_DBE_VOL_TOTAL', 'DCGM_FI_DEV_XID_ERRORS'])
const metricOptions = ref([])
const series = ref([])
const events = ref([])
const loading = ref(false)

// 指标目录 → 选择器选项（按维度分组）
async function loadCatalog() {
  const defs = await fetchMetricCatalog()
  metricOptions.value = defs.map(d => ({
    label: `${d.display_name || d.metric_key}`,
    value: d.metric_key,
    // 可按 d.dimension 分组：用 n-select 的 group 需改造，这里从简
  }))
}

function rangeFromTo() {
  if (preset.value === 'custom' && customRange.value) {
    return { from: new Date(customRange.value[0]), to: new Date(customRange.value[1]) }
  }
  const to = new Date()
  const map = { '1h': 1, '6h': 6, '24h': 24, '7d': 24 * 7 }
  const from = new Date(to.getTime() - (map[preset.value] || 24) * 3600 * 1000)
  return { from, to }
}

function onPreset() {
  if (preset.value !== 'custom') load()
}

async function load() {
  if (!props.uuid || selected.value.length === 0) {
    series.value = []; events.value = []
    return
  }
  loading.value = true
  try {
    const { from, to } = rangeFromTo()
    const data = await fetchGpuMetrics(props.uuid, {
      metrics: selected.value,
      from: from.toISOString(),
      to: to.toISOString(),
    })
    series.value = data.series || []
    events.value = data.events || []
  } finally {
    loading.value = false
  }
}

// 每个指标一张折线图；XID 事件作为竖线标注叠加，便于关联
function chartOption(s) {
  const markLines = events.value.map(e => ({
    xAxis: new Date(e.ts).getTime(),
    label: { formatter: `XID ${e.code}`, color: '#d97706', fontSize: 10 },
    lineStyle: { color: '#d97706', type: 'dashed' },
  }))
  return {
    grid: { left: 50, right: 16, top: 16, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'time' },
    yAxis: { type: 'value', scale: s.type === 'gauge' },
    dataZoom: [{ type: 'inside' }],
    series: [{
      type: 'line', showSymbol: false, smooth: s.type === 'gauge',
      step: s.type === 'counter' ? 'end' : false, // 计数器用阶梯更贴语义
      data: (s.points || []).map(p => [new Date(p.ts).getTime(), p.v]),
      markLine: markLines.length ? { symbol: 'none', data: markLines } : undefined,
    }],
  }
}

function fmtTime(ts) { return new Date(ts).toLocaleString() }

// 打开 / 切换卡时加载
watch(() => [props.show, props.uuid], ([show]) => {
  if (show) { if (!metricOptions.value.length) loadCatalog(); load() }
})
</script>