// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qbft

import (
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/33cn/chain33/common/crypto"
	ttypes "github.com/33cn/chain33/plugin/consensus/qbft/types"
)

const (
	numBufferedConnections = 10
	maxNumPeers            = 50
	tryListenSeconds       = 5
	handshakeTimeout       = 20 // * time.Second,
	maxSendQueueSize       = 1024
	defaultSendTimeout     = 60 * time.Second
	//MaxMsgPacketPayloadSize define
	MaxMsgPacketPayloadSize            = 2 * 1024 * 1024
	defaultDialTimeout                 = 3 * time.Second
	dialRandomizerIntervalMilliseconds = 3000
	// repeatedly try to reconnect for a few minutes
	// ie. 5 * 20 = 100s
	reconnectAttempts = 20
	reconnectInterval = 5 * time.Second

	// then move into exponential backoff mode for ~1day
	// ie. 3**10 = 16hrs
	reconnectBackOffAttempts    = 10
	reconnectBackOffBaseSeconds = 3

	minReadBufferSize  = 1024
	minWriteBufferSize = 65536
)

// Parallel method
func Parallel(tasks ...func()) {
	var wg sync.WaitGroup
	wg.Add(len(tasks))
	for _, task := range tasks {
		go func(task func()) {
			task()
			wg.Done()
		}(task)
	}
	wg.Wait()
}

// GenIDByPubKey method
func GenIDByPubKey(pubkey crypto.PubKey) ID {
	//must add 3 bytes ahead to make compatibly
	typeAddr := append([]byte{byte(0x01), byte(0x01), byte(0x20)}, pubkey.Bytes()[:32]...)
	address := crypto.Ripemd160(typeAddr)
	return ID(hex.EncodeToString(address))
}

// IP2IPPort struct
type IP2IPPort struct {
	mutex   sync.RWMutex
	mapList map[string]string
}

// NewMutexMap method
func NewMutexMap() *IP2IPPort {
	return &IP2IPPort{
		mapList: make(map[string]string),
	}
}

// Has method
func (ipp *IP2IPPort) Has(ip string) bool {
	ipp.mutex.RLock()
	defer ipp.mutex.RUnlock()
	_, ok := ipp.mapList[ip]
	return ok
}

// Set method
func (ipp *IP2IPPort) Set(ip string, ipport string) {
	ipp.mutex.Lock()
	defer ipp.mutex.Unlock()
	ipp.mapList[ip] = ipport
}

// Delete method
func (ipp *IP2IPPort) Delete(ip string) {
	ipp.mutex.Lock()
	defer ipp.mutex.Unlock()
	delete(ipp.mapList, ip)
}

// NodeInfo struct
type NodeInfo struct {
	ID      ID     `json:"id"`
	Network string `json:"network"`
	Version string `json:"version"`
	IP      string `json:"ip,omitempty"`
}

// Node struct
type Node struct {
	listener    net.Listener
	connections chan net.Conn
	privKey     crypto.PrivKey
	Network     string
	Version     string
	ID          ID
	IP          string //get ip from connect to ourself

	localIPs map[string]net.IP
	peerSet  *PeerSet

	dialing      *IP2IPPort
	reconnecting *IP2IPPort

	seeds    []string
	protocol string
	lAddr    string

	state            *ConsensusState
	broadcastChannel chan MsgInfo
	unicastChannel   chan MsgInfo
	started          uint32 // atomic
	stopped          uint32 // atomic
}

// NewNode method
func NewNode(seeds []string, protocol string, lAddr string, privKey crypto.PrivKey, network string, version string, state *ConsensusState) *Node {
	node := &Node{
		peerSet:     NewPeerSet(),
		seeds:       seeds,
		protocol:    protocol,
		lAddr:       lAddr,
		connections: make(chan net.Conn, numBufferedConnections),

		privKey:          privKey,
		Network:          network,
		Version:          version,
		ID:               GenIDByPubKey(privKey.PubKey()),
		dialing:          NewMutexMap(),
		reconnecting:     NewMutexMap(),
		broadcastChannel: make(chan MsgInfo, maxSendQueueSize),
		unicastChannel:   make(chan MsgInfo, maxSendQueueSize),
		state:            state,
		localIPs:         make(map[string]net.IP),
	}

	state.SetOurID(node.ID)
	state.SetBroadcastChannel(node.broadcastChannel)
	state.SetUnicastChannel(node.unicastChannel)

	localIPs := getNaiveExternalAddress(true)
	if len(localIPs) > 0 {
		for _, item := range localIPs {
			node.localIPs[item.String()] = item
		}
	}
	return node
}

// Start node
func (node *Node) Start() {
	if atomic.CompareAndSwapUint32(&node.started, 0, 1) {
		// Create listener
		var listener net.Listener
		var err error
		for i := 0; i < tryListenSeconds; i++ {
			listener, err = net.Listen(node.protocol, node.lAddr)
			if err == nil {
				break
			} else if i < tryListenSeconds-1 {
				time.Sleep(time.Second)
			}
		}
		if err != nil {
			panic(err)
		}
		node.listener = listener
		// Actual listener local IP & port
		listenerIP, listenerPort := splitHostPort(listener.Addr().String())
		qbftlog.Info("Local listener", "ip", listenerIP, "port", listenerPort)

		go node.listenRoutine()

		for i := 0; i < len(node.seeds); i++ {
			go func(i int) {
				addr := node.seeds[i]
				ip, _ := splitHostPort(addr)
				_, ok := node.localIPs[ip]
				if ok {
					qbftlog.Info("find our ip ", "ourIP", ip)
					node.IP = ip
					return
				}

				randomSleep(0)
				err := node.DialPeerWithAddress(addr)
				if err != nil {
					qbftlog.Error("Error dialing peer", "err", err)
				}
			}(i)
		}

		go node.StartConsensusRoutine()
		go node.BroadcastRoutine()
		go node.UnicastRoutine()
	}
}

// DialPeerWithAddress ...
func (node *Node) DialPeerWithAddress(addr string) error {
	ip, _ := splitHostPort(addr)
	node.dialing.Set(ip, addr)
	defer node.dialing.Delete(ip)
	return node.addOutboundPeerWithConfig(addr)
}

func (node *Node) addOutboundPeerWithConfig(addr string) error {
	qbftlog.Info("Dialing peer", "address", addr)

	peerConn, err := newOutboundPeerConn(addr, node.privKey, node.StopPeerForError, node.state)
	if err != nil {
		go node.reconnectToPeer(addr)
		return err
	}

	if err := node.addPeer(peerConn); err != nil {
		peerConn.CloseConn()
		return err
	}
	return nil
}

// Stop ...
func (node *Node) Stop() {
	atomic.CompareAndSwapUint32(&node.stopped, 0, 1)
	err := node.listener.Close()
	if err != nil {
		qbftlog.Error("Close listener failed", "err", err)
	}
	// Stop peers
	for _, peer := range node.peerSet.List() {
		peer.Stop()
		node.peerSet.Remove(peer)
	}
	//stop consensus
	node.state.Stop()
}

// IsRunning ...
func (node *Node) IsRunning() bool {
	return atomic.LoadUint32(&node.started) == 1 && atomic.LoadUint32(&node.stopped) == 0
}

func (node *Node) listenRoutine() {
	for {
		conn, err := node.listener.Accept()

		if !node.IsRunning() {
			break // Go to cleanup
		}

		// listener wasn't stopped,
		// yet we encountered an error.
		if err != nil {
			panic(err)
		}

		go node.connectComming(conn)
	}

	// Cleanup
	close(node.connections)
	for range node.connections {
		// Drain
	}
}

// StartConsensusRoutine if peers reached the threshold start consensus routine
func (node *Node) StartConsensusRoutine() {
	for {
		//TODO:the peer count need be optimized
		if node.peerSet.Size() >= 0 {
			node.state.Start()
			break
		}
		time.Sleep(1 * time.Second)
	}
}

// BroadcastRoutine receive to broadcast
func (node *Node) BroadcastRoutine() {
	for {
		msg, ok := <-node.broadcastChannel
		if !ok {
			qbftlog.Info("broadcastChannel closed")
			return
		}
		node.Broadcast(msg)
	}
}

// UnicastRoutine receive to broadcast
func (node *Node) UnicastRoutine() {
	for {
		msg, ok := <-node.unicastChannel
		if !ok {
			qbftlog.Info("unicastChannel closed")
			return
		}
		for _, peer := range node.peerSet.List() {
			if peer.ID() == msg.PeerID {
				peerIP, _ := peer.RemoteIP()
				msg.PeerIP = peerIP.String()
				success := peer.Send(msg)
				if !success {
					qbftlog.Error("send failure in UnicastRoutine", "peerID", peer.ID())
				}
				break
			}
		}
	}
}

func (node *Node) connectComming(inConn net.Conn) {
	maxPeers := maxNumPeers
	if maxPeers <= node.peerSet.Size() {
		qbftlog.Debug("Ignoring inbound connection: already have enough peers", "address", inConn.RemoteAddr().String(), "numPeers", node.peerSet.Size(), "max", maxPeers)
		return
	}

	// New inbound connection!
	err := node.addInboundPeer(inConn)
	if err != nil {
		qbftlog.Info("Ignoring inbound connection: error while adding peer", "address", inConn.RemoteAddr().String(), "err", err)
		return
	}
}

func (node *Node) stopAndRemovePeer(peer Peer, reason interface{}) {
	node.peerSet.Remove(peer)
	peer.Stop()
}

// StopPeerForError called if error occurred
func (node *Node) StopPeerForError(peer Peer, reason interface{}) {
	qbftlog.Error("Stopping peer for error", "peer", peer, "err", reason)
	addr, err := peer.RemoteAddr()
	node.stopAndRemovePeer(peer, reason)

	if peer.IsPersistent() {
		if err == nil && addr != nil {
			go node.reconnectToPeer(addr.String())
		}
	}
}

func (node *Node) isSeed(addr string) bool {
	ip, _ := splitHostPort(addr)
	for _, seed := range node.seeds {
		host, _ := splitHostPort(seed)
		if host == ip {
			return true
		}
	}
	return false
}

func (node *Node) addInboundPeer(conn net.Conn) error {
	peerConn, err := newInboundPeerConn(conn, node.isSeed(conn.RemoteAddr().String()), node.privKey, node.StopPeerForError, node.state)
	if err != nil {
		if er := conn.Close(); er != nil {
			qbftlog.Error("addInboundPeer close conn failed", "er", er)
		}
		return err
	}
	if err = node.addPeer(peerConn); err != nil {
		peerConn.CloseConn()
		return err
	}

	return nil
}

// addPeer checks the given peer's validity, performs a handshake, and adds the
// peer to the switch and to all registered reactors.
// NOTE: This performs a blocking handshake before the peer is added.
// NOTE: If error is returned, caller is responsible for calling peer.CloseConn()
func (node *Node) addPeer(pc *peerConn) error {
	addr := pc.conn.RemoteAddr()
	if err := node.FilterConnByAddr(addr); err != nil {
		return err
	}

	remoteIP, rErr := pc.RemoteIP()

	nodeinfo := NodeInfo{
		ID:      node.ID,
		Network: node.Network,
		Version: node.Version,
		IP:      node.IP,
	}
	// Exchange NodeInfo on the conn
	peerNodeInfo, err := pc.HandshakeTimeout(nodeinfo, handshakeTimeout*time.Second)
	if err != nil {
		return err
	}

	peerID := peerNodeInfo.ID

	// ensure connection key matches self reported key
	connID := pc.ID()

	if peerID != connID {
		return fmt.Errorf(
			"nodeInfo.ID() (%v) doesn't match conn.ID() (%v)",
			peerID,
			connID,
		)
	}

	// Avoid self
	if node.ID == peerID {
		return fmt.Errorf("Connect to self: %v", addr)
	}

	// Avoid duplicate
	if node.peerSet.Has(peerID) {
		return fmt.Errorf("Duplicate peer ID %v", peerID)
	}

	// Check for duplicate connection or peer info IP.
	if rErr == nil && node.peerSet.HasIP(remoteIP) {
		return fmt.Errorf("Duplicate peer IP %v", remoteIP)
	} else if rErr != nil {
		return fmt.Errorf("get remote ip failed:%v", rErr)
	}

	// Check version, chain id
	if err := node.CompatibleWith(peerNodeInfo); err != nil {
		return err
	}

	qbftlog.Info("Successful handshake with peer", "peerNodeInfo", peerNodeInfo)

	// All good. Start peer
	if node.IsRunning() {
		pc.SetTransferChannel(node.state.peerMsgQueue)
		if err = node.startInitPeer(pc); err != nil {
			return err
		}
	}

	// Add the peer to .peers.
	// We start it first so that a peer in the list is safe to Stop.
	// It should not err since we already checked peers.Has().
	if err := node.peerSet.Add(pc); err != nil {
		return err
	}

	qbftlog.Info("Added peer", "ip", pc.ip, "peer", pc)
	rs := node.state.GetRoundState()
	stateMsg := MsgInfo{TypeID: ttypes.NewRoundStepID, Msg: rs.RoundStateMessage(), PeerID: pc.id, PeerIP: pc.ip.String()}
	pc.Send(stateMsg)
	qbftlog.Info("Send state msg", "msg", stateMsg, "ourIP", node.IP, "ourID", node.ID)
	return nil
}

// Broadcast to peers in set
func (node *Node) Broadcast(msg MsgInfo) chan bool {
	successChan := make(chan bool, len(node.peerSet.List()))
	qbftlog.Debug("Broadcast", "msgtype", msg.TypeID)
	var wg sync.WaitGroup
	for _, peer := range node.peerSet.List() {
		wg.Add(1)
		go func(peer Peer) {
			defer wg.Done()
			msg.PeerID = peer.ID()
			peerIP, _ := peer.RemoteIP()
			msg.PeerIP = peerIP.String()
			success := peer.Send(msg)
			successChan <- success
		}(peer)
	}
	go func() {
		wg.Wait()
		close(successChan)
	}()
	return successChan
}

func (node *Node) startInitPeer(peer *peerConn) error {
	err := peer.Start() // spawn send/recv routines
	if err != nil {
		// Should never happen
		qbftlog.Error("Error starting peer", "peer", peer, "err", err)
		return err
	}

	return nil
}

// FilterConnByAddr TODO:can make fileter by addr
func (node *Node) FilterConnByAddr(addr net.Addr) error {
	return nil
}

// CompatibleWith one node by nodeInfo
func (node *Node) CompatibleWith(other NodeInfo) error {
	iMajor, iMinor, _, iErr := splitVersion(node.Version)
	oMajor, oMinor, _, oErr := splitVersion(other.Version)

	// if our own version number is not formatted right, we messed up
	if iErr != nil {
		return iErr
	}

	// version number must be formatted correctly ("x.x.x")
	if oErr != nil {
		return oErr
	}

	// major version must match
	if iMajor != oMajor {
		return fmt.Errorf("Peer is on a different major version. Got %v, expected %v", oMajor, iMajor)
	}

	// minor version can differ
	if iMinor != oMinor {
		// ok
	}

	// nodes must be on the same network
	if node.Network != other.Network {
		return fmt.Errorf("Peer is on a different network. Got %v, expected %v", other.Network, node.Network)
	}

	return nil
}

func (node *Node) reconnectToPeer(addr string) {
	host, _ := splitHostPort(addr)
	if node.reconnecting.Has(host) {
		return
	}
	node.reconnecting.Set(host, addr)
	defer node.reconnecting.Delete(host)

	start := time.Now()
	qbftlog.Info("Reconnecting to peer", "addr", addr)
	for i := 0; i < reconnectAttempts; i++ {
		if !node.IsRunning() {
			return
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			qbftlog.Info("LookupIP failed", "host", host)
			continue
		}
		if node.peerSet.HasIP(ips[0]) {
			qbftlog.Info("Reconnecting to peer exit, already connect to the peer", "peer", host)
			return
		}

		err = node.DialPeerWithAddress(addr)
		if err == nil {
			return // success
		}

		qbftlog.Info("Error reconnecting to peer. Trying again", "tries", i, "err", err, "addr", addr)
		// sleep a set amount
		randomSleep(reconnectInterval)
		continue
	}

	qbftlog.Error("Failed to reconnect to peer. Beginning exponential backoff",
		"addr", addr, "elapsed", time.Since(start))
	for i := 0; i < reconnectBackOffAttempts; i++ {
		if !node.IsRunning() {
			return
		}

		// sleep an exponentially increasing amount
		sleepIntervalSeconds := math.Pow(reconnectBackOffBaseSeconds, float64(i))
		time.Sleep(time.Duration(sleepIntervalSeconds) * time.Second)
		err := node.DialPeerWithAddress(addr)
		if err == nil {
			return // success
		}
		qbftlog.Info("Error reconnecting to peer. Trying again", "tries", i, "err", err, "addr", addr)
	}
	qbftlog.Error("Failed to reconnect to peer. Giving up", "addr", addr, "elapsed", time.Since(start))
}

//---------------------------------------------------------------------
func randomSleep(interval time.Duration) {
	r := time.Duration(ttypes.RandInt63n(dialRandomizerIntervalMilliseconds)) * time.Millisecond
	time.Sleep(r + interval)
}

func isIpv6(ip net.IP) bool {
	v4 := ip.To4()
	if v4 != nil {
		return false
	}

	ipString := ip.String()

	// Extra check just to be sure it's IPv6
	return (strings.Contains(ipString, ":") && !strings.Contains(ipString, "."))
}

func getNaiveExternalAddress(defaultToIPv4 bool) []net.IP {
	var ips []net.IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		panic(fmt.Sprintf("Could not fetch interface addresses: %v", err))
	}

	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if defaultToIPv4 || !isIpv6(ipnet.IP) {
			v4 := ipnet.IP.To4()
			if v4 == nil || v4[0] == 127 {
				// loopback
				continue
			}
		} else if ipnet.IP.IsLoopback() {
			// IPv6, check for loopback
			continue
		}
		ips = append(ips, ipnet.IP)
	}
	return ips
}

func splitVersion(version string) (string, string, string, error) {
	spl := strings.Split(version, ".")
	if len(spl) != 3 {
		return "", "", "", fmt.Errorf("Invalid version format %v", version)
	}
	return spl[0], spl[1], spl[2], nil
}

func splitHostPort(addr string) (host string, port int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		panic(err)
	}
	return host, port
}

func dial(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, defaultDialTimeout)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func newOutboundPeerConn(addr string, ourNodePrivKey crypto.PrivKey, onPeerError func(Peer, interface{}), state *ConsensusState) (*peerConn, error) {
	conn, err := dial(addr)
	if err != nil {
		return &peerConn{}, fmt.Errorf("newOutboundPeerConn dial fail:%v", err)
	}

	pc, err := newPeerConn(conn, true, true, ourNodePrivKey, onPeerError, state)
	if err != nil {
		if cerr := conn.Close(); cerr != nil {
			return &peerConn{}, fmt.Errorf("newPeerConn failed:%v, connection close failed:%v", err, cerr)
		}
		return &peerConn{}, err
	}

	return pc, nil
}

func newInboundPeerConn(
	conn net.Conn,
	persistent bool,
	ourNodePrivKey crypto.PrivKey,
	onPeerError func(Peer, interface{}),
	state *ConsensusState,
) (*peerConn, error) {

	// TODO: issue PoW challenge

	return newPeerConn(conn, false, persistent, ourNodePrivKey, onPeerError, state)
}

func newPeerConn(
	rawConn net.Conn,
	outbound, persistent bool,
	ourNodePrivKey crypto.PrivKey,
	onPeerError func(Peer, interface{}),
	state *ConsensusState,
) (pc *peerConn, err error) {
	conn := rawConn

	// Set deadline for secret handshake
	dl := time.Now().Add(handshakeTimeout * time.Second)
	if err := conn.SetDeadline(dl); err != nil {
		return pc, fmt.Errorf("Error setting deadline while encrypting connection:%v", err)
	}

	// Encrypt connection
	conn, err = MakeSecretConnection(conn, ourNodePrivKey)
	if err != nil {
		return pc, fmt.Errorf("MakeSecretConnection fail:%v", err)
	}

	// Only the information we already have
	return &peerConn{
		outbound:    outbound,
		persistent:  persistent,
		conn:        conn,
		onPeerError: onPeerError,
		myState:     state,
	}, nil
}
