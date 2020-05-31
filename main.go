package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

const billion = 1000000000

type Command struct {
	addr 		net.IP
	hostname 	string
	count 		uint
	ttl 		uint
	interval	time.Duration
	deadline	time.Duration
	timeout		time.Duration
}

func parseArgs() (*Command, error){
	var hostname string = ""	// default val
	var count uint
	var ttl uint
	var interval int64
	var deadline int64
	var timeout int64

	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "NOTE: Endpoint should come after all flags!")
		flag.PrintDefaults()
	}

	flag.UintVar(&count, "c", 4, "The number of pings to be sent out (default = 4).")
	flag.UintVar(&ttl, "t", 255, "The ttl for the ping (TTL, default = 255).")
	flag.Int64Var(&interval, "i", 1, "The interval (in seconds) to send pings out at (default = 1).")
	flag.Int64Var(&deadline, "w", 0, "The number of seconds allowed for all ping requests to finish (default = 0). 0 indicates unlimited time.")
	flag.Int64Var(&timeout, "W", 1, "The number of seconds waited for a response for each packet (default = 4).")
	flag.Parse()

	if flag.NArg() == 0 {
		return nil, errors.New(fmt.Sprintf("Error parsing arguments: no hostname or IP address specified."))
	} else if flag.NArg() > 1 {
		return nil, errors.New(fmt.Sprintf("Error parsing arguments: cannot determine hostname or IP address from %v", flag.Args()))
	}
	endpoint := flag.Arg(0)

	ip := net.ParseIP(endpoint)
	if ip == nil {
		ips, err := net.LookupIP(endpoint)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Unable to resolve hostname: %s", endpoint))
		}
		hostname = endpoint
		ip = ips[0]	// defaults to the first IP that this hostname resolves to
	}

	return &Command{ ip, hostname, count, ttl, time.Duration(interval * billion), time.Duration(deadline * billion), time.Duration(timeout * billion)}, nil
}


func main() {
	cmd, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", *cmd)

}