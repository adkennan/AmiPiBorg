package main

type Connection struct {
	connId      uint16
	handler     Handler
	inChan      chan *InPacket
	outChan     chan *OutPacket
	handlerChan chan *OutPacket
	ctrlChan    chan bool
}

func NewConnection(connId uint16, h Handler, outChan chan *OutPacket) (cnn *Connection) {

	cnn = &Connection{
		connId:      connId,
		handler:     h,
		inChan:      make(chan *InPacket, 100),
		outChan:     outChan,
		handlerChan: make(chan *OutPacket, 1000),
		ctrlChan:    make(chan bool)}

	h.Init(cnn.handlerChan)

	return cnn
}

func (this *Connection) HandlePacket(p *InPacket) {
	this.inChan <- p
}

func (this *Connection) GetControlChannel() chan bool {
	return this.ctrlChan
}

func (this *Connection) Run() {

	done := false
	for !done {
		select {
		case p := <-this.inChan:
			this.handler.HandlePacket(p)
		case p := <-this.handlerChan:
			p.ConnId = this.connId
			this.outChan <- p
		case done = <-this.ctrlChan:
		}
	}

	this.handler.Quit()
}
