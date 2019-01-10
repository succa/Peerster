package web

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/succa/Peerster/pkg/gossiper"
	"github.com/succa/Peerster/pkg/utils"
)

const fileFolder string = "/_SharedFiles"

type WebServer struct {
	port     string
	router   *http.ServeMux
	gossiper *gossiper.Gossiper
}

func New(port string, g *gossiper.Gossiper) *WebServer {
	return &WebServer{port: port, router: http.NewServeMux(), gossiper: g}
}

func (w *WebServer) Start() {
	w.router.HandleFunc("/message", w.MessageHandler)
	w.router.HandleFunc("/node", w.NodeHandler)
	w.router.HandleFunc("/id", w.IdHandler)
	w.router.Handle("/", http.FileServer(http.Dir("client")))
	http.ListenAndServe("localhost:"+w.port, w.router)
}

func decode(wr http.ResponseWriter, r *http.Request, v interface{}) error {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, v)
	if err != nil {
		return err
	}
	return nil
}

func (w *WebServer) MessageHandler(wr http.ResponseWriter, r *http.Request) {

	switch r.Method {

	case "GET":
		dest := r.URL.Query().Get("dest")
		file := r.URL.Query().Get("file")
		keywords := r.URL.Query().Get("keywords")

		switch {
		case dest == "" && file == "" && keywords == "":
			// Message to gossip
			wr.WriteHeader(http.StatusOK)
			messages := w.gossiper.GetMessages()
			data, _ := json.Marshal(messages)
			wr.Write(data)
			return
		case dest != "" && file == "" && keywords == "":
			// Private Message
			wr.WriteHeader(http.StatusOK)
			messages := w.gossiper.GetPrivateMessages(dest)
			data, _ := json.Marshal(messages)
			wr.Write(data)
			return
		case dest == "" && file == "all" && keywords == "": //TODO to see
			// Get File list
			ex, _ := os.Executable()
			fileDir := filepath.Dir(ex) + fileFolder + "/"
			files, err := ioutil.ReadDir(fileDir)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			var list []string
			for _, f := range files {
				if f.IsDir() {
					continue // especially important to skip ./chunks dir, where temporary chunks are stored
				}
				list = append(list, f.Name())
			}

			wr.WriteHeader(http.StatusOK)
			data, _ := json.Marshal(list)
			wr.Write(data)
			return
		case dest == "" && file != "" && keywords == "":
			// List of searched files
			wr.WriteHeader(http.StatusOK)
			err := w.gossiper.DownloadSearchedFileFromName(file)
			//fmt.Println(err)
			var data []byte
			if err != nil {
				data, _ = json.Marshal(err.Error())
			} else {
				data, _ = json.Marshal("File Downloaded")
			}
			wr.Write(data)
			return
		case dest == "" && file == "" && keywords != "":
			// List of searched files
			wr.WriteHeader(http.StatusOK)
			k := strings.Split(keywords, ",")
			files := w.gossiper.GetCompletedSearches(k)
			data, _ := json.Marshal(files)
			wr.Write(data)
			return
		}

	case "POST":
		var msg interface{}
		err := decode(wr, r, &msg)
		if err != nil {
			wr.WriteHeader(http.StatusBadRequest)
			return
		}
		//fmt.Println(msg)
		//fmt.Println(reflect.TypeOf(msg))
		clientMessage := msg.(map[string]interface{})

		message := clientMessage["Msg"].(string)
		dest := clientMessage["Dest"].(string)
		file := clientMessage["File"].(string)
		request := clientMessage["Request"].(string)
		keywords := clientMessage["Keywords"].(string)
		budget := uint64(clientMessage["Budget"].(float64))
		tor := clientMessage["Tor"].(string)

		switch {
		case (message != "" &&
			dest == "" &&
			file == "" &&
			request == "" &&
			keywords == "" &&
			budget == 0 &&
			tor == ""):
			//Gossip message
			err = w.gossiper.SendClientMessage(message)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message != "" &&
			dest != "" &&
			file == "" &&
			request == "" &&
			keywords == "" &&
			budget == 0 &&
			tor == ""):
			//Private message
			err = w.gossiper.SendPrivateMessage(message, dest)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message == "" &&
			dest == "" &&
			file != "" &&
			request == "" &&
			keywords == "" &&
			budget == 0 &&
			tor == ""):
			//File to index
			utils.PrintRequestIndexing(file)
			filePath, _ := filepath.Abs(path.Join(utils.SharedFilesPath, file))
			err = w.gossiper.ShareFile(filePath)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message == "" &&
			dest != "" &&
			file != "" &&
			request != "" &&
			keywords == "" &&
			budget == 0 &&
			tor == ""):
			//Request for a file
			utils.PrintRequestFile(file, dest, request)
			var byteReq32 [32]byte
			byteReq, err := hex.DecodeString(request)
			if err != nil {
				fmt.Println("The hex request is incorrect")
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			copy(byteReq32[:], byteReq) //TODO test this part
			err = w.gossiper.RequestFile(dest, file, byteReq32)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message == "" &&
			dest != "" &&
			file != "" &&
			request != "" &&
			keywords == "" &&
			budget == 0 &&
			tor != ""):
			//Request for a file
			fmt.Println("Riccardo request file onion")
			utils.PrintRequestFile(file, dest, request)
			var byteReq32 [32]byte
			byteReq, err := hex.DecodeString(request)
			if err != nil {
				fmt.Println("The hex request is incorrect")
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			copy(byteReq32[:], byteReq) //TODO test this part
			err = w.gossiper.RequestFileOnion(dest, file, byteReq32)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message == "" &&
			dest == "" &&
			file != "" &&
			request != "" &&
			keywords == "" &&
			budget == 0 &&
			tor == ""):
			//Request for a file that was previously searched
			var byteReq32 [32]byte
			byteReq, err := hex.DecodeString(request)
			if err != nil {
				fmt.Println("The hex request is incorrect")
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			copy(byteReq32[:], byteReq) //TODO test this part
			err = w.gossiper.DownloadSearchedFile(file, byteReq32)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		case (message == "" &&
			dest == "" &&
			file == "" &&
			request == "" &&
			keywords != "" &&
			tor == ""):
			keyw := strings.Split(keywords, ",")
			fmt.Println(keyw)
			if len(keyw) == 0 {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			err = w.gossiper.SearchFiles(keyw, budget)
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			//wr.WriteHeader(http.StatusOK)
			return
		case (message != "" &&
			dest != "" &&
			file == "" &&
			request == "" &&
			keywords == "" &&
			budget == 0 &&
			tor != ""):
			//Private message
			err = w.gossiper.SendOnionPrivateMessage(message, dest)
			//fmt.Println(err.Error())
			if err != nil {
				wr.WriteHeader(http.StatusInternalServerError)
				return
			}
			return
		default:
			wr.WriteHeader(http.StatusBadRequest)
			return
		}
	default:
		wr.WriteHeader(http.StatusMethodNotAllowed)
	}

}

func (w *WebServer) NodeHandler(wr http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case "GET":

		type NodeReplay struct {
			Peers []string
			Nodes []string
		}

		wr.WriteHeader(http.StatusOK)

		reply := &NodeReplay{
			Peers: w.gossiper.GetPeerList(),
			Nodes: w.gossiper.GetNodeList(),
		}

		data, _ := json.Marshal(reply)
		wr.Write(data)

	case "POST":
		var peerAddress string
		err := decode(wr, r, &peerAddress)
		if err != nil {
			wr.WriteHeader(http.StatusBadRequest)
			return
		}
		err = w.gossiper.AddNewPeer(peerAddress)
		if err != nil {
			wr.WriteHeader(http.StatusInternalServerError)
			return
		}
		wr.WriteHeader(http.StatusOK)

	default:
		wr.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (w *WebServer) IdHandler(wr http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		wr.WriteHeader(http.StatusOK)
		raw, _ := json.Marshal(w.gossiper.Name)
		wr.Write(raw)
	default:
		wr.WriteHeader(http.StatusMethodNotAllowed)
	}
}
