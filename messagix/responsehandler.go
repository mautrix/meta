package messagix

import (
	"time"
)

type ChannelType int

const (
	RequestChannel ChannelType = iota
	PacketChannel
)

type ResponseHandler struct {
	client          *Client
	requestChannels map[uint16]chan interface{}
	packetChannels  map[uint16]chan interface{}
}

func (p *ResponseHandler) hasPacket(packetId uint16) bool {
	_, ok := p.requestChannels[packetId]
	return ok
}

func (p *ResponseHandler) addPacketChannel(packetId uint16) {
	p.packetChannels[packetId] = make(chan interface{}, 1) // buffered channel with capacity of 1
}

func (p *ResponseHandler) addRequestChannel(packetId uint16) {
	p.requestChannels[packetId] = make(chan interface{}, 1)
}

func (p *ResponseHandler) updatePacketChannel(packetId uint16, packetData interface{}) bool {
	if ch, ok := p.packetChannels[packetId]; ok {
		ch <- packetData
		return true
	}
	return false
}

func (p *ResponseHandler) updateRequestChannel(packetId uint16, packetData interface{}) bool {
	if ch, ok := p.requestChannels[packetId]; ok {
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
	if ch, ok := p.getChannel(packetId, channelType); ok {
		close(ch)
		if channelType == RequestChannel {
			delete(p.requestChannels, packetId)
		} else {
			delete(p.packetChannels, packetId)
		}
	}
}

func (p *ResponseHandler) getChannel(packetId uint16, channelType ChannelType) (chan interface{}, bool) {
	var ch chan interface{}
	var ok bool
	switch channelType {
	case RequestChannel:
		ch, ok = p.requestChannels[packetId]
	case PacketChannel:
		ch, ok = p.packetChannels[packetId]
	}
	return ch, ok
}
