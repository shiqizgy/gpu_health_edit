package redisclient

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gpu-health/platform/internal/config"
	"github.com/gpu-health/platform/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// Client 封装 Redis，作为仿真服务 → 评分服务的实时指标通道。
//
// 设计说明：
//   - 选 Redis 而非 Kafka：本场景是"每张卡保留最新一条全量指标供评分读取"，
//     属于 KV 覆盖写，Redis Hash 完美契合，本地负载极小。
//   - key 设计：gpu:metrics:{uuid} → 一个 Hash，field=指标key，value=数值。
//     用 Hash 而非 String，便于单指标读取和整卡读取，也省内存。
//   - 仿真服务每分钟 HSet 覆盖写入，评分服务每分钟读取。带 TTL 防止脏数据堆积。
type Client struct {
	rdb *redis.Client
}

const metricKeyPrefix = "gpu:metrics:"

// 初始化redis
func New(cfg config.RedisConfig) (*Client, error) {
	//初始化了一个结构体对象，配置了地址和密码
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	//给接下来的 Ping 操作设置一个 5 秒的硬性截止时间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		//连接失败时关闭客户端，避免资源泄漏
		if err := rdb.Close(); err != nil {
			logger.L.Errorf("客户端关闭失败: %v", err)
		}
		return nil, err
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error { return c.rdb.Close() }

// MetricFrame 一张卡某时刻的全量指标
type MetricFrame struct {
	UUID    string             `json:"uuid"`
	TS      int64              `json:"ts"`
	Metrics map[string]float64 `json:"metrics"`
}

// WriteFrame 仿真服务写入一张卡的全量指标（覆盖最新态）。
func (c *Client) WriteFrame(ctx context.Context, frame MetricFrame, ttl time.Duration) error {
	key := metricKeyPrefix + frame.UUID
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	// 用 String 存整帧 JSON：读取时一次拿全量，最简单高效。
	return c.rdb.Set(ctx, key, payload, ttl).Err()
}

// WriteFramePipeline 批量写入（仿真 2000 卡时用管道，大幅减少往返）。
func (c *Client) WriteFramePipeline(ctx context.Context, frames []MetricFrame, ttl time.Duration) error {
	pipe := c.rdb.Pipeline()
	for _, f := range frames {
		payload, err := json.Marshal(f)
		if err != nil {
			return err
		}
		pipe.Set(ctx, metricKeyPrefix+f.UUID, payload, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ReadAllFrames 评分服务读取所有卡的最新指标。
// 用 SCAN 遍历 key（生产安全，不阻塞），再批量 MGet 取值。
func (c *Client) ReadAllFrames(ctx context.Context) ([]MetricFrame, error) {
	var keys []string
	var cursor uint64
	for {
		batch, cur, err := c.rdb.Scan(ctx, cursor, metricKeyPrefix+"*", 500).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = cur
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}

	frames := make([]MetricFrame, 0, len(keys))
	// 分批 MGet，避免一次性参数过多
	const batchSize = 500
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		vals, err := c.rdb.MGet(ctx, keys[i:end]...).Result()
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			s, ok := v.(string)
			if !ok {
				continue
			}
			var f MetricFrame
			if err = json.Unmarshal([]byte(s), &f); err != nil {
				continue
			}
			frames = append(frames, f)
		}
	}
	return frames, nil
}

// ReadFrame 按 UUID 读取单张卡的最新指标
func (c *Client) ReadFrame(ctx context.Context, uuid string) (*MetricFrame, error) {
	key := metricKeyPrefix + uuid
	s, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // 该卡暂无实时数据
	}
	if err != nil {
		return nil, err
	}
	var f MetricFrame
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// FaultMode 故障注入相关（演示用）。
// key: gpu:fault:{uuid} → mode 字符串
const faultKeyPrefix = "gpu:fault:"

// faultInjectTTL 故障注入意图的存活时间：演示用，过期自动清理，避免 key 永久堆积。
const faultInjectTTL = 6 * time.Hour

func (c *Client) SetFault(ctx context.Context, uuid, mode string) error {
	if mode == "" || mode == "healthy" {
		return c.rdb.Del(ctx, faultKeyPrefix+uuid).Err()
	}
	return c.rdb.Set(ctx, faultKeyPrefix+uuid, mode, faultInjectTTL).Err()
}

func (c *Client) GetFault(ctx context.Context, uuid string) (string, error) {
	v, err := c.rdb.Get(ctx, faultKeyPrefix+uuid).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (c *Client) ListFaults(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	var cursor uint64
	for {
		batch, cur, err := c.rdb.Scan(ctx, cursor, faultKeyPrefix+"*", 200).Result()
		if err != nil {
			return nil, err
		}
		for _, k := range batch {
			uuid := k[len(faultKeyPrefix):]
			mode, _ := c.rdb.Get(ctx, k).Result()
			out[uuid] = mode
		}
		cursor = cur
		if cursor == 0 {
			break
		}
	}
	return out, nil
}
