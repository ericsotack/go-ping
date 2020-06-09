package pinger

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/icmp"
)

type IPVersion int

const (
	IPv4 = 4
	IPv6 = 6
	UNSET = 0
)

type Command struct {
	Addr		*net.IPAddr
	Version		IPVersion
	Count 		int
	Ttl 		int			// TODO implememt -> can do
	Interval	time.Duration	// in the smallest unit available of time (i.e. 3 seconds = 3*10^9 ns)
	Timeout		time.Duration
	Logging		*log.Logger
}

type Result struct {
	Response		bool
	Hostname		string
	ResponseAddr	*net.IPAddr
	IcmpSeq			int
	Size			int
	RoundTripTime 	time.Duration
}

func numberErrors(p *sync.Map, count uint) int {
	counter := 0
	for i := 0; uint(i) < count; i++ {
		if val, ok := p.Load(i); val != nil || ok == true {
			counter++
		}
	}

	return counter
}

func sendPing(pc *icmp.PacketConn, dest *net.IPAddr, message icmp.Echo, protocol int) error {
	wb, err := message.Marshal(protocol)
	if err != nil {
		return err
	}
	if _, err := pc.WriteTo(wb, dest); err == nil {
		return err
	}
	return nil
}

// needs to send a timer for each thread to notify the listener if something takes too long to come back
// doesn't do "DONE" for waitgroup unless it finishes properly
func (command *Command) sendPings(pc *icmp.PacketConn, timeArr []time.Time, messages []icmp.Echo, protocol int, errs *sync.Map, wg *sync.WaitGroup) {
	defer wg.Done()
	for i, msg := range messages {
		timeArr[i] = time.Now()
		if err := sendPing(pc, command.Addr, msg, protocol); err != nil {
			command.Logging.Println(err)
			errs.Store(i, errors.New(fmt.Sprintf("unable to send ping %v, error: %v", i, err)))
		}
		command.Logging.Println(fmt.Sprintf("sent echo request, seq = %v", i + 1))
		go func() {	// watchdog timer and a closure
			time.Sleep(command.Timeout)
			errs.Store(i, errors.New(fmt.Sprintf("maxium timeout exceeded for ping %v", i)))
		}()
		time.Sleep(command.Interval)
	}
}

// listen on all interfaces with 0.0.0.0 addr
// needs to know how many to listen for
// opens a socket that listens for all of them, as a go routine
// when it receives one, checks the array to see which one it is and when it left, then
func (command *Command) listenPings(pc *icmp.PacketConn, timeArr []time.Time, errs *sync.Map, ch chan []Result, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(ch)
	results := make([]Result, command.Count)

	rb := make([]byte, 1500)	// max mac segment size
	var ipProtocol int
	if command.Version == IPv4 {
		ipProtocol = 1
	} else if command.Version == IPv6 {
		ipProtocol = 58
	} else {	// something is wrong
		command.Logging.Println("unable to determine IP Version")
		return
	}

	curErrs := numberErrors(errs, command.Count)

	for counter:= curErrs; counter < int(command.Count); counter++ {
		// if an error message is sent that the ping is bad, then put it in the rejected set and increment counter
		// if a ping comes in later that matches the rejected, fix it but don't increment received counter
		if newErrs := numberErrors(errs, command.Count); newErrs != curErrs {
			counter += newErrs - curErrs
			curErrs = newErrs
		}


		n, peer, err := pc.ReadFrom(rb)
		curTime := time.Now()

		if err != nil {
			command.Logging.Println("something went wrong reading from the connection")
			ch <- nil
			return
		}
		rm, err := icmp.ParseMessage(ipProtocol, rb[:n])
		if err != nil {
			command.Logging.Println("something went wrong parsing the message")
			ch <- nil
			return
		}


		// process the message body for sequence  number
		// seq := rm.Body
		seq := counter
		rtt := curTime.Sub(timeArr[seq]).Milliseconds()
		if val, ok := errs.Load(seq) ; ok != false {
			command.Logging.Println(fmt.Sprintf("Errored ping [%v] received from %v. RTT: %v ms, %v", val, peer, rtt, rm.Body))
		} else {
			command.Logging.Println(fmt.Sprintf("Errored ping received from %v. RTT: %v ms, %v", peer, rtt, rm.Body))
			// need to process message body type to determine type of message
			// add an entry to results
		}
	}

	ch <- results
	return
}

// prints out results as things are sent
func (command *Command) Ping() ([]Result, error) {
	timeArr := make([]time.Time, command.Count)
	var connNetwork string
	var listenAddr string

	// check for ipv4 or ipv6
	var protocol int
	if command.Version == IPv4 {
		protocol = 8
		connNetwork = "ip4:1"	// ip protocol number for icmp
		listenAddr = "0.0.0.0"
	} else if command.Version == IPv6{
		protocol = 128
		connNetwork = "ip6:58"	// ip protocol number for icmpv6
		listenAddr = "::"
	} else {
		return nil, errors.New("address type is not IPv4 or IPv6")
	}

	// create the messages
	messages := make([]icmp.Echo, command.Count)
	for i := 1; uint(i) <= command.Count; i++ {
		message := icmp.Echo{
			ID:   os.Getpid() & 0xffff,	// TODO change this to be random later
			Seq:  i,
			Data: []byte(fmt.Sprintf("ABCDEFGHIJKLMNOPQRSTUVWabcdefghi")),
		}
		messages[i - 1] = message
	}

	// create a connection
	//conn, err := icmp.ListenPacket(connNetwork, (*command.Addr).String())
	conn, err := icmp.ListenPacket(connNetwork, listenAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	wg := sync.WaitGroup{}
	errs := sync.Map{}
	ch := make(chan []Result)
	wg.Add(1)
	command.Logging.Println("starting listening go routine")
	go command.listenPings(conn, timeArr, &errs, ch, &wg)
	wg.Add(1)
	command.Logging.Println("starting sending go routine")
	go command.sendPings(conn, timeArr, messages, protocol, &errs, &wg)

	// pull final return value from go routine, or nil if error
	res, _ := <- ch
	if res == nil {
		return nil, errors.New("error occurred while listening for pings")
	}
	wg.Wait()

	// TODO print overall stats
	command.Logging.Println("Finished!")

	return res, nil
}