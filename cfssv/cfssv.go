package main

import (
	"fmt"
	"github.com/haramako/cfs/server"
	"os"
	"strconv"
)

func main() {

	var port int
	var err error
	port_str := os.Getenv("PORT")
	if len(port_str) == 0 {
		port = 3000
	} else {
		port, err = strconv.Atoi(os.Getenv("PORT"))
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}
	}

	sv := server.Server{
		FpRoot: ".",
		Port:   port,
	}

	err = sv.Init()
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}

	sv.Start()
}
