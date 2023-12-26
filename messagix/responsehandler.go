package messagix

import (
	"fmt"
	"time"
)

type ChannelType int

const (
	RequestChannel ChannelType = iota
	PacketChannel
)

type ResponseHandler struct {
	client *Client
	requestChannels map[uint16]chan interface{}
	packetChannels map[uint16]chan interface{}
    packetTimeout time.Duration
}

func (p *ResponseHandler) SetPacketTimeout(d time.Duration) {
	p.packetTimeout = d
}

func (p *ResponseHandler) hasPacket(packetId uint16) bool {
	_, ok := p.packetChannels[packetId]
	return ok
}

func (p *ResponseHandler) addPacketChannel(packetId uint16) {
	p.packetChannels[packetId] = make(chan interface{}, 1) // buffered channel with capacity of 1
}

func (p *ResponseHandler) addRequestChannel(packetId uint16) {
	p.requestChannels[packetId] = make(chan interface{}, 1)
}

func (p *ResponseHandler) updatePacketChannel(packetId uint16, packetData interface{}) error {
	if ch, ok := p.packetChannels[packetId]; ok {
		ch <- packetData
		//p.client.Logger.Info().Any("data", packetData).Any("packetId", packetId).Msg("Updated packet channel!")
		return nil
	}

	return fmt.Errorf("failed to update packet channel for packetId %d", packetId)
}

func (p *ResponseHandler) updateRequestChannel(packetId uint16, packetData interface{}) error {
	if ch, ok := p.requestChannels[packetId]; ok {
		ch <- packetData
		//p.client.Logger.Info().Any("data", packetData).Any("packetId", packetId).Msg("Updated request channel!")
		return nil
	}

	return fmt.Errorf("failed to update request channel for packetId %d", packetId)
}


func (p *ResponseHandler) waitForPubACKDetails(packetId uint16) *Event_PublishACK {
	return p.waitForDetails(packetId, PacketChannel).(*Event_PublishACK)
}

func (p *ResponseHandler) waitForSubACKDetails(packetId uint16) *Event_SubscribeACK {
	return p.waitForDetails(packetId, PacketChannel).(*Event_SubscribeACK)
}

func (p *ResponseHandler) waitForPubResponseDetails(packetId uint16) *Event_PublishResponse {
	return p.waitForDetails(packetId, RequestChannel).(*Event_PublishResponse)
}

func (p *ResponseHandler) waitForDetails(packetId uint16, channelType ChannelType) interface{} {
	ch, ok := p.getChannel(packetId, channelType)
    if !ok {
        return nil
	}

    select {
		case response := <- ch:
			p.deleteDetails(packetId, channelType)
			return response
		case <-time.After(p.packetTimeout):
			p.deleteDetails(packetId, channelType)
			return &Event_PublishResponse{}
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