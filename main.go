package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"go-ping/pinger"
)

func parseArgs() (*pinger.Command, error){
	var version pinger.IPVersion = pinger.UNSET	// default val
	var logging = log.New(os.Stdout, "GO-PING: ", 0)
	var count int
	var ttl int
	var interval int64		// TODO change this to be Time.Duration
	var timeout int64		// TODO change this to be Time.Duration


	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "NOTE: Endpoint should come after all flags!")
		flag.PrintDefaults()
	}

	flag.IntVar(&count, "c", 4, "The number of pings to be sent out (default = 4).")
	flag.IntVar(&ttl, "t", 255, "The ttl for the ping (TTL, default = 255).")
	flag.Int64Var(&interval, "i", 1, "The interval (in seconds) to send pings out at (default = 1).")
	flag.Int64Var(&timeout, "W", 4, "The number of seconds waited for a response for each packet (default = 4).")
	flag.Parse()

	if flag.NArg() == 0 {
		return nil, errors.New(fmt.Sprintf("Error parsing arguments: no hostname or IP address specified."))
	} else if flag.NArg() > 1 {
		return nil, errors.New(fmt.Sprintf("Error parsing arguments: cannot determine hostname or IP address from %v", flag.Args()))
	}
	endpoint := flag.Arg(0)

	ip, err := net.ResolveIPAddr("ip", endpoint)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Unable to resolve hostname: %s", endpoint))
	}

	if strings.ContainsRune(ip.String(), '.'){
		version = pinger.IPv4
	} else if strings.ContainsRune(ip.String(), ':') {
		version = pinger.IPv6
	} else {
		return nil, errors.New(fmt.Sprintf("Unable to determine if %v is IPv4 or IPv6.", ip.String()))
	}
	return &pinger.Command{ Addr: ip, Version: version, Count: count,
		Ttl: ttl, Interval: time.Duration(interval) * time.Second,
		Timeout: time.Duration(timeout) * time.Second, Logging: logging}, nil
}


func main() {
	cmd, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", *cmd)	// TODO replace with "running with args"
	if _, err := cmd.Ping(); err != nil {
		cmd.Logging.Println(fmt.Sprintf("error occured while performing Ping: %v", err))
	}
}