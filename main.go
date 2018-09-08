package main

func main() {

	r, _ := NewSerialRemote("/dev/ttyUSB0", 19200)

	hf := NewHandlerFactory()

	hf.AddHandler(HT_Ping, "PING", NewPingHandler)
	hf.AddHandler(HT_Date, "DATE", NewDateHandler)
	hf.AddHandler(HT_Input, "INPUT", NewInputHandler)
	hf.AddHandler(HT_FS, "FS", NewFsHandler)

	srv := NewServer(r, hf)

	srv.Run()
}
