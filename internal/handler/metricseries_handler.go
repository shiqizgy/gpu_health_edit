package handler

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/internal/scoring"
	"github.com/gpu-health/platform/internal/service"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/gpu-health/platform/pkg/response"
)

const xidMetricKey = "DCGM_FI_DEV_XID_ERRORS"

// MetricSeriesHandler 单卡指标时序（下钻曲线）
type MetricSeriesHandler struct {
	ck       *ckclient.Client
	table    string
	topo     *repository.TopologyRepo
	metric   *repository.MetricRepo
	health   *repository.HealthRepo
	strategy *service.StrategyService
}

func NewMetricSeriesHandler(ck *ckclient.Client, table string,
	topo *repository.TopologyRepo, metric *repository.MetricRepo,
	health *repository.HealthRepo, strategy *service.StrategyService) *MetricSeriesHandler {
	return &MetricSeriesHandler{ck: ck, table: table, topo: topo, metric: metric, health: health, strategy: strategy}
}

type metricSeries struct {
	Metric      string                 `json:"metric"`       // 官方字段名，如 DCGM_FI_DEV_ECC_DBE_AGG_TOTAL
	OfficialNo  string                 `json:"official_no"`  // 官方编号/说明
	DisplayName string                 `json:"display_name"` // 中文概念
	Dimension   string                 `json:"dimension"`
	Type        string                 `json:"type"`
	Unit        string                 `json:"unit"`
	Agg         string                 `json:"agg"`
	Status      string                 `json:"status"`   // ok / no_data / error
	IsAlive     bool                   `json:"is_alive"` // 近期 CK 是否还有该指标上报
	Latest      *float64               `json:"latest"`   // 窗口内最后一个值
	AlertUpper  *float64               `json:"alert_upper"`
	UpperBound  *float64               `json:"upper_bound"`
	AlertLower  *float64               `json:"alert_lower"`
	LowerBound  *float64               `json:"lower_bound"`
	Points      []ckclient.SeriesPoint `json:"points"`
}

type metricEvent struct {
	Metric string    `json:"metric"`
	TS     time.Time `json:"ts"`
	Code   int       `json:"code"`
}

type seriesResp struct {
	UUID      string         `json:"uuid"`
	SN        string         `json:"sn"`
	GPUIndex  int            `json:"gpu_index"`
	From      time.Time      `json:"from"`
	To        time.Time      `json:"to"`
	BucketSec int            `json:"bucket_sec"`
	Series    []metricSeries `json:"series"`
	Events    []metricEvent  `json:"events"`
}

// GPUMetrics GET /health/gpus/:uuid/metrics
//
//	?metrics=DCGM_FI_DEV_GPU_TEMP,DCGM_FI_DEV_ECC_DBE_VOL_TOTAL,DCGM_FI_DEV_XID_ERRORS
//	&from=2026-06-07T00:00:00Z&to=2026-06-08T00:00:00Z&max_points=1500
func (h *MetricSeriesHandler) GPUMetrics(c *gin.Context) {
	if h.ck == nil {
		response.ServerError(c, "未配置 ClickHouse 数据源")
		return
	}
	uuid := c.Param("uuid")

	// 1) 校验卡是否存在；uuid 即 CK 里的 gpu_sn，直接用于查询
	g, err := h.topo.GetGPUByUUID(uuid)
	if err != nil {
		response.Fail(c, 404, "GPU 不存在")
		return
	}
	sn := g.SN

	// 2) 时间范围（默认近 24h，RFC3339）
	to := parseTime(c.Query("to"), time.Now())
	from := parseTime(c.Query("from"), to.Add(-24*time.Hour))
	if !to.After(from) {
		response.Fail(c, 400, "时间范围非法")
		return
	}

	// 3) 自适应桶宽：保证返回点数有界
	maxPoints, _ := strconv.Atoi(c.DefaultQuery("max_points", "1500"))
	if maxPoints <= 0 || maxPoints > 5000 {
		maxPoints = 1500
	}
	bucket := pickBucket(from, to, maxPoints)

	// 4) 指标元数据（类型/维度/单位）来自 metric_definition，单一真相源
	// 用全量 AllDefsMap（勿用带默认分页的 List，否则 meta 只有前 20 条，
	// 导致大部分参评指标在下方 meta[m] 查不到而被跳过，明细图缺失）
	defsMap, err := h.metric.AllDefsMap()
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}

	// 5) 请求的指标（默认给一组代表指标）
	metrics := splitCSV(c.Query("metrics"))
	if len(metrics) == 0 {
		compiled := h.resolveStrategy(uuid)
		if compiled != nil {
			metrics = make([]string, 0, len(compiled.Rules))
			for k := range compiled.Rules {
				metrics = append(metrics, k)
			}
		}
	}
	sort.Strings(metrics) // 放在填充之后，默认列表顺序才稳定

	ctx := c.Request.Context()
	resp := seriesResp{UUID: uuid, SN: sn, GPUIndex: g.GPUIndex,
		From: from, To: to, BucketSec: bucket}

	for _, m := range metrics {
		d, ok := defsMap[m]
		if !ok {
			continue
		}
		if m == xidMetricKey { // XID 仍走事件
			evs, err := h.ck.QueryEvents(ctx, h.table, uuid, m, from, to)
			if err != nil {
				logger.L.Warnf("查询 %s XID 事件失败: %v", uuid, err)
				continue
			}
			for _, e := range evs {
				resp.Events = append(resp.Events, metricEvent{Metric: m, TS: e.TS, Code: int(e.V)})
			}
			continue
		}
		agg := "avg"
		if d.ValueType == scoring.VTCounter || d.ValueType == scoring.VTDuration || d.ValueType == scoring.VTLevel {
			agg = "max"
		}
		s := metricSeries{
			Metric: m, OfficialNo: d.OfficialNum, DisplayName: d.Conception, Dimension: d.Dimension,
			Type: valueTypeName(d.ValueType), Unit: d.Unit, Agg: agg, IsAlive: d.IsAlive,
			Points: []ckclient.SeriesPoint{}, // 保证 JSON 是 [] 而不是 null
		}
		// 只有连续量的阈值和曲线值同一量纲；累计类画的是原始累计值，阈值是速率，不能画在一起
		if d.ValueType == scoring.VTGauge || d.ValueType == scoring.VTGaugeRate {
			s.AlertUpper, s.UpperBound, s.AlertLower, s.LowerBound = d.WarnupBound, d.UpperBond, d.WarnlowBound, d.LowerBound
		}
		pts, err := h.ck.QuerySeries(ctx, h.table, uuid, m, from, to, bucket, agg)
		switch {
		case err != nil:
			s.Status = "error"
			logger.L.Warnf("查询 %s 指标 %s 失败: %v", uuid, m, err)
		case len(pts) == 0:
			s.Status = "no_data"
		default:
			s.Status = "ok"
			s.Points = pts
			v := pts[len(pts)-1].V
			s.Latest = &v
		}
		resp.Series = append(resp.Series, s)
	}
	response.OK(c, resp)
}

// —— 辅助 ——
func parseTime(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return def
}

func pickBucket(from, to time.Time, maxPoints int) int {
	span := to.Sub(from).Seconds()
	b := int(math.Ceil(span/float64(maxPoints)/60) * 60)
	if b < 60 {
		b = 60 // 不细于原始分辨率 1min
	}
	return b
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

type trendPoint struct {
	TS    time.Time `json:"ts"`
	Score float64   `json:"score"`
	Level string    `json:"level"`
}
type trendEvent struct {
	TS    time.Time `json:"ts"`  // 段开始
	End   time.Time `json:"end"` // 段结束（新增）
	Type  string    `json:"type"`
	Code  int       `json:"code"`
	Count int       `json:"count"` // 段内上报次数（新增）
	Fatal bool      `json:"fatal"` // 是否致命 XID（新增）
	Label string    `json:"label"`
}

type trendResp struct {
	UUID      string       `json:"uuid"`
	From      time.Time    `json:"from"`
	To        time.Time    `json:"to"`
	BucketSec int          `json:"bucket_sec"`
	Points    []trendPoint `json:"points"`
	Events    []trendEvent `json:"events"`
}

// ScoreTrend GET /health/gpus/:uuid/score-trend?from=&to=&max_points=300
//
// 说明：方案A历史回溯。用该卡"当前策略"对历史各时间桶指标重算总分，
// 调整维度/权重后整条数据趋势会随之更新（反映当前策略视角）。不含预测。
func (h *MetricSeriesHandler) ScoreTrend(c *gin.Context) {
	if h.ck == nil {
		response.ServerError(c, "未配置 ClickHouse 数据源")
		return
	}
	uuid := c.Param("uuid")

	if _, err := h.topo.GetGPUByUUID(uuid); err != nil { // uuid 即 CK 的 gpu_sn
		response.Fail(c, 404, "GPU 不存在")
		return
	}

	to := parseTime(c.Query("to"), time.Now())
	from := parseTime(c.Query("from"), to.Add(-6*time.Hour))
	if !to.After(from) {
		response.Fail(c, 400, "时间范围非法")
		return
	}
	maxPoints, _ := strconv.Atoi(c.DefaultQuery("max_points", "300"))
	if maxPoints <= 0 || maxPoints > 2000 {
		maxPoints = 300
	}
	bucket := pickBucket(from, to, maxPoints)

	// 1) 取该卡策略：优先快照记录的 strategy_id，回退默认
	compiled := h.resolveStrategy(uuid)
	if compiled == nil {
		response.ServerError(c, "无法加载评分策略")
		return
	}

	// 2) 只查该策略参评的指标；分离 XID（事件）与连续量
	ctx := c.Request.Context()
	defsMap, _ := h.metric.AllDefsMap()

	// 收集参评 metric key
	metricKeys := make([]string, 0, len(compiled.Rules))
	for k := range compiled.Rules {
		metricKeys = append(metricKeys, k)
	}

	// 3) 逐指标取桶序列，按 ts 归集成 map[ts]map[metric]value
	byTS := map[int64]map[string]float64{}
	resp := trendResp{UUID: uuid, From: from, To: to, BucketSec: bucket}

	for _, m := range metricKeys {
		if m == xidMetricKey {
			evs, err := h.ck.QueryEvents(ctx, h.table, uuid, m, from, to)
			if err == nil {
				gap := 2 * time.Duration(bucket) * time.Second
				if gap < 5*time.Minute {
					gap = 5 * time.Minute
				}
				resp.Events = append(resp.Events, mergeXidEvents(evs, gap)...)
			}
			// XID 值也参与评分：按事件时刻落到对应桶
			for _, e := range evs2(evs, err) {
				b := bucketStart(e.TS, bucket)
				if byTS[b] == nil {
					byTS[b] = map[string]float64{}
				}
				byTS[b][m] = e.V
			}
			continue
		}
		agg := "avg"
		if d, ok := defsMap[m]; ok &&
			(d.ValueType == scoring.VTCounter || d.ValueType == scoring.VTDuration || d.ValueType == scoring.VTLevel) {
			agg = "max"
		}
		pts, err := h.ck.QuerySeries(ctx, h.table, uuid, m, from, to, bucket, agg)
		if err != nil {
			continue
		}
		for _, p := range pts {
			b := p.TS.Unix()
			if byTS[b] == nil {
				byTS[b] = map[string]float64{}
			}
			byTS[b][m] = p.V
		}
	}

	// 4) 逐桶重算分数
	bs := make([]int64, 0, len(byTS))
	for b := range byTS {
		bs = append(bs, b)
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i] < bs[j] })
	for _, b := range bs {
		res := scoring.Score(byTS[b], compiled)
		resp.Points = append(resp.Points, trendPoint{
			TS:    time.Unix(b, 0),
			Score: round1(res.Score),
			Level: res.Level,
		})
	}

	response.OK(c, resp)
}

// resolveStrategy 取该卡当前策略的编译结果（快照 strategy_id → 默认）
func (h *MetricSeriesHandler) resolveStrategy(uuid string) *scoring.CompiledStrategy {
	if snap, err := h.health.GetSnapshot(uuid); err == nil && snap.StrategyID > 0 {
		if cs, err := h.strategy.GetCompiledByID(snap.StrategyID); err == nil {
			return cs
		}
	}
	cs, err := h.strategy.GetCompiledDefault()
	if err != nil {
		return nil
	}
	return cs
}

func bucketStart(t time.Time, bucketSec int) int64 {
	return t.Unix() - (t.Unix() % int64(bucketSec))
}
func round1(v float64) float64 { return math.Round(v*10) / 10 }
func itoaLocal(n int) string   { return strconv.Itoa(n) }

// evs2 小工具：err 非空时返回空切片，避免上面重复取 events
func evs2(evs []ckclient.SeriesPoint, err error) []ckclient.SeriesPoint {
	if err != nil {
		return nil
	}
	return evs
}

// valueTypeName 把 value_type 数值码转成前端展示用的类型名
func valueTypeName(vt int) string {
	switch vt {
	case scoring.VTGauge:
		return "gauge"
	case scoring.VTGaugeRate:
		return "gauge_rate"
	case scoring.VTCounter:
		return "counter"
	case scoring.VTDuration:
		return "counter_duration"
	case scoring.VTLevel:
		return "level_count"
	case scoring.VTBool:
		return "bool"
	case scoring.VTOrdinal:
		return "ordinal"
	case scoring.VTOther:
		return "other"
	default:
		return "gauge"
	}
}

// mergeXidEvents 把间隔不超过 gap 的同码 XID 合并成一段
func mergeXidEvents(evs []ckclient.SeriesPoint, gap time.Duration) []trendEvent {
	sorted := append([]ckclient.SeriesPoint(nil), evs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })
	var out []trendEvent
	for _, e := range sorted {
		code := int(e.V)
		if n := len(out); n > 0 && out[n-1].Code == code && e.TS.Sub(out[n-1].End) <= gap {
			out[n-1].End = e.TS
			out[n-1].Count++
			continue
		}
		out = append(out, trendEvent{
			TS: e.TS, End: e.TS, Type: "xid", Code: code, Count: 1,
			Fatal: scoring.XIDIsFatal(code), Label: "Xid " + strconv.Itoa(code),
		})
	}
	return out
}
