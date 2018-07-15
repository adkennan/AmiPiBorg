package main

import (
	//"go.bug.st/serial.v1"
	"github.com/tarm/serial"
	"time"
)

type SerialRemote struct {
	bufferPool *BufferPool
	readChan   chan []byte
	devName    string
	baud       int
	port       *serial.Port
	running    bool
	writeChan  chan []byte
	ctrlChan   chan bool
}

func NewSerialRemote(devName string, baud int) (sr *SerialRemote, err error) {

	sr = &SerialRemote{
		bufferPool: nil,
		readChan:   make(chan []byte, 10),
		devName:    devName,
		baud:       baud,
		port:       nil,
		running:    false,
		writeChan:  make(chan []byte, 100),
		ctrlChan:   make(chan bool)}

	return sr, nil
}

func (this *SerialRemote) Init(bufferPool *BufferPool) (err error) {
	this.bufferPool = bufferPool

	return nil
}

func (this *SerialRemote) Open() (err error) {

	/*config := &serial.Mode{
		BaudRate: this.baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit}

	this.port, err = serial.Open(this.devName, config)
	*/
	config := &serial.Config{
		Name:     this.devName,
		Baud:     19200,
		Size:     serial.DefaultSize,
		Parity:   serial.ParityNone,
		StopBits: serial.Stop1}

	this.port, err = serial.OpenPort(config)
	if err != nil {
		return err
	}

	this.running = true
	go this.reader()
	go this.writer()
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

func (this *SerialRemote) Write(data []byte) {

	this.writeChan <- data
}

func (this *SerialRemote) writer() {

	for {
		select {
		case <-this.ctrlChan:
			return
		case buf := <-this.writeChan:
			this.port.Write(buf)
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (this *SerialRemote) reader() {

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
