package main

const (
	HT_Ping = 0
)

type HandlerFactory struct {
	handlers map[uint8](func() Handler)
}

func NewHandlerFactory() (hf *HandlerFactory) {
	hf = &HandlerFactory{handlers: make(map[uint8](func() Handler))}
	return hf
}

func (this *HandlerFactory) AddHandler(handlerId uint8, builder func() Handler) {
	this.handlers[handlerId] = builder
}

func (this *HandlerFactory) CreateHandler(handlerId uint8) Handler {
	if builder, ok := this.handlers[handlerId]; ok {
		return builder()
	}
	return nil
}

type Handler interface {
	Init(outChan chan *OutPacket)
	HandlePacket(p *InPacket)
}
