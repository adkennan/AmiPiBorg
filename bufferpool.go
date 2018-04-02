package main

type BufferPool struct {
	pool chan []byte
}

func NewBufferPool(capacity int) (bp *BufferPool) {

	bp = &BufferPool{
		pool: make(chan []byte, capacity)}

	return bp
}

func (this *BufferPool) AllocBuffer() (b []byte) {
	select {
	case b = <-this.pool:
		b = b[:0]
	default:
		b = make([]byte, 100)
	}
	return b
}

func (this *BufferPool) ReleaseBuffer(b []byte) {

	select {
	case this.pool <- b:
	default:
	}

}
