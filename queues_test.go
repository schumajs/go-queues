/*
  Copyright 2016 Jens Schumann <schumajs@gmail.com>

  Use of this source code is governed by the MIT license that can be found in
  the LICENSE file.
*/

package queues

import (
	"io/ioutil"
	"testing"

	"github.com/boltdb/bolt"
)

type queueFunc func() (Queue, error)

var queues = map[string]queueFunc{
	"Queue": queueFunc(func() (Queue, error) {
		return NewQueue()
	}),
	"BoltDbQueue": queueFunc(func() (Queue, error) {
		tempFile, err := ioutil.TempFile("", "")
		if err != nil {
			return nil, err
		}

		tempFilePath := tempFile.Name()

		err = tempFile.Close()
		if err != nil {
			return nil, err
		}

		db, err := bolt.Open(tempFilePath, 0600, nil)
		if err != nil {
			return nil, err
		}

		return NewBoltDbQueue(db, []interface{}{int(0)})
	}),
}

func testQueue(t *testing.T, queue queueFunc) {
	nonNegativeInts := func(count int) Dequeuer {
		outQ, err := queue()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			defer outQ.Close()

			for i := 0; i < count; i++ {
				err = outQ.Enqueue(i)
				if err != nil {
					t.Fatal(err)
				}
			}
		}()

		return outQ
	}

	nonNegativeIntsDeq := nonNegativeInts(1000)

	for i := 0; ; i++ {
		v, closed, err := nonNegativeIntsDeq.Dequeue(true)
		switch {
		case closed:
			return
		case err != nil:
			t.Fatal(err)
		}

		if v.(int) != i {
			t.Errorf("%d expected %d", v, i)
		}

		_, closed, err = nonNegativeIntsDeq.Dequeue()
		switch {
		case closed:
			t.Errorf("expected queue to be open")
		case err != nil:
			t.Fatal(err)
		}
	}
}

func TestQueue(t *testing.T) {
	for name, queue := range queues {
		t.Run(name, func(t *testing.T) {
			testQueue(t, queue)
		})
	}
}

func testMerge(t *testing.T, queue queueFunc) {
	nonNegativeInts := func(count int) Dequeuer {
		outQ, err := queue()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			defer outQ.Close()

			for i := 0; i < count; i++ {
				err = outQ.Enqueue(i)
				if err != nil {
					t.Fatal(err)
				}
			}
		}()

		return outQ
	}

	square := func(inDeq Dequeuer) Dequeuer {
		outQ, err := queue()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			defer outQ.Close()

			for {
				v, closed, err := inDeq.Dequeue()
				switch {
				case closed:
					return
				case err != nil:
					t.Fatal(err)
				}

				outQ.Enqueue(v.(int) * v.(int))
				if err != nil {
					t.Fatal(err)
				}
			}
		}()

		return outQ
	}

	nonNegativeIntsDeq := nonNegativeInts(1000)

	squaresDeq1 := square(nonNegativeIntsDeq)

	squaresDeq2 := square(nonNegativeIntsDeq)

	squaresDeq, err := NewMultiplexedDequeuer(squaresDeq1, squaresDeq2)
	if err != nil {
		t.Fatal(err)
	}

	sum := 0

	for i := 0; ; i++ {
		v, closed, err := squaresDeq.Dequeue(true)
		if closed {
			break
		}
		if err != nil {
			t.Fatal(err)
		}

		sum = sum + v.(int)

		_, closed, err = squaresDeq.Dequeue()
		switch {
		case closed:
			t.Errorf("expected queue to be open")
		case err != nil:
			t.Fatal(err)
		}
	}

	if sum != 332833500 {
		t.Errorf("%d expected 332833500", sum)
	}
}

func TestMerge(t *testing.T) {
	for name, queue := range queues {
		t.Run(name, func(t *testing.T) {
			testMerge(t, queue)
		})
	}
}

type chanQueue struct {
	elements chan interface{}
}

func newChanQueue() (Queue, error) {
	q := &chanQueue{}

	q.elements = make(chan interface{})

	return q, nil
}

func (q *chanQueue) Dequeue(peek ...bool) (interface{}, bool, error) {
	// peek not supported

	v, open := <-q.elements

	return v, !open, nil
}

func (q *chanQueue) Enqueue(v interface{}) error {
	q.elements <- v

	return nil
}

func (q *chanQueue) Close() error {
	close(q.elements)

	return nil
}

func benchmarkQueue(b *testing.B, queue queueFunc) {
	nonNegativeInts := func(count int) Dequeuer {
		outQ, err := queue()
		if err != nil {
			b.Fatal(err)
		}

		go func() {
			defer outQ.Close()

			for i := 0; i < count; i++ {
				err = outQ.Enqueue(i)
				if err != nil {
					b.Fatal(err)
				}
			}
		}()

		return outQ
	}

	nonNegativeIntsDeq := nonNegativeInts(100000)

	for i := 0; ; i++ {
		v, closed, err := nonNegativeIntsDeq.Dequeue()
		switch {
		case closed:
			return
		case err != nil:
			b.Fatal(err)
		}

		if v.(int) != i {
			b.Errorf("%d expected %d", v, i)
		}
	}
}

func BenchmarkQueue(b *testing.B) {
	queues["ChanQueue"] = queueFunc(func() (Queue, error) {
		return newChanQueue()
	})
	defer delete(queues, "chanQueue")

	for name, queue := range queues {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchmarkQueue(b, queue)
			}
		})
	}
}
