package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: holdport <port>")
		os.Exit(2)
	}
	ln, err := net.Listen("tcp4", "0.0.0.0:"+os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer ln.Close()
	time.Sleep(30 * time.Second)
}
