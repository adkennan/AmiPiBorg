package main

const (
	HT_Ping = 1
	HT_Date = 2
)

type HandlerFactory struct {
	handlers map[uint16](func() Handler)
}

func NewHandlerFactory() (hf *HandlerFactory) {
	hf = &HandlerFactory{handlers: make(map[uint16](func() Handler))}
	return hf
}

func (this *HandlerFactory) AddHandler(handlerId uint16, builder func() Handler) {
	this.handlers[handlerId] = builder
}

func (this *HandlerFactory) CreateHandler(handlerId uint16) Handler {
	if builder, ok := this.handlers[handlerId]; ok {
		return builder()
	}
	return nil
}

type Handler interface {
	Init(outChan chan *OutPacket)
	HandlePacket(p *InPacket)
}
