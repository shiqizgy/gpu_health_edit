package assistant

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goccy/go-json"
)

// LLMClient 大模型客户端接口。
// 第一版只用 ChatStream 做纯流式对话(tools 传 nil);
// 未来 function calling 时,tools 传工具定义,onDelta 之外再处理 tool_calls。
type LLMClient interface {
	ChatStream(ctx context.Context, messages []Message, tools []Tool, onDelta func(string)) error
}

// DeepSeekClient DeepSeek 实现(OpenAI 兼容接口)
type DeepSeekClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewDeepSeekClient(baseURL, apiKey, model string, timeoutSec int) *DeepSeekClient {
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return &DeepSeekClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// ChatStream 流式对话。每收到一段文字,调用 onDelta 回调。
func (c *DeepSeekClient) ChatStream(ctx context.Context, messages []Message, tools []Tool, onDelta func(string)) error {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Tools:    tools, // 第一版为 nil
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求 DeepSeek 失败: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("关闭失败: %w", err)
		}
	}()

	// 非 200:读出错误体返回
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var er errorResponse
		if json.Unmarshal(body, &er) == nil && er.Error.Message != "" {
			return fmt.Errorf("DeepSeek 错误(%d): %s", resp.StatusCode, er.Error.Message)
		}
		return fmt.Errorf("DeepSeek 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	// 逐行解析 SSE 流
	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("读取流失败: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue // SSE 的空行分隔
		}
		// SSE 格式: "data: {...}"
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break // DeepSeek 流结束标记
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // 跳过解析失败的分块(容错)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta != "" {
			onDelta(delta) // 把这段文字回调出去(交给 SSE 写给前端)
		}
	}
	return nil
}
