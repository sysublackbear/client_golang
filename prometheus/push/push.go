// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package push provides functions to push metrics to a Pushgateway. It uses a
// builder approach. Create a Pusher with New and then add the various options
// by using its methods, finally calling Add or Push, like this:
//
//    // Easy case:
//    push.New("http://example.org/metrics", "my_job").Gatherer(myRegistry).Push()
//
//    // Complex case:
//    push.New("http://example.org/metrics", "my_job").
//        Collector(myCollector1).
//        Collector(myCollector2).
//        Grouping("zone", "xy").
//        Client(&myHTTPClient).
//        BasicAuth("top", "secret").
//        Add()
//
// See the examples section for more detailed examples.
//
// See the documentation of the Pushgateway to understand the meaning of
// the grouping key and the differences between Push and Add:
// https://github.com/prometheus/pushgateway
package push

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/prometheus/client_golang/prometheus"
)

const contentTypeHeader = "Content-Type"

// HTTPDoer is an interface for the one method of http.Client that is used by Pusher
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Pusher manages a push to the Pushgateway. Use New to create one, configure it
// with its methods, and finally use the Add or Push method to push.
type Pusher struct {
	error error

	url, job string
	grouping map[string]string

	gatherers  prometheus.Gatherers
	registerer prometheus.Registerer

	client             HTTPDoer
	useBasicAuth       bool
	username, password string

	expfmt expfmt.Format
}

// New creates a new Pusher to push to the provided URL with the provided job
// name. You can use just host:port or ip:port as url, in which case “http://”
// is added automatically. Alternatively, include the schema in the
// URL. However, do not include the “/metrics/jobs/…” part.
//
// Note that until https://github.com/prometheus/pushgateway/issues/97 is
// resolved, a “/” character in the job name is prohibited.
func New(url, job string) *Pusher {
	var (
		reg = prometheus.NewRegistry()
		err error
	)
	if !strings.Contains(url, "://") {
		url = "http://" + url
	}
	if strings.HasSuffix(url, "/") {
		url = url[:len(url)-1]
	}
	if strings.Contains(job, "/") {
		err = fmt.Errorf("job contains '/': %s", job)
	}

	return &Pusher{
		error:      err,
		url:        url,
		job:        job,
		grouping:   map[string]string{},
		gatherers:  prometheus.Gatherers{reg},
		registerer: reg,
		client:     &http.Client{},
		expfmt:     expfmt.FmtProtoDelim,
	}
}

// Push collects/gathers all metrics from all Collectors and Gatherers added to
// this Pusher. Then, it pushes them to the Pushgateway configured while
// creating this Pusher, using the configured job name and any added grouping
// labels as grouping key. All previously pushed metrics with the same job and
// other grouping labels will be replaced with the metrics pushed by this
// call. (It uses HTTP method “PUT” to push to the Pushgateway.)
//
// Push returns the first error encountered by any method call (including this
// one) in the lifetime of the Pusher.
func (p *Pusher) Push() error {
	return p.push("PUT")
}

// Add works like push, but only previously pushed metrics with the same name
// (and the same job and other grouping labels) will be replaced. (It uses HTTP
// method “POST” to push to the Pushgateway.)
func (p *Pusher) Add() error {
	return p.push("POST")
}

// Gatherer adds a Gatherer to the Pusher, from which metrics will be gathered
// to push them to the Pushgateway. The gathered metrics must not contain a job
// label of their own.
//
// For convenience, this method returns a pointer to the Pusher itself.
func (p *Pusher) Gatherer(g prometheus.Gatherer) *Pusher {
	p.gatherers = append(p.gatherers, g)
	return p
}

// Collector adds a Collector to the Pusher, from which metrics will be
// collected to push them to the Pushgateway. The collected metrics must not
// contain a job label of their own.
//
// For convenience, this method returns a pointer to the Pusher itself.
func (p *Pusher) Collector(c prometheus.Collector) *Pusher {
	if p.error == nil {
		// 注册新的collector
		p.error = p.registerer.Register(c)
	}
	return p
}

// Grouping adds a label pair to the grouping key of the Pusher, replacing any
// previously added label pair with the same label name. Note that setting any
// labels in the grouping key that are already contained in the metrics to push
// will lead to an error.
//
// For convenience, this method returns a pointer to the Pusher itself.
//
// Note that until https://github.com/prometheus/pushgateway/issues/97 is
// resolved, this method does not allow a “/” character in the label value.
// 拼接url有用
func (p *Pusher) Grouping(name, value string) *Pusher {
	if p.error == nil {
		if !model.LabelName(name).IsValid() {
			p.error = fmt.Errorf("grouping label has invalid name: %s", name)
			return p
		}
		if strings.Contains(value, "/") {
			p.error = fmt.Errorf("value of grouping label %s contains '/': %s", name, value)
			return p
		}
		p.grouping[name] = value
	}
	return p
}

// Client sets a custom HTTP client for the Pusher. For convenience, this method
// returns a pointer to the Pusher itself.
// Pusher only needs one method of the custom HTTP client: Do(*http.Request).
// Thus, rather than requiring a fully fledged http.Client,
// the provided client only needs to implement the HTTPDoer interface.
// Since *http.Client naturally implements that interface, it can still be used normally.
func (p *Pusher) Client(c HTTPDoer) *Pusher {
	p.client = c
	return p
}

// BasicAuth configures the Pusher to use HTTP Basic Authentication with the
// provided username and password. For convenience, this method returns a
// pointer to the Pusher itself.
func (p *Pusher) BasicAuth(username, password string) *Pusher {
	p.useBasicAuth = true
	p.username = username
	p.password = password
	return p
}

// Format configures the Pusher to use an encoding format given by the
// provided expfmt.Format. The default format is expfmt.FmtProtoDelim and
// should be used with the standard Prometheus Pushgateway. Custom
// implementations may require different formats. For convenience, this
// method returns a pointer to the Pusher itself.
func (p *Pusher) Format(format expfmt.Format) *Pusher {
	p.expfmt = format
	return p
}

func (p *Pusher) push(method string) error {
	if p.error != nil {
		return p.error
	}
	urlComponents := []string{url.QueryEscape(p.job)}
	for ln, lv := range p.grouping {
		urlComponents = append(urlComponents, ln, lv)
	}
	// 计算推送数据url
	pushURL := fmt.Sprintf("%s/metrics/job/%s", p.url, strings.Join(urlComponents, "/"))

	// 收集数据（metricFamily）
	mfs, err := p.gatherers.Gather()
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	enc := expfmt.NewEncoder(buf, p.expfmt)
	// Check for pre-existing grouping labels:
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "job" {
					return fmt.Errorf("pushed metric %s (%s) already contains a job label", mf.GetName(), m)
				}
				if _, ok := p.grouping[l.GetName()]; ok {
					return fmt.Errorf(
						"pushed metric %s (%s) already contains grouping label %s",
						mf.GetName(), m, l.GetName(),
					)
				}
			}
		}
		enc.Encode(mf)  // 对每个MetricFamily编码，写到buffer里面
	}
	// 发起http方法调用
	req, err := http.NewRequest(method, pushURL, buf)
	if err != nil {
		return err
	}
	if p.useBasicAuth {
		req.SetBasicAuth(p.username, p.password)
	}
	req.Header.Set(contentTypeHeader, string(p.expfmt))
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		body, _ := ioutil.ReadAll(resp.Body) // Ignore any further error as this is for an error message only.
		return fmt.Errorf("unexpected status code %d while pushing to %s: %s", resp.StatusCode, pushURL, body)
	}
	return nil
}

// finish
