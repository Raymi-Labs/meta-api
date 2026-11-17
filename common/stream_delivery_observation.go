package common

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const StreamDeliveryObservationKey = "stream_delivery_observation"

// StreamDeliverySnapshot describes successful server writes and flushes, never
// acknowledgement by the remote client. Nil times mean no such observation.
type StreamDeliverySnapshot struct {
	Observed           bool   `json:"observed"`
	Heartbeats         int64  `json:"heartbeats"`
	EffectiveFrames    int64  `json:"effective_frames"`
	FirstHeartbeatMs   *int64 `json:"first_heartbeat_ms"`
	FirstEffectiveMs   *int64 `json:"first_effective_ms"`
	LastEffectiveAgoMs *int64 `json:"last_effective_ago_ms"`
	WriteFailed        bool   `json:"write_failed"`
}

type StreamDeliveryObservation struct {
	mu            sync.Mutex
	start         time.Time
	lastEffective time.Time
	state         StreamDeliverySnapshot
	frozen        bool
}

func NewStreamDeliveryObservation(start time.Time) *StreamDeliveryObservation {
	return &StreamDeliveryObservation{start: start}
}

func ObserveStreamDelivery(c *gin.Context, heartbeat, effective bool, err error) {
	value, ok := c.Get(StreamDeliveryObservationKey)
	if !ok {
		return
	}
	o, ok := value.(*StreamDeliveryObservation)
	if !ok || o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.frozen || c.Request == nil || c.Request.Context().Err() != nil {
		return
	}
	o.state.Observed = true
	if err != nil {
		o.state.WriteFailed = true
		return
	}
	now := time.Now()
	ms := now.Sub(o.start).Milliseconds()
	if heartbeat {
		o.state.Heartbeats++
		if o.state.FirstHeartbeatMs == nil {
			o.state.FirstHeartbeatMs = &ms
		}
	}
	if effective {
		o.state.EffectiveFrames++
		if o.state.FirstEffectiveMs == nil {
			o.state.FirstEffectiveMs = &ms
		}
		o.lastEffective = now
	}
}

func (o *StreamDeliveryObservation) Freeze(now time.Time) StreamDeliverySnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.frozen {
		o.frozen = true
		if !o.lastEffective.IsZero() {
			ms := max(int64(0), now.Sub(o.lastEffective).Milliseconds())
			o.state.LastEffectiveAgoMs = &ms
		}
	}
	return o.state
}
