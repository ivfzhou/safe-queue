# 一、说明

[![codecov](https://codecov.io/gh/ivfzhou/safe-queue/graph/badge.svg?token=QYBRAOTH5K)](https://codecov.io/gh/ivfzhou/safe-queue)
[![Go Reference](https://pkg.go.dev/badge/gitee.com/ivfzhou/safe-queue.svg)](https://pkg.go.dev/gitee.com/ivfzhou/safe-queue)

**无锁（Lock-Free）、并发安全、支持阻塞等待的泛型 FIFO 环形队列。**

基于 **CAS + 自旋等待 + 序列号（Sequence）** 实现的高性能无锁队列，使用缓存行填充（Cache-Line Padding）避免伪共享（False Sharing），适用于高并发生产者-消费者场景。

---

# 二、特性

- **无锁设计**：基于 CAS 原子操作与自旋等待，无互斥锁（Mutex/RWMutex）开销
- **并发安全**：多生产者多消费者（MPMC）安全，经溢出测试验证（`uint32` 溢出边界）
- **泛型支持**：Go 1.18+ 泛型实现，`Queue[E any]` 支持任意元素类型
- **阻塞等待**：提供 `MustPut` / `MustGet` 方法，队列满/空时自动阻塞重试
- **批量操作**：支持 `PutEnough` / `GetEnough` 批量读写，提升吞吐量
- **缓存行优化**：对 `headIndex`、`tailIndex` 及每个 `element` 进行缓存行填充，消除伪共享
- **容量自动对齐**：创建队列时自动向上对齐为 2 的幂次方，支持位运算取模

# 三、安装

```shell
go get gitee.com/ivfzhou/safe-queue@latest
```


# 四、快速开始

```go
package main

import (
    "fmt"
    queue "gitee.com/ivfzhou/safe-queue"
)

type Task struct {
    ID int
}

func main() {
    // 创建容量为 256 的队列（实际会向上对齐到最近的 2 的幂）
    q := queue.New[*Task](256)

    // 写入单个元素
    left, err := q.Put(&Task{ID: 1})
    // left: 剩余可写入个数, err: 队列满时返回 ErrQueueIsFull

    // 批量写入
    inserted, left := q.PutEnough(&Task{ID: 2}, &Task{ID: 3})
    // inserted: 实际写入个数, left: 剩余可写入个数

    // 读取单个元素
    value, used, err := q.Get()
    // value: 元素值, used: 剩余可读取个数, err: 队列空时返回 ErrQueueIsEmpty

    // 批量读取
    elements, got, left := q.GetEnough(10)
    // elements: 元素切片, got: 实际读取个数, left: 剩余可读取个数

    fmt.Println(q.Len())   // 当前元素数量
    fmt.Println(q.Cap())   // 队列总容量
    fmt.Println(q.IsEmpty()) // 是否为空
    fmt.Println(q.IsFull())  // 是否已满
}
```

### 阻塞式用法

当需要队列满时阻塞写入、队列空时阻塞读取时，使用 `MustPut` 和 `MustGet`：

```go
// 生产者：队列满时自动阻塞等待
q.MustPut(&Task{ID: 42})

// 消费者：队列空时自动阻塞等待
task, _ := q.MustGet()
```

> 典型用法：配合 goroutine 构建生产者-消费者模型。

# 五、使用场景

本库适用于需要 **多线程并发读写** 且追求 **低延迟、高吞吐** 的场景，典型包括：

## 1. 生产者-消费者任务队列（Worker Pool）

将耗时任务投递到队列，由固定数量的 worker goroutine 并发消费，天然解决任务分发与负载均衡问题：

```go
tasks := queue.New[*Job](1024)

// 启动 N 个 worker 消费任务
for i := 0; i < runtime.NumCPU(); i++ {
    go func() {
        for {
            job, _ := tasks.MustGet() // 队列空时阻塞等待
            job.Do()
        }
    }()
}

// 生产者并发投递任务
tasks.MustPut(&Job{...})
```

## 2. 日志采集与异步落盘

高并发请求产生的日志先写入内存队列，由后台 goroutine 批量取出并写入磁盘/远端，解耦业务逻辑与 IO 耗时：

```go
logQueue := queue.New[*LogEntry](1 << 16)

// 业务方无阻塞地记录日志
logQueue.Put(&LogEntry{Time: time.Now(), Msg: msg})

// 后台批量落盘，减少 IO 次数
go func() {
    for {
        batch, _, _ := logQueue.GetEnough(512)
        flush(batch)
    }
}()
```

## 3. 事件驱动 / 消息分发缓冲

作为事件源与消费者之间的中间缓冲，平滑上游突发流量，避免下游被瞬时洪峰压垮（削峰填谷）。

## 4. 数据管道（Pipeline）各级之间的缓冲

在流水线处理中，把每个处理阶段的输出接入队列，作为下一阶段的输入，实现各级并行解耦：

```go
stage1Out := queue.New[RawData](4096)
stage2Out := queue.New[Result](4096)

// Stage1 -> Stage2 -> Stage3 之间通过队列衔接
```

## 5. 高频消息 / 指标采集

游戏服务器、实时交易、监控埋点等对延迟敏感的系统，利用无锁 + 缓存行优化避免锁竞争与伪共享带来的性能损耗。

> 注意：`MustPut` / `MustGet` 采用自旋等待实现阻塞，适合持有时间极短、竞争强度高的场景；若单次处理耗时较长（如执行磁盘/网络 IO），建议配合 `Put` / `Get` 自行控制重试与退避策略，避免忙等占用 CPU。

# 六、设计细节

### 数据结构

本库采用 **环形数组 + 序列号** 的经典无锁队列设计（类似 [LMAX Disruptor](https://lmax-exchange.github.io/disruptor/) 的核心思想）：

1. **环形缓冲区**：内部维护一个长度为 2^n 的 `[]element` 数组，通过位运算 (`position & mask`) 实现高效循环索引
2. **序列号控制**：每个槽位（slot）维护独立的 `putSequence` 和 `getSequence`，生产者和消费者通过 CAS 操作竞争位置，并通过序列号自旋等待确保可见性
3. **CAS 分配**：`acquirePut` / `acquireGet` 通过 `atomic.CompareAndSwapUint32` 原子地抢占总位置，失败则让出 CPU（`runtime.Gosched()`）
4. **缓存行填充**：`headIndex`、`tailIndex` 及各 `element` 字段之间插入 `cacheLinePadSize` 字节填充，防止不同 CPU 核心之间的伪共享

### 容量对齐

调用 `New` 时传入的 capacity 会被向上舍入到最近的 2 的幂次方（最小为 2）。例如：
- 传入 5 → 实际容量 8
- 传入 256 → 实际容量 256
- 传入 1000 → 实际容量 1024

这保证了 `mask = capacity - 1` 后，可以用 `position & mask` 代替取模运算。

### 并发安全保证

- 测试覆盖了 `uint32` 溢出边界场景（`TestOverflow`），验证了索引回绕后的正确性
- 多生产者多消费者并发测试（`TestConcurrent`）验证数据不丢失、不重复
