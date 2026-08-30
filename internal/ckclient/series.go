package ckclient

import (
	"context"
	"fmt"
	"time"
)

// SeriesPoint 时序点（桶起始时间 + 值）
type SeriesPoint struct {
	TS time.Time `json:"ts"`
	V  float64   `json:"v"`
}

// QuerySeries 连续量按桶聚合曲线。agg ∈ {"avg","max"}。
//   - gauge   用 avg（看包络可另查 min/max）
//   - counter 用 max（单调递增，桶内 max = 桶内最新值，保持单调；用 avg 会失真）
//
// 性能：dt BETWEEN 做分区裁剪；sn/tags/mib 命中排序键前缀，区间扫描。
func (c *Client) QuerySeries(ctx context.Context, table, sn, tags, mib string,
	from, to time.Time, bucketSec int, agg string) ([]SeriesPoint, error) {

	if agg != "avg" && agg != "max" {
		agg = "avg"
	}
	q := fmt.Sprintf(`
SELECT toStartOfInterval(timestamp, INTERVAL %d SECOND) AS ts, %s(value) AS v
FROM %s
WHERE sn = ? AND tags = ? AND mib = ?
    AND toDate(dt) BETWEEN toDate(?) AND toDate(?) AND timestamp BETWEEN ? AND ?
GROUP BY ts ORDER BY ts`, bucketSec, agg, table)

	rows, err := c.db.QueryContext(ctx, q, sn, tags, mib, from, to, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SeriesPoint, 0, 256)
	for rows.Next() {
		var p SeriesPoint
		if err := rows.Scan(&p.TS, &p.V); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// QueryEvents 事件型指标（如 XID）：取范围内 value>0 的原始点，不做聚合。
func (c *Client) QueryEvents(ctx context.Context, table, sn, tags, mib string,
	from, to time.Time) ([]SeriesPoint, error) {

	q := fmt.Sprintf(`
SELECT timestamp AS ts, value AS v
FROM %s
WHERE sn = ? AND tags = ? AND mib = ? AND value > 0
    AND toDate(dt) BETWEEN toDate(?) AND toDate(?) AND timestamp BETWEEN ? AND ?
ORDER BY ts`, table)

	rows, err := c.db.QueryContext(ctx, q, sn, tags, mib, from, to, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SeriesPoint, 0, 32)
	for rows.Next() {
		var p SeriesPoint
		if err := rows.Scan(&p.TS, &p.V); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
