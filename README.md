# Peerster

### Group Project: Private Message Exchange and File Sharing using Onion Routing for Peerster
Authors: Artem Shevchenko, Riccardo Succa, Aleksandr Tukallo

The Peerster works as always, but there is the new flag **-tor** to set if we want to make a node able to use anonymous routing.
Es. ./Peerster -UIPort=10000 -gossipAddr=127.0.0.1:5000 -peers=127.0.0.1:5001 -name=A -rtimer=5 **-tor** .

Be aware that the system needs at least 5 nodes with tor enabled: source, 3 nodes for the path and destination. You also have to wait some minutes so that all the public keys are mined.

In the client, two new buttons are added:
  - *Send with Tor*, when we want to send a private message to a node
  - *Download with Tor*, to download a file
  
These two will use onion encryption and anonymous routing to forward the messages.
