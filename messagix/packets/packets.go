package packets

const (
	CONNECT = 1 // CONNECT packet does not include any flags in the packet type
	CONNACK = 2 // CONNECT ACKNOWLEDGEMENT packet does not include any flags in the packet type

	PUBLISH = 3 // PUBLISH packet without any flags
	PUBACK  = 4 // PUBLISH ACKNOWLEDGMENT packet

	SUBSCRIBE = 8 // SUBSCRIBE packet without any flags
	SUBACK    = 9 // SUBSCRIBE ACKNOWLEDGMENT packet

	PINGREQ  = 12 // PING_REQ packet without any flags
	PINGRESP = 13 // PING_RESP packet without any flags
)

const (
	DUP    = 0x08
	RETAIN = 0x01
)

type QoS uint8

func (q QoS) IsEnum() {}
func (q QoS) toByte() byte {
	return byte(q & 0x03)
}

const (
	QOS_LEVEL_0 QoS = 0x00
	QOS_LEVEL_1 QoS = 0x01
	QOS_LEVEL_2 QoS = 0x02
)

type Packet interface {
	Compress() byte
	Decompress(packetByte byte) error
}
