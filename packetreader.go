package main

import (
	"encoding/binary"
	"fmt"
)

type InPacket struct {
	PacketType uint8
	ConnId     uint16
	PacketId   uint16
	Length     uint16
	Data       []byte
}

type PacketReader struct {
	remote     Remote
	control    chan bool
	bufferPool *BufferPool
	outChan    chan *InPacket
	buf        []byte
}

func NewPacketReader(bufferPool *BufferPool, remote Remote) *PacketReader {

	pr := &PacketReader{
		remote,
		make(chan bool),
		bufferPool,
		make(chan *InPacket, 100),
		make([]byte, 0, 100)}

	return pr
}

func (this *PacketReader) GetOutputChannel() (outChan chan *InPacket) {
	return this.outChan
}

func (this *PacketReader) Start() {
	go this.run()
}

func (this *PacketReader) Stop() {
	this.control <- true
}

func (this *PacketReader) run() {

	done := false

	for !done {
		select {
		case buf := <-this.remote.GetReadChan():
			this.processBuffer(buf)

		case done = <-this.control:
		}
	}
}

func (this *PacketReader) processBuffer(buf []byte) {

	this.buf = append(this.buf, buf...)

	this.bufferPool.ReleaseBuffer(buf)

	maxIx := len(this.buf) - 14

	ij := 0
	packetCount := 0
	for ij <= maxIx {
		ix := ij
		found := false
		for ix <= maxIx {

			id := binary.BigEndian.Uint32(this.buf[ix:])
			if id == PACKET_ID {
				ix += 4
				found = true
				break
			}

			ix++
		}

		if !found {
			// No packet found at all.
			fmt.Printf("Bad data? %d bytes\n", len(this.buf))

			this.buf = this.buf[:0]
			return
		}

		if ix > maxIx+4 {
			// Not enough bytes for a header.
			fmt.Printf("Incomplete packet\n")
			this.buf = this.buf[ij:]
			return
		}

		pacType := this.buf[ix]
		ix++
		pacFlags := this.buf[ix]
		ix++
		connId := binary.BigEndian.Uint16(this.buf[ix:])
		ix += 2
		packId := binary.BigEndian.Uint16(this.buf[ix:])
		ix += 2
		// TODO: Checksum...
		ix += 2
		length := binary.BigEndian.Uint16(this.buf[ix:])
		ix += 2

		if ix+int(length) > len(this.buf) {
			// Not enough bytes for all the data
			this.buf = this.buf[ij:]
			return
		}

		packet := &InPacket{
			PacketType: pacType,
			ConnId:     connId,
			PacketId:   packId,
			Length:     length,
			Data:       this.buf[ix : ix+int(length)]}

		packetCount++
		this.outChan <- packet

		ij = ix + int(length)
		if (pacFlags & PF_PadByte) == PF_PadByte {
			ij++
		}
	}

	this.buf = this.buf[:0]
}
