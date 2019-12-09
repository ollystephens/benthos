// Copyright (c) 2019 Ashley Jeffs
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

package output

import (
	"github.com/ollystephens/benthos/v3/lib/log"
	"github.com/ollystephens/benthos/v3/lib/metrics"
	"github.com/ollystephens/benthos/v3/lib/output/writer"
	"github.com/ollystephens/benthos/v3/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeTCP] = TypeSpec{
		constructor: NewTCP,
		description: `
Sends messages as a continuous stream of line delimited data over TCP by
connecting to a server.

If batched messages are sent the final message of the batch will be followed by
two line breaks in order to indicate the end of the batch.`,
	}
}

// NewTCP creates a new TCP output type.
func NewTCP(conf Config, mgr types.Manager, log log.Modular, stats metrics.Type) (Type, error) {
	t, err := writer.NewTCP(conf.TCP, mgr, log, stats)
	if err != nil {
		return nil, err
	}
	return NewWriter(TypeTCP, t, log, stats)
}

//------------------------------------------------------------------------------
