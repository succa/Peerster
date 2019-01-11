package main

import (
	"flag"
	"os"

	gossiper "github.com/succa/Peerster/pkg/gossiper"
	utils "github.com/succa/Peerster/pkg/utils"
	web "github.com/succa/Peerster/pkg/web"
)

func main() {

	// Reading the variables
	var UIport = flag.String("UIPort", "8080", "port for the UI client (default 8080)")
	var gossipAddr = flag.String("gossipAddr", "127.0.0.1:5000", "ip:port for the gossiper (default 127.0.0.1:5000)")
	var name = flag.String("name", "", "name of the gossiper")
	var peersString = flag.String("peers", "", "comma separated list of peers of the form ip:port")
	var rtimer = flag.Int("rtimer", 0, "route rumor sending period in seconds, 0 to disable sending of route rumors (default 0)")
	var mode = flag.Bool("simple", false, "run gossiper in simple broadcast mode")
	var tor = flag.Bool("tor", false, "abilitate the gossiper for onion routing")

	flag.Parse()

	if *name == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Parse peers
	peers := utils.ParsePeers(*peersString)

	// create Gossiper
	gossiper := gossiper.NewGossiper(*UIport, *gossipAddr, *name, *rtimer, *mode, *tor)

	// Add known initial Peers
	gossiper.AddKnownPeers(peers)

	// Start listening to the gossip Address
	go gossiper.Start()

	// Create the WebServer (handles the client requests)
	webServer := web.New(*UIport, gossiper)

	// THIS IS BLOCKING !!
	webServer.Start()
}
