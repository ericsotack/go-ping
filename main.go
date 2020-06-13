package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"go-ping/pinger"
)

func parseArgs() (*pinger.Command, error){
	var version pinger.IPVersion = pinger.UNSET	// default val
	var logging = log.New(os.Stdout, "GO-PING: ", 0)
	var count int
	var ttl int
	var interval time.Duration
	var maxTime time.Duration


	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), "NOTE: Endpoint should come after all flags!")
		fmt.Fprintln(flag.CommandLine.Output(), "NOTE: Must be run as root!")
		flag.PrintDefaults()
	}

	flag.IntVar(&count, "c", 4, "The number of pings to be sent out (default = 4).")
	flag.IntVar(&ttl, "t", 255, "The ttl for the ping (TTL, default = 255).")
	flag.DurationVar(&interval, "i", time.Second, "The interval (in seconds) to send pings out at (default = 1).")
	flag.DurationVar(&maxTime, "W", time.Second * 0, "The number of seconds the entire program has to run (0 for unlimited).")
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
	return pinger.New(ip, version, count, ttl, interval, maxTime, logging), nil
}


func main() {
	cmd, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer close(c)
	go func() {
		for _ = range c {
			cmd.End()
		}
	}()

	cmd.Logging.Println("Running with args:")
	cmd.Logging.Printf("Addr: %v\n", cmd.Addr.String())
	cmd.Logging.Printf("Version: %v\n", cmd.Version)
	cmd.Logging.Printf("Count: %v\n", cmd.Count)
	cmd.Logging.Printf("TTL: %v\n", cmd.Ttl)
	cmd.Logging.Printf("Interval: %v\n", cmd.Interval)
	cmd.Logging.Printf("MaxTime: %v\n", cmd.MaxTime)

	if err := cmd.Ping(); err != nil {
		cmd.Logging.Println(fmt.Sprintf("error occured while performing Ping: %v", err))
	}
}