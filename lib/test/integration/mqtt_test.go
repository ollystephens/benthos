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

package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ollystephens/benthos/v3/lib/input/reader"
	"github.com/ollystephens/benthos/v3/lib/log"
	"github.com/ollystephens/benthos/v3/lib/message"
	"github.com/ollystephens/benthos/v3/lib/metrics"
	"github.com/ollystephens/benthos/v3/lib/output/writer"
	"github.com/ollystephens/benthos/v3/lib/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/ory/dockertest"
)

func TestMQTTIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	t.Skip("Skipping MQTT tests because the library crashes on shutdown")

	t.Parallel()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("Could not connect to docker: %s", err)
	}
	pool.MaxWait = time.Second * 30

	resource, err := pool.Run("ncarlier/mqtt", "latest", nil)
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}
	defer func() {
		if err = pool.Purge(resource); err != nil {
			t.Logf("Failed to clean up docker resource: %v", err)
		}
	}()
	resource.Expire(900)

	url := fmt.Sprintf("tcp://localhost:%v", resource.GetPort("1883/tcp"))

	if err = pool.Retry(func() error {
		inConf := mqtt.NewClientOptions().
			SetClientID("UNIT_TEST")
		inConf = inConf.AddBroker(url)

		mIn := mqtt.NewClient(inConf)
		tok := mIn.Connect()
		tok.Wait()
		if cErr := tok.Error(); cErr != nil {
			return cErr
		}
		mIn.Disconnect(0)
		return nil
	}); err != nil {
		t.Fatalf("Could not connect to docker resource: %s", err)
	}

	t.Run("TestMQTTSinglePart", func(te *testing.T) {
		testMQTTSinglePart(url, te)
	})
	t.Run("TestMQTTMultiplePart", func(te *testing.T) {
		testMQTTMultiplePart(url, te)
	})
	t.Run("TestMQTTDisconnect", func(te *testing.T) {
		testMQTTDisconnect(url, te)
	})
}

func createMQTTInputOutput(
	inConf reader.MQTTConfig, outConf writer.MQTTConfig,
) (mInput reader.Type, mOutput writer.Type, err error) {
	if mInput, err = reader.NewMQTT(inConf, log.Noop(), metrics.Noop()); err != nil {
		return
	}
	if err = mInput.Connect(); err != nil {
		return
	}
	if mOutput, err = writer.NewMQTT(outConf, log.Noop(), metrics.Noop()); err != nil {
		return
	}
	if err = mOutput.Connect(); err != nil {
		return
	}
	return
}

func testMQTTSinglePart(url string, t *testing.T) {
	inConf := reader.NewMQTTConfig()
	inConf.ClientID = "foo"
	inConf.Topics = []string{"test_input_1"}
	inConf.URLs = []string{url}

	outConf := writer.NewMQTTConfig()
	outConf.ClientID = "bar"
	outConf.Topic = "test_input_1"
	outConf.URLs = []string{url}

	mInput, mOutput, err := createMQTTInputOutput(inConf, outConf)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		mInput.CloseAsync()
		if cErr := mInput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
		mOutput.CloseAsync()
		if cErr := mOutput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
	}()

	N := 10

	wg := sync.WaitGroup{}
	wg.Add(N)

	testMsgs := map[string]struct{}{}
	for i := 0; i < N; i++ {
		str := fmt.Sprintf("hello world: %v", i)
		testMsgs[str] = struct{}{}
		go func(testStr string) {
			msg := message.New([][]byte{
				[]byte(testStr),
			})
			msg.Get(0).Metadata().Set("foo", "bar")
			msg.Get(0).Metadata().Set("root_foo", "bar2")
			if gerr := mOutput.Write(msg); gerr != nil {
				t.Fatal(gerr)
			}
			wg.Done()
		}(str)
	}

	lMsgs := len(testMsgs)
	for lMsgs > 0 {
		var actM types.Message
		actM, err = mInput.Read()
		if err != nil {
			t.Error(err)
		} else {
			act := string(actM.Get(0).Get())
			if _, exists := testMsgs[act]; !exists {
				t.Errorf("Unexpected message: %v", act)
			}
			delete(testMsgs, act)
		}
		if err = mInput.Acknowledge(nil); err != nil {
			t.Error(err)
		}
		lMsgs = len(testMsgs)
	}

	wg.Wait()
}

func testMQTTMultiplePart(url string, t *testing.T) {
	inConf := reader.NewMQTTConfig()
	inConf.ClientID = "foo"
	inConf.Topics = []string{"test_input_1"}
	inConf.URLs = []string{url}

	outConf := writer.NewMQTTConfig()
	outConf.ClientID = "bar"
	outConf.Topic = "test_input_1"
	outConf.URLs = []string{url}

	mInput, mOutput, err := createMQTTInputOutput(inConf, outConf)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		mInput.CloseAsync()
		if cErr := mInput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
		mOutput.CloseAsync()
		if cErr := mOutput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
	}()

	N := 10

	wg := sync.WaitGroup{}
	wg.Add(N)

	testMsgs := map[string]struct{}{}
	for i := 0; i < N; i++ {
		str1 := fmt.Sprintf("hello world: %v part 1", i)
		str2 := fmt.Sprintf("hello world: %v part 2", i)
		str3 := fmt.Sprintf("hello world: %v part 3", i)
		testMsgs[str1] = struct{}{}
		testMsgs[str2] = struct{}{}
		testMsgs[str3] = struct{}{}
		go func(testStr1, testStr2, testStr3 string) {
			msg := message.New([][]byte{
				[]byte(testStr1),
				[]byte(testStr2),
				[]byte(testStr3),
			})
			msg.Get(0).Metadata().Set("foo", "bar")
			msg.Get(1).Metadata().Set("root_foo", "bar2")
			if gerr := mOutput.Write(msg); gerr != nil {
				t.Fatal(gerr)
			}
			wg.Done()
		}(str1, str2, str3)
	}

	lMsgs := len(testMsgs)
	for lMsgs > 0 {
		var actM types.Message
		actM, err = mInput.Read()
		if err != nil {
			t.Error(err)
		} else {
			act := string(actM.Get(0).Get())
			if _, exists := testMsgs[act]; !exists {
				t.Errorf("Unexpected message: %v", act)
			}
			delete(testMsgs, act)
		}
		if err = mInput.Acknowledge(nil); err != nil {
			t.Error(err)
		}
		lMsgs = len(testMsgs)
	}

	wg.Wait()
}

func testMQTTDisconnect(url string, t *testing.T) {
	inConf := reader.NewMQTTConfig()
	inConf.ClientID = "foo"
	inConf.Topics = []string{"test_input_1"}
	inConf.URLs = []string{url}

	outConf := writer.NewMQTTConfig()
	outConf.ClientID = "bar"
	outConf.Topic = "test_input_1"
	outConf.URLs = []string{url}

	mInput, mOutput, err := createMQTTInputOutput(inConf, outConf)
	if err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		mInput.CloseAsync()
		if cErr := mInput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
		mOutput.CloseAsync()
		if cErr := mOutput.WaitForClose(time.Second); cErr != nil {
			t.Error(cErr)
		}
		wg.Done()
	}()

	if _, err = mInput.Read(); err != types.ErrTypeClosed && err != types.ErrNotConnected {
		t.Errorf("Wrong error: %v != %v", err, types.ErrTypeClosed)
	}

	wg.Wait()
}
