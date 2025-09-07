package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/oiz"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// oizHandler implements the oiz.Backend interface to handle the various network
// packets that are sent as broadcasts.
type oizHandler handler

func (h *oizHandler) Chain() *core.BlockChain { return h.chain }

// RunPeer is invoked when a peer joins on the `oiz` protocol.
func (h *oizHandler) RunPeer(peer *oiz.Peer, hand oiz.Handler) error {
	if err := peer.Handshake(); err != nil {
		// ensure that waitOizExtension receives the exit signal normally
		// otherwise, can't graceful shutdown
		ps := h.peers
		id := peer.ID()

		// Ensure nobody can double connect
		ps.lock.Lock()
		if wait, ok := ps.oizWait[id]; ok {
			delete(ps.oizWait, id)
			peer.Log().Error("Oiz extension Handshake failed", "err", err)
			wait <- nil
		}
		ps.lock.Unlock()
		return err
	}
	return (*handler)(h).runOizExtension(peer, hand)
}

// PeerInfo retrieves all known `bsc` information about a peer.
func (h *oizHandler) PeerInfo(id enode.ID) interface{} {
	if p := h.peers.peer(id.String()); p != nil && p.oizExt != nil {
		return p.oizExt.info()
	}
	return nil
}

// Handle is invoked from a peer's message handler when it receives a new remote
// message that the handler couldn't consume and serve itself.
func (h *oizHandler) Handle(peer *oiz.Peer, packet oiz.Packet) error {
	// DeliverSnapPacket is invoked from a peer's message handler when it transmits a
	// data packet for the local node to consume.
	switch packet := packet.(type) {
	case *oiz.VotesPacket:
		return h.handleVotesBroadcast(peer, packet.Votes)

	default:
		return fmt.Errorf("unexpected oiz packet type: %T", packet)
	}
}

// handleVotesBroadcast is invoked from a peer's message handler when it transmits a
// votes broadcast for the local node to process.
func (h *oizHandler) handleVotesBroadcast(peer *oiz.Peer, votes []*types.VoteEnvelope) error {
	if peer.IsOverLimitAfterReceiving() {
		return nil
	}
	// Here we only put the first vote, to avoid ddos attack by sending a large batch of votes.
	// This won't abandon any valid vote, because one vote is sent every time referring to func voteBroadcastLoop
	if len(votes) > 0 {
		h.votepool.PutVote(votes[0])
	}

	return nil
}
