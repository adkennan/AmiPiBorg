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

var keyMap map[uint16]uint16 = map[uint16]uint16{
	evdev.KEY_ESC:        0x45,
	evdev.KEY_F1:         0x50,
	evdev.KEY_F2:         0x51,
	evdev.KEY_F3:         0x52,
	evdev.KEY_F4:         0x53,
	evdev.KEY_F5:         0x54,
	evdev.KEY_F6:         0x55,
	evdev.KEY_F7:         0x56,
	evdev.KEY_F8:         0x57,
	evdev.KEY_F9:         0x58,
	evdev.KEY_F10:        0x59,
	evdev.KEY_1:          0x01,
	evdev.KEY_GRAVE:      0x00,
	evdev.KEY_2:          0x02,
	evdev.KEY_3:          0x03,
	evdev.KEY_4:          0x04,
	evdev.KEY_5:          0x05,
	evdev.KEY_6:          0x06,
	evdev.KEY_7:          0x07,
	evdev.KEY_8:          0x08,
	evdev.KEY_9:          0x09,
	evdev.KEY_0:          0x0A,
	evdev.KEY_MINUS:      0x0B,
	evdev.KEY_EQUAL:      0x0C,
	evdev.KEY_BACKSLASH:  0x0d,
	evdev.KEY_BACKSPACE:  0x41,
	evdev.KEY_TAB:        0x42,
	evdev.KEY_Q:          0x10,
	evdev.KEY_W:          0x11,
	evdev.KEY_E:          0x12,
	evdev.KEY_R:          0x13,
	evdev.KEY_T:          0x14,
	evdev.KEY_Y:          0x15,
	evdev.KEY_U:          0x16,
	evdev.KEY_I:          0x17,
	evdev.KEY_O:          0x18,
	evdev.KEY_P:          0x19,
	evdev.KEY_LEFTBRACE:  0x1A,
	evdev.KEY_RIGHTBRACE: 0x1B,
	evdev.KEY_ENTER:      0x44,
	evdev.KEY_LEFTCTRL:   0x63,
	evdev.KEY_RIGHTCTRL:  0x63,
	evdev.KEY_CAPSLOCK:   0x62,
	evdev.KEY_A:          0x20,
	evdev.KEY_S:          0x21,
	evdev.KEY_D:          0x22,
	evdev.KEY_F:          0x23,
	evdev.KEY_G:          0x24,
	evdev.KEY_H:          0x25,
	evdev.KEY_J:          0x26,
	evdev.KEY_K:          0x27,
	evdev.KEY_L:          0x28,
	evdev.KEY_SEMICOLON:  0x29,
	evdev.KEY_APOSTROPHE: 0x2A,
	evdev.KEY_LEFTSHIFT:  0x60,
	evdev.KEY_Z:          0x31,
	evdev.KEY_X:          0x32,
	evdev.KEY_C:          0x33,
	evdev.KEY_V:          0x34,
	evdev.KEY_B:          0x35,
	evdev.KEY_N:          0x36,
	evdev.KEY_M:          0x37,
	evdev.KEY_COMMA:      0x38,
	evdev.KEY_DOT:        0x39,
	evdev.KEY_SLASH:      0x3A,
	evdev.KEY_RIGHTSHIFT: 0x61,
	evdev.KEY_LEFTMETA:   0x66,
	evdev.KEY_LEFTALT:    0x67,
	evdev.KEY_SPACE:      0x40,
	evdev.KEY_RIGHTALT:   0x68,
	evdev.KEY_RIGHTMETA:  0x67,
	evdev.KEY_HELP:       0x5F,
	evdev.KEY_SYSRQ:      0x5F,
	evdev.KEY_DELETE:     0x46,

	evdev.KEY_HOME:     0x4F,
	evdev.KEY_UP:       0x4C,
	evdev.KEY_PAGEUP:   0x4C,
	evdev.KEY_LEFT:     0x4F,
	evdev.KEY_RIGHT:    0x4E,
	evdev.KEY_END:      0x4E,
	evdev.KEY_DOWN:     0x4D,
	evdev.KEY_PAGEDOWN: 0x4D,

	evdev.KEY_NUMLOCK:    0x5A,
	evdev.KEY_SCROLLLOCK: 0x5B,
	evdev.KEY_KPSLASH:    0x5C,
	evdev.KEY_KPASTERISK: 0x5D,
	evdev.KEY_KP7:        0x3D,
	evdev.KEY_KP8:        0x3E,
	evdev.KEY_KP9:        0x3F,
	evdev.KEY_KPMINUS:    0x4A,
	evdev.KEY_KP4:        0x2D,
	evdev.KEY_KP5:        0x2E,
	evdev.KEY_KP6:        0x2F,
	evdev.KEY_KPPLUS:     0x5E,
	evdev.KEY_KP1:        0x1D,
	evdev.KEY_KP2:        0x1E,
	evdev.KEY_KP3:        0x1F,
	evdev.KEY_KP0:        0x0F,
	evdev.KEY_KPDOT:      0x3C,
	evdev.KEY_KPENTER:    0x43,
}

var keyQualifiers map[uint16]uint16 = map[uint16]uint16{
	evdev.KEY_LEFTCTRL:   0x08,
	evdev.KEY_RIGHTCTRL:  0x08,
	evdev.KEY_CAPSLOCK:   0x04,
	evdev.KEY_LEFTSHIFT:  0x01,
	evdev.KEY_RIGHTSHIFT: 0x02,
	evdev.KEY_LEFTMETA:   0x40,
	evdev.KEY_LEFTALT:    0x10,
	evdev.KEY_RIGHTALT:   0x20,
	evdev.KEY_RIGHTMETA:  0x80,
}

var keyTempQualifiers map[uint16]uint16 = map[uint16]uint16{
	evdev.KEY_HOME:     0x01,
	evdev.KEY_PAGEUP:   0x01,
	evdev.KEY_END:      0x01,
	evdev.KEY_PAGEDOWN: 0x01,
}

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

			isMouse := true
			isKeyboard := true

			cap := dev.Capabilities[evdev.CapabilityType{evdev.EV_REL, evdev.EV[evdev.EV_REL]}]
			if cap == nil {
				fmt.Printf("%s has no EV_REL capability\n", path)
				isMouse = false
			}

			cap = dev.Capabilities[evdev.CapabilityType{evdev.EV_KEY, evdev.EV[evdev.EV_KEY]}]
			if cap == nil {
				fmt.Printf("%s has no EV_KEY capability\n", path)
				isMouse = false
				isKeyboard = false
			}

			if !hasEventCode(cap, evdev.BTN_LEFT) {
				fmt.Printf("%s has no BTN_LEFT code\n", path)
				isMouse = false
			}

			if !hasEventCode(cap, evdev.BTN_RIGHT) {
				fmt.Printf("%s has no BTN_RIGHT code\n", path)
				isMouse = false
			}

			if isMouse {
				fmt.Printf("%s looks like a mouse.\n", path)

				this.devices = append(this.devices, dev)
			} else if isKeyboard {
				fmt.Printf("%s looks like a keyboard.\n", path)

				this.devices = append(this.devices, dev)
			}
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
		go func(d *evdev.InputDevice) {
			if d.Grab() == nil {
				defer d.Release()
			}
			for !done {
				e, err := d.ReadOne()
				if err != nil {
					fmt.Println(err.Error())
				}
				if e != nil {
					eventReader <- e
				}
			}
		}(dev)
	}

	var dx int8 = 0
	var dy int8 = 0
	var currQual uint16 = 0
	var capsLock uint16 = 0

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

				c := ev.Code
				if c >= evdev.BTN_LEFT && c <= evdev.BTN_RIGHT {

					btn := uint16(0)
					if ev.Value == 0 {
						btn = iecode_up_prefix
					}
					switch {
					case c == evdev.BTN_LEFT:
						btn |= iecode_lbutton
					case c == evdev.BTN_MIDDLE:
						btn |= iecode_mbutton
					case c == evdev.BTN_RIGHT:
						btn |= iecode_rbutton
					}

					binary.Write(buf, binary.BigEndian, uint16(mousebtn))
					binary.Write(buf, binary.BigEndian, btn)
					this.outChan <- &OutPacket{
						PacketType: MT_Data,
						Data:       buf.Bytes()}
				} else {

					key, ok := keyMap[c]
					if ok {
						qual := keyQualifiers[c]

						if c == evdev.KEY_CAPSLOCK && ev.Value == 1 {
							capsLock ^= qual
						} else if ev.Value == 0 {
							key |= iecode_up_prefix
							currQual &= ^qual
						} else {
							currQual |= qual
						}

						binary.Write(buf, binary.BigEndian, uint16(keyboard))
						binary.Write(buf, binary.BigEndian, key)
						binary.Write(buf, binary.BigEndian, currQual|keyTempQualifiers[c]|capsLock)
						this.outChan <- &OutPacket{
							PacketType: MT_Data,
							Data:       buf.Bytes()}

					}
				}
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
