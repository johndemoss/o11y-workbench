package remotetap

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	collectorslogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"golang.org/x/net/websocket"
)

type signalType int

const (
	signalUnknown signalType = iota
	signalTraces
	signalLogs
	signalMetrics
)

type RemotetapData struct {
	Traces  []*tracev1.ResourceSpans
	Logs    []*logsv1.ResourceLogs
	Metrics []*metricspb.ResourceMetrics
}

type Client struct {
	endpoint string
	conn     *websocket.Conn
	dataCh   chan *RemotetapData
	errCh    chan error
	done     chan struct{}
	mu       sync.Mutex
	capacity int
}

type ClientOption func(*Client)

func WithChannelCapacity(cap int) ClientOption {
	return func(c *Client) {
		c.capacity = cap
	}
}

func NewClient(endpoint string, opts ...ClientOption) *Client {
	c := &Client{
		endpoint: endpoint,
		dataCh:   make(chan *RemotetapData, 100),
		errCh:    make(chan error, 10),
		done:     make(chan struct{}),
		capacity: 100,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.dataCh = make(chan *RemotetapData, c.capacity)
	return c
}

func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	url := fmt.Sprintf("ws://%s/", c.endpoint)
	config, err := websocket.NewConfig(url, "http://"+c.endpoint)
	if err != nil {
		return fmt.Errorf("websocket config: %w", err)
	}

	conn, err := websocket.DialConfig(config)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	c.conn = conn

	go c.readLoop()

	return nil
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.done:
	default:
		close(c.done)
	}

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	return nil
}

func (c *Client) Data() <-chan *RemotetapData {
	return c.dataCh
}

func (c *Client) Err() <-chan error {
	return c.errCh
}

func (c *Client) readLoop() {
	for {
		var msg []byte
		err := websocket.Message.Receive(c.conn, &msg)
		if err != nil {
			select {
			case <-c.done:
			default:
				c.errCh <- fmt.Errorf("websocket receive: %w", err)
			}
			close(c.dataCh)
			return
		}

		if len(msg) == 0 {
			continue
		}

		data, err := c.parseLine(msg)
		if err != nil {
			c.errCh <- fmt.Errorf("parse line: %w", err)
			continue
		}

		select {
		case c.dataCh <- data:
		case <-c.done:
			close(c.dataCh)
			return
		}
	}
}

func (c *Client) parseLine(line []byte) (*RemotetapData, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal raw: %w", err)
	}

	sig, err := detectSignal(raw)
	if err != nil {
		return nil, err
	}

	data := &RemotetapData{}
	switch sig {
	case signalTraces:
		var req collectortracev1.ExportTraceServiceRequest
		if err := protojson.Unmarshal(line, &req); err != nil {
			return nil, fmt.Errorf("unmarshal traces: %w", err)
		}
		data.Traces = req.ResourceSpans
	case signalLogs:
		var req collectorslogsv1.ExportLogsServiceRequest
		if err := protojson.Unmarshal(line, &req); err != nil {
			return nil, fmt.Errorf("unmarshal logs: %w", err)
		}
		data.Logs = req.ResourceLogs
	case signalMetrics:
		var req collectormetricsv1.ExportMetricsServiceRequest
		if err := protojson.Unmarshal(line, &req); err != nil {
			return nil, fmt.Errorf("unmarshal metrics: %w", err)
		}
		data.Metrics = req.ResourceMetrics
	}

	return data, nil
}

func detectSignal(raw map[string]json.RawMessage) (signalType, error) {
	if _, ok := raw["resourceSpans"]; ok {
		return signalTraces, nil
	}
	if _, ok := raw["resourceLogs"]; ok {
		return signalLogs, nil
	}
	if _, ok := raw["resourceMetrics"]; ok {
		return signalMetrics, nil
	}
	return signalUnknown, fmt.Errorf("unknown signal type")
}