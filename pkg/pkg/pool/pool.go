package pool

import (
	"runtime"
	"sync"
)

// 使用长期复用的有界协程池。启动时固定开个数为“workers”个worker goroutine
// 之后每轮评分复用，不用每轮创建上万个goroutine

type Pool struct {
	tasks   chan func()
	workers int
}

// New 创建协程池。workers<0 时取cpu核数
func New(workers int) *Pool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	p := &Pool{
		tasks:   make(chan func(), workers),
		workers: workers,
	}
	for i := 0; i < workers; i++ {
		go func() {
			for t := range p.tasks {
				t()
			}
		}()
	}
	return p
}

//Workers返回worker数

func (p *Pool) Workers() int {
	return p.workers
}

//Partition把[0,n)切成个数为“workers”的连续区间，并行执行fn(start,end),阻塞到全部完成
//只往channel发workers个任务（不是n个），避免上万次channel传递和闭包分配

func (p *Pool) Partition(n int, fn func(start, end int)) {
	if n <= 0 {
		return
	}
	chunks := p.workers
	if chunks > n {
		chunks = n
	}
	size := (n + chunks - 1) / chunks
	var wg sync.WaitGroup
	for start := 0; start < n; start += size {
		end := start + size
		if end > n {
			end = n
		}
		s, e := start, end
		wg.Add(1)
		p.tasks <- func() {
			defer wg.Done()
			fn(s, e)
		}
	}
	wg.Wait()
}

// Close关闭协程池（进程退出时调用）
func (p *Pool) Close() {
	close(p.tasks)
}

//想用现成轮子的话，可以把这层换成 github.com/panjf2000/ants/v2 ，调用方接口保持Partition不变即可；但要go get，自实现则零新依赖。
