package protocols

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/PlatONnetwork/PlatON-Go/common"
	ctypes "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/types"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/utils"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
)

// Maximum cap on the size of a cbft protocol message
const CbftProtocolMaxMsgSize = 10 * 1024 * 1024

const (
	CBFTStatusMsg        = 0x00 // Protocol messages belonging to cbft
	PrepareBlockMsg      = 0x01
	PrepareVoteMsg       = 0x02
	ViewChangeMsg        = 0x03
	GetPrepareBlockMsg   = 0x04
	GetQuorumCertMsg     = 0x05
	QuorumCertMsg        = 0x06
	GetQCPrepareBlockMsg = 0x07
	QCPrepareBlockMsg    = 0x08
	GetPrepareVoteMsg    = 0x09
	PrepareBlockHashMsg  = 0x0a
	PrepareVotesMsg      = 0x0b
	QCBlockListMsg       = 0x0c
	GetLatestStatusMsg   = 0x0d
	LatestStatusMsg      = 0x0e
	PingMsg              = 0x0f
	PongMsg              = 0x10
)

// A is used to convert specific message types according to the message body.
// The program is forcibly terminated if there is an unmatched message type and
// all types must exist in the match list.
func MessageType(msg interface{}) uint64 {
	// todo: need to process depending on mmessageType.
	switch msg.(type) {
	case *CbftStatusData:
		return CBFTStatusMsg
	case *PrepareBlock:
		return PrepareBlockMsg
	case *PrepareVote:
		return PrepareVoteMsg
	case *ViewChange:
		return ViewChangeMsg
	case *GetPrepareBlock:
		return GetPrepareBlockMsg
	case *GetQuorumCert:
		return GetQuorumCertMsg
	case *QuorumCert:
		return QuorumCertMsg
	case *GetQCPrepareBlock:
		return GetQCPrepareBlockMsg
	case *GetPrepareVote:
		return GetPrepareVoteMsg
	case *PrepareBlockHash:
		return PrepareBlockHashMsg
	case *PrepareVotes:
		return PrepareVotesMsg
	case *QCBlockList:
		return QCBlockListMsg
	case *GetLatestStatus:
		return GetLatestStatusMsg
	case *LatestStatus:
		return LatestStatusMsg
	case *Ping:
		return PingMsg
	case *Pong:
		return PongMsg
	default:
	}
	panic(fmt.Sprintf("unknown message type [%v]", reflect.TypeOf(msg)))
}

// Proposed block carrier.
type PrepareBlock struct {
	Epoch         uint64               `json:"epoch"`
	ViewNumber    uint64               `json:"view_number"`
	Block         *types.Block         `json:"block_hash"`
	BlockIndex    uint32               `json:"block_index"`      // The block number of the current ViewNumber proposal, 0....10
	ProposalIndex uint32               `json:"proposal_index"`   // Proposer index
	ProposalAddr  common.Address       `json:"proposal_address"` // Proposer address
	PrepareQC     *ctypes.QuorumCert   `json:"prepare_qc"`       // N-f aggregate signature
	ViewChangeQC  []*ctypes.QuorumCert `json:"viewchange_qc"`    // viewChange aggregate signature
	Signature     ctypes.Signature     `json:"signature"`
}

func (s *PrepareBlock) String() string {
	return fmt.Sprintf("[ViewNumber: %d] - [Hash: %s] - [Number: %d] - [BlockIndex: %d]"+
		"- [ProposalIndex: %d] - [ProposalAddr: %s]",
		s.ViewNumber, s.Block.Hash(), s.Block.NumberU64(), s.BlockIndex, s.ProposalIndex, s.ProposalAddr)
}

func (s *PrepareBlock) MsgHash() common.Hash {
	return utils.BuildHash(PrepareBlockMsg,
		utils.MergeBytes(common.Uint64ToBytes(s.ViewNumber), s.Block.Hash().Bytes(), s.Signature.Bytes()))
}

func (s *PrepareBlock) BHash() common.Hash {
	return s.Block.Hash()
}

func (s *PrepareBlock) CannibalizeBytes() ([]byte, error) {
	buf, err := rlp.EncodeToBytes([]interface{}{
		s.Epoch,
		s.ViewNumber,
		s.BlockIndex,
		s.ProposalIndex,
		s.ProposalAddr,
	})
	if err != nil {
		return nil, err
	}
	return crypto.Keccak256(buf), nil
}

func (pb *PrepareBlock) Sign() []byte {
	return pb.Signature.Bytes()
}

// Removed the validator address, index. Mainly to ensure that the signature hash of the aggregate signature is consistent
type PrepareVote struct {
	Epoch       uint64             `json:"epoch"`
	ViewNumber  uint64             `json:"view_number"`
	BlockHash   common.Hash        `json:"block_hash"`
	BlockNumber uint64             `json:"block_number"`
	BlockIndex  uint32             `json:"block_index"` // The block number of the current ViewNumber proposal, 0....10
	ParentQC    *ctypes.QuorumCert `json:"parent_qc"`
	Signature   ctypes.Signature   `json:"signature"`
}

func (s *PrepareVote) String() string {
	return fmt.Sprintf("[Epoch: %d] - [VN: %d] - [BlockHash: %s] - [BlockNumber: %d] - "+
		"[BlockIndex: %d]",
		s.Epoch, s.ViewNumber, s.BlockHash, s.BlockNumber, s.BlockIndex)
}

func (s *PrepareVote) MsgHash() common.Hash {
	return utils.BuildHash(PrepareVoteMsg,
		utils.MergeBytes(common.Uint64ToBytes(s.ViewNumber), s.BlockHash.Bytes(), common.Uint32ToBytes(s.BlockIndex), s.Signature.Bytes()))
}

func (s *PrepareVote) BHash() common.Hash {
	return s.BlockHash
}

// Message structure for view switching.
type ViewChange struct {
	Epoch       uint64             `json:"epoch"`
	ViewNumber  uint64             `json:"view_number"`
	BlockHash   common.Hash        `json:"block_hash"`
	BlockNumber uint64             `json:"block_number"`
	PrepareQC   *ctypes.QuorumCert `json:"prepare_qc"`
	Signature   ctypes.Signature   `json:"signature"`
}

func (s *ViewChange) String() string {
	return fmt.Sprintf("[Epoch: %d] - [Vn: %d] - [BlockHash: %s] - [BlockNumber: %d]",
		s.Epoch, s.ViewNumber, s.BlockHash, s.BlockNumber)
}

func (s *ViewChange) MsgHash() common.Hash {
	return utils.BuildHash(ViewChangeMsg, utils.MergeBytes(common.Uint64ToBytes(s.ViewNumber),
		s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber)))
}

func (s *ViewChange) BHash() common.Hash {
	return s.BlockHash
}

// cbftStatusData implement Message and including status information about peer.
type CbftStatusData struct {
	ProtocolVersion uint32      `json:"protocol_version"` // CBFT protocol version number.
	QCBn            *big.Int    `json:"qc_bn"`            // The highest local block number for collecting block signatures.
	QCBlock         common.Hash `json:"qc_block"`         // The highest local block hash for collecting block signatures.
	LockBn          *big.Int    `json:"lock_bn"`          // Locally locked block number.
	LockBlock       common.Hash `json:"lock_block"`       // Locally locked block hash.
	CmtBn           *big.Int    `json:"cmt_bn"`           // Locally submitted block number.
	CmtBlock        common.Hash `json:"cmt_block"`        // Locally submitted block hash.
}

func (s *CbftStatusData) String() string {
	return fmt.Sprintf("[ProtocolVersion:%d] - [QCBn:%d] - [LockBn:%d] - [CmtBn:%d]",
		s.ProtocolVersion, s.QCBn.Uint64(), s.LockBn.Uint64(), s.CmtBn.Uint64())
}

func (s *CbftStatusData) MsgHash() common.Hash {
	return utils.BuildHash(CBFTStatusMsg, utils.MergeBytes(s.QCBlock.Bytes(),
		s.LockBlock.Bytes(), s.CmtBlock.Bytes()))
}

func (s *CbftStatusData) BHash() common.Hash {
	return s.QCBlock
}

// CBFT protocol message - used to get the
// proposed block information.
type GetPrepareBlock struct {
	BlockHash   common.Hash `json:"hash"`   // The hash of the block to be acquired
	BlockNumber uint64      `json:"number"` // The number of the block to be acquired
}

func (s *GetPrepareBlock) String() string {
	return fmt.Sprintf("[Hash: %s] - [Number: %d]", s.BlockHash, s.BlockNumber)
}

func (s *GetPrepareBlock) MsgHash() common.Hash {
	return utils.BuildHash(GetPrepareBlockMsg, utils.MergeBytes(s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber)))
}

func (s *GetPrepareBlock) BHash() common.Hash {
	return s.BlockHash
}

// Protocol message for obtaining an aggregated signature.
// todo: Need to determine the attribute field - ParentQC.
type GetQuorumCert struct {
	BlockHash   common.Hash `json:"block_hash"`   // The hash of the block to be acquired.
	BlockNumber uint64      `json:"block_number"` // The number of the block to be acquired.
	ParentQC    *QuorumCert `json:"parent_qc"`    // The aggregated signature of the parent block of the block to be acquired.
}

func (s *GetQuorumCert) String() string {
	return fmt.Sprintf("[Hash: %s] - [Number: %d]", s.BlockHash, s.BlockNumber)
}

func (s *GetQuorumCert) MsgHash() common.Hash {
	return utils.BuildHash(GetQuorumCertMsg, utils.MergeBytes(s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber)))
}

func (s *GetQuorumCert) BHash() common.Hash {
	return s.BlockHash
}

// Aggregate signature response message, representing
// aggregated signature information for a block.
type QuorumCert struct {
	ViewNumber  uint64      `json:"view_number"`  // The view number corresponding to the block.
	BlockHash   common.Hash `json:"block_hash"`   // The hash corresponding to the block.
	BlockNumber uint64      `json:"block_number"` // The number corresponding to the block.
	Signature   []byte      `json:"signature"`    // The aggregate signature corresponding to the block.
}

func (s *QuorumCert) String() string {
	return fmt.Sprintf("[ViewNumber: %d] - [Hash: %s] - [Number: %d] - [Sig: %s]",
		s.ViewNumber, s.BlockHash, s.BlockNumber, common.BytesToHash(s.Signature))
}

func (s *QuorumCert) MsgHash() common.Hash {
	return utils.BuildHash(QuorumCertMsg, utils.MergeBytes(
		s.BlockHash.Bytes(),
		common.Uint64ToBytes(s.BlockNumber), s.Signature))
}

func (s *QuorumCert) BHash() common.Hash {
	return s.BlockHash
}

// Used to get block information that has reached QC.
// todo: need confirm.
type GetQCPrepareBlock struct {
	BlockNumber uint64      `json:"block_number"` // The number corresponding to the block.
	ParentQC    *QuorumCert `json:"parent_qc"`    // QC information of the parent block of the block to be acquired.
}

func (s *GetQCPrepareBlock) String() string {
	return fmt.Sprintf("[Number: %d]", s.BlockNumber)
}

func (s *GetQCPrepareBlock) MsgHash() common.Hash {
	return utils.BuildHash(GetQCPrepareBlockMsg, utils.MergeBytes(
		common.Uint64ToBytes(s.BlockNumber), s.ParentQC.Signature))
}

func (s *GetQCPrepareBlock) BHash() common.Hash {
	return common.Hash{}
}

// Block information that satisfies QC.
type QCPrepareBlock struct {
	Block     *types.Block `json:"block"`      // block information.
	PrepareQC *QuorumCert  `json:"prepare_qc"` // the aggregation signature of block.
}

func (s *QCPrepareBlock) String() string {
	return fmt.Sprintf("[Hash: %s] - [Number: %d] - [ViewNumber: %d]", s.Block.Hash(), s.Block.NumberU64(), s.PrepareQC.ViewNumber)
}

func (s *QCPrepareBlock) MsgHash() common.Hash {
	return utils.BuildHash(QCPrepareBlockMsg, utils.MergeBytes(
		s.Block.Hash().Bytes(),
		common.Uint64ToBytes(s.Block.NumberU64()), s.PrepareQC.Signature))
}

func (s *QCPrepareBlock) BHash() common.Hash {
	return s.Block.Hash()
}

// Message used to get block voting.
type GetPrepareVote struct {
	BlockHash   common.Hash
	BlockNumber uint64
	VoteBits    *utils.BitArray
}

func (s *GetPrepareVote) String() string {
	return fmt.Sprintf("[Hash: %s] - [Number: %d] - [VoteBits: %s]", s.BlockHash, s.BlockNumber, s.VoteBits.String())
}

func (s *GetPrepareVote) MsgHash() common.Hash {
	return utils.BuildHash(GetPrepareVoteMsg, utils.MergeBytes(
		s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber),
		s.VoteBits.Bytes()))
}

func (s *GetPrepareVote) BHash() common.Hash {
	return s.BlockHash
}

// Message used to respond to the number of block votes.
type PrepareVotes struct {
	BlockHash   common.Hash
	BlockNumber uint64
	Votes       []*PrepareVote // Block voting set.
}

func (s *PrepareVotes) String() string {
	return fmt.Sprintf("[Hash:%s] - [Number:%d] - [Votes:%d]", s.BlockHash.String(), s.BlockNumber, len(s.Votes))
}

func (s *PrepareVotes) MsgHash() common.Hash {
	return utils.BuildHash(PrepareVotesMsg, utils.MergeBytes(s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber)))
}

func (s *PrepareVotes) BHash() common.Hash {
	return s.BlockHash
}

// Represents the hash of the proposed block for secondary propagation.
type PrepareBlockHash struct {
	BlockHash   common.Hash
	BlockNumber uint64
}

func (s *PrepareBlockHash) String() string {
	return fmt.Sprintf("[Hash: %s] - [Number: %d]", s.BlockHash, s.BlockNumber)
}

func (s *PrepareBlockHash) MsgHash() common.Hash {
	return utils.BuildHash(PrepareBlockHashMsg, utils.MergeBytes(s.BlockHash.Bytes(), common.Uint64ToBytes(s.BlockNumber)))
}

func (s *PrepareBlockHash) BHash() common.Hash {
	return s.BlockHash
}

// For time detection.
type Ping [1]string

func (s *Ping) String() string {
	return fmt.Sprintf("[pingTime: %s]", s[0])
}

func (s *Ping) MsgHash() common.Hash {
	return utils.BuildHash(PingMsg, utils.MergeBytes([]byte(s[0])))
}

func (s *Ping) BHash() common.Hash {
	return common.Hash{}
}

// Response to ping.
type Pong [1]string

func (s *Pong) String() string {
	return fmt.Sprintf("[pongTime: %s]", s[0])
}

func (s *Pong) MsgHash() common.Hash {
	return utils.BuildHash(PongMsg, utils.MergeBytes([]byte(s[0])))
}

func (s *Pong) BHash() common.Hash {
	return common.Hash{}
}

// CBFT synchronize blocks that have reached qc.
type QCBlockList struct {
	QC     []*ctypes.QuorumCert
	Blocks []*types.Block
}

func (s *QCBlockList) String() string {
	return fmt.Sprintf("[QC.Len: %d] - [Blocks.Len: %d]", len(s.QC), len(s.Blocks))
}

func (s *QCBlockList) MsgHash() common.Hash {
	if len(s.QC) != 0 {
		return utils.BuildHash(QCBlockListMsg, utils.MergeBytes(s.QC[0].BlockHash.Bytes(),
			s.QC[0].Signature.Bytes()))
	}
	if len(s.Blocks) != 0 {
		return utils.BuildHash(QCBlockListMsg, utils.MergeBytes(s.Blocks[0].Hash().Bytes(),
			s.Blocks[0].Number().Bytes()))
	}
	return common.Hash{}
}

func (s *QCBlockList) BHash() common.Hash {
	// No explicit hash value and return empty hash.
	return common.Hash{}
}

// State synchronization for nodes.
type GetLatestStatus struct {
	BlockNumber uint64 // Block height sent by the requester
	LogicType   uint64 // LogicType: 1 QCBn, 2 LockedBn, 3 CommitBn
}

func (s *GetLatestStatus) String() string {
	return fmt.Sprintf("[BlockNumber: %d] - [LogicType: %d]", s.BlockNumber, s.LogicType)
}

func (s *GetLatestStatus) MsgHash() common.Hash {
	return utils.BuildHash(GetLatestStatusMsg, utils.MergeBytes(common.Uint64ToBytes(s.BlockNumber), common.Uint64ToBytes(s.LogicType)))
}

func (s *GetLatestStatus) BHash() common.Hash {
	return common.Hash{}
}

// Response message to GetLatestStatus request.
type LatestStatus struct {
	BlockNumber uint64 // Block height sent by responder.
	LogicType   uint64 // LogicType: 1 QCBn, 2 LockedBn, 3 CommitBn
}

func (s *LatestStatus) String() string {
	return fmt.Sprintf("[BlockNumber: %d] - [LogicType: %d]", s.BlockNumber, s.LogicType)
}

func (s *LatestStatus) MsgHash() common.Hash {
	return utils.BuildHash(LatestStatusMsg, utils.MergeBytes(common.Uint64ToBytes(s.BlockNumber), common.Uint64ToBytes(s.LogicType)))
}

func (s *LatestStatus) BHash() common.Hash {
	return common.Hash{}
}
