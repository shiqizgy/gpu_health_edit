package ckclient

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/gpu-health/platform/internal/config"
)

type Client struct{ conn driver.Conn }

// 根据配置创建 ClickHouse 客户端连接。
// 它会尝试建立连接并通过 Ping 验证连通性，连接和 Ping 的超时时间均为 5 秒。
// 成功时返回已就绪的 *Client，失败时返回 nil 和对应的错误。

func New(cfg config.CKConfig) (*Client, error) {
	//Addr: []string{cfg.Addr}是个字符串切片。这是因为ClickHouse原生支持分布式集群，可以在这里配置多个节点地址
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	//使用 Ping 验证真实连通性，并设置了 5 秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// 关闭CK连接
func (c *Client) Close() error {
	return c.conn.Close()
}

type SampleRow struct {
	SN, Tags, Source, NodeGroup, IP, MIB string
	Value                                float64
}

// 从ClickHouse查询返回的一条GPU采样指标数据
// 使用 argMax(value, timestamp) 聚合函数，按 sn/tags/source/gpu_node_group/ip/mib 分组，
// 取每组中时间戳最大的value值，同时限定dt>=today()-1以利用分区裁剪优化查询性能。

func (c *Client) LatestSamples(ctx context.Context, table string, window time.Duration) ([]SampleRow, error) {
	q := fmt.Sprintf(`
		SELECT sn,tags,source,gpu_node_group,ip,mib,argMax(value,timestamp) AS value
		FROM %s
		Where dt >= today()-1
		AND timestamp >= now()-INTERVAL %d SECOND
		GROUP BY sn,tags,source,gpu_node_group,ip,mib`, table, int(window.Seconds()))

	rows, err := c.conn.Query(ctx, q)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := rows.Close(); err != nil {
		}
	}()

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

// MetricRow与CK表9列对应
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
	batch, err := c.conn.PrepareBatch(ctx,
		"INSERT INTO "+table+"(timestamp,ip,sn,source,mib,tags,value,dt,gpu_node_group)")
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := batch.Append(r.Timestamp, r.IP, r.SN, r.Source, r.MIB, r.Tags, r.Value, r.DT, r.NodeGroup); err != nil {
			return err
		}
	}
	return batch.Send()
}
