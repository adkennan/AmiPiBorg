package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gvalkov/golang-evdev"
)

const (
	mouse    = 1
	keyboard = 2
)

type InputHandler struct {
	outChan  chan *OutPacket
	ctrlChan chan bool
	device   *evdev.InputDevice
	running  bool
}

func (this *InputHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan
	this.ctrlChan = make(chan bool)

	this.device, _ = evdev.Open("/dev/input/mice")
}

func (this *InputHandler) Run() {

	done := false

	eventReader := make(chan *evdev.InputEvent, 100)

	go func() {
		for !done {
			e, err := this.device.ReadOne()
			if err != nil {
				fmt.Println(err.Error())
			}
			if e != nil {
				eventReader <- e
			}
		}
	}()

	for !done {
		select {
		case ev := <-eventReader:
			//	fmt.Println(ev.String())
			buf := new(bytes.Buffer)

			binary.Write(buf, binary.BigEndian, uint16(mouse))
			binary.Write(buf, binary.BigEndian, int8(ev.Value))
			binary.Write(buf, binary.BigEndian, int8(0))

			this.outChan <- &OutPacket{
				PacketType: MT_Data,
				Data:       buf.Bytes()}
		case done = <-this.ctrlChan:
		}
	}

}

func (this *InputHandler) Quit() {
	this.ctrlChan <- true
}

func (this *InputHandler) HandlePacket(p *InPacket) {

	this.running = true
	go this.Run()
}

func NewInputHandler() Handler {
	return &InputHandler{}
}
