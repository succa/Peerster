package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net"
	"net/http"
)

type ClientMessage struct {
	Dest    string
	Msg     string
	File    string
	Request string
}

const fileFolder string = "/../_SharedFiles"

func main() {

	// Reading the variables
	var UIport = flag.String("UIPort", "8080", "port for the UI client (default 8080)")
	var msg = flag.String("msg", "", "message to be sent")
	var dest = flag.String("dest", "", "destination for the private message")
	var file = flag.String("file", "", "file to be indexed by the gossiper")
	var request = flag.String("request", "", "metafile to request")
	//d3ae55aaba41249e5fcfe761fee90237121fad0337b5304e89fed911143e0fdd

	flag.Parse()

	udpAddr, err := net.ResolveUDPAddr("udp4", "localhost:"+*UIport)
	if err != nil {
		panic(err)
	}

	url := "http://" + udpAddr.String() + "/message"

	/*
		var filePath string
		if *file != "" {
			ex, err := os.Executable()
			if err != nil {
				panic(err)
			}
			filePath = filepath.Dir(ex) + fileFolder + "/" + *file
		}
		fmt.Println(filePath)
	*/

	message := &ClientMessage{
		Dest:    *dest,
		Msg:     *msg,
		File:    *file,
		Request: *request,
	}

	jsonStr, err := json.Marshal(message)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

}
