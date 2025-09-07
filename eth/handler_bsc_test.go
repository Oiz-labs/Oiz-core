package eth

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/oiz"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type testOizHandler struct {
	voteBroadcasts event.Feed
}

func (h *testOizHandler) Chain() *core.BlockChain { panic("no backing chain") }
func (h *testOizHandler) RunPeer(peer *oiz.Peer, handler oiz.Handler) error {
	panic("not used in tests")
}
func (h *testOizHandler) PeerInfo(enode.ID) interface{} { panic("not used in tests") }
func (h *testOizHandler) Handle(peer *oiz.Peer, packet oiz.Packet) error {
	switch packet := packet.(type) {
	case *oiz.VotesPacket:
		h.voteBroadcasts.Send(packet.Votes)
		return nil

	default:
		panic(fmt.Sprintf("unexpected oiz packet type in tests: %T", packet))
	}
}

func TestSendVotes68(t *testing.T) { testSendVotes(t, eth.ETH68) }

func testSendVotes(t *testing.T, protocol uint) {
	t.Parallel()

	// Create a message handler and fill the pool with big votes
	handler := newTestHandler()
	defer handler.close()

	insert := make([]*types.VoteEnvelope, 100)
	for index := range insert {
		vote := types.VoteEnvelope{
			VoteAddress: types.BLSPublicKey{},
			Signature:   types.BLSSignature{},
			Data: &types.VoteData{
				SourceNumber: uint64(0),
				SourceHash:   common.BytesToHash(common.Hex2Bytes(string(rune(0)))),
				TargetNumber: uint64(index),
				TargetHash:   common.BytesToHash(common.Hex2Bytes(string(rune(index)))),
			},
		}
		insert[index] = &vote
		go handler.votepool.PutVote(&vote)
	}
	time.Sleep(250 * time.Millisecond) // Wait until vote events get out of the system (can't use events, vote broadcaster races with peer join)

	protos := []p2p.Protocol{
		{
			Name:    "eth",
			Version: eth.ETH68,
		},
		{
			Name:    "oiz",
			Version: oiz.Oiz1,
		},
	}
	caps := []p2p.Cap{
		{
			Name:    "eth",
			Version: eth.ETH68,
		},
		{
			Name:    "oiz",
			Version: oiz.Oiz1,
		},
	}

	// Create a source handler to send messages through and a sink peer to receive them
	p2pEthSrc, p2pEthSink := p2p.MsgPipe()
	defer p2pEthSrc.Close()
	defer p2pEthSink.Close()

	localEth := eth.NewPeer(protocol, p2p.NewPeerWithProtocols(enode.ID{1}, protos, "", caps), p2pEthSrc, nil)
	remoteEth := eth.NewPeer(protocol, p2p.NewPeerWithProtocols(enode.ID{2}, protos, "", caps), p2pEthSink, nil)
	defer localEth.Close()
	defer remoteEth.Close()

	p2pOizSrc, p2pOizSink := p2p.MsgPipe()
	defer p2pOizSrc.Close()
	defer p2pOizSink.Close()

	localOiz := oiz.NewPeer(oiz.Oiz1, p2p.NewPeerWithProtocols(enode.ID{1}, protos, "", caps), p2pOizSrc)
	remoteOiz := oiz.NewPeer(oiz.Oiz1, p2p.NewPeerWithProtocols(enode.ID{3}, protos, "", caps), p2pOizSink)
	defer localOiz.Close()
	defer remoteOiz.Close()

	go func(p *oiz.Peer) {
		(*oizHandler)(handler.handler).RunPeer(p, func(peer *oiz.Peer) error {
			return oiz.Handle((*oizHandler)(handler.handler), peer)
		})
	}(localOiz)

	time.Sleep(200 * time.Millisecond)
	remoteOiz.Handshake()

	time.Sleep(200 * time.Millisecond)
	go func(p *eth.Peer) {
		handler.handler.runEthPeer(p, func(peer *eth.Peer) error {
			return eth.Handle((*ethHandler)(handler.handler), peer)
		})
	}(localEth)

	// Run the handshake locally to avoid spinning up a source handler
	var (
		head = handler.chain.CurrentBlock()
		td   = handler.chain.GetTd(head.Hash(), head.Number.Uint64())
	)
	time.Sleep(200 * time.Millisecond)
	if err := remoteEth.Handshake(1, handler.chain, eth.BlockRangeUpdatePacket{}, td, nil); err != nil {
		t.Fatalf("failed to run protocol handshake: %d", err)
	}
	// After the handshake completes, the source handler should stream the sink
	// the votes, subscribe to all inbound network events
	backend := new(testOizHandler)
	bcasts := make(chan []*types.VoteEnvelope)
	bcastSub := backend.voteBroadcasts.Subscribe(bcasts)
	defer bcastSub.Unsubscribe()

	go oiz.Handle(backend, remoteOiz)

	// Make sure we get all the votes on the correct channels
	seen := make(map[common.Hash]struct{})
	for len(seen) < len(insert) {
		votes := <-bcasts
		for _, vote := range votes {
			if _, ok := seen[vote.Hash()]; ok {
				t.Errorf("duplicate vote broadcast: %x", vote.Hash())
			}
			seen[vote.Hash()] = struct{}{}
		}
	}
	for _, vote := range insert {
		if _, ok := seen[vote.Hash()]; !ok {
			t.Errorf("missing vote: %x", vote.Hash())
		}
	}
}

func TestRecvVotes68(t *testing.T) { testRecvVotes(t, eth.ETH68) }

func testRecvVotes(t *testing.T, protocol uint) {
	t.Parallel()

	// Create a message handler and fill the pool with big votes
	handler := newTestHandler()
	defer handler.close()

	protos := []p2p.Protocol{
		{
			Name:    "eth",
			Version: eth.ETH68,
		},
		{
			Name:    "oiz",
			Version: oiz.Oiz1,
		},
	}
	caps := []p2p.Cap{
		{
			Name:    "eth",
			Version: eth.ETH68,
		},
		{
			Name:    "oiz",
			Version: oiz.Oiz1,
		},
	}

	// Create a source handler to send messages through and a sink peer to receive them
	p2pEthSrc, p2pEthSink := p2p.MsgPipe()
	defer p2pEthSrc.Close()
	defer p2pEthSink.Close()

	localEth := eth.NewPeer(protocol, p2p.NewPeerWithProtocols(enode.ID{1}, protos, "", caps), p2pEthSrc, nil)
	remoteEth := eth.NewPeer(protocol, p2p.NewPeerWithProtocols(enode.ID{2}, protos, "", caps), p2pEthSink, nil)
	defer localEth.Close()
	defer remoteEth.Close()

	p2pOizSrc, p2pOizSink := p2p.MsgPipe()
	defer p2pOizSrc.Close()
	defer p2pOizSink.Close()

	localOiz := oiz.NewPeer(oiz.Oiz1, p2p.NewPeerWithProtocols(enode.ID{1}, protos, "", caps), p2pOizSrc)
	remoteOiz := oiz.NewPeer(oiz.Oiz1, p2p.NewPeerWithProtocols(enode.ID{3}, protos, "", caps), p2pOizSink)
	defer localOiz.Close()
	defer remoteOiz.Close()

	go func(p *oiz.Peer) {
		(*oizHandler)(handler.handler).RunPeer(p, func(peer *oiz.Peer) error {
			return oiz.Handle((*oizHandler)(handler.handler), peer)
		})
	}(localOiz)

	time.Sleep(200 * time.Millisecond)
	remoteOiz.Handshake()

	time.Sleep(200 * time.Millisecond)
	go func(p *eth.Peer) {
		handler.handler.runEthPeer(p, func(peer *eth.Peer) error {
			return eth.Handle((*ethHandler)(handler.handler), peer)
		})
	}(localEth)

	// Run the handshake locally to avoid spinning up a source handler
	var (
		head = handler.chain.CurrentBlock()
		td   = handler.chain.GetTd(head.Hash(), head.Number.Uint64())
	)
	time.Sleep(200 * time.Millisecond)
	if err := remoteEth.Handshake(1, handler.chain, eth.BlockRangeUpdatePacket{}, td, nil); err != nil {
		t.Fatalf("failed to run protocol handshake: %d", err)
	}

	votesCh := make(chan core.NewVoteEvent)
	sub := handler.votepool.SubscribeNewVoteEvent(votesCh)
	defer sub.Unsubscribe()
	// Send the vote to the sink and verify that it's added to the vote pool
	vote := types.VoteEnvelope{
		VoteAddress: types.BLSPublicKey{},
		Signature:   types.BLSSignature{},
		Data: &types.VoteData{
			SourceNumber: uint64(0),
			SourceHash:   common.BytesToHash(common.Hex2Bytes(string(rune(0)))),
			TargetNumber: uint64(1),
			TargetHash:   common.BytesToHash(common.Hex2Bytes(string(rune(1)))),
		},
	}

	remoteOiz.AsyncSendVotes([]*types.VoteEnvelope{&vote})
	time.Sleep(100 * time.Millisecond)
	select {
	case event := <-votesCh:
		if event.Vote.Hash() != vote.Hash() {
			t.Errorf("added wrong vote hash: got %v, want %v", event.Vote.Hash(), vote.Hash())
		}
	case <-time.After(2 * time.Second):
		t.Errorf("no NewVotesEvent received within 2 seconds")
	}
}
