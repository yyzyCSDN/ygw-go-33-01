package transfer

import (
	"fmt"
	"sync"

	"zonedns/internal/message"
	"zonedns/internal/model"
)

type Notifier interface {
	Notify(event model.NotifyEvent) error
}

type MemoryNotifier struct {
	mu      sync.Mutex
	events  []model.NotifyEvent
	packets [][]byte
}

func NewMemoryNotifier() *MemoryNotifier {
	return &MemoryNotifier{}
}

func (n *MemoryNotifier) Notify(event model.NotifyEvent) error {
	packet, err := message.BuildNotify(event.Zone, event.Serial)
	if err != nil {
		return fmt.Errorf("build notify packet: %w", err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, event)
	n.packets = append(n.packets, packet)
	return nil
}

func (n *MemoryNotifier) Events() []model.NotifyEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]model.NotifyEvent(nil), n.events...)
}

func (n *MemoryNotifier) PacketCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.packets)
}

func (n *MemoryNotifier) LastPacket() []byte {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.packets) == 0 {
		return nil
	}
	return append([]byte(nil), n.packets[len(n.packets)-1]...)
}
