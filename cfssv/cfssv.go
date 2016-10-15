package main

import (
	"encoding/json"
	"fmt"
	"github.com/haramako/cfs"
	"io/ioutil"
	"os"
	"strconv"
)

func main() {

	sv := &cfs.Server{
		FpRoot: ".",
		Port:   8086,
	}

	conf, err := ioutil.ReadFile("cfssv.conf")
	if err == nil {
		println("read cfssv.conf")
		err = json.Unmarshal(conf, &sv)
		if err != nil {
			fmt.Printf("cannot load cfssv.conf, %s\n", err)
			os.Exit(1)
		}
	}

	port_str := os.Getenv("PORT")
	if port_str != "" {
		sv.Port, err = strconv.Atoi(port_str)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	err = sv.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sv.Start()
}
