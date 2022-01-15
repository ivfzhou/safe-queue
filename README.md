## 说明

这是一个高性能的多协程安全的 FIFO 队列。

## 快速使用
```golang
// 初始化队列
q := New[int](1>>8) // 容量为256.
// 往队列填充数据
q.Put(1)
// 往队列填充多个数据
q.PutEnough(2, 3, 4)
// 取出最开始填的一个数据
q.Get() // 1
// 查看此时队列里数据个数
q.Len()
// 取出多个数据
data, _, _ := q.GetEnough(3)
fmt.Println(data) // []int{2, 3, 4}
```

## 联系

wxid: zivfzhou

email: ivfzhou@aliyun.com