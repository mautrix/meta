package messagix

import (
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
	requestChannels map[uint16]chan interface{}
	packetChannels  map[uint16]chan interface{}
}

var ErrTimeout = fmt.Errorf("timeout waiting for response")
var ErrConnectionClosed = fmt.Errorf("connection closed")
var ErrChannelNotFound = fmt.Errorf("channel not found for packet")

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

func (p *ResponseHandler) waitForPubACKDetails(packetId uint16) (*Event_PublishACK, error) {
	response, err := p.waitForDetails(packetId, PacketChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_PublishACK](response), nil
}

func (p *ResponseHandler) waitForSubACKDetails(packetId uint16) (*Event_SubscribeACK, error) {
	response, err := p.waitForDetails(packetId, PacketChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_SubscribeACK](response), nil
}

func (p *ResponseHandler) waitForPubResponseDetails(packetId uint16) (*Event_PublishResponse, error) {
	response, err := p.waitForDetails(packetId, RequestChannel)
	if err != nil {
		return nil, err
	}
	return castIfNotNil[Event_PublishResponse](response), nil
}

func castIfNotNil[T any](i interface{}) *T {
	if i != nil {
		return i.(*T)
	}
	return nil
}

func (p *ResponseHandler) waitForDetails(packetId uint16, channelType ChannelType) (interface{}, error) {
	ch, ok := p.getChannel(packetId, channelType)
	if !ok {
		return nil, ErrChannelNotFound
	}

	select {
	case response := <-ch:
		if response == nil {
			return nil, ErrConnectionClosed
		}
		p.deleteDetails(packetId, channelType)
		return response, nil
	case <-time.After(packetTimeout):
		p.deleteDetails(packetId, channelType)
		return nil, ErrTimeout
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

func (p *ResponseHandler) CancelAllRequests() {
	p.lock.Lock()
	defer p.lock.Unlock()
	for packetId := range p.requestChannels {
		p.deleteDetails(packetId, RequestChannel)
	}
	for packetId := range p.packetChannels {
		p.deleteDetails(packetId, PacketChannel)
	}
}
