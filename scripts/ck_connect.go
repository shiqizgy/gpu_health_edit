package main

//
//import (
//	"context"
//	"fmt"
//	"log"
//	"time"
//
//	"github.com/ClickHouse/clickhouse-go/v2"
//	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
//)
//
//func main() {
//	// 建立连接
//	conn, err := connect()
//	if err != nil {
//		log.Fatalf("连接失败: %v", err)
//	}
//
//	ctx := context.Background()
//
//	// 执行一个简单查询测试连接
//	var result uint8
//	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
//		log.Fatalf("查询失败: %v", err)
//	}
//
//	fmt.Printf("✅ TCP 连接成功！查询结果为: %d\n", result)
//}
//
//func connect() (driver.Conn, error) {
//	// 配置连接选项
//	options := &clickhouse.Options{
//		// 关键：设置 TCP 协议的地址和端口
//		Addr: []string{"ckpub332.olap.jd.com:2000"}, // 请确认端口是否为 9000[reference:4][reference:5]
//
//		// 认证信息
//		Auth: clickhouse.Auth{
//			Database: "monitor",              // 数据库名
//			Username: "read_db_monitor",      // 用户名
//			Password: "OvOYQmI39wzWTUsxPs-E", // 密码
//		},
//
//		// 客户端信息（可选）
//		ClientInfo: clickhouse.ClientInfo{
//			Products: []struct {
//				Name    string
//				Version string
//			}{
//				{Name: "my-go-app", Version: "0.1"},
//			},
//		},
//
//		// 连接超时设置
//		DialTimeout: 10 * time.Second,
//		Debug:       true, // 开启调试，方便查看日志
//	}
//
//	// 打开连接
//	conn, err := clickhouse.Open(options)
//	if err != nil {
//		return nil, err
//	}
//
//	// Ping 一下，验证连接是否真正可用
//	ctx := context.Background()
//	if err := conn.Ping(ctx); err != nil {
//		if exception, ok := err.(*clickhouse.Exception); ok {
//			fmt.Printf("连接异常 [%d]: %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
//		}
//		return nil, err
//	}
//
//	return conn, nil
//}
