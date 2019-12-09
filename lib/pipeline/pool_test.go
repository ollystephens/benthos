// Copyright (c) 2018 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package pipeline

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ollystephens/benthos/v3/lib/log"
	"github.com/ollystephens/benthos/v3/lib/message"
	"github.com/ollystephens/benthos/v3/lib/metrics"
	"github.com/ollystephens/benthos/v3/lib/processor"
	"github.com/ollystephens/benthos/v3/lib/response"
	"github.com/ollystephens/benthos/v3/lib/types"
)

func TestPoolBasic(t *testing.T) {
	mockProc := &mockMsgProcessor{dropChan: make(chan bool)}

	go func() {
		mockProc.dropChan <- true
	}()

	constr := func(i *int) (types.Pipeline, error) {
		return NewProcessor(
			log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
			metrics.DudType{},
			mockProc,
		), nil
	}

	proc, err := NewPool(
		constr, 1,
		log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
		metrics.DudType{},
	)
	if err != nil {
		t.Fatal(err)
	}

	tChan, resChan := make(chan types.Transaction), make(chan types.Response)

	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}
	if err := proc.Consume(tChan); err == nil {
		t.Error("Expected error from dupe receiving")
	}

	msg := message.New([][]byte{
		[]byte(`one`),
		[]byte(`two`),
	})

	// First message should be dropped and return immediately
	select {
	case tChan <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second):
		t.Fatal("Timed out")
	}
	select {
	case _, open := <-proc.TransactionChan():
		if !open {
			t.Fatal("Closed early")
		} else {
			t.Fatal("Message was not dropped")
		}
	case res, open := <-resChan:
		if !open {
			t.Fatal("Closed early")
		}
		if res.Error() != errMockProc {
			t.Error(res.Error())
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	// Do not drop next message
	go func() {
		mockProc.dropChan <- false
	}()

	// Send message
	select {
	case tChan <- types.NewTransaction(msg, resChan):
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	var procT types.Transaction
	var open bool

	// Receive new message
	select {
	case procT, open = <-proc.TransactionChan():
		if !open {
			t.Error("Closed early")
		}
		if exp, act := [][]byte{[]byte("foo"), []byte("bar")}, message.GetAllBytes(procT.Payload); !reflect.DeepEqual(exp, act) {
			t.Errorf("Wrong message received: %s != %s", act, exp)
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	// Respond without error
	go func() {
		select {
		case procT.ResponseChan <- response.NewAck():
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}
	}()

	// Receive response
	select {
	case res, open := <-resChan:
		if !open {
			t.Error("Closed early")
		}
		if res.Error() != nil {
			t.Error(res.Error())
		}
	case <-time.After(time.Second * 5):
		t.Fatal("Timed out")
	}

	proc.CloseAsync()
	if err := proc.WaitForClose(time.Second * 5); err != nil {
		t.Error(err)
	}
}

func TestPoolMultiMsgs(t *testing.T) {
	mockProc := &mockMultiMsgProcessor{N: 3}

	constr := func(i *int) (types.Pipeline, error) {
		return NewProcessor(
			log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
			metrics.DudType{},
			mockProc,
		), nil
	}

	proc, err := NewPool(
		constr, 1,
		log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
		metrics.DudType{},
	)
	if err != nil {
		t.Fatal(err)
	}

	tChan, resChan := make(chan types.Transaction), make(chan types.Response)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	for j := 0; j < 10; j++ {
		expMsgs := map[string]struct{}{}
		for i := 0; i < mockProc.N; i++ {
			expMsgs[fmt.Sprintf("test%v", i)] = struct{}{}
		}

		// Send message
		select {
		case tChan <- types.NewTransaction(message.New(nil), resChan):
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		for i := 0; i < mockProc.N; i++ {
			// Receive messages
			var procT types.Transaction
			var open bool
			select {
			case procT, open = <-proc.TransactionChan():
				if !open {
					t.Error("Closed early")
				}
				act := string(procT.Payload.Get(0).Get())
				if _, exists := expMsgs[act]; !exists {
					t.Errorf("Unexpected result: %v", act)
				} else {
					delete(expMsgs, act)
				}
			case <-time.After(time.Second * 5):
				t.Fatal("Timed out")
			}

			// Respond with no error
			select {
			case procT.ResponseChan <- response.NewAck():
			case <-time.After(time.Second * 5):
				t.Fatal("Timed out")
			}

		}

		// Receive response
		select {
		case res, open := <-resChan:
			if !open {
				t.Error("Closed early")
			} else if res.Error() != nil {
				t.Error(res.Error())
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		if len(expMsgs) != 0 {
			t.Errorf("Expected messages were not received: %v", expMsgs)
		}
	}

	proc.CloseAsync()
	if err := proc.WaitForClose(time.Second * 5); err != nil {
		t.Error(err)
	}
}

func TestPoolMultiThreads(t *testing.T) {
	conf := NewConfig()
	conf.Threads = 2
	conf.Processors = append(conf.Processors, processor.NewConfig())

	proc, err := New(
		conf, nil,
		log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
		metrics.DudType{},
	)
	if err != nil {
		t.Fatal(err)
	}

	tChan, resChan := make(chan types.Transaction), make(chan types.Response)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	msg := message.New([][]byte{
		[]byte(`one`),
		[]byte(`two`),
	})

	for j := 0; j < conf.Threads; j++ {
		// Send message
		select {
		case tChan <- types.NewTransaction(msg, resChan):
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}
	}
	for j := 0; j < conf.Threads; j++ {
		// Receive messages
		var procT types.Transaction
		var open bool
		select {
		case procT, open = <-proc.TransactionChan():
			if !open {
				t.Error("Closed early")
			}
			if exp, act := [][]byte{[]byte("one"), []byte("two")}, message.GetAllBytes(procT.Payload); !reflect.DeepEqual(exp, act) {
				t.Errorf("Wrong message received: %s != %s", act, exp)
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}

		go func(tran types.Transaction) {
			// Respond with no error
			select {
			case tran.ResponseChan <- response.NewAck():
			case <-time.After(time.Second * 5):
				t.Fatal("Timed out")
			}
		}(procT)
	}
	for j := 0; j < conf.Threads; j++ {
		// Receive response
		select {
		case res, open := <-resChan:
			if !open {
				t.Error("Closed early")
			} else if res.Error() != nil {
				t.Error(res.Error())
			}
		case <-time.After(time.Second * 5):
			t.Fatal("Timed out")
		}
	}

	proc.CloseAsync()
	if err := proc.WaitForClose(time.Second * 5); err != nil {
		t.Error(err)
	}
}

func TestPoolMultiNaturalClose(t *testing.T) {
	conf := NewConfig()
	conf.Threads = 2
	conf.Processors = append(conf.Processors, processor.NewConfig())

	proc, err := New(
		conf, nil,
		log.New(os.Stdout, log.Config{LogLevel: "NONE"}),
		metrics.DudType{},
	)
	if err != nil {
		t.Fatal(err)
	}

	tChan := make(chan types.Transaction)
	if err := proc.Consume(tChan); err != nil {
		t.Fatal(err)
	}

	close(tChan)

	if err := proc.WaitForClose(time.Second * 5); err != nil {
		t.Error(err)
	}
}
