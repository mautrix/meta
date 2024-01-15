package packets

type ConnectPacket struct {
	Packet byte
}

func (p *ConnectPacket) Compress() byte {
	return CONNECT << 4
}

func (p *ConnectPacket) Decompress(packetByte byte) error {
	p.Packet = packetByte
	return nil
}

type ConnACKPacket struct {
	Packet byte
}

func (p *ConnACKPacket) Compress() byte {
	return CONNACK << 4
}

func (p *ConnACKPacket) Decompress(packetByte byte) error {
	p.Packet = packetByte
	return nil
}

type ConnectFlags struct {
	Username     bool
	Password     bool
	Retain       bool
	QoS          uint8 // 0, 1, 2, or 3
	CleanSession bool
}

func CreateConnectFlagByte(flags ConnectFlags) uint8 {
	var connectFlagsByte uint8

	if flags.Username {
		connectFlagsByte |= 0x80
	}
	if flags.Password {
		connectFlagsByte |= 0x40
	}
	if flags.Retain {
		connectFlagsByte |= 0x20
	}

	connectFlagsByte |= (flags.QoS << 3) & 0x18
	if flags.CleanSession {
		connectFlagsByte |= 0x02
	}

	return connectFlagsByte
}
