# Prometheus的golang版本client库（带注释）



## 1.简介

关于Prometheus的上报client库（golang版本），其中代码已经做了中文注释，更加方便理解代码。

本质上通过`Collect`，`Gather`方法，进行数据采集，返回。

该库主要由业务代码进行应用，可以搭建本地的HttpServer，暴露http接口，让Prometheus定时访问http接口，收集数据。（拉模式）也可以通过PushGateway进行主动推送，让Prometheus定时访问PushGateway收集数据（推模式）



更多结构细节请看：[这里](https://ryanyang.gitbook.io/prometheus/di-yi-zhang-jie-shao/overview)



## 2.几个重要的概念

- `Metric`：指标的基础接口，内含`Desc`和`Write`两个方法；
- `Collector`：收集器基础接口，内含`Describe`和`Collect`两个方法，一个用来收集`Desc`，一个用来收集`Metric`；
- `Gatherer`：聚集器基础接口，内含`Gather`方法，负责将各个`Collector`收集回来的`Metric`进行聚集操作。（`Metric`-->`dto.Metric`—>`dto.MetricFamily`)；
- `Registerer`：负责将`Collector`注册到`Gatherer`里面。一个`Gather`不同同时注册两个相同的id的`Collector`（注册时会报错），内含`Register`，`MustRegister`和`Unregister`三个方法；



Metric，Collector和Gatherer三者之间的关系：

- 一个Collector对应多个Metric，Collector的`Collect`方法负责将多个Metric收集到channel里面，Collector对相同的Metric不做去重工作，只管塞到channel里面；
- 一个Gatherer对应多个Collector，Gatherer的`Gather`方法会将下挂的所有的Collector的channel的Metric收集到一个静态值结构里面（`dto.Metric`），然后统一收集到`dto.MetricFamily`中。相同的Metric不做去重，打上时间戳，放到同个`MetricFamily`下挂的`[]*Metric `成员当中。



`dto.MetricFamily`的结构如下：

```protobuf
type MetricFamily struct {
	Name                 *string     `protobuf:"bytes,1,opt,name=name" json:"name,omitempty"`
	Help                 *string     `protobuf:"bytes,2,opt,name=help" json:"help,omitempty"`
	Type                 *MetricType `protobuf:"varint,3,opt,name=type,enum=io.prometheus.client.MetricType" json:"type,omitempty"`
	Metric               []*Metric   `protobuf:"bytes,4,rep,name=metric" json:"metric,omitempty"`
}

type MetricType int32

const (
	MetricType_COUNTER   MetricType = 0
	MetricType_GAUGE     MetricType = 1
	MetricType_SUMMARY   MetricType = 2
	MetricType_UNTYPED   MetricType = 3
	MetricType_HISTOGRAM MetricType = 4
)

type Metric struct {
	Label                []*LabelPair `protobuf:"bytes,1,rep,name=label" json:"label,omitempty"`
	Gauge                *Gauge       `protobuf:"bytes,2,opt,name=gauge" json:"gauge,omitempty"`
	Counter              *Counter     `protobuf:"bytes,3,opt,name=counter" json:"counter,omitempty"`
	Summary              *Summary     `protobuf:"bytes,4,opt,name=summary" json:"summary,omitempty"`
	Untyped              *Untyped     `protobuf:"bytes,5,opt,name=untyped" json:"untyped,omitempty"`
	Histogram            *Histogram   `protobuf:"bytes,7,opt,name=histogram" json:"histogram,omitempty"`
	TimestampMs          *int64       `protobuf:"varint,6,opt,name=timestamp_ms,json=timestampMs" json:"timestamp_ms,omitempty"`
}
```



更多细节请看：[Prometheus类图](https://github.com/sysublackbear/client_golang/blob/master/prometheus%E7%B1%BB%E5%9B%BE.jpg)



- `Counter`：只增不减的计数器；
- `Gauge`：仪表盘，标量；
- `Histogram`：柱状图；
- `Summary`：概要；
- `CounterVec`：支持带Labels的Counter；
- `GaugeVec`：支持带Labels的Gauge；
- `HistogramVec`：支持带Labels的Histogram；
- `SummaryVec`：支持带Labels的Summary。

值得一提，上面这些数据模型既是Metric，又是Collector。



pusher主要与pushgateway相关联：

- 业务通过对pusher进行封装，添加上自己业务特有的Metrics，进行wrap包装上报；
- 底层还是依赖pusher的Push（本质是PUT）方法和Add（本质是POST）方法，把数据gather好送到pushgateway；
- pushgateway那边，自带的DiskMetricStore（实现了MetricStore接口），在NewDiskMetricStore方法里面会新开一个goroutine执行loop方法，定期将接收到的MetricGroup落地到持久化存储中。



## 3.目录结构

如下：

```bash
client_golang
├── api
│   └── client.go  -- PromQL的Http Client版本
├── examples
│   ├── random
│   │   └── main.go -- 带Histogram和Summary的HttpSever示例
│   └── simple
│       └── main.go -- 简单HttpServer Gather示例
├── prometheus
│   ├── collector.go
│   ├── counter.go
│   ├── desc.go
│   ├── doc.go
│   ├── example_clustermanager_test.go
│   ├── example_timer_complex_test.go
│   ├── example_timer_gauge_test.go
│   ├── example_timer_test.go
│   ├── examples_test.go
│   ├── expvar_collector.go
│   ├── fnv.go
│   ├── gauge.go
│   ├── go_collector.go
│   ├── graphite  --- Prometheus的client_golang支持对接另一套监控系统graphite，这里实现了bridge进行数据推送到graphite
│   │   └── bridge.go
│   ├── histogram.go
│   ├── http.go -- http逻辑
│   ├── internal
│   │   └── metric.go
│   ├── labels.go
│   ├── metric.go
│   ├── observer.go
│   ├── process_collector.go
│   ├── promauto
│   │   └── auto.go
│   ├── promhttp
│   │   ├── delegator.go
│   │   ├── http.go
│   │   ├── instrument_client.go
│   │   └── instrument_server.go -- 信息采集的server逻辑，装饰器进行对HttpHandler进行包装
│   ├── push
│   │   ├── deprecated.go
│   │   ├── example_add_from_gatherer_test.go
│   │   └── push.go -- 往服务器主动推送Metric的pusher逻辑
│   ├── registry.go
│   ├── summary.go
│   ├── testutil
│   │   ├── testutil.go
│   ├── timer.go
│   ├── untyped.go
│   ├── value.go
│   ├── vec.go
│   ├── wrap.go
└── vendor
```





## 4.更多

更多还是看官方的 [client_golang.README.md](https://github.com/sysublackbear/client_golang/blob/master/client_golang.README.md)吧。
