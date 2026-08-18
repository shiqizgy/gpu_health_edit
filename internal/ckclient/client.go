package ckclient

import (
	"context"
	"database/sql"
	"fmt"
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

type SampleRow struct {
	SN, Tags, Source, NodeGroup, IP, MIB string
	Value                                float64
}

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
	q := fmt.Sprintf(`
		SELECT sn,tags,source,gpu_node_group,ip,mib,argMax(value,timestamp) AS value
		FROM %s
		WHERE toDate(dt) >= today()-1
		AND timestamp >= now()-INTERVAL %d SECOND
		AND source = ?
		GROUP BY sn,tags,source,gpu_node_group,ip,mib`, table, int(window.Seconds()))
	//每组内取 timestamp 最大那行的 value；只扫今天和昨天分区；并按「卡+指标」分组
	//table是配置里写好的的table

	rows, err := c.db.QueryContext(ctx, q, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.SN, &r.Tags, &r.Source, &r.NodeGroup, &r.IP, &r.MIB, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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

	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.SN, &r.Tags, &r.Source, &r.NodeGroup, &r.IP, &r.MIB, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
