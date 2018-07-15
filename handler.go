package main

import (
	"sort"
)

const (
	HT_Ping  = 1
	HT_Date  = 2
	HT_Input = 3
)

type handlerDesc struct {
	name    string
	builder (func() Handler)
}

type HandlerFactory struct {
	handlers map[uint16]*handlerDesc
}

func NewHandlerFactory() (hf *HandlerFactory) {
	hf = &HandlerFactory{handlers: make(map[uint16]*handlerDesc)}
	return hf
}

func (this *HandlerFactory) AddHandler(handlerId uint16, name string, builder func() Handler) {
	this.handlers[handlerId] = &handlerDesc{name, builder}
}

func (this *HandlerFactory) CreateHandler(handlerId uint16) Handler {
	if hd, ok := this.handlers[handlerId]; ok {
		return hd.builder()
	}
	return nil
}

func (this *HandlerFactory) HandlerCount() uint16 {
	return uint16(len(this.handlers))
}

func (this *HandlerFactory) GetHandlerDescriptions() ([]uint16, []string) {

	iids := make([]int, 0, len(this.handlers))
	for iid := range this.handlers {
		iids = append(iids, int(iid))
	}

	sort.Ints(iids)

	ids := make([]uint16, 0, len(this.handlers))
	names := make([]string, 0, len(this.handlers))
	for _, id := range iids {
		ids = append(ids, uint16(id))
		names = append(names, this.handlers[uint16(id)].name)
	}

	return ids, names
}

type Handler interface {
	Init(outChan chan *OutPacket)
	Quit()
	HandlePacket(p *InPacket)
}
