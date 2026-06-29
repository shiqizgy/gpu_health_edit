package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func main() {
	conn, err := connect()
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	ctx := context.Background()

	table := "gpu_metrics" // ← 替换为实际表名

	fmt.Println("====== 1. 所有指标名(mib)列表 ======")
	listAllMIBs(ctx, conn, table)

	fmt.Println("\n====== 2. 每张卡拥有的指标数量 ======")
	metricCountPerGPU(ctx, conn, table)

	fmt.Println("\n====== 3. 每个指标的数值分布(min/max/avg/p50/p95/p99) ======")
	metricDistribution(ctx, conn, table)

	fmt.Println("\n====== 4. 不同卡的指标集合差异 ======")
	metricSetDiff(ctx, conn, table)

	fmt.Println("\n====== 5. 按 source 分组看厂商/集群 ======")
	sourceOverview(ctx, conn, table)
}

// 1. 列出所有不重复的指标名
func listAllMIBs(ctx context.Context, conn driver.Conn, table string) {
	q := fmt.Sprintf(`SELECT DISTINCT mib FROM %s ORDER BY mib`, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var mib string
		rows.Scan(&mib)
		i++
		fmt.Printf("  %d. %s\n", i, mib)
	}
	fmt.Printf("  共 %d 个不同指标\n", i)
}

// 2. 每张卡(sn+tags)有多少个不同指标
func metricCountPerGPU(ctx context.Context, conn driver.Conn, table string) {
	q := fmt.Sprintf(`
		SELECT sn, tags, count(DISTINCT mib) AS cnt
		FROM %s
		GROUP BY sn, tags
		ORDER BY cnt DESC`, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sn, tags string
		var cnt uint64
		rows.Scan(&sn, &tags, &cnt)
		fmt.Printf("  卡 %s:%s → %d 个指标\n", sn, tags, cnt)
	}
}

// 3. 每个指标的数值分布
func metricDistribution(ctx context.Context, conn driver.Conn, table string) {
	q := fmt.Sprintf(`
		SELECT 
			mib,
			count() AS samples,
			min(value) AS v_min,
			max(value) AS v_max,
			avg(value) AS v_avg,
			quantile(0.5)(value) AS p50,
			quantile(0.95)(value) AS p95,
			quantile(0.99)(value) AS p99
		FROM %s
		GROUP BY mib
		ORDER BY mib`, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()
	fmt.Printf("  %-50s %8s %10s %10s %10s %10s %10s %10s\n",
		"指标", "样本数", "min", "max", "avg", "p50", "p95", "p99")
	fmt.Println("  " + "─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")
	for rows.Next() {
		var mib string
		var samples uint64
		var vMin, vMax, vAvg, p50, p95, p99 float64
		rows.Scan(&mib, &samples, &vMin, &vMax, &vAvg, &p50, &p95, &p99)
		fmt.Printf("  %-50s %8d %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f\n",
			mib, samples, vMin, vMax, vAvg, p50, p95, p99)
	}
}

// 4. 看不同卡之间指标集合的差异
func metricSetDiff(ctx context.Context, conn driver.Conn, table string) {
	q := fmt.Sprintf(`
		SELECT sn, tags, groupArray(DISTINCT mib) AS mibs
		FROM %s
		GROUP BY sn, tags
		ORDER BY sn, tags`, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()

	type card struct {
		sn, tags string
		mibs     []string
	}
	var cards []card
	for rows.Next() {
		var c card
		rows.Scan(&c.sn, &c.tags, &c.mibs)
		cards = append(cards, c)
	}

	// 按指标集合分组
	groups := map[string][]string{} // fingerprint → []卡名
	for _, c := range cards {
		key := fmt.Sprintf("%d个指标", len(c.mibs))
		groups[key] = append(groups[key], c.sn+":"+c.tags)
	}
	for k, v := range groups {
		fmt.Printf("  %s → %d 张卡\n", k, len(v))
	}

	// 如果有差异，打印第一张和最后一张的独有指标
	if len(cards) >= 2 {
		first := toSet(cards[0].mibs)
		last := toSet(cards[len(cards)-1].mibs)
		fmt.Printf("\n  卡[%s:%s] 独有指标:\n", cards[0].sn, cards[0].tags)
		for m := range first {
			if _, ok := last[m]; !ok {
				fmt.Printf("    + %s\n", m)
			}
		}
		fmt.Printf("  卡[%s:%s] 独有指标:\n", cards[len(cards)-1].sn, cards[len(cards)-1].tags)
		for m := range last {
			if _, ok := first[m]; !ok {
				fmt.Printf("    + %s\n", m)
			}
		}
	}
}

// 5. 按 source 分组
func sourceOverview(ctx context.Context, conn driver.Conn, table string) {
	q := fmt.Sprintf(`
		SELECT source, count(DISTINCT sn) AS nodes, count(DISTINCT concat(sn,':',tags)) AS gpus, count() AS rows
		FROM %s
		GROUP BY source`, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var source string
		var nodes, gpus, total uint64
		rows.Scan(&source, &nodes, &gpus, &total)
		fmt.Printf("  source=%s → %d节点, %d张卡, %d条记录\n", source, nodes, gpus, total)
	}
}

func toSet(arr []string) map[string]struct{} {
	m := make(map[string]struct{}, len(arr))
	for _, s := range arr {
		m[s] = struct{}{}
	}
	return m
}

func connect() (driver.Conn, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"127.0.0.1:9000"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "123456",
		},
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}
	return conn, nil
}
