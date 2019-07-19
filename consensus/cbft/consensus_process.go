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
			//todo fetch block
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
			//todo fetch block
		}
	}

	cbft.prepareVoteFetchRules(id, msg)
	//todo parse pubkey as id
	cbft.state.AddPrepareVote(id, msg)

	cbft.findQCBlock()
	return nil
}

// Perform security rule verification, view switching
func (cbft *Cbft) OnViewChange(id string, msg *protocols.ViewChange) error {
	if err := cbft.safetyRules.ViewChangeRules(msg); err != nil {
		if err.Fetch() {
			cbft.fetchBlock(id, msg.BlockHash, msg.BlockNum)
		}
	}

	//todo parse pubkey as id
	cbft.state.AddViewChange("", msg)
	return nil
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

func (cbft *Cbft) signBlock(hash common.Hash, number uint64, index uint32) {
	//todo sign vote

	prepareVote := &protocols.PrepareVote{
		Epoch:       cbft.state.Epoch(),
		ViewNumber:  cbft.state.ViewNumber(),
		BlockHash:   hash,
		BlockNumber: number,
		BlockIndex:  index,
	}

	cbft.state.PendingPrepareVote().Push(prepareVote)

	cbft.sendPrepareVote()
}

// Send a signature,
// obtain a signature from the pending queue,
// determine whether the parent block has reached QC,
// and send a signature if it is reached, otherwise exit the sending logic.
func (cbft *Cbft) sendPrepareVote() {
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
			cbft.state.AddPrepareVote("", p)
			pending.Pop()
			//todo send prepareVote
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
		//todo generation qc
		var qc *ctypes.QuorumCert
		lock, commit := cbft.blockTree.InsertQCBlock(block, qc)
		cbft.state.SetHighestQCBlock(block)
		cbft.tryCommitNewBlock(lock, commit)
	}
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
	if cbft.state.MaxQCIndex() == cbft.config.sys.Amount {
		//todo change view
	}

	viewChangeQC := func() bool {
		if cbft.state.ViewChangeLen() > 17 {
			return true
		}
		return false
	}
	if viewChangeQC() {
		//todo change view
	}
}

// Clean up invalid blocks in the previous view
func (cbft *Cbft) clearInvalidBlocks() {
	//todo reset txpool
}
