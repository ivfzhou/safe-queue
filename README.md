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

> **要求**：Go 1.26+

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

# 五、API 参考

### 创建队列

| 方法 | 说明 |
|------|------|
| `New[E any](capacity uint32) *Queue[E]` | 创建指定容量的队列。容量会自动向上对齐到 2 的幂次方，最小值为 2 |

### 写入操作

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Put(value E)` | `(leftSize uint32, err error)` | 写入一个元素。成功返回剩余可写个数；队列为空时返回 `ErrQueueIsFull` |
| `PutEnough(values ...E)` | `(actualInsertedSize uint32, leftSize uint32)` | 批量写入多个元素。返回实际写入个数和剩余可写个数；空间不足时会部分写入 |
| `MustPut(value E)` | `(leftSize uint32)` | 写入一个元素，队列已满时自旋阻塞直到成功。返回剩余可写个数 |

### 读取操作

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Get()` | `(value E, usedSize uint32, err error)` | 读取一个元素。返回元素值、剩余可读个数；队列为空时返回 `ErrQueueIsEmpty` |
| `GetEnough(size uint32)` | `(elements []E, actualGetSize uint32, leftSize uint32)` | 批量读取最多 size 个元素。返回元素切片、实际读取个数、剩余可读个数 |
| `MustGet()` | `(value E, leftSize uint32)` | 读取一个元素，队列为空时自旋阻塞直到有数据。返回元素值和剩余可读个数 |

### 查询操作

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Cap()` | `uint32` | 返回队列总容量 |
| `Len()` | `uint32` | 返回当前队列中的元素数量（原子读取） |
| `IsEmpty()` | `bool` | 判断队列是否为空 |
| `IsFull()` | `bool` | 判断队列是否已满 |
| `String()` | `string` | 返回队列的状态字符串（Head/Tail/Len/Cap） |

### 错误类型

| 变量 | 说明 |
|------|------|
| `ErrQueueIsFull` | 队列已满，无法继续写入 |
| `ErrQueueIsEmpty` | 队列为空，无法继续读取 |

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
