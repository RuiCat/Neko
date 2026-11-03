// One /monitor/ws connection: per-connection seq, snapshot-then-delta ordering
// and a bounded outbound queue that degrades to a fresh snapshot when full.
package ws

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// OUT_BUFFER bounds the per-connection outbound frame queue.
	OUT_BUFFER = 256
	// WRITE_TIMEOUT bounds one frame write before the connection is dropped.
	WRITE_TIMEOUT = 10 * time.Second
)

// Client is one accepted WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	mu     sync.Mutex
	seq    uint64
	out    chan []byte
	resync chan struct{}
	topics map[string]bool
	entity string
	paths  []string
}

// newClient builds one connection state; it is registered only after the
// initial hello/snapshot frames were queued.
func newClient(hub *Hub, conn *websocket.Conn, topics map[string]bool, entity string, paths []string) *Client {
	return &Client{
		hub: hub, conn: conn, out: make(chan []byte, OUT_BUFFER),
		resync: make(chan struct{}, 1), topics: topics,
		entity: entity, paths: paths,
	}
}

// accepts reports whether this connection subscribed to topic and matches the
// optional entity/path filters.
func (c *Client) accepts(topic, entityID, path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.topics[topic] {
		return false
	}
	if entityID != "" && c.entity != "" && c.entity != entityID {
		return false
	}
	if path != "" && len(c.paths) > 0 && !pathMatches(c.paths, path) {
		return false
	}
	return true
}

// pathMatches reports an exact or descendant match of one dotted path.
func pathMatches(filters []string, path string) bool {
	for _, filter := range filters {
		if path == filter || strings.HasPrefix(path, filter+".") {
			return true
		}
	}
	return false
}

// push assigns the next seq and enqueues one frame without blocking; a full
// queue drops pending frames and schedules a snapshot resync instead.
func (c *Client) push(frame map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pushLocked(frame)
}

// pushLocked is push for callers already holding c.mu.
func (c *Client) pushLocked(frame map[string]any) {
	c.seq++
	frame["seq"] = c.seq
	raw, err := json.Marshal(frame)
	if err != nil {
		return
	}
	select {
	case c.out <- raw:
	default:
		c.drainLocked()
		c.scheduleResyncLocked()
	}
}

// drainLocked drops every queued frame so a snapshot can replace the gap.
func (c *Client) drainLocked() {
	for {
		select {
		case <-c.out:
		default:
			return
		}
	}
}

// scheduleResyncLocked asks the writer for a fresh snapshot after a drop.
func (c *Client) scheduleResyncLocked() {
	select {
	case c.resync <- struct{}{}:
	default:
	}
}

// sendInitial queues hello followed by one snapshot per subscribed topic. The
// caller (Hub.Register) holds the hub lock, so no publish can enqueue a frame
// before these.
func (c *Client) sendInitial() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pushLocked(helloFrame(0, c.hub.deps.Version, c.hub.deps.ReadOnly, time.Now()))
	c.enqueueSnapshotsLocked()
}

// enqueueSnapshotsLocked queues one snapshot per current topic, in stable order.
func (c *Client) enqueueSnapshotsLocked() {
	for _, topic := range orderedTopics(c.topics) {
		c.seq++
		frame := snapshotFrame(c.seq, topic, c.hub.snapshotFor(topic))
		raw, err := json.Marshal(frame)
		if err != nil {
			continue
		}
		select {
		case c.out <- raw:
		default:
			return
		}
	}
}

// resyncSnapshots replaces any dropped frames with fresh per-topic snapshots.
func (c *Client) resyncSnapshots() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drainLocked()
	c.enqueueSnapshotsLocked()
}

// writeLoop owns every connection write.
func (c *Client) writeLoop(ctx context.Context) {
	for {
		select {
		case raw := <-c.out:
			if err := c.write(ctx, raw); err != nil {
				return
			}
		case <-c.resync:
			c.resyncSnapshots()
		case <-ctx.Done():
			return
		}
	}
}

// write sends one text frame under the per-frame timeout.
func (c *Client) write(ctx context.Context, raw []byte) error {
	writeCtx, cancel := context.WithTimeout(ctx, WRITE_TIMEOUT)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, raw)
}

// readLoop handles subscribe/unsubscribe/ping frames. It reports true on a
// protocol error so Run can close with code 1008.
func (c *Client) readLoop(ctx context.Context) bool {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return false
		}
		message := clientMessage{}
		if err := json.Unmarshal(data, &message); err != nil {
			c.push(map[string]any{"type": "error", "code": "bad_request", "message": "帧不是合法 JSON"})
			return true
		}
		switch message.Type {
		case "ping":
			c.push(map[string]any{"type": "pong"})
		case "subscribe":
			c.subscribe(message)
		case "unsubscribe":
			c.unsubscribe(message)
		default:
			c.push(map[string]any{"type": "error", "code": "bad_request", "message": "未知帧类型 " + message.Type})
			return true
		}
	}
}

// subscribe adds topics and optional entity/path filters, then resnapshots.
func (c *Client) subscribe(message clientMessage) {
	c.mu.Lock()
	for _, topic := range message.Topics {
		switch topic {
		case TOPIC_STATE, TOPIC_DIAGNOSTICS, TOPIC_LOGS, TOPIC_TICK:
			c.topics[topic] = true
		}
	}
	if message.Entity != "" {
		c.entity = message.Entity
	}
	if len(message.Paths) > 0 {
		c.paths = append([]string(nil), message.Paths...)
	}
	c.scheduleResyncLocked()
	c.mu.Unlock()
}

// unsubscribe removes the listed topics.
func (c *Client) unsubscribe(message clientMessage) {
	c.mu.Lock()
	for _, topic := range message.Topics {
		delete(c.topics, topic)
	}
	c.mu.Unlock()
}

// Run serves one accepted connection until it closes. The hub registers the
// connection and queues hello/snapshot atomically, so no delta can precede the
// snapshot.
func (c *Client) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.hub.Register(c)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		c.writeLoop(ctx)
	}()
	protocolError := c.readLoop(ctx)
	c.hub.unregister(c)
	cancel()
	<-writerDone
	code := websocket.StatusNormalClosure
	if protocolError {
		code = websocket.StatusPolicyViolation
	}
	_ = c.conn.Close(code, "")
}

// closeNow force-closes the connection during server shutdown.
func (c *Client) closeNow() {
	_ = c.conn.CloseNow()
}
