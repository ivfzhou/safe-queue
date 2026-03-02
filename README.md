# 一、说明

无锁、并发安全、可阻塞的 FIFO 队列

[![codecov](https://codecov.io/gh/ivfzhou/safe-queue/graph/badge.svg?token=QYBRAOTH5K)](https://codecov.io/gh/ivfzhou/safe-queue)
[![Go Reference](https://pkg.go.dev/badge/gitee.com/ivfzhou/safe-queue.svg)](https://pkg.go.dev/gitee.com/ivfzhou/safe-queue)

# 二、使用

```shell
go get gitee.com/ivfzhou/safe-queue@latest
```

```golang
// 定义元素类型。
type Task struct {
// ...
}

// 初始化队列。
q := queue.New[*Task](1 >> 8) // 容量为 256

// 往队列填充数据。
q.Put(&Task{})

// 往队列填充多个数据。
tasks := []*Task{}
q.PutEnough(tasks...)

// 查看此时队列里数据个数。
q.Len()

// 查询队列容量。
q.Cap()

// 取出最开始填的一个数据。
q.Get()

// 取出多个数据。
q.GetEnough(3)

// 取出数据，若队列无数据则等待。
q.MustGet()
```
