// Copyright (c) 2019 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, sub to the following conditions:
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

package metrics

import (
	"fmt"
	"time"

	"github.com/Jeffail/benthos/v3/lib/log"
	statsd "github.com/smira/go-statsd"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeStatsd] = TypeSpec{
		constructor: NewStatsd,
		description: `
Pushes metrics using the [StatsD protocol](https://github.com/statsd/statsd).
Supported tagging formats are 'none', 'datadog' and 'influxdb'.

The 'network' field is deprecated and scheduled for removal. If you currently
rely on sending Statsd metrics over TCP and want it to be supported long term
please [raise an issue](https://github.com/Jeffail/benthos/issues).

WARNING: The underlying client library has recently been changed. If you have
noticed regressions then you can force Benthos to use the old library by setting
the field ` + "`tag_format` to `legacy`" + `.`,
	}
}

//------------------------------------------------------------------------------

type wrappedDatadogLogger struct {
	log log.Modular
}

func (s wrappedDatadogLogger) Printf(msg string, args ...interface{}) {
	s.log.Warnf(fmt.Sprintf(msg, args...))
}

//------------------------------------------------------------------------------

// StatsdConfig is config for the Statsd metrics type.
type StatsdConfig struct {
	Prefix      string `json:"prefix" yaml:"prefix"`
	Address     string `json:"address" yaml:"address"`
	FlushPeriod string `json:"flush_period" yaml:"flush_period"`
	Network     string `json:"network" yaml:"network"`
	TagFormat   string `json:"tag_format" yaml:"tag_format"`
}

// NewStatsdConfig creates an StatsdConfig struct with default values.
func NewStatsdConfig() StatsdConfig {
	return StatsdConfig{
		Prefix:      "benthos",
		Address:     "localhost:4040",
		FlushPeriod: "100ms",
		Network:     "udp",
		TagFormat:   TagFormatNone,
	}
}

// Tag formats supported by the statsd metric type.
const (
	TagFormatNone     = "none"
	TagFormatDatadog  = "datadog"
	TagFormatInfluxDB = "influxdb"
)

//------------------------------------------------------------------------------

// StatsdStat is a representation of a single metric stat. Interactions with
// this stat are thread safe.
type StatsdStat struct {
	path string
	s    *statsd.Client
	tags []statsd.Tag
}

// Incr increments a metric by an amount.
func (s *StatsdStat) Incr(count int64) error {
	s.s.Incr(s.path, count, s.tags...)
	return nil
}

// Decr decrements a metric by an amount.
func (s *StatsdStat) Decr(count int64) error {
	s.s.Decr(s.path, count, s.tags...)
	return nil
}

// Timing sets a timing metric.
func (s *StatsdStat) Timing(delta int64) error {
	s.s.Timing(s.path, delta, s.tags...)
	return nil
}

// Set sets a gauge metric.
func (s *StatsdStat) Set(value int64) error {
	s.s.Gauge(s.path, value, s.tags...)
	return nil
}

//------------------------------------------------------------------------------

// Statsd is a stats object with capability to hold internal stats as a JSON
// endpoint.
type Statsd struct {
	config Config
	s      *statsd.Client
	log    log.Modular
}

// NewStatsd creates and returns a new Statsd object.
func NewStatsd(config Config, opts ...func(Type)) (Type, error) {
	if config.Statsd.Network != "udp" || config.Statsd.TagFormat == "legacy" {
		return NewStatsdLegacy(config, opts...)
	}

	flushPeriod, err := time.ParseDuration(config.Statsd.FlushPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to parse flush period: %s", err)
	}

	s := &Statsd{
		config: config,
		log:    log.Noop(),
	}
	for _, opt := range opts {
		opt(s)
	}

	prefix := config.Statsd.Prefix
	if len(prefix) > 0 && prefix[len(prefix)-1] != '.' {
		prefix = prefix + "."
	}

	var tagFormat *statsd.TagFormat

	if TagFormatInfluxDB == config.Statsd.TagFormat {
		tagFormat = statsd.TagFormatInfluxDB
	} else if TagFormatDatadog == config.Statsd.TagFormat {
		tagFormat = statsd.TagFormatDatadog
	} else {
		return nil, fmt.Errorf("tag format '%s' was not recognised", config.Statsd.TagFormat)
	}

	client := statsd.NewClient(config.Statsd.Address,
		statsd.FlushInterval(flushPeriod),
		statsd.MetricPrefix(prefix),
		statsd.TagStyle(tagFormat),
		statsd.Logger(wrappedDatadogLogger{log: s.log}))

	s.s = client
	return s, nil
}

//------------------------------------------------------------------------------

// GetCounter returns a stat counter object for a path.
func (h *Statsd) GetCounter(path string) StatCounter {
	return &StatsdStat{
		path: path,
		s:    h.s,
	}
}

// GetCounterVec returns a stat counter object for a path with the labels
func (h *Statsd) GetCounterVec(path string, n []string) StatCounterVec {
	return &fCounterVec{
		f: func(l []string) StatCounter {
			return &StatsdStat{
				path: path,
				s:    h.s,
				tags: tags(n, l),
			}
		},
	}
}

// GetTimer returns a stat timer object for a path.
func (h *Statsd) GetTimer(path string) StatTimer {
	return &StatsdStat{
		path: path,
		s:    h.s,
	}
}

// GetTimerVec returns a stat timer object for a path with the labels
func (h *Statsd) GetTimerVec(path string, n []string) StatTimerVec {
	return &fTimerVec{
		f: func(l []string) StatTimer {
			return &StatsdStat{
				path: path,
				s:    h.s,
				tags: tags(n, l),
			}
		},
	}
}

// GetGauge returns a stat gauge object for a path.
func (h *Statsd) GetGauge(path string) StatGauge {
	return &StatsdStat{
		path: path,
		s:    h.s,
	}
}

// GetGaugeVec returns a stat timer object for a path with the labels
func (h *Statsd) GetGaugeVec(path string, n []string) StatGaugeVec {
	return &fGaugeVec{
		f: func(l []string) StatGauge {
			return &StatsdStat{
				path: path,
				s:    h.s,
				tags: tags(n, l),
			}
		},
	}
}

// SetLogger sets the logger used to print connection errors.
func (h *Statsd) SetLogger(log log.Modular) {
	h.log = log
}

// Close stops the Statsd object from aggregating metrics and cleans up
// resources.
func (h *Statsd) Close() error {
	h.s.Close()
	return nil
}

// tags merges tag labels with their interpolated values
//
// no attempt is made to merge labels and values if slices
// are not the same length
func tags(labels []string, values []string) []statsd.Tag {
	if len(labels) != len(values) {
		return nil
	}
	tags := make([]statsd.Tag, len(labels))
	for i := range labels {
		tags[i] = statsd.StringTag(labels[i], values[i])
	}
	return tags
}

//------------------------------------------------------------------------------
