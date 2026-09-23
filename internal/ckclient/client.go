package ckclient

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gpu-health/platform/internal/config"
)

type Client struct{ db *sql.DB }

func New(cfg config.CKConfig) (*Client, error) {
	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr:     []string{cfg.Addr},
		Protocol: clickhouse.HTTP,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: 30 * time.Second,
		ReadTimeout: 120 * time.Second,
		Settings: clickhouse.Settings{
			"max_execution_time": 120,
		},
	})
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Client{db: db}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

// SampleRow 对应 monitor_gpu_all_metric_v2_dist 的一行聚合结果。
// v2 表用 gpu_sn 唯一标识一张卡（老表只能用 sn+tags 拼，tags 为空时会串卡）。
type SampleRow struct {
	GPUSN      string // GPU SN：卡的唯一标识
	SN         string // 机器 SN（节点）
	Tags       string // 卡号，可能为空
	Source     string // 集群
	IP         string
	MIB        string
	Model      string // gpu_model
	Vendor     string // gpu_manufacturer
	VRAM       string // gpu_vram
	Plat       string // gpu_plat
	PodID      string
	BuildingID string
	IdcID      string
	Value      float64
}

// v2 表的公共投影：一张卡一个指标一行，附带机型/机房等静态信息
const sampleSelect = `SELECT gpu_sn,
       any(sn) AS sn, any(tags) AS tags, source, any(ip) AS ip, mib,
       any(gpu_model) AS gpu_model, any(gpu_manufacturer) AS gpu_manufacturer,
       any(gpu_vram) AS gpu_vram, any(gpu_plat) AS gpu_plat,
       any(pod_id) AS pod_id, any(building_id) AS building_id, any(idc_id) AS idc_id,
       argMax(ifNull(value, 0), timestamp) AS value`

// ListSources 查出当前窗口内活跃的所有 source（集群），用于分批拉取
func (c *Client) ListSources(ctx context.Context, table string, window time.Duration) ([]string, error) {
	q := fmt.Sprintf(`
		SELECT DISTINCT source
		FROM %s
		WHERE toDate(dt) >= today()-1
		AND timestamp >= now()-INTERVAL %d SECOND`, table, int(window.Seconds()))

	rows, err := c.db.QueryContext(ctx, q) //
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LatestSamplesBySource 按单个 source 分片查询，避免一次返回过大结果集
func (c *Client) LatestSamplesBySource(ctx context.Context, table, source string, window time.Duration) ([]SampleRow, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ { // 最多试 3 次
		rows, err := c.doQueryBySource(ctx, table, source, window)
		if err == nil {
			return rows, nil
		} // 成功即返回
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
	return nil, lastErr //3 次全败，返回最后错误
}

// 按单个 source(集群)分片查询 ClickHouse 最新指标数据
func (c *Client) doQueryBySource(ctx context.Context, table, source string, window time.Duration) ([]SampleRow, error) {
	q := fmt.Sprintf(`%s
		FROM %s
		WHERE toDate(dt) >= today()-1
		AND timestamp >= now()-INTERVAL %d SECOND
		AND source = ?
		AND gpu_sn != '' AND value IS NOT NULL
		GROUP BY gpu_sn, source, mib`, sampleSelect, table, int(window.Seconds()))
	//每组内取 timestamp 最大那行的 value；只扫今天和昨天分区；并按「卡+指标」分组
	//table是配置里写好的的table

	rows, err := c.db.QueryContext(ctx, q, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSamples(rows)
}

func scanSamples(rows *sql.Rows) ([]SampleRow, error) {
	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.GPUSN, &r.SN, &r.Tags, &r.Source, &r.IP, &r.MIB,
			&r.Model, &r.Vendor, &r.VRAM, &r.Plat, &r.PodID, &r.BuildingID, &r.IdcID, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestSamplesByGPUSNs 抽样模式专用：只拉已锁定的那批卡，避免每分钟全量拉整个 source
func (c *Client) LatestSamplesByGPUSNs(ctx context.Context, table, source string, gpuSNs []string, window time.Duration) ([]SampleRow, error) {
	const chunk = 500
	var out []SampleRow
	for i := 0; i < len(gpuSNs); i += chunk {
		end := i + chunk
		if end > len(gpuSNs) {
			end = len(gpuSNs)
		}
		var rows []SampleRow
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			rows, err = c.doQueryByGPUSNs(ctx, table, source, gpuSNs[i:end], window)
			if err == nil {
				break
			}
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func (c *Client) doQueryByGPUSNs(ctx context.Context, table, source string, gpuSNs []string, window time.Duration) ([]SampleRow, error) {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(gpuSNs)), ",")
	q := fmt.Sprintf(`%s
        FROM %s
        WHERE toDate(dt) >= today()-1
          AND timestamp >= now()-INTERVAL %d SECOND
          AND source = ?
          AND gpu_sn IN (%s)
          AND value IS NOT NULL
        GROUP BY gpu_sn, source, mib`, sampleSelect, table, int(window.Seconds()), ph)
	args := make([]any, 0, len(gpuSNs)+1)
	args = append(args, source)
	for _, s := range gpuSNs {
		args = append(args, s)
	}
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSamples(rows)
}

// LatestSamples 全量查询（保留兼容，小规模场景可用）
func (c *Client) LatestSamples(ctx context.Context, table string, window time.Duration) ([]SampleRow, error) {
	q := fmt.Sprintf(`
		SELECT sn,tags,source,gpu_node_group,ip,mib,argMax(value,timestamp) AS value
		FROM %s
		WHERE toDate(dt) >= today()-1
		AND timestamp >= now()-INTERVAL %d SECOND
		GROUP BY sn,tags,source,gpu_node_group,ip,mib`, table, int(window.Seconds()))

	rows, err := c.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSamples(rows)
}

// LatestByGPU 取某张卡(sn+tags)最近窗口内每个指标的最新值。
func (c *Client) LatestByGPU(ctx context.Context, table, sn, tags string, window time.Duration) (map[string]float64, error) {
	q := fmt.Sprintf(`
		SELECT mib, argMax(value,timestamp) AS value
		FROM %s
		WHERE sn = ? AND tags = ?
		  AND toDate(dt) >= today()-1
		  AND timestamp >= now()-INTERVAL %d SECOND
		GROUP BY mib`, table, int(window.Seconds()))

	rows, err := c.db.QueryContext(ctx, q, sn, tags)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var mib string
		var v float64
		if err := rows.Scan(&mib, &v); err != nil {
			return nil, err
		}
		out[mib] = v
	}
	return out, rows.Err()
}

type MetricRow struct {
	Timestamp time.Time
	IP        string
	SN        string
	Source    string
	MIB       string
	Tags      string
	Value     float64
	DT        time.Time
	NodeGroup string
}

func (c *Client) InsertSamples(ctx context.Context, table string, rows []MetricRow) error {
	stmt := "INSERT INTO " + table + " (timestamp,ip,sn,source,mib,tags,value,dt,gpu_node_group) VALUES (?,?,?,?,?,?,?,?,?)"
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, stmt, r.Timestamp, r.IP, r.SN, r.Source, r.MIB, r.Tags, r.Value, r.DT, r.NodeGroup); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
