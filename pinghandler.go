package main

import (
	"fmt"
)

type PingHandler struct {
	outChan chan *OutPacket
}

func (this *PingHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan
}

func (this *PingHandler) HandlePacket(p *InPacket) {

	fmt.Println(p.ConnId, ": Ping")

	data := make([]byte, len(p.Data))
	copy(data, p.Data)

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       data}
}

func (this *PingHandler) Quit() {
}

func NewPingHandler() Handler {
	return &PingHandler{}
}
