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

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       p.Data}
}

func NewPingHandler() Handler {
	return &PingHandler{}
}
