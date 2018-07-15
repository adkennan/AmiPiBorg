package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	SS_Disconnected = iota
	SS_Connected
)

const (
	MaxRecentPackets = 100
	ServerVersion    = 1
)

type Server struct {
	bufPool        *BufferPool
	packetReader   *PacketReader
	packetWriter   *PacketWriter
	remote         Remote
	state          uint16
	packId         uint16
	lastInPackId   uint16
	connChan       chan *OutPacket
	connections    map[uint16]*Connection
	handlerFactory *HandlerFactory
	recentPackets  []*OutPacket
}

type OutPacket struct {
	ConnId     uint16
	PackId     uint16
	PacketType uint8
	Data       []byte
}

func NewServer(remote Remote, handlerFac *HandlerFactory) (srv *Server) {

	bp := NewBufferPool(100)

	srv = &Server{
		bufPool:        bp,
		packetReader:   NewPacketReader(bp, remote),
		packetWriter:   NewPacketWriter(remote),
		remote:         remote,
		state:          SS_Disconnected,
		packId:         1,
		connChan:       make(chan *OutPacket, 100),
		connections:    nil,
		handlerFactory: handlerFac,
		recentPackets:  make([]*OutPacket, MaxRecentPackets+1)}

	remote.Init(bp)

	return srv
}

func (this *Server) GetConnection(connId uint16) *Connection {

	return this.connections[connId]
}

func (this *Server) HandleControlPacket(p *InPacket) {

	switch p.PacketType {
	case MT_Init:
		this.state = SS_Connected
		this.packId = 1
		this.lastInPackId = 1
		this.SendHello()
		this.connections = make(map[uint16]*Connection)
		fmt.Printf("Server Connected\n")

	case MT_Ping:
		this.WritePacket(DEFAULT_CONNECTION, MT_Pong, []byte{})

	case MT_Shutdown:
		this.state = SS_Disconnected
		this.WritePacket(DEFAULT_CONNECTION, MT_Goodbye, []byte{})
		fmt.Printf("Server Disconnected\n")

	case MT_Resend:
		this.resendPacket(binary.BigEndian.Uint16(p.Data))
	}
}

func (this *Server) SendHello() {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint16(ServerVersion))

	ids, names := this.handlerFactory.GetHandlerDescriptions()

	binary.Write(buf, binary.BigEndian, uint16(len(ids)))

	for ix, id := range ids {

		binary.Write(buf, binary.BigEndian, id)
		name := names[ix]
		buf.Write([]byte(name))
		for ij := len(name); ij < 10; ij++ {
			binary.Write(buf, binary.BigEndian, uint8(0))
		}
	}

	this.WritePacket(DEFAULT_CONNECTION, MT_Hello, buf.Bytes())
}

func (this *Server) CreateConnection(p *InPacket) {

	handlerId := binary.BigEndian.Uint16(p.Data)

	h := this.handlerFactory.CreateHandler(handlerId)
	if h == nil {
		fmt.Printf("No handler of type %d\n", handlerId)
		this.WritePacket(p.ConnId, MT_NoHandler, []byte{})
	} else {

		c := NewConnection(p.ConnId, h, this.connChan)

		this.connections[p.ConnId] = c

		fmt.Printf("Create connection %d\n", p.ConnId)

		this.WritePacket(p.ConnId, MT_Connected, []byte{})

		go c.Run()
	}
}

func (this *Server) HandlePacket(p *InPacket) (err error) {

	fmt.Printf("Packet %d, flags %d\n", p.PacketId, p.Flags)
	if (p.Flags & PF_Resend) == 0 {

		if p.PacketId-1 > this.lastInPackId {

			fmt.Printf("Expecting packet %d, got packet %d\n", this.lastInPackId+1, p.PacketId)

			for ix := this.lastInPackId + 1; ix < p.PacketId; ix++ {
				this.RequestResend(ix)
			}
		}

		this.lastInPackId = p.PacketId
	}

	if p.ConnId == DEFAULT_CONNECTION {
		this.HandleControlPacket(p)
	} else {

		cnn := this.GetConnection(p.ConnId)
		if cnn == nil {
			switch p.PacketType {
			case MT_Connect:
				this.CreateConnection(p)
			case MT_Disconnect:
			default:
				this.WritePacket(p.ConnId, MT_NoConnection, []byte{})
			}
		} else if p.PacketType == MT_Disconnect {
			cnn.GetControlChannel() <- true
			fmt.Printf("Disconnect connection %d\n", p.ConnId)
			delete(this.connections, p.ConnId)
			this.WritePacket(p.ConnId, MT_Disconnected, []byte{})
		} else {
			cnn.HandlePacket(p)
		}
	}

	return nil
}

func (this *Server) RequestResend(packId uint16) {

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, packId)

	this.WritePacket(DEFAULT_CONNECTION, MT_Resend, buf.Bytes())
}

func (this *Server) WritePacket(connId uint16, packetType uint8, data []byte) (packId uint16, err error) {
	pId := this.packId
	err = this.packetWriter.Write(packetType, 0, connId, pId, data)
	if err != nil {
		return 0, err
	}
	this.packId++
	return pId, nil
}

func (this *Server) storeOutPacket(op *OutPacket) {
	this.recentPackets = append(this.recentPackets, op)
	if len(this.recentPackets) > MaxRecentPackets {
		this.recentPackets = this.recentPackets[1:]
	}
}

func (this *Server) resendPacket(packId uint16) {

	fmt.Printf("Request resend of packet %d\n", packId)
	for _, p := range this.recentPackets {
		if p != nil && p.PackId == packId {
			fmt.Printf("Sent\n")
			this.packetWriter.Write(p.PacketType, PF_Resend, p.ConnId, p.PackId, p.Data)
			break
		}
	}
}

func (this *Server) Run() (err error) {

	err = this.remote.Open()
	if err != nil {
		return err
	}

	this.packetReader.Start()

	rc := this.packetReader.GetOutputChannel()

	fmt.Printf("Listening\n")

	for {
		select {
		case ip := <-rc:
			this.HandlePacket(ip)
		case op := <-this.connChan:
			op.PackId, err = this.WritePacket(op.ConnId, op.PacketType, op.Data)
			if err != nil {
				return err
			}
			this.storeOutPacket(op)
		}
	}

	return err
}
