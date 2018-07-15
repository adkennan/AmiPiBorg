package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type PacketWriter struct {
	remote Remote
}

func NewPacketWriter(remote Remote) *PacketWriter {
	return &PacketWriter{remote: remote}
}

func (this *PacketWriter) Write(packType uint8, flags uint8, connId uint16, packId uint16, data []byte) (err error) {

	buf := new(bytes.Buffer)

	if len(data)%2 != 0 {
		flags |= PF_PadByte
	}

	packet := []interface{}{
		uint32(PACKET_ID),
		packType,
		flags,
		connId,
		packId,
		uint16(0),
		uint16(len(data))}

	for _, v := range packet {
		err = binary.Write(buf, binary.BigEndian, v)
		if err != nil {
			fmt.Printf("Error: %s\n", err.Error())
			return err
		}
	}

	buf.Write(data)

	if (flags & PF_PadByte) == PF_PadByte {
		//err = binary.Write(buf, binary.BigEndian, uint8(0))
		//if err != nil {
		//	return err
		//}
		buf.Write([]uint8{0})
	}

	b := buf.Bytes()
	checksum := calculateChecksum(b, uint16(len(b)))
	b[10] = byte((checksum >> 8) & 0xff)
	b[11] = byte(checksum & 0xff)
	this.remote.Write(b)

	return err
}
