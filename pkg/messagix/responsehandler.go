package messagix

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type ChannelType int

const (
	RequestChannel ChannelType = iota
	PacketChannel
)

type ResponseHandler struct {
	client          *Client
	lock            sync.RWMutex
	requestChannels map[uint16]chan any
	packetChannels  map[uint16]chan any
}

var ErrTimeout = fmt.Errorf("timeout waiting for response")
var ErrConnectionClosed = fmt.Errorf("connection closed")
var ErrChannelNotFound = fmt.Errorf("channel not found for packet")

func (p *ResponseHandler) hasPacket(packetID uint16) bool {
	p.lock.RLock()
	_, ok := p.requestChannels[packetID]
	p.lock.RUnlock()
	return ok
}

func (p *ResponseHandler) addPacketChannel(packetID uint16) {
	p.lock.Lock()
	p.packetChannels[packetID] = make(chan any, 1) // buffered channel with capacity of 1
	p.lock.Unlock()
}

func (p *ResponseHandler) addRequestChannel(packetID uint16) {
	p.lock.Lock()
	p.requestChannels[packetID] = make(chan any, 1)
	p.lock.Unlock()
}

func (p *ResponseHandler) updatePacketChannel(packetID uint16, packetData any) bool {
	p.lock.RLock()
	ch, ok := p.packetChannels[packetID]
	p.lock.RUnlock()
	if ok {
		ch <- packetData
		return true
	}
	return false
}

func (p *ResponseHandler) updateRequestChannel(packetID uint16, packetData any) bool {
	p.lock.RLock()
	ch, ok := p.requestChannels[packetID]
	p.lock.RUnlock()
	if ok {
		ch <- packetData
		return true
	}
	return false
}

func (p *ResponseHandler) waitForPubACKDetails(ctx context.Context, packetID uint16) (*Event_PublishACK, error) {
	response, err := p.waitForDetails(ctx, packetID, PacketChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_PublishACK](response), nil
}

func (p *ResponseHandler) waitForSubACKDetails(ctx context.Context, packetID uint16) (*Event_SubscribeACK, error) {
	response, err := p.waitForDetails(ctx, packetID, PacketChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_SubscribeACK](response), nil
}

func (p *ResponseHandler) waitForPubResponseDetails(ctx context.Context, packetID uint16) (*Event_PublishResponse, error) {
	response, err := p.waitForDetails(ctx, packetID, RequestChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_PublishResponse](response), nil
}

func castIfNotNil[T any](i any) *T {
	if i != nil {
		return i.(*T)
	}
	return nil
}

func (p *ResponseHandler) waitForDetails(ctx context.Context, packetID uint16, channelType ChannelType) (any, error) {
	ch, ok := p.getChannel(packetID, channelType)
	if !ok {
		return nil, ErrChannelNotFound
	}

	select {
	case response := <-ch:
		if response == nil {
			return nil, ErrConnectionClosed
		}
		p.deleteDetails(packetID, channelType)
		return response, nil
	case <-ctx.Done():
		p.deleteDetails(packetID, channelType)
		return nil, ctx.Err()
	case <-time.After(packetTimeout):
		p.deleteDetails(packetID, channelType)
		return nil, ErrTimeout
	}
}

func (p *ResponseHandler) deleteDetails(packetID uint16, channelType ChannelType) {
	p.lock.Lock()
	defer p.lock.Unlock()
	switch channelType {
	case RequestChannel:
		ch, ok := p.requestChannels[packetID]
		if ok {
			close(ch)
			delete(p.requestChannels, packetID)
		}
	case PacketChannel:
		ch, ok := p.packetChannels[packetID]
		if ok {
			close(ch)
			delete(p.packetChannels, packetID)
		}
	}
}

func (p *ResponseHandler) getChannel(packetID uint16, channelType ChannelType) (chan any, bool) {
	var ch chan any
	var ok bool
	p.lock.RLock()
	switch channelType {
	case RequestChannel:
		ch, ok = p.requestChannels[packetID]
	case PacketChannel:
		ch, ok = p.packetChannels[packetID]
	}
	p.lock.RUnlock()
	return ch, ok
}

func (p *ResponseHandler) CancelAllRequests() {
	p.lock.Lock()
	defer p.lock.Unlock()
	for _, ch := range p.requestChannels {
		close(ch)
	}
	for _, ch := range p.packetChannels {
		close(ch)
	}
	p.requestChannels = make(map[uint16]chan any)
	p.packetChannels = make(map[uint16]chan any)
}
