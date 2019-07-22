package cbft

import (
	"fmt"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/math"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/executor"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/protocols"
	ctypes "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/types"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
)

// Perform security rule verification，store in blockTree, Whether to start synchronization
func (cbft *Cbft) OnPrepareBlock(id string, msg *protocols.PrepareBlock) error {
	if err := cbft.safetyRules.PrepareBlockRules(msg); err != nil {
		if err.Fetch() {
			cbft.fetchBlock(id, msg.Block.Hash(), msg.Block.NumberU64())
		} else if err.NewView() {
			_, hash, _ := msg.ViewChangeQC.MaxBlock()
			cbft.changeView(msg.Epoch, msg.ViewNumber, cbft.blockTree.FindBlockByHash(hash), msg.ViewChangeQC)
		} else {
			return nil
		}
	}

	cbft.state.AddPrepareBlock(msg)

	cbft.prepareBlockFetchRules(id, msg)

	cbft.findExecutableBlock()
	return nil
}

// Perform security rule verification，store in blockTree, Whether to start synchronization
func (cbft *Cbft) OnPrepareVote(id string, msg *protocols.PrepareVote) error {
	if err := cbft.safetyRules.PrepareVoteRules(msg); err != nil {
		if err.Fetch() {
			cbft.fetchBlock(id, msg.BlockHash, msg.BlockNumber)
		} else {
			return err
		}
	}

	cbft.prepareVoteFetchRules(id, msg)

	node, _ := cbft.validatorPool.GetValidatorByNodeID(cbft.state.HighestQCBlock().NumberU64(), cbft.config.Sys.NodeID)

	cbft.state.AddPrepareVote(uint32(node.Index), msg)

	cbft.findQCBlock()
	return nil
}

// Perform security rule verification, view switching
func (cbft *Cbft) OnViewChange(id string, msg *protocols.ViewChange) error {
	if err := cbft.safetyRules.ViewChangeRules(msg); err != nil {
		if err.Fetch() {
			cbft.fetchBlock(id, msg.BlockHash, msg.BlockNumber)
		}
	}

	node, _ := cbft.validatorPool.GetValidatorByNodeID(cbft.state.HighestQCBlock().NumberU64(), cbft.config.Sys.NodeID)

	cbft.state.AddViewChange(uint32(node.Index), msg)

	// It is possible to achieve viewchangeQC every time you add viewchange
	cbft.tryChangeView()
	return nil
}

func (cbft *Cbft) OnViewTimeout() {
	viewChange := &protocols.ViewChange{
		Epoch:       cbft.state.Epoch(),
		ViewNumber:  cbft.state.ViewNumber(),
		BlockHash:   cbft.state.HighestQCBlock().Hash(),
		BlockNumber: cbft.state.HighestQCBlock().NumberU64(),
	}

	cbft.network.Broadcast(viewChange)
}

//Perform security rule verification, view switching
func (cbft *Cbft) OnInsertQCBlock(blocks []*types.Block, qcs []*ctypes.QuorumCert) error {
	//todo insert tree, update view
	return nil
}

// Asynchronous execution block callback function
func (cbft *Cbft) onAsyncExecuteStatus(s *executor.BlockExecuteStatus) {
	index, finish := cbft.state.Executing()
	if !finish {
		block := cbft.state.ViewBlockByIndex(index)
		if block != nil {
			if block.Hash() == s.Hash {
				cbft.state.SetExecuting(index, true)
				cbft.signBlock(block.Hash(), block.NumberU64(), index)
			}
		}
	}
	cbft.findExecutableBlock()
}

// Sign the block that has been executed
// Every time try to trigger a send PrepareVote
func (cbft *Cbft) signBlock(hash common.Hash, number uint64, index uint32) {
	// todo sign vote
	// parentQC added when sending
	prepareVote := &protocols.PrepareVote{
		Epoch:       cbft.state.Epoch(),
		ViewNumber:  cbft.state.ViewNumber(),
		BlockHash:   hash,
		BlockNumber: number,
		BlockIndex:  index,
	}

	cbft.state.PendingPrepareVote().Push(prepareVote)

	cbft.trySendPrepareVote()
}

// Send a signature,
// obtain a signature from the pending queue,
// determine whether the parent block has reached QC,
// and send a signature if it is reached, otherwise exit the sending logic.
func (cbft *Cbft) trySendPrepareVote() {
	pending := cbft.state.PendingPrepareVote()
	hadSend := cbft.state.HadSendPrepareVote()

	for !pending.Empty() {
		p := pending.Top()
		if err := cbft.voteRules.AllowVote(p); err != nil {
			break
		}

		block := cbft.state.ViewBlockByIndex(p.BlockIndex)
		if b, qc := cbft.blockTree.FindBlockAndQC(block.ParentHash(), block.NumberU64()-1); b != nil {
			p.ParentQC = qc
			hadSend.Push(p)
			node, _ := cbft.validatorPool.GetValidatorByNodeID(cbft.state.HighestQCBlock().NumberU64(), cbft.config.Sys.NodeID)
			cbft.state.AddPrepareVote(uint32(node.Index), p)
			pending.Pop()
			cbft.network.Broadcast(p)
		} else {
			break
		}
	}
}

// Every time there is a new block or a new executed block result will enter this judgment, find the next executable block
func (cbft *Cbft) findExecutableBlock() {
	blockIndex, finish := cbft.state.Executing()

	if blockIndex == math.MaxUint32 {
		block := cbft.state.ViewBlockByIndex(blockIndex + 1)
		if block != nil {
			parent, _ := cbft.blockTree.FindBlockAndQC(block.ParentHash(), block.NumberU64()-1)
			if parent == nil {
				cbft.log.Error(fmt.Sprintf("Find executable block's parent failed :[%d,%d,%s]", blockIndex, block.NumberU64(), block.Hash()))
			}

			if err := cbft.asyncExecutor.Execute(block, parent); err != nil {
				cbft.log.Error("Async Execute block failed", "error", err)
			}
			cbft.state.SetExecuting(0, false)
		}
	}

	if finish {
		block := cbft.state.ViewBlockByIndex(blockIndex + 1)
		if block != nil {
			parent := cbft.state.ViewBlockByIndex(blockIndex)
			if parent == nil {
				cbft.log.Error(fmt.Sprintf("Find executable block's parent failed :[%d,%d,%s]", blockIndex, block.NumberU64(), block.Hash()))
				return
			}

			if err := cbft.asyncExecutor.Execute(block, parent); err != nil {
				cbft.log.Error("Async Execute block failed", "error", err)
			}
		}
		cbft.state.SetExecuting(blockIndex+1, false)
	}
}

// Each time a new vote is triggered, a new QC Block will be triggered, and a new one can be found by the commit block.
func (cbft *Cbft) findQCBlock() {
	index := cbft.state.MaxQCIndex()
	next := index + 1
	size := cbft.state.PrepareVoteLenByIndex(next)

	prepareQC := func() bool {
		return size > 17 && cbft.state.HadSendPrepareVote().Had(next)
	}

	if prepareQC() {
		block := cbft.state.ViewBlockByIndex(next)
		qc := cbft.generatePrepareQC(cbft.state.AllPrepareVoteByIndex(next))
		cbft.state.AddQC(qc)
		lock, commit := cbft.blockTree.InsertQCBlock(block, qc)
		cbft.state.SetHighestQCBlock(block)
		cbft.tryCommitNewBlock(lock, commit)
	}
	cbft.tryChangeView()
}

func (cbft *Cbft) generatePrepareQC(map[uint32]*protocols.PrepareVote) *ctypes.QuorumCert {
	qc := &ctypes.QuorumCert{}
	return qc
}

func (cbft *Cbft) generateViewChangeQC(map[uint32]*protocols.ViewChange) *ctypes.ViewChangeQC {
	qc := &ctypes.ViewChangeQC{}
	return qc
}

// Try commit a new block
func (cbft *Cbft) tryCommitNewBlock(lock *types.Block, commit *types.Block) {
	_, oldCommit := cbft.state.HighestLockBlock(), cbft.state.HighestCommitBlock()

	// Incremental commit block
	if oldCommit.NumberU64()+1 == commit.NumberU64() {
		_, qc := cbft.blockTree.FindBlockAndQC(commit.Hash(), commit.NumberU64())
		cbft.commitBlock(commit, qc)
	}
	cbft.state.SetHighestLockBlock(lock)
	cbft.state.SetHighestCommitBlock(commit)

}

// According to the current view QC situation, try to switch view
func (cbft *Cbft) tryChangeView() {
	// Had receive all qcs of current view
	if cbft.state.MaxQCIndex()+1 == cbft.config.Sys.Amount {
		cbft.changeView(cbft.state.Epoch(), cbft.state.ViewNumber()+1, cbft.state.HighestQCBlock(), nil)
	}

	viewChangeQC := func() bool {
		if cbft.state.ViewChangeLen() > cbft.validatorPool.Len(cbft.state.HighestQCBlock().NumberU64()) {
			return true
		}
		return false
	}
	if viewChangeQC() {
		qc := cbft.generateViewChangeQC(cbft.state.AllViewChange())
		cbft.changeView(cbft.state.Epoch(), cbft.state.ViewNumber()+1, cbft.state.HighestQCBlock(), qc)
	}
}

// change view
func (cbft *Cbft) changeView(epoch, viewNumber uint64, block *types.Block, qc *ctypes.ViewChangeQC) {
	cbft.state.ResetView(epoch, viewNumber)
	cbft.state.SetViewChangeQC(qc)
	cbft.clearInvalidBlocks(block)
}

// Clean up invalid blocks in the previous view
func (cbft *Cbft) clearInvalidBlocks(newBlock *types.Block) {
	var rollback []*types.Block
	newHead := newBlock.Header()
	for _, p := range cbft.state.HadSendPrepareVote().Peek() {
		if p.BlockNumber > newBlock.NumberU64() {
			block := cbft.state.ViewBlockByIndex(p.BlockIndex)
			rollback = append(rollback, block)
			cbft.blockCacheWriter.ClearCache(block)
		}
	}
	for _, p := range cbft.state.PendingPrepareVote().Peek() {
		if p.BlockNumber > newBlock.NumberU64() {
			block := cbft.state.ViewBlockByIndex(p.BlockIndex)
			rollback = append(rollback, block)
			cbft.blockCacheWriter.ClearCache(block)
		}
	}

	//todo proposer is myself
	cbft.txPool.ForkedReset(newHead, rollback)
}

// signFn use private key to sign byte slice.
func (cbft *Cbft) signFn(m []byte) ([]byte, error) {
	// TODO: really signature
	return []byte{}, nil
}

// signMsg use private key to sign msg.
func (cbft *Cbft) signMsg(msg ctypes.ConsensusMsg) error {
	buf, err := msg.CannibalizeBytes()
	if err != nil {
		return err
	}
	sign, err := cbft.signFn(buf)
	if err != nil {
		return err
	}
	msg.SetSign(sign)
	return nil
}
