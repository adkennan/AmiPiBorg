package main

import (
	"go.bug.st/serial.v1"
)

type SerialRemote struct {
	bufferPool *BufferPool
	readChan   chan []byte
	devName    string
	baud       int
	port       serial.Port
	running    bool
}

func NewSerialRemote(devName string, baud int) (sr *SerialRemote, err error) {

	sr = &SerialRemote{
		bufferPool: nil,
		readChan:   make(chan []byte, 10),
		devName:    devName,
		baud:       baud,
		port:       nil}

	return sr, nil
}

func (this *SerialRemote) Init(bufferPool *BufferPool) (err error) {
	this.bufferPool = bufferPool

	return nil
}

func (this *SerialRemote) Open() (err error) {

	config := &serial.Mode{
		BaudRate: this.baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit}

	this.port, err = serial.Open(this.devName, config)
	if err != nil {
		return err
	}

	this.running = true
	go this.run()
	return nil
}

func (this *SerialRemote) Close() {
	this.running = false

	if this.port != nil {
		this.port.Close()
		this.port = nil
	}

}

func (this *SerialRemote) GetReadChan() (readChan chan []byte) {
	return this.readChan
}

func (this *SerialRemote) Write(data []byte) (bytesWritten int, err error) {

	return this.port.Write(data)
}

func (this *SerialRemote) run() {

	for this.running {

		buf := this.bufferPool.AllocBuffer()
		bytesRead, err := this.port.Read(buf)
		if err != nil {
			break
		}

		if bytesRead > 0 {

			this.readChan <- buf[0:bytesRead]
		}
	}
}
