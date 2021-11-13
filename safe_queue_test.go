package safe_queue_test

import (
	"strconv"
	"sync"
	"testing"

	"gitee.com/ivfzhou/safe-queue"
)

func TestPutGet(t *testing.T) {
	d := safe_queue.New(4)

	if d.Cap() != 3 {
		t.Fatal("Cap fail")
	}

	s1 := "Hello1"
	s2 := "Hello2"
	s3 := "Hello3"
	remian, err := d.Put(s1)
	if err != nil || remian != 2 {
		t.Fatal(err)
	}
	remian, err = d.Put(s2)
	if err != nil || remian != 1 {
		t.Fatal(err)
	}
	remian, err = d.Put(s3)
	if err != nil || remian != 0 {
		t.Fatal(err)
	}

	if !d.IsFull() {
		t.Fatal("is full fail")
	}

	if d.Len() != 3 {
		t.Fatal("len fail")
	}

	s, left, err := d.Get()
	if s != s1 || err != nil || left != 2 {
		t.Fatal("get1 fail")
	}
	s, left, err = d.Get()
	if s != s2 || err != nil || left != 1 {
		t.Fatal("get2 fail")
	}
	s, left, err = d.Get()
	if s != s3 || err != nil || left != 0 {
		t.Fatal("get3 fail")
	}

	if !d.IsEmpty() {
		t.Fatal("is empty fail")
	}

	if d.Len() != 0 {
		t.Fatal("len fail")
	}

	remian, err = d.Put(s1)
	if err != nil || remian != 2 {
		t.Fatal(err)
	}
	remian, err = d.Put(s2)
	if err != nil || remian != 1 {
		t.Fatal(err)
	}
	remian, err = d.Put(s3)
	if err != nil || remian != 0 {
		t.Fatal(err)
	}

	more, u, err := d.GetMore(3)
	if len(more) != 3 || err != nil || u != 0 {
		t.Fatal("get more")
	}
}

func TestConcurrent(t *testing.T) {
	d := safe_queue.New(2048)
	wg := sync.WaitGroup{}
	for i := 0; i < 2047; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := d.Put(i)
			if err != nil {
				t.Error("put " + strconv.Itoa(i))
			}
		}(i)
	}
	for i := 0; i < 1047; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := d.Get()
			if err != nil {
				t.Error("get " + strconv.Itoa(i))
			}
		}(i)
	}

	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := d.GetMore(2)
			if err != nil {
				t.Error("get " + strconv.Itoa(i))
			}
		}(i)
	}
	wg.Wait()
	if d.Len() != 0 {
		t.Fatal("fail")
	}
}

func TestString(t *testing.T) {
	d := safe_queue.New(3)
	_, _ = d.Put("hello")
	t.Log(d)
}
