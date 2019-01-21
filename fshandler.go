package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FsRequest struct {
	reqId   uint32
	arg1    int32
	arg2    int32
	arg3    int32
	arg4    int32
	volId   uint16
	reqType uint16
	dataLen uint16
	strData []byte
}

func (this *FsRequest) getString(offset int32) string {

	l := int32(this.strData[offset])
	return string(this.strData[offset+1 : offset+1+l])
}

func (this *FsRequest) getBytes(offset int32, length int32) []byte {
	return this.strData[offset : offset+length]
}

const (
	PT_ACTION_LOCATE_OBJECT  = 8
	PT_ACTION_FREE_LOCK      = 15
	PT_ACTION_DELETE_OBJECT  = 16
	PT_ACTION_RENAME_OBJECT  = 17
	PT_ACTION_CREATE_DIR     = 22
	PT_ACTION_EXAMINE_OBJECT = 23
	PT_ACTION_EXAMINE_NEXT   = 24
	PT_ACTION_DISK_INFO      = 25
	PT_ACTION_INFO           = 26
	PT_ACTION_PARENT         = 29
	PT_ACTION_SAME_LOCK      = 40
	PT_ACTION_READ           = 82
	PT_ACTION_WRITE          = 87
	PT_ACTION_FIND_UPDATE    = 1004
	PT_ACTION_FIND_INPUT     = 1005
	PT_ACTION_FIND_OUTPUT    = 1006
	PT_ACTION_END            = 1007
	PT_ACTION_SEEK           = 1008
	PT_ACTION_FH_FROM_LOCK   = 1026
	PT_ACTION_PARENT_FH      = 1031
	PT_ACTION_EXAMINE_FH     = 1034
)

const (
	SHARED_LOCK    = -2
	ACCESS_READ    = -2
	EXCLUSIVE_LOCK = -1
	ACCESS_WRITE   = -1

	DOS_FALSE = 0
	DOS_TRUE  = -1

	ERROR_OBJECT_IN_USE      = 202
	ERROR_OBJECT_WRONG_TYPE  = 212
	ERROR_DIR_NOT_FOUND      = 204
	ERROR_OBJECT_NOT_FOUND   = 205
	ERROR_OBJECT_EXISTS      = 203
	ERROR_DEVICE_NOT_MOUNTED = 218
	ERROR_NO_MORE_ENTRIES    = 232

	OFFSET_BEGINNING = -1
	OFFSET_CURRENT   = 0
	OFFSET_END       = 1
)

const defaultFsName string = "AmiPiBorg"
const defaultFsPath string = "/home/pi"
const mountPath string = "/media/pi"

func translateError(err error) int32 {

	fmt.Println(err.Error())

	if os.IsExist(err) {
		return ERROR_OBJECT_EXISTS
	}

	if os.IsNotExist(err) {
		return ERROR_OBJECT_NOT_FOUND
	}

	if os.IsPermission(err) {
		return ERROR_OBJECT_IN_USE
	}

	switch err.(type) {
	case *os.PathError:
		return ERROR_DIR_NOT_FOUND
	}

	return ERROR_OBJECT_NOT_FOUND
}

var AmigaEpoch time.Time = time.Date(1978, 1, 1, 0, 0, 0, 0, time.Local)

type fsLock struct {
	id    int32
	name  string
	mode  int32
	freed bool
}

type fsFileHandle struct {
	id   int32
	fh   *os.File
	path string
}

type fileSystem struct {
	isDefault bool
	isMounted bool
	outChan   chan *OutPacket
	id        uint16
	name      string
	rootPath  string
	nextId    int32
	locks     map[int32]*fsLock
	files     map[int32]*fsFileHandle
}

func createFileSystem(outChan chan *OutPacket, id uint16, name string, rootPath string) *fileSystem {

	isDefault := false
	if name == defaultFsName {
		isDefault = true
	}

	fs := &fileSystem{isDefault, true, outChan, id, name, rootPath, 1, make(map[int32]*fsLock), make(map[int32]*fsFileHandle)}

	return fs
}

type FsHandler struct {
	outChan     chan *OutPacket
	quitChan    chan bool
	nextId      uint16
	fileSystems map[uint16]*fileSystem
}

func (this *FsHandler) Init(outChan chan *OutPacket) {
	this.outChan = outChan
	this.fileSystems = make(map[uint16]*fileSystem)
	this.nextId = 1
	this.fileSystems[0] = createFileSystem(this.outChan, 0, defaultFsName, defaultFsPath)
	this.fileSystems[0].mount()

	this.checkMountedVolumes()

	go this.monitorVolumes()
}

func (this *FsHandler) monitorVolumes() {

	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Unable to create FS Watcher")
		return
	}
	defer w.Close()

	done := make(chan bool)
	go func() {
		for {
			select {

			case <-w.Events:
				this.checkMountedVolumes()

			case <-done:
				return

			}
		}
	}()

	err = w.Add(mountPath)
	if err != nil {
		done <- true
	}
	<-this.quitChan
	done <- true
}

func (this *FsHandler) checkMountedVolumes() {

	entries, err := ioutil.ReadDir(mountPath)
	if err != nil {
		return
	}

	newVolumes := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found := false
		for _, vol := range this.fileSystems {
			if entry.Name() == vol.name {
				found = true
				if !vol.isMounted {
					vol.mount()
				}
				break
			}
		}

		if !found {
			fmt.Printf("New volume \"%s\"\n", entry.Name())
			newVolumes = append(newVolumes, entry)
		}
	}

	missingVolumes := make([]*fileSystem, 0, len(entries))
	for _, vol := range this.fileSystems {
		if vol.isDefault {
			continue
		}
		found := false
		for _, entry := range entries {
			if vol.name == entry.Name() {
				found = true
				break
			}
		}

		if !found {
			fmt.Printf("Missing volume \"%s\"\n", vol.name)
			missingVolumes = append(missingVolumes, vol)
		}
	}

	for _, vol := range missingVolumes {
		vol.unmount()
	}

	for _, entry := range newVolumes {
		vol := createFileSystem(this.outChan, this.nextId, entry.Name(), filepath.Join(mountPath, entry.Name()))
		vol.mount()
		this.nextId++
		this.fileSystems[vol.id] = vol
	}
}

func (this *fileSystem) findLock(id int32) *fsLock {

	return this.locks[id]
}

func (this *fileSystem) findLockByPath(path string) *fsLock {

	for _, l := range this.locks {
		if l.name == path {
			return l
		}
	}

	return nil
}

func (this *fileSystem) resolvePath(srcLockId int32, origPath string) string {

	srcLock := this.findLock(srcLockId)

	path := origPath
	colonPos := strings.Index(origPath, ":")
	if colonPos >= 0 {
		path = string(path[colonPos+1:])
	}

	if srcLock != nil {
		path = filepath.Join(srcLock.name, path)
	} else {
		path = filepath.Join(this.rootPath, path)
	}

	if path == "" || path == "/" || len(path) < len(this.rootPath) {
		path = this.rootPath
	}

	return path
}

func (this *fileSystem) sendCreateNotification() {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint32(0xFFFFFFFF))
	binary.Write(buf, binary.BigEndian, this.id)
	buf.Write([]byte(this.name))
	buf.Write([]byte{0})

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       buf.Bytes()}
}

func (this *fileSystem) sendRemoveNotification() {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint32(0xFFFFFFFE))
	binary.Write(buf, binary.BigEndian, this.id)

	this.outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       buf.Bytes()}
}

func (this *fileSystem) mount() {

	this.isMounted = true

	this.locks[0] = &fsLock{0, this.rootPath, SHARED_LOCK, false}

	this.sendCreateNotification()
}

func (this *fileSystem) unmount() {

	this.sendRemoveNotification()
	this.isMounted = false

	this.locks = make(map[int32]*fsLock)

	for _, fh := range this.files {
		fh.fh.Close()
	}

	this.files = make(map[int32]*fsFileHandle)
}

func replyToPacket(outChan chan *OutPacket, p *InPacket, req *FsRequest, res1 int32, res2 int32, data []byte) {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, req.reqId)
	binary.Write(buf, binary.BigEndian, res1)
	binary.Write(buf, binary.BigEndian, res2)

	binary.Write(buf, binary.BigEndian, uint16(len(data)))
	if len(data) > 0 {
		buf.Write(data)
	}

	outChan <- &OutPacket{
		PacketType: MT_Data,
		Data:       buf.Bytes()}
}

func (this *fileSystem) replyToPacket(p *InPacket, req *FsRequest, res1 int32, res2 int32, data []byte) {
	replyToPacket(this.outChan, p, req, res1, res2, data)
}

func (this *fileSystem) createLock(path string, access int32) (l *fsLock, code int32) {

	fmt.Printf("Locking path '%s'\n", path)
	l = this.findLockByPath(path)

	if l != nil && access == EXCLUSIVE_LOCK && !l.freed {

		return nil, ERROR_OBJECT_IN_USE
	}

	if _, err := os.Stat(path); err != nil {
		return nil, translateError(err)
	}

	l = &fsLock{this.nextId, path, access, false}

	this.locks[this.nextId] = l

	this.nextId++

	return l, 0
}

func (this *fileSystem) actionLocateObject(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, req.getString(req.arg2))

	l, code := this.createLock(path, req.arg3)

	if l == nil {
		this.replyToPacket(p, req, DOS_FALSE, code, []byte{})
	} else {
		this.replyToPacket(p, req, l.id, 0, []byte{})
	}
}

func (this *fileSystem) actionFreeLock(p *InPacket, req *FsRequest) {

	l := this.findLock(req.arg1)

	if l != nil {
		fmt.Printf("Unlocking path '%s', id %d\n", l.name, req.arg1)

		if l.mode == EXCLUSIVE_LOCK {
			delete(this.locks, req.arg1)
		} else {
			l.freed = true
		}
	}
	this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
}

func (this *fileSystem) actionOpenFile(p *InPacket, req *FsRequest) {

	fileName := req.getString(req.arg3)
	path := this.resolvePath(req.arg2, fileName)

	this.openFile(p, req, path)
}

func (this *fileSystem) openFile(p *InPacket, req *FsRequest, path string) {
	mode := os.O_RDWR

	fi, err := os.Stat(path)

	switch req.reqType {
	case PT_ACTION_FIND_INPUT, PT_ACTION_FH_FROM_LOCK:
		if err != nil && os.IsNotExist(err) {
			fmt.Printf("Failed to open existing file %s: %s\n", path, err.Error())
			this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_NOT_FOUND, []byte{})
			return
		}

	case PT_ACTION_FIND_UPDATE:
		mode = os.O_RDWR | os.O_CREATE

	case PT_ACTION_FIND_OUTPUT:
		if err == nil {
			if err = os.Remove(path); err != nil {
				fmt.Printf("Failed to replace existing file %s: %s\n", path, err.Error())
				this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
				return
			}
		}
		mode = os.O_RDWR | os.O_CREATE
	}

	if fi != nil && !fi.Mode().IsRegular() {
		fmt.Printf("%s: is not a file\n", path)
		this.replyToPacket(p, req, DOS_FALSE, ERROR_OBJECT_WRONG_TYPE, []byte{})
		return
	}

	if f, err := os.OpenFile(path, mode, 0755); err == nil {

		fmt.Printf("Open file %s\n", path)

		fh := &fsFileHandle{this.nextId, f, path}
		this.files[this.nextId] = fh
		this.nextId++
		this.replyToPacket(p, req, DOS_TRUE, fh.id, []byte{})
	} else {
		fmt.Printf("Failed to open %s: %s\n", path, err.Error())
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
	}
}

func (this *fileSystem) actionCloseFile(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh != nil {
		fmt.Printf("Closed file %s\n", fh.path)
		fh.fh.Close()
	} else {
		fmt.Printf("Could not close file %d\n", req.arg1)
	}

	delete(this.files, req.arg1)
}

func (this *fileSystem) actionInfo(p *InPacket, req *FsRequest) {

	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, 100000)
	binary.Write(buf, binary.BigEndian, 1000)

	this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
}

func (this *fileSystem) actionExamine(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")

	fi, err := os.Stat(path)
	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
		return
	}

	buf := new(bytes.Buffer)

	this.writeFileInfoBlock(0, path, fi, buf)

	this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
}

func (this *fileSystem) actionExamineFh(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh == nil {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	fmt.Printf("Examining %s\n", fh.path)

	fi, err := os.Stat(fh.path)
	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
		return
	}

	buf := new(bytes.Buffer)

	this.writeFileInfoBlock(0, fh.path, fi, buf)

	this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
}

func (this *fileSystem) actionExamineNext(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")
	ix := req.arg2
	entries, err := ioutil.ReadDir(path)

	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
		return
	}

	if ix < 0 || ix >= int32(len(entries)) {
		this.replyToPacket(p, req, DOS_FALSE, ERROR_NO_MORE_ENTRIES, []byte{})
	} else {
		fi := entries[ix]

		buf := new(bytes.Buffer)

		this.writeFileInfoBlock(ix+1, filepath.Join(path, entries[ix].Name()), fi, buf)

		this.replyToPacket(p, req, DOS_TRUE, 0, buf.Bytes())
	}
}

func (this *fileSystem) actionSameLock(p *InPacket, req *FsRequest) {

	l1 := this.findLock(req.arg1)
	l2 := this.findLock(req.arg2)

	if l1 == nil && l2 == nil {
		this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
		return
	}

	if l1 == nil || l2 == nil {
		this.replyToPacket(p, req, DOS_FALSE, 0, []byte{})
		return
	}

	if l1.name != l2.name {
		this.replyToPacket(p, req, DOS_FALSE, 0, []byte{})
		return
	}

	this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
}

func (this *fileSystem) actionParent(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg1, "")

	if path == this.rootPath {
		this.replyToPacket(p, req, 0, 0, []byte{})
	} else {
		parentPath := filepath.Dir(path)
		l, code := this.createLock(parentPath, SHARED_LOCK)
		if l == nil {
			this.replyToPacket(p, req, DOS_FALSE, code, []byte{})
		} else {
			this.replyToPacket(p, req, l.id, 0, []byte{})
		}
	}
}

func (this *fileSystem) actionRead(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh == nil {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	bytesToRead := req.arg3
	if bytesToRead == 0 {
		this.replyToPacket(p, req, 0, -1, []byte{})
		return
	}

	fmt.Printf("Read up to %d bytes\n", bytesToRead)
	status := int32(0)

	for bytesToRead > 0 && status == 0 {

		maxRead := bytesToRead
		if maxRead > 512 {
			maxRead = 512
		}
		data := make([]byte, maxRead)

		bytesRead, err := fh.fh.Read(data)
		if err != nil && err != io.EOF {
			this.replyToPacket(p, req, -1, translateError(err), []byte{})
			return
		}

		bytesToRead -= int32(bytesRead)
		if int32(bytesRead) < maxRead || bytesToRead == 0 {
			status = -1
		}

		this.replyToPacket(p, req, int32(bytesRead), status, data)
	}
}

func (this *fileSystem) actionWrite(p *InPacket, req *FsRequest) {
	fh := this.files[req.arg1]
	if fh == nil {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	bytesToWrite := req.arg4
	bytesRemaining := req.arg3

	fmt.Printf("Write %d bytes. %d remaining\n", bytesToWrite, bytesRemaining)

	data := req.getBytes(0, bytesToWrite)

	fmt.Printf("%d bytes in packet\n", len(data))

	bytesWritten, err := fh.fh.Write(data)
	if err != nil {
		fmt.Printf("Write failed: %s\n", err.Error())
		this.replyToPacket(p, req, -1, translateError(err), []byte{})
	}

	status := int32(0)
	if bytesRemaining == 0 {
		fmt.Printf("No more data expected\n")
		status = -1
	}

	this.replyToPacket(p, req, int32(bytesWritten), status, []byte{})
}

func (this *fileSystem) actionCreateDir(p *InPacket, req *FsRequest) {

	dirName := req.getString(req.arg2)
	path := this.resolvePath(req.arg1, dirName)

	err := os.Mkdir(path, 0755)
	if err != nil {
		fmt.Printf("Error creating dir %s: %s\n", path, err.Error())
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
	}

	l, code := this.createLock(path, SHARED_LOCK)

	if l == nil {
		this.replyToPacket(p, req, DOS_FALSE, code, []byte{})
	} else {
		fmt.Printf("Create dir %s\n", path)
		this.replyToPacket(p, req, l.id, 0, []byte{})
	}
}

func (this *fileSystem) actionDeleteObject(p *InPacket, req *FsRequest) {
	dirName := req.getString(req.arg2)
	path := this.resolvePath(req.arg1, dirName)

	err := os.Remove(path)
	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
	} else {
		fmt.Printf("Delete %s\n", path)
		this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
	}
}

func (this *fileSystem) actionRenameObject(p *InPacket, req *FsRequest) {
	fn1 := req.getString(req.arg2)
	path1 := this.resolvePath(req.arg1, fn1)

	fn2 := req.getString(req.arg4)
	path2 := this.resolvePath(req.arg3, fn2)

	err := os.Rename(path1, path2)

	if err != nil {
		this.replyToPacket(p, req, DOS_FALSE, translateError(err), []byte{})
	} else {
		fmt.Printf("Rename %s to %s\n", path1, path2)
		this.replyToPacket(p, req, DOS_TRUE, 0, []byte{})
	}
}

func (this *fileSystem) actionSeek(p *InPacket, req *FsRequest) {

	fh := this.files[req.arg1]
	if fh == nil {
		this.replyToPacket(p, req, -1, ERROR_OBJECT_NOT_FOUND, []byte{})
		return
	}

	oldPos, err := fh.fh.Seek(0, io.SeekCurrent)
	if err != nil {
		this.replyToPacket(p, req, -1, translateError(err), []byte{})
		return
	}

	var whence int
	switch req.arg3 {
	case OFFSET_BEGINNING:
		whence = io.SeekStart
	case OFFSET_CURRENT:
		whence = io.SeekCurrent
	case OFFSET_END:
		whence = io.SeekEnd
	}

	_, err = fh.fh.Seek(int64(req.arg2), whence)
	if err != nil {
		this.replyToPacket(p, req, -1, translateError(err), []byte{})
		return
	}

	this.replyToPacket(p, req, int32(oldPos), 0, []byte{})
}

func (this *fileSystem) actionFhFromLock(p *InPacket, req *FsRequest) {

	path := this.resolvePath(req.arg2, "")

	this.openFile(p, req, path)
}

func (this *fileSystem) actionParentFh(p *InPacket, req *FsRequest) {
	path := this.resolvePath(req.arg2, "")
	parentPath := filepath.Dir(path)

	l, code := this.createLock(parentPath, req.arg3)

	if l == nil {
		this.replyToPacket(p, req, DOS_FALSE, code, []byte{})
	} else {
		this.replyToPacket(p, req, l.id, 0, []byte{})
	}
}

func (this *fileSystem) writeFileInfoBlock(diskKey int32, path string, fi os.FileInfo, buf *bytes.Buffer) {
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

	fn := fi.Name()

	// fib_DiskKey
	binary.Write(buf, binary.BigEndian, diskKey)

	// fib_DirEntryType
	et := int32(0)
	if fi.IsDir() {
		if path == this.rootPath {
			et = 2 // Root
		} else {
			et = 1 // Dir
		}
	} else {
		et = -3 // File
	}
	binary.Write(buf, binary.BigEndian, et)

	// fib_FileName
	if path == this.rootPath {
		fn = this.name
	}
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
	req.arg1 = int32(binary.BigEndian.Uint32(p.Data[ix:]))
	ix += 4
	req.arg2 = int32(binary.BigEndian.Uint32(p.Data[ix:]))
	ix += 4
	req.arg3 = int32(binary.BigEndian.Uint32(p.Data[ix:]))
	ix += 4
	req.arg4 = int32(binary.BigEndian.Uint32(p.Data[ix:]))
	ix += 4
	req.volId = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.reqType = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.dataLen = binary.BigEndian.Uint16(p.Data[ix:])
	ix += 2
	req.strData = p.Data[ix:]

	fs := this.fileSystems[req.volId]
	if fs == nil {
		replyToPacket(this.outChan, p, req, DOS_FALSE, ERROR_DEVICE_NOT_MOUNTED, []byte{})
		return
	}

	if !fs.isMounted {
		replyToPacket(this.outChan, p, req, DOS_FALSE, ERROR_DEVICE_NOT_MOUNTED, []byte{})
		return
	}

	switch req.reqType {
	case PT_ACTION_LOCATE_OBJECT:
		fs.actionLocateObject(p, req)
	case PT_ACTION_FREE_LOCK:
		fs.actionFreeLock(p, req)
	case PT_ACTION_FIND_INPUT:
		fs.actionOpenFile(p, req)
	case PT_ACTION_FIND_OUTPUT:
		fs.actionOpenFile(p, req)
	case PT_ACTION_FIND_UPDATE:
		fs.actionOpenFile(p, req)
	case PT_ACTION_END:
		fs.actionCloseFile(p, req)
	case PT_ACTION_INFO:
		fs.actionInfo(p, req)
	case PT_ACTION_DISK_INFO:
		fs.actionInfo(p, req)
	case PT_ACTION_EXAMINE_OBJECT:
		fs.actionExamine(p, req)
	case PT_ACTION_EXAMINE_NEXT:
		fs.actionExamineNext(p, req)
	case PT_ACTION_EXAMINE_FH:
		fs.actionExamineFh(p, req)
	case PT_ACTION_PARENT:
		fs.actionParent(p, req)
	case PT_ACTION_READ:
		fs.actionRead(p, req)
	case PT_ACTION_WRITE:
		fs.actionWrite(p, req)
	case PT_ACTION_SAME_LOCK:
		fs.actionSameLock(p, req)
	case PT_ACTION_SEEK:
		fs.actionSeek(p, req)
	case PT_ACTION_FH_FROM_LOCK:
		fs.actionFhFromLock(p, req)
	case PT_ACTION_PARENT_FH:
		fs.actionParentFh(p, req)
	case PT_ACTION_CREATE_DIR:
		fs.actionCreateDir(p, req)
	case PT_ACTION_RENAME_OBJECT:
		fs.actionRenameObject(p, req)
	case PT_ACTION_DELETE_OBJECT:
		fs.actionDeleteObject(p, req)
	}
}

func (this *FsHandler) Quit() {
	this.quitChan <- true
}

func NewFsHandler() Handler {
	return &FsHandler{}
}
