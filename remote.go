package main

type Remote interface {
	Init(bufferPool *BufferPool) (err error)
	Open() (err error)
	Close()
	GetReadChan() (readChan chan []byte)
	Write(data []byte) (bytesWritten int, err error)
}
