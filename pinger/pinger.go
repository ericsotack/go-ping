// Package pinger emulates the Ping utility provided on most unix systems.
package pinger

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Type to represent IP Version Types.
// Can either be "IPv4", "IPv6", or "" for unset.
type IPVersion string

// Constants for IPVersion type and for proto fields of functions and methods.
const (
	IPv4 IPVersion = "IPv4"
	IPv6 IPVersion = "IPv6"
	UNSET IPVersion = ""

	protocolICMP 	= 1
	protocolICMPv6 	= 58
)

// Represents a ping command run.
// Gathers the necessary information to perform a ping.
// Addr: the destination address
// Version:  the IPVersion
// Count: the number of ICMP echo requests to send out
// Ttl: the TTL to set on each ICMP request (doesn't work on Windows)
// Interval: the time between each echo request send
// MaxTime: the maximum amount of time allowed for the entire program to run
// Logging: the logger to which the output is written
type Command struct {
	Addr      		*net.IPAddr
	Version   		IPVersion
	Count     		int
	Ttl       		int
	Interval  		time.Duration
	MaxTime			time.Duration
	Logging   		*log.Logger
	id        		int				// the id to assign inside the IP packet
	seqStart  		int				// the starting sequence number for the packets
	done      		chan bool		// the channel that is notified for a force quit or system interrupt
	wg        		sync.WaitGroup	// the waitgroup that Ping() waits on
	sentCount 		int				// the number of pings actually sent
	sentAttempts 	int				// the number of pings that it attempted to send
	recvCount		int				// the number of pings received
	startTime		time.Time		// the time we started trying to run pings
	rtts			[]time.Duration	// the array of round-trip-times used to generate post-run stats
}


// Creates and returns a new *Command struct that can be used to run a ping.
// addr: the destination address
// version:  the IPVersion
// count: the number of ICMP echo requests to send out
// ttl: the TTL to set on each ICMP request (doesn't work on Windows)
// interval: the time between each echo request send
// maxTime: the maximum amount of time allowed for the entire program to run
// logging: the logger to which the output is written
func New(addr *net.IPAddr, version IPVersion, count int, ttl int, interval time.Duration, maxTime time.Duration, logging *log.Logger) *Command {
	return &Command{
		Addr:     		addr,
		Version:	  	version,
		Count:    		count,
		Ttl:      		ttl,
		Interval: 		interval,
		MaxTime: 		maxTime,
		Logging:  		logging,
		id:				rand.New(rand.NewSource(time.Now().UnixNano())).Intn(math.MaxInt16),
		seqStart: 		rand.New(rand.NewSource(time.Now().UnixNano())).Intn(math.MaxInt16),
		done:			make(chan bool),
		wg: 			sync.WaitGroup{},
		sentCount: 		0,
		sentAttempts: 	0,
		recvCount: 		0,
		startTime: 		time.Now(),
		rtts: 			[]time.Duration{},
	}
}


// For a *command, create and return a new *icmp.PacketConn from which
// we can send and receive pings.
// Returns nil on error
func (command *Command) listen()  *icmp.PacketConn {
	var connNetwork string

	// check for ipv4 or ipv6
	if command.Version == IPv4 {
		connNetwork = "ip4:1"	// ip protocol number for icmp
	} else if command.Version == IPv6 {
		connNetwork = "ip6:58"	// ip protocol number for icmpv6
	} else {
		command.Logging.Println("address type is not IPv4 or IPv6")
		return nil
	}

	conn, err := icmp.ListenPacket(connNetwork, "")
	if err != nil {
		command.Logging.Printf("Error trying to listen for ICMP packets: %s\n", err.Error())
		return nil
	}

	return conn
}


// Send a single ping.
// pc: The packet connection on which to send the ping
// dest: The destination endpoint to send the ping to
// message: The echo request to send
// Returns nil, or error
func sendPing(pc *icmp.PacketConn, dest *net.IPAddr, message *icmp.Message) error {
	// need to make a byte array for storing the message
	wb, err := message.Marshal(nil)
	if err != nil {
		return err
	}
	if _, err := pc.WriteTo(wb, dest); err == nil {
		return err
	}
	return nil
}


// Sends the pings desired by command.
// pc: The packet connection on which to send the pings
// timeArr: The array of times that pings were sent at (filled by this func for use in calculating post-run stats)
// typ: The type field of an IP packet for this ping to use
// errs: The channel to notify if an error occurs while trying to send an ICMP message
func (command *Command) sendPings(pc *icmp.PacketConn, timeArr []time.Time, typ icmp.Type, errs chan int) {
	defer command.wg.Done()

	messages := make([]*icmp.Message, command.Count)
	for i := 0; i < command.Count; i++ {
		message := &icmp.Message {
			Type:     	typ,
			Code:     	0,
			Checksum: 	0,
			Body:     	&icmp.Echo{
				ID:		command.id,
				Seq:  	i + command.seqStart,
				Data: 	[]byte(fmt.Sprintf("ABCDEFGHIJKLMNOPQRSTUVWabcdefghi")),
			},
		}
		messages[i] = message
	}

	for i, msg := range messages {
		select {
		case <- command.done:
			command.done <- true
			return
		default:
			timeArr[i] = time.Now()
			if err := sendPing(pc, command.Addr, msg); err != nil {
				command.Logging.Println(err)
				errs <- i
			} else {
				command.sentCount += 1
				command.Logging.Println(fmt.Sprintf("sent echo request, seq = %v", i - command.seqStart))
			}
			command.sentAttempts += 1
			time.Sleep(command.Interval)
		}
	}
}


// Receive pings on the given icmp.PacketConn
// Continues listening until its received information on all the pings that have been attempted or a signal is sent
// on the command.done channel
// pc: The packet connection to listen for echo replies (or dst unreachable or time expired) on
// timeArr: The array of times (to be filled in by sendPings()) that pings were originally sent at for use in calculating rtt
// errs: Channel to write the relative seq number of messages that had errors that prevented them from being sent. Is used
//			to notify receivePings() that a reply is not coming for this message.
func (command *Command) receivePings(pc *icmp.PacketConn, timeArr []time.Time, errs chan int) {
	defer command.wg.Done()
	processed := 0
	for processed < command.sentAttempts || command.sentAttempts < command.Count {
		select {
		case <- command.done:
			command.done <- true
			return
		case relSeq, _ := <- errs:
			command.Logging.Println(fmt.Sprintf("error occurred during ping send for %d", relSeq + command.seqStart))
			processed += 1
		default:
			rbuf := make([]byte, 1500) // max mac segment size

			if err := pc.SetReadDeadline(time.Now().Add(time.Second * 1)); err != nil {
				command.Logging.Println("fatal error: failed to set read deadline on connection")
				command.done <- true
				return
			}

			var ipProtocol int
			var peer net.Addr
			var err error
			if command.Version == IPv4 {
				ipProtocol = protocolICMP
				_, _, peer, err = pc.IPv4PacketConn().ReadFrom(rbuf)
			} else if command.Version == IPv6 {
				ipProtocol = protocolICMPv6
				_, _, peer, err = pc.IPv6PacketConn().ReadFrom(rbuf)
			} else { // something is wrong
				command.Logging.Println("unable to determine IP Version")
				return
			}
			if err != nil {
				if neterr, ok := err.(*net.OpError); ok {
					if neterr.Timeout() {
						// Read timeout
						continue
					} else {
						command.done <- true
						command.Logging.Println("something went wrong reading from the connection")
						return
					}
				}
			}

			err = command.processPacket(rbuf, peer, ipProtocol, timeArr)
			if err != nil {
				command.Logging.Println(err)
			} else {
				command.recvCount += 1
				processed += 1
			}
		}
	}
}

// Process a packet read in by receivePings(). Returns nil, or error on
// bytes: The byte buffer that the message is stored in.
// source: The address that the response was received from.
// proto: The protocol number that the IP packet will have (ICMP over IPv4 or ICMP over IPv6)
// timeArr: The array of times (filled in by sendPings()) with the times that echo requests are sent. Used to calculate the rtt.
func (command *Command) processPacket(bytes []byte, source net.Addr, proto int, timeArr []time.Time) error {
	curTime := time.Now()
	var msg *icmp.Message
	var err error
	if msg, err = icmp.ParseMessage(proto, bytes); err != nil {
		return errors.New(fmt.Sprintf("error parsing icmp message %v", err.Error()))
	}

	switch pkt := msg.Body.(type) {
	case *icmp.Echo:
		// do something
		if pkt.ID != command.id {
			return errors.New("this packet is not part of this ping")
		}
		rtt := curTime.Sub(timeArr[pkt.Seq - command.seqStart])
		command.rtts = append(command.rtts, rtt)
		command.Logging.Println(fmt.Sprintf("from %v : echo response received in %v ms", source, rtt.Milliseconds()))
	case *icmp.DstUnreach:
		command.Logging.Println(fmt.Sprintf("from %v : Destination host unreachable", source))
	case *icmp.TimeExceeded:
		command.Logging.Println(fmt.Sprintf("from %v : Time Exceeded before reaching destination", source))
	default:
		return fmt.Errorf("unhandled ICMP response type: %T, %v", pkt, pkt)
	}

	return nil
}


// Prints statistics after running the ping command. Similar to the output of *nix ping
func (command *Command) printStats() {
	command.Logging.Printf("--- %v ping statistics ---\n", command.Addr)
	percent := float64(command.sentCount - command.recvCount) * float64(100) / float64(command.sentCount)
	timeElapsed := time.Now().Sub(command.startTime).Milliseconds()
	command.Logging.Printf("%d packets transmitted, %d received, %0.2f%% packet loss, time %d ms",
		command.sentCount, command.recvCount, percent, timeElapsed)
	if command.recvCount > 0 {
		min := command.rtts[0]
		for _, val := range command.rtts {
			if val < min {
				min = val
			}
		}
		max := command.rtts[0]
		for _, val := range command.rtts {
			if val > max {
				max = val
			}
		}
		var avg time.Duration
		for _, val := range command.rtts {
			avg += val
		}
		avg = avg / time.Duration(len(command.rtts))

		var sumSquares time.Duration
		for _, val := range command.rtts {
			sumSquares += (val - avg) * (val - avg)
		}
		stdDev := time.Duration(math.Sqrt(float64(sumSquares / time.Duration(len(command.rtts)))))

		command.Logging.Printf("rtt min/avg/max/mdev = %v/%v/%v/%v ms", min, max, avg, stdDev)
	}
}


// Send the done channel a message indicating that the program run is over.
func (command *Command) End() {
	command.done <- true
}


// Run the pings specified in command, and print the results and statistics to the specified logger.
// TODO change logging to a generic writer or something
func (command *Command) Ping() error {
	timeArr := make([]time.Time, command.Count)

	conn := command.listen()
	if conn == nil {
		command.Logging.Fatal("Fatal Error: Unable to Listen. Exiting...")
	}
	defer conn.Close()

	// check for ipv4 or ipv6
	var typ icmp.Type
	if command.Version == IPv4 {
		typ = ipv4.ICMPTypeEcho	// below is unimplemented on Windows
		err1 := conn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
		err2 := conn.IPv4PacketConn().SetTTL(command.Ttl)
		if err1 == nil || err2 == nil {
			command.Logging.Println("failed to set TTL... continuing with default")
		}

	} else if command.Version == IPv6 {
		typ = ipv6.ICMPTypeEchoRequest	// below is unimplemented on Windows
		err1 := conn.IPv6PacketConn().SetControlMessage(ipv6.FlagHopLimit, true)
		err2 := conn.IPv6PacketConn().SetHopLimit(command.Ttl)
		if err1 == nil || err2 == nil {
			command.Logging.Println("failed to set TTL... continuing with default")
		}
	} else {
		return errors.New("fatal error: address type is not IPv4 or IPv6")
	}

	// Setup completed
	if command.MaxTime > 0 {	// start the watchdog timer
		go func(done chan bool ) {
			time.Sleep(command.MaxTime)
			done <- true
		}(command.done)
	}

	errs := make(chan int)
	command.wg.Add(1)
	go command.receivePings(conn, timeArr, errs)
	command.wg.Add(1)
	go command.sendPings(conn, timeArr, typ, errs)

	command.wg.Wait()

	close(command.done)

	command.printStats()
	return nil
}