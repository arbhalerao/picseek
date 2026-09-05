package pipeline

import (
	"sync"
	"time"
)

const meterWindow = 30

type Meter struct {
	mu      sync.Mutex
	buckets [meterWindow]int
	stamps  [meterWindow]int64
}

func (m *Meter) Add(n int) {
	now := time.Now().Unix()
	i := now % meterWindow
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stamps[i] != now {
		m.stamps[i] = now
		m.buckets[i] = 0
	}
	m.buckets[i] += n
}

func (m *Meter) Rate() float64 {
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	total, oldest := 0, now
	for i := range m.buckets {
		if now-m.stamps[i] < meterWindow {
			total += m.buckets[i]
			oldest = min(oldest, m.stamps[i])
		}
	}
	if total == 0 {
		return 0
	}
	return float64(total) / float64(now-oldest+1)
}
