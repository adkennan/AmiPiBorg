package main

func main() {

	r, _ := NewSerialRemote("/dev/ttyUSB0", 19200)

	hf := NewHandlerFactory()

	hf.AddHandler(HT_Ping, NewPingHandler)

	srv := NewServer(r, hf)

	srv.Run()
}
