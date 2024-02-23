package messagix

import (
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
	requestChannels map[uint16]chan interface{}
	packetChannels  map[uint16]chan interface{}
}

func (p *ResponseHandler) hasPacket(packetId uint16) bool {
	p.lock.RLock()
	_, ok := p.requestChannels[packetId]
	p.lock.RUnlock()
	return ok
}

func (p *ResponseHandler) addPacketChannel(packetId uint16) {
	p.lock.Lock()
	p.packetChannels[packetId] = make(chan interface{}, 1) // buffered channel with capacity of 1
	p.lock.Unlock()
}

func (p *ResponseHandler) addRequestChannel(packetId uint16) {
	p.lock.Lock()
	p.requestChannels[packetId] = make(chan interface{}, 1)
	p.lock.Unlock()
}

func (p *ResponseHandler) updatePacketChannel(packetId uint16, packetData interface{}) bool {
	p.lock.RLock()
	ch, ok := p.packetChannels[packetId]
	p.lock.RUnlock()
	if ok {
		ch <- packetData
		return true
	}
	return false
}

func (p *ResponseHandler) updateRequestChannel(packetId uint16, packetData interface{}) bool {
	p.lock.RLock()
	ch, ok := p.requestChannels[packetId]
	p.lock.RUnlock()
	if ok {
		ch <- packetData
		return true
	}
	return false
}

func (p *ResponseHandler) waitForPubACKDetails(packetId uint16) *Event_PublishACK {
	return castIfNotNil[Event_PublishACK](p.waitForDetails(packetId, PacketChannel))
}

func (p *ResponseHandler) waitForSubACKDetails(packetId uint16) *Event_SubscribeACK {
	return castIfNotNil[Event_SubscribeACK](p.waitForDetails(packetId, PacketChannel))
}

func (p *ResponseHandler) waitForPubResponseDetails(packetId uint16) *Event_PublishResponse {
	return castIfNotNil[Event_PublishResponse](p.waitForDetails(packetId, RequestChannel))
}

func castIfNotNil[T any](i interface{}) *T {
	if i != nil {
		return i.(*T)
	}
	return nil
}

func (p *ResponseHandler) waitForDetails(packetId uint16, channelType ChannelType) interface{} {
	ch, ok := p.getChannel(packetId, channelType)
	if !ok {
		return nil
	}

	select {
	case response := <-ch:
		p.deleteDetails(packetId, channelType)
		return response
	case <-time.After(packetTimeout):
		p.deleteDetails(packetId, channelType)
		// TODO this should probably be an error
		return nil
	}
}

func (p *ResponseHandler) deleteDetails(packetId uint16, channelType ChannelType) {
	p.lock.Lock()
	defer p.lock.Unlock()
	switch channelType {
	case RequestChannel:
		ch, ok := p.requestChannels[packetId]
		if ok {
			close(ch)
			delete(p.requestChannels, packetId)
		}
	case PacketChannel:
		ch, ok := p.packetChannels[packetId]
		if ok {
			close(ch)
			delete(p.packetChannels, packetId)
		}
	}
}

func (p *ResponseHandler) getChannel(packetId uint16, channelType ChannelType) (chan interface{}, bool) {
	var ch chan interface{}
	var ok bool
	p.lock.RLock()
	switch channelType {
	case RequestChannel:
		ch, ok = p.requestChannels[packetId]
	case PacketChannel:
		ch, ok = p.packetChannels[packetId]
	}
	p.lock.RUnlock()
	return ch, ok
}
