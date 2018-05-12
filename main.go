package main

func main() {

	r, _ := NewSerialRemote("/dev/ttyUSB0", 19200)

	hf := NewHandlerFactory()

	hf.AddHandler(HT_Ping, NewPingHandler)
	hf.AddHandler(HT_Date, NewDateHandler)

	srv := NewServer(r, hf)

	srv.Run()
}
