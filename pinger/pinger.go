package pinger

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/icmp"
)

type ipVersion string

const (
	IPv4 = "ip4"
	IPv6 = "ip6"
)

type Command struct {
	Addr		net.IP
	Hostname 	string
	Count 		uint
	Ttl 		uint
	Interval	time.Duration
	Timeout		time.Duration
}

type Result struct {
	Response		bool
	Hostname		string
	ResponseAddr	net.IPAddr
	IcmpSeq			uint
	Size			uint
	RoundTripTime 	time.Duration
}
// listen on all interfaces with 0.0.0.0 addr

func sendPing(message icmp.Message) {

}

func listenPing() {
}

func Ping(command *Command) ([]Result, error) {
	// determine if ipv4 or ipv6
	// start thread to send pings out and assigns a spot in a map for them
	// start thread to receive pings -> prints when it is received
	// return results
	// print statistics
}