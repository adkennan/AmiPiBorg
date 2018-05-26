package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/gvalkov/golang-evdev"
	"time"
)

const (
	mousemove = 1
	mousebtn  = 2
	keyboard  = 4
)

const (
	iecode_lbutton   = 0x68
	iecode_rbutton   = 0x69
	iecode_mbutton   = 0x6a
	iecode_nobutton  = 0xff
	iecode_up_prefix = 0x80
)

type InputHandler struct {
	outChan  chan *OutPacket
	ctrlChan chan bool
	devices  []*evdev.InputDevice
	running  bool
}

func (this *InputHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan
	this.ctrlChan = make(chan bool)
	this.devices = make([]*evdev.InputDevice, 1)

	this.openInputDevices()
}

func hasEventCode(caps []evdev.CapabilityCode, ev int) bool {
	for _, cc := range caps {
		if cc.Code == ev {
			return true
		}
	}
	return false
}

func (this *InputHandler) openInputDevices() {

	paths, err := evdev.ListInputDevicePaths("/dev/input/event*")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, path := range paths {
		if evdev.IsInputDevice(path) {

			dev, err := evdev.Open(path)
			if err != nil {
				fmt.Printf("Can't open %s\n", path)
				continue
			}

			fmt.Println(dev)

			cap := dev.Capabilities[evdev.CapabilityType{evdev.EV_REL, evdev.EV[evdev.EV_REL]}]
			if cap == nil {
				fmt.Printf("%s has no EV_REL capability\n", path)
				continue
			}

			cap = dev.Capabilities[evdev.CapabilityType{evdev.EV_KEY, evdev.EV[evdev.EV_KEY]}]
			if cap == nil {
				fmt.Printf("%s has no EV_KEY capability\n", path)
				continue
			}

			if !hasEventCode(cap, evdev.BTN_LEFT) {
				fmt.Printf("%s has no BTN_LEFT code\n", path)
				continue
			}

			if !hasEventCode(cap, evdev.BTN_RIGHT) {
				fmt.Printf("%s has no BTN_RIGHT code\n", path)
				continue
			}

			fmt.Printf("%s looks like a mouse.\n", path)

			this.devices = append(this.devices, dev)
		}
	}
}

func (this *InputHandler) Run() {

	done := false

	eventReader := make(chan *evdev.InputEvent, 100)
	ticker := time.NewTicker(time.Millisecond * 20)

	for _, dev := range this.devices {
		if dev == nil {
			continue
		}
		go func() {
			for !done {
				e, err := dev.ReadOne()
				if err != nil {
					fmt.Println(err.Error())
				}
				if e != nil {
					eventReader <- e
				}
			}
		}()
	}

	var dx int8 = 0
	var dy int8 = 0

	for !done {
		select {
		case ev := <-eventReader:
			switch ev.Type {
			case evdev.EV_REL:
				switch ev.Code {
				case evdev.REL_X:
					dx += int8(ev.Value)
				case evdev.REL_Y:
					dy += int8(ev.Value)
				}
			case evdev.EV_KEY:
				buf := new(bytes.Buffer)

				binary.Write(buf, binary.BigEndian, uint16(mousebtn))

				btn := uint16(0)
				if ev.Value == 0 {
					btn = iecode_up_prefix
				}
				switch ev.Code {
				case evdev.BTN_LEFT:
					btn |= iecode_lbutton
				case evdev.BTN_MIDDLE:
					btn |= iecode_mbutton
				case evdev.BTN_RIGHT:
					btn |= iecode_rbutton

				}

				binary.Write(buf, binary.BigEndian, btn)
				this.outChan <- &OutPacket{
					PacketType: MT_Data,
					Data:       buf.Bytes()}
			}

		case _ = <-ticker.C:
			if dx != 0 || dy != 0 {
				buf := new(bytes.Buffer)

				binary.Write(buf, binary.BigEndian, uint16(mousemove))
				binary.Write(buf, binary.BigEndian, dx)
				binary.Write(buf, binary.BigEndian, dy)

				dx = 0
				dy = 0

				this.outChan <- &OutPacket{
					PacketType: MT_Data,
					Data:       buf.Bytes()}
			}
		case done = <-this.ctrlChan:
		}
	}

	ticker.Stop()
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
