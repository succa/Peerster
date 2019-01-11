package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net"
	"net/http"
)

type ClientMessage struct {
	Dest     string
	Msg      string
	File     string
	Request  string
	Keywords string
	Budget   float64
	Tor      string
}

const fileFolder string = "/../_SharedFiles"

func main() {

	// Reading the variables
	var UIport = flag.String("UIPort", "8080", "port for the UI client (default 8080)")
	var msg = flag.String("msg", "", "message to be sent")
	var dest = flag.String("dest", "", "destination for the private message")
	var file = flag.String("file", "", "file to be indexed by the gossiper")
	var request = flag.String("request", "", "metafile to request")
	var keywords = flag.String("keywords", "", "*keywords*")
	var budget = flag.Uint64("budget", 0, "budget to use")
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
		Dest:     *dest,
		Msg:      *msg,
		File:     *file,
		Request:  *request,
		Keywords: *keywords,
		Budget:   float64(*budget),
		Tor:      "",
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

	/*
		// If the keywords were set, we have to download the files
		if *keywords != "" {
			//continue to send a get for the files, return if founded or timeout
			var foundedFiles []string
			timer := time.NewTimer(10 * time.Second)
			ticker := time.NewTicker(500 * time.Millisecond)
			for {
				select {
				case <-ticker.C:
					searchRest, err := http.Get("http://" + udpAddr.String() + "/message?file=search")
					if err != nil {
						fmt.Println(err)
						return
					}
					defer searchRest.Body.Close()
					body, err := ioutil.ReadAll(searchRest.Body)

					files, err := getSearchedFiles(body)
					if err != nil {
						fmt.Println(err)
						return
					}
					//fmt.Println(files)

					if len(files) != 0 {
						//Download the files
						for _, file := range files {
							if !contains(foundedFiles, file) {
								foundedFiles = append(foundedFiles, file)
								getFileResp, err := http.Get("http://" + udpAddr.String() + "/message?file=" + file)
								if err != nil {
									fmt.Println(err)
									return
								}
								defer getFileResp.Body.Close()
								body, err := ioutil.ReadAll(getFileResp.Body)
								errorString, err := getError(body)
								if err != nil {
									fmt.Println(err)
									return
								}
								if errorString != "" {
									fmt.Println(errorString)
								}
							}
						}
					}
				case <-timer.C:
					fmt.Println("timeout")
					return
				}
			}
		}
	*/

}

func contains(s []string, el string) bool {
	for _, a := range s {
		if a == el {
			return true
		}
	}
	return false
}

func getSearchedFiles(body []byte) ([]string, error) {
	var ret []string
	err := json.Unmarshal(body, &ret)
	return ret, err
}

func getError(body []byte) (string, error) {
	var ret string
	err := json.Unmarshal(body, &ret)
	return ret, err
}
