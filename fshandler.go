package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FsRequest struct {
	reqId   uint32
	arg1    uint32
	arg2    uint32
	arg3    uint32
	arg4    uint32
	volId   uint16
	reqType uint16
	dataLen uint16
	strData []byte
}

func (this *FsRequest) getString(offset uint32) string {

	l := uint32(this.strData[offset])
	return string(this.strData[offset+1 : offset+1+l])
}

const (
	PT_ACTION_LOCATE_OBJECT  = 8
	PT_ACTION_FREE_LOCK      = 15
	PT_ACTION_EXAMINE_OBJECT = 23
	PT_ACTION_EXAMINE_NEXT   = 24
	PT_ACTION_DISK_INFO      = 25
	PT_ACTION_INFO           = 26
	PT_ACTION_PARENT         = 29
	PT_ACTION_READ           = 82
	PT_ACTION_FIND_UPDATE    = 1004
	PT_ACTION_FIND_INPUT     = 1005
	PT_ACTION_FIND_OUTPUT    = 1006
	PT_ACTION_END            = 1007
)

const (
	SHARED_LOCK    = -2
	ACCESS_READ    = -2
	EXCLUSIVE_LOCK = -1
	ACCESS_WRITE   = -1

	DOS_FALSE = 0
	DOS_TRUE  = -1

	ERROR_OBJECT_IN_USE     = 202
	ERROR_OBJECT_WRONG_TYPE = 212
	ERROR_OBJECT_NOT_FOUND  = 205
	ERROR_NO_MORE_ENTRIES   = 232
)

var AmigaEpoch time.Time = time.Date(1978, 1, 1, 0, 0, 0, 0, time.Local)

type fsLock struct {
	id   uint32
	name string
	mode int32
}

type fsFileHandle struct {
	id uint32
	fh *os.File
}

type FsHandler struct {
	outChan chan *OutPacket
	nextId  uint32
	locks   map[uint32]*fsLock
	files   map[uint32]*fsFileHandle
}

func (this *FsHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan
	this.nextId = 1
	this.locks = make(map[uint32]*fsLock)
	this.files = make(map[uint32]*fsFileHandle)

	this.locks[0] = &fsLock{0, "/", SHARED_LOCK}
}

func (this *FsHandler) findLock(id uint32) *fsLock {

	return this.locks[id]
}

func (this *FsHandler) findLockByPath(path string) *fsLock {

	for _, l := range this.locks {
		if l.name == path {
			return l
		}
	}

	return nil
}

func (this *FsHandler) resolvePath(srcLockId uint32, origPath string) string {

	srcLock := this.findLock(srcLockId)

	path := origPath
	colonPos := strings.Index(origPath, ":")
	if colonPos >= 0 {
		path = string(path[colonPos+1:])
	}

	if srcLock != nil {
		path = filepath.Join(srcLock.name, path)
	}

	if path == "" {
		path = "/"
	}

	return path
}

func (this *FsHandler) replyToPacket(p *InPacket, req *FsRequest, res1 int32, res2 int32, data []byte) {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, req.reqId)
	binary.Write(buf, binary.BigEndian, res1)
	binary.Write(buf, binary.BigEndian, res2)

	binary.Write(buf, binary.BigEndian, uint16(len(data)))
	if len(data) > 0 {
		buf.Write(data)
	}

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       buf.Bytes()}
}

func (this *FsHandler) createLock(path string, access int32) (l *fsLock, code int) {

	l = this.findLockByPath(path)

	if l != nil {

		if access == EXCLUSIVE_LOCK {

			return nil, ERROR_OBJECT_IN_USE
		}

	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, ERROR_OBJECT_NOT_FOUND
	}

	l = &fsLock{this.nextId, path, access}

	this.locks[this.nextId] = l

	fmt.Printf("Locking path '%s', id %d\n", path, this.nextId)

	this.nextId++

	return l, 0
}

func (this *FsHandler) actionLocateObject(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, req.getString(req.arg2))

	l, code := this.createLock(path, int32(req.arg3))

	if l == nil {
		this.replyToPacket(p, req, DOS_FALSE, int32(code), []byte{})
	} else {
		this.replyToPacket(p, req, DOS_TRUE, int32(l.id), []byte{})
	}
}

func (this *FsHandler) actionFreeLock(p *InPacket, req *FsRequest) {

	fmt.Printf("Unlocking path '%s', id %d\n", this.resolvePath(req.arg1, ""), req.arg1)

	//if req.arg1 > 0 {
	//	delete(this.locks, req.arg1)
	//}
	this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
}

func (this *FsHandler) actionOpenFile(p *InPacket, req *FsRequest) {

	fileName := req.getString(req.arg3)
	path := this.resolvePath(req.arg2, fileName)

	mode := os.O_RDWR

	fi, err := os.Stat(path)

	switch req.reqType {
	case PT_ACTION_FIND_INPUT:
		if err != nil && os.IsNotExist(err) {
			this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_NOT_FOUND, []byte{})
			return
		}

	case PT_ACTION_FIND_UPDATE:
		mode = os.O_RDWR | os.O_CREATE

	case PT_ACTION_FIND_OUTPUT:
		if err == nil {
			if err = os.Remove(path); err != nil {
				this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_IN_USE, []byte{})
				return
			}
		}
	}

	if fi != nil && !fi.Mode().IsRegular() {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_WRONG_TYPE, []byte{})
		return
	}

	if f, err := os.OpenFile(path, mode, 755); err == nil {
		fh := &fsFileHandle{this.nextId, f}
		this.files[this.nextId] = fh
		this.nextId++
		this.replyToPacket(p, req, DOS_TRUE, int32(fh.id), []byte{})
	} else {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_IN_USE, []byte{})
	}
}

func (this *FsHandler) actionCloseFile(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh != nil {
		fh.fh.Close()
	}

	delete(this.files, req.arg1)
}

func (this *FsHandler) actionInfo(p *InPacket, req *FsRequest) {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, 100000)
	binary.Write(buf, binary.BigEndian, 1000)

	this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
}

func (this *FsHandler) actionExamine(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")

	fi, err := os.Stat(path)
	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	buf := new(bytes.Buffer)

	fmt.Printf("Examine %s\n", path)

	this.writeFileInfoBlock(0, path, fi, buf)

	this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
}

func (this *FsHandler) actionExamineNext(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")
	ix := int(req.arg2)
	entries, err := ioutil.ReadDir(path)

	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_NO_MORE_ENTRIES, []byte{})
		return
	}

	if ix < 0 || ix >= len(entries) {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_NO_MORE_ENTRIES, []byte{})
	} else {
		fi := entries[ix]

		buf := new(bytes.Buffer)

		fmt.Printf("Examine Next %s - %s\n", path, fi.Name())

		this.writeFileInfoBlock(int32(ix+1), path, fi, buf)

		this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
	}
}

func (this *FsHandler) actionParent(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")

	if path == "/" {
		this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
	} else {
		parentPath := filepath.Dir(path)
		l, code := this.createLock(parentPath, SHARED_LOCK)
		if l == nil {
			this.replyToPacket(p, req, DOS_FALSE, int32(code), []byte{})
		} else {
			this.replyToPacket(p, req, DOS_TRUE, int32(l.id), []byte{})
		}
	}
}

func (this *FsHandler) actionRead(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh == nil {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	data := make([]byte, req.arg3)
	bytesRead, err := fh.fh.Read(data)

	if err != nil && err != io.EOF {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	this.replyToPacket(p, req, int32(bytesRead), 0, data)
}

func (this *FsHandler) writeFileInfoBlock(diskKey int32, path string, fi os.FileInfo, buf *bytes.Buffer) {
	/* Fill in FileInfoBlock

	struct FileInfoBlock {
		LONG	  fib_DiskKey;
		LONG	  fib_DirEntryType;  // Type of Directory. If < 0, then a plain file.   If > 0 a directory
		char	  fib_FileName[108]; // Null terminated. Max 30 chars used for now
		LONG	  fib_Protection;    // bit mask of protection, rwxd are 3-0.
		LONG	  fib_EntryType;
		LONG	  fib_Size;	     	 // Number of bytes in file
		LONG	  fib_NumBlocks;     // Number of blocks in file
		struct DateStamp fib_Date;	 // Date file last changed
		char	  fib_Comment[80];   // Null terminated comment associated with file
		// Don't worry about the rest of it.
	};
	*/

	// fib_DiskKey
	binary.Write(buf, binary.BigEndian, diskKey)

	// fib_DirEntryType
	et := int32(0)
	if fi.IsDir() {
		if path == "/" {
			et = 2 // Root
		} else {
			et = 1 // Dir
		}
	} else {
		et = -3 // File
	}
	binary.Write(buf, binary.BigEndian, et)

	// fib_FileName
	fn := fi.Name()
	if len(fn) > 30 {
		fn = fn[:30]
	}
	buf.WriteByte(uint8(len(fn)))
	buf.Write([]byte(fn))
	remainder := 107 - len(fn)
	buf.Write(make([]byte, remainder, remainder))

	// fib_Protection
	binary.Write(buf, binary.BigEndian, int32(0))

	// fib_EntryType
	binary.Write(buf, binary.BigEndian, et)

	// fib_Size
	binary.Write(buf, binary.BigEndian, int32(fi.Size()))

	// fib_NumBlocks
	nb := fi.Size() / 512
	if fi.Size()%512 > 0 {
		nb++
	}
	binary.Write(buf, binary.BigEndian, int32(nb))

	// fib_Date
	mt := fi.ModTime()
	_, o := mt.Zone()
	mt = mt.Add(time.Second * time.Duration(o))

	t := mt.Sub(AmigaEpoch)
	binary.Write(buf, binary.BigEndian, int32(t.Hours()/24))

	ds := mt.Truncate(time.Hour * 24)
	binary.Write(buf, binary.BigEndian, int32(mt.Sub(ds).Minutes()))

	ds = mt.Truncate(time.Minute)
	binary.Write(buf, binary.BigEndian, int32(mt.Sub(ds).Nanoseconds()/20000000))

	// fib_Comment
	buf.Write(make([]byte, 80, 80))

}

func (this *FsHandler) HandlePacket(p *InPacket) {

	ix := 0
	req := &FsRequest{}
	req.reqId = binary.BigEndian.Uint32(p.Data[ix:])
	ix += 4
	req.arg1 = binary.BigEndian.Uint32(p.Data[ix:])
	ix += 4
	req.arg2 = binary.BigEndian.Uint32(p.Data[ix:])
	ix += 4
	req.arg3 = binary.BigEndian.Uint32(p.Data[ix:])
	ix += 4
	req.arg4 = binary.BigEndian.Uint32(p.Data[ix:])
	ix += 4
	req.volId = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.reqType = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.dataLen = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.strData = p.Data[ix:]

	switch req.reqType {
	case PT_ACTION_LOCATE_OBJECT:
		this.actionLocateObject(p, req)
	case PT_ACTION_FREE_LOCK:
		this.actionFreeLock(p, req)
	case PT_ACTION_FIND_INPUT:
		this.actionOpenFile(p, req)
	case PT_ACTION_FIND_OUTPUT:
		this.actionOpenFile(p, req)
	case PT_ACTION_FIND_UPDATE:
		this.actionOpenFile(p, req)
	case PT_ACTION_END:
		this.actionCloseFile(p, req)
	case PT_ACTION_INFO:
		this.actionInfo(p, req)
	case PT_ACTION_DISK_INFO:
		this.actionInfo(p, req)
	case PT_ACTION_EXAMINE_OBJECT:
		this.actionExamine(p, req)
	case PT_ACTION_EXAMINE_NEXT:
		this.actionExamineNext(p, req)
	case PT_ACTION_PARENT:
		this.actionParent(p, req)
	case PT_ACTION_READ:
		this.actionRead(p, req)
	}
}

func (this *FsHandler) Quit() {
}

func NewFsHandler() Handler {
	return &FsHandler{}
}
