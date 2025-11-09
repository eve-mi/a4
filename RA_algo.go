package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"a4/grpc/proto"

	"google.golang.org/grpc"
)

type State int

const (
	RELEASED State = iota
	WANTED
	HELD
)

type Node struct {
	id       int32
	clock    int64
	state    State
	myStamp  int64
	peers    []string
	clients  map[string]proto.RicardAlgoClient
	mu       sync.Mutex
	deferred []proto.RequestMsg
	replyCh  chan struct{}

	proto.UnimplementedRicardAlgoServer
}

// just create a new node
func NewNode(id int32, peers []string) *Node {
	return &Node{
		id:      id,
		peers:   peers,
		clients: make(map[string]proto.RicardAlgoClient),
	}
}

// simply start the server
func (n *Node) startServer(port int) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterRicardAlgoServer(s, n)
	go s.Serve(lis)
}

// simply connect all peers
func (n *Node) connectPeers() {
	for _, p := range n.peers {
		conn, err := grpc.Dial(p, grpc.WithInsecure())
		if err != nil {
			log.Printf("Node %d failed to connect to %s", n.id, p)
			continue
		}
		n.clients[p] = proto.NewRicardAlgoClient(conn)
	}
}

// ensure logical clock stays updated
func (n *Node) updateClock(t int64) {
	if t > n.clock {
		n.clock = t
	}
	n.clock++
}

// handles any received request from other nodes (either defer request if CS HELD or WANTED, or grant permission immediately)
func (n *Node) Request(ctx context.Context, req *proto.RequestMsg) (*proto.ReplyMsg, error) {
	n.mu.Lock()
	n.updateClock(req.Timestamp)

	if n.state == HELD || (n.state == WANTED &&
		(n.myStamp < req.Timestamp || (n.myStamp == req.Timestamp && n.id < req.NodeId))) {
		//Defer
		n.deferred = append(n.deferred, *req)
		log.Printf("Node %d defers reply to %d", n.id, req.NodeId)
	} else {
		// Grant permission immediately (have to call another method, i.e. grantRequest bc here we can only return empty message)
		go n.grantRequest(req.NodeId)
		log.Printf("Node %d grants request to %d", n.id, req.NodeId)
	}

	timestamp := n.clock + 1
	n.mu.Unlock()
	return &proto.ReplyMsg{Timestamp: timestamp, NodeId: n.id}, nil
}

// handles any received reply from other nodes
func (n *Node) Reply(ctx context.Context, rep *proto.ReplyMsg) (*proto.Empty, error) {
	n.mu.Lock()
	n.updateClock(rep.Timestamp)
	n.mu.Unlock()

	//a reply arrived
	n.replyCh <- struct{}{}

	log.Printf("Node %d got Reply from %d", n.id, rep.NodeId)
	return &proto.Empty{}, nil
}

// grant request by sending reply to peers
func (n *Node) grantRequest(to int32) {
	addr := fmt.Sprintf("localhost:%d", 5000+int(to))
	peer := n.clients[addr]
	n.mu.Lock()
	timestamp := n.clock + 1
	n.mu.Unlock()

	//send reply to the requesting node
	_, err := peer.Reply(context.Background(), &proto.ReplyMsg{
		Timestamp: timestamp,
		NodeId:    n.id,
	})
	if err != nil {
		log.Printf("Node %d failed to reply to %d: %v", n.id, to, err)
	}
}

// handles logic when the local node requests Critical section
func (n *Node) requestCS() {
	n.mu.Lock()
	n.state = WANTED
	n.myStamp = n.clock + 1
	n.replyCh = make(chan struct{}, len(n.peers)-1)
	stamp := n.myStamp
	n.mu.Unlock()

	log.Printf("Node %d requests CS", n.id)

	//send request to all peers (excluding itself)
	for _, p := range n.peers {
		if !strings.Contains(p, fmt.Sprint(5000+int(n.id))) { //exclude itself
			go func(addr string) {
				peer := n.clients[addr]
				_, err := peer.Request(context.Background(), &proto.RequestMsg{Timestamp: stamp, NodeId: n.id})
				if err != nil {
					log.Printf("Request to %s failed: %v", addr, err)
				}
			}(p)
		}
	}

	//need to wait to have replies from all peers (n.peers - itself, i.e. n-1)
	for i := 0; i < len(n.peers)-1; i++ {
		<-n.replyCh
	}

	//use Mutex lock to ensure safe state change
	n.mu.Lock()
	n.state = HELD
	n.mu.Unlock()

	//enter and leave critical section
	log.Printf("Node %d ENTERS CS", n.id)
	time.Sleep(5 * time.Second) //wait a bit for the sake of simulation
	log.Printf("Node %d LEAVES CS", n.id)

	//again use of mutex lock to ensure safe state change and handle deferred requests
	n.mu.Lock()
	n.state = RELEASED
	deferred := n.deferred
	n.deferred = nil
	n.mu.Unlock()

	//grant all deferred requests now that we left CS
	for _, req := range deferred {
		go n.grantRequest(req.NodeId)
	}
}

func main() {
	id := flag.Int("id", 1, "node id")
	port := flag.Int("port", 5001, "port")
	peers := flag.String("peers", "localhost:5001,localhost:5002,localhost:5003", "peers")
	flag.Parse()

	logFile, err := os.OpenFile("ricart.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	n := NewNode(int32(*id), strings.Split(*peers, ","))
	n.startServer(*port)
	time.Sleep(time.Second)
	n.connectPeers()

	log.Printf("Node %d ready", n.id)
	for {
		_, _ = fmt.Fscanln(os.Stdin)
		go n.requestCS()
	}
}
