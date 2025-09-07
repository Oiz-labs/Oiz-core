package oiz

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Constants to match up protocol versions and messages
const (
	Oiz1 = 1
	Oiz2 = 2
)

// ProtocolName is the official short name of the `bsc` protocol used during
// devp2p capability negotiation.
const ProtocolName = "oiz"

// ProtocolVersions are the supported versions of the `bsc` protocol (first
// is primary).
var ProtocolVersions = []uint{Oiz1, Oiz2}

// protocolLengths are the number of implemented message corresponding to
// different protocol versions.
var protocolLengths = map[uint]uint64{Oiz1: 2, Oiz2: 4}

// maxMessageSize is the maximum cap on the size of a protocol message.
const maxMessageSize = 10 * 1024 * 1024

const (
	OizCapMsg           = 0x00 // oiz capability msg used upon handshake
	VotesMsg            = 0x01
	GetBlocksByRangeMsg = 0x02 // it can request (StartBlockHeight-Count, StartBlockHeight] range blocks from remote peer
	BlocksByRangeMsg    = 0x03 // the replied blocks from remote peer
)

var defaultExtra = []byte{0x00}

var (
	errNoOizCapMsg             = errors.New("no oiz capability message")
	errMsgTooLarge             = errors.New("message too long")
	errDecode                  = errors.New("invalid message")
	errInvalidMsgCode          = errors.New("invalid message code")
	errProtocolVersionMismatch = errors.New("protocol version mismatch")
)

// Packet represents a p2p message in the `bsc` protocol.
type Packet interface {
	Name() string // Name returns a string corresponding to the message type.
	Kind() byte   // Kind returns the message type.
}

// OizCapPacket is the network packet for oiz capability message.
type OizCapPacket struct {
	ProtocolVersion uint
	Extra           rlp.RawValue // for extension
}

// VotesPacket is the network packet for votes record.
type VotesPacket struct {
	Votes []*types.VoteEnvelope
}

func (*OizCapPacket) Name() string { return "OizCap" }
func (*OizCapPacket) Kind() byte   { return OizCapMsg }

func (*VotesPacket) Name() string { return "Votes" }
func (*VotesPacket) Kind() byte   { return VotesMsg }

type GetBlocksByRangePacket struct {
	RequestId        uint64
	StartBlockHeight uint64      // The start block height expected to be obtained from
	StartBlockHash   common.Hash // The start block hash expected to be obtained from
	Count            uint64      // Get the number of blocks from the start
}

func (*GetBlocksByRangePacket) Name() string { return "GetBlocksByRange" }
func (*GetBlocksByRangePacket) Kind() byte   { return GetBlocksByRangeMsg }

// BlockData contains types.extblock + sidecars
type BlockData struct {
	Header      *types.Header
	Txs         []*types.Transaction
	Uncles      []*types.Header
	Withdrawals []*types.Withdrawal `rlp:"optional"`
	Sidecars    types.BlobSidecars  `rlp:"optional"`
}

// NewBlockData creates a new BlockData object from a block
func NewBlockData(block *types.Block) *BlockData {
	return &BlockData{
		Header:      block.Header(),
		Txs:         block.Transactions(),
		Uncles:      block.Uncles(),
		Withdrawals: block.Withdrawals(),
		Sidecars:    block.Sidecars(),
	}
}

type BlocksByRangePacket struct {
	RequestId uint64
	Blocks    []*BlockData
}

func (*BlocksByRangePacket) Name() string { return "BlocksByRange" }
func (*BlocksByRangePacket) Kind() byte   { return BlocksByRangeMsg }
