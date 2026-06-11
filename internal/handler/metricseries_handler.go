package handler

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gpu-health/platform/internal/ckclient"
	"github.com/gpu-health/platform/internal/repository"
	"github.com/gpu-health/platform/pkg/response"
)

const xidMetricKey = "DCGM_FI_DEV_XID_ERRORS"

// MetricSeriesHandler 单卡指标时序（下钻曲线）
type MetricSeriesHandler struct {
	ck     *ckclient.Client
	table  string
	topo   *repository.TopologyRepo
	metric *repository.MetricRepo
}

func NewMetricSeriesHandler(ck *ckclient.Client, table string,
	topo *repository.TopologyRepo, metric *repository.MetricRepo) *MetricSeriesHandler {
	return &MetricSeriesHandler{ck: ck, table: table, topo: topo, metric: metric}
}

type metricSeries struct {
	Metric      string                 `json:"metric"`
	DisplayName string                 `json:"display_name"`
	Dimension   string                 `json:"dimension"`
	Type        string                 `json:"type"` // gauge/counter/xid
	Unit        string                 `json:"unit"`
	Agg         string                 `json:"agg"`
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

	// 1) 解析 uuid → sn / tags（稳健：查库而非拆字符串）
	g, err := h.topo.GetGPUByUUID(uuid)
	if err != nil {
		response.Fail(c, 404, "GPU 不存在")
		return
	}
	sn := g.SN
	tags := strconv.Itoa(g.GPUIndex)

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
	defs, _, err := h.metric.List(repository.MetricQuery{})
	if err != nil {
		response.ServerError(c, err.Error())
		return
	}
	meta := map[string]struct{ disp, dim, typ, unit string }{}
	for _, d := range defs {
		meta[d.MetricKey] = struct{ disp, dim, typ, unit string }{
			disp: d.DisplayName,
			dim:  d.Dimension,
			typ:  d.MetricType,
			unit: d.Unit}
	}

	// 5) 请求的指标（默认给一组代表指标）
	metrics := splitCSV(c.Query("metrics"))
	if len(metrics) == 0 {
		metrics = []string{"DCGM_FI_DEV_GPU_TEMP", "DCGM_FI_DEV_POWER_USAGE",
			"DCGM_FI_DEV_ECC_DBE_VOL_TOTAL", xidMetricKey}
	}

	ctx := context.Background()
	resp := seriesResp{UUID: uuid, SN: sn, GPUIndex: g.GPUIndex,
		From: from, To: to, BucketSec: bucket}

	for _, m := range metrics {
		md, ok := meta[m]
		if !ok {
			continue // 未知指标忽略
		}
		// XID 走事件
		if m == xidMetricKey {
			evs, err := h.ck.QueryEvents(ctx, h.table, sn, tags, m, from, to)
			if err != nil {
				response.ServerError(c, err.Error())
				return
			}
			for _, e := range evs {
				resp.Events = append(resp.Events, metricEvent{Metric: m, TS: e.TS, Code: int(e.V)})
			}
			continue
		}
		// gauge → avg；counter → max
		agg := "avg"
		if md.typ == "counter" {
			agg = "max"
		}
		pts, err := h.ck.QuerySeries(ctx, h.table, sn, tags, m, from, to, bucket, agg)
		if err != nil {
			response.ServerError(c, err.Error())
			return
		}
		resp.Series = append(resp.Series, metricSeries{
			Metric: m, DisplayName: md.disp, Dimension: md.dim,
			Type: md.typ, Unit: md.unit, Agg: agg, Points: pts,
		})
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
