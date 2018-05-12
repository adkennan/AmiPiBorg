package main

const (
	AMIPIBORG_VERSION  = 1
	DEFAULT_CONNECTION = 0
	PACKET_ID          = 0x416d5069
)

const (
	MT_Init         = 0x00
	MT_Hello        = 0x01
	MT_Shutdown     = 0x02
	MT_Goodbye      = 0x03
	MT_Connect      = 0x10
	MT_Connected    = 0x11
	MT_Disconnect   = 0x12
	MT_Disconnected = 0x13
	MT_Data         = 0x20
	MT_Resend       = 0x22
	MT_Ping         = 0x23
	MT_Pong         = 0x24
	MT_Error        = 0x30
	MT_NoHandler    = 0x31
	MT_NoConnection = 0x32
)

const (
	PF_PadByte = 0x01
	PF_Resend  = 0x02
)

func calculateChecksum(data []byte, length uint16) uint16 {

	sum := uint32(0)

	for ix := uint16(0); ix < length; ix += 2 {
		x := uint16(data[ix])<<8 + uint16(data[ix+1])
		sum += uint32(x)
	}

	for sum>>16 > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}

	return uint16((^sum) + 1)
}
