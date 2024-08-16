package packets

import "fmt"

type SubscribePacket struct {
	packetType uint8
}

func (sp *SubscribePacket) Compress() byte {
	return (SUBSCRIBE << 4) | 0x02
}

func (sp *SubscribePacket) Decompress(packetByte byte) error {
	sp.packetType = packetByte >> 4
	if sp.packetType != SUBSCRIBE {
		return fmt.Errorf("tried to decompress subscribe packet type but result was not of SUBSCRIBE packet type")
	}

	flags := packetByte & 0x0F // 0x02 reserved
	if flags != 0x02 {
		return fmt.Errorf("invalid flags for SUBSCRIBE packet")
	}

	return nil
}

func (sp *SubscribePacket) GetPacketType() uint8 {
	return sp.packetType
}
