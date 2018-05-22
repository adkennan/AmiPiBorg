package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

type DateHandler struct {
	outChan chan *OutPacket
}

func (this *DateHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan

	t1 := time.Now().Unix()
	t2 := time.Date(1978, 1, 1, 0, 0, 0, 0, time.Local).Unix()

	fmt.Printf("Amiga time is %ld\n", t1-t2)

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint32(t1-t2))

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       buf.Bytes()}
}

func (this *DateHandler) HandlePacket(p *InPacket) {
}

func (this *DateHandler) Quit() {
}

func NewDateHandler() Handler {
	return &DateHandler{}
}
