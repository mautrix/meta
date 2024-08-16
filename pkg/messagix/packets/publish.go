package packets

import "fmt"

// 00110010
// 0011
// 0010
type PublishPacket struct {
	packetType uint8 // packet type. (PUBLISH)
	DUP        uint8 // duplicate delivery flag (bit 3) - this is 1 or 0, 1 = is re-delivery/duplicate of message
	QOSLevel   QoS   // quality of service level (bit 2 & 1)
	RetainFlag uint8 // retain flag (bit 0) - if set to 1, the message should be retained by the broker and delivered to future subscribers with a matching subscription
}

func (p *PublishPacket) GetPacketType() uint8 {
	return p.packetType
}

func (p *PublishPacket) Compress() byte {
	var result byte = PUBLISH << 4
	if p.DUP == 1 {
		result |= DUP
	}

	result |= p.QOSLevel.toByte() << 1
	if p.RetainFlag == 1 {
		result |= RETAIN
	}
	return result
}

func (p *PublishPacket) Decompress(packetByte byte) error {
	p.packetType = packetByte >> 4
	if p.packetType != PUBLISH {
		return fmt.Errorf("tried to decompress publish packet type but result was not of PUBLISH packet type")
	}

	if (packetByte & 0x08) != 0 {
		p.DUP = 1
	} else {
		p.DUP = 0
	}

	p.QOSLevel = QoS((packetByte & 0x06) >> 1)

	if (packetByte & 0x01) != 0 {
		p.RetainFlag = 1
	} else {
		p.RetainFlag = 0
	}

	return nil
}
