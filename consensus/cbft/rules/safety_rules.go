package rules

import (
	"fmt"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/protocols"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/state"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/types"
	"time"
)

type SafetyError interface {
	error
	Fetch() bool   //Is the error need fetch
	NewView() bool //need change view
}

type safetyError struct {
	text    string
	fetch   bool
	newView bool
}

func (s safetyError) Error() string {
	return s.text
}

func (s safetyError) Fetch() bool {
	return s.fetch
}
func (s safetyError) NewView() bool {
	return s.newView
}

//func newSafetyError(text string, fetch, newView bool) SafetyError {
//	return &safetyError{
//		text:    text,
//		fetch:   fetch,
//		newView: newView,
//	}
//}

func newFetchError(text string) SafetyError {
	return &safetyError{
		text:    text,
		fetch:   true,
		newView: false,
	}
}
func newViewError(text string) SafetyError {
	return &safetyError{
		text:    text,
		fetch:   false,
		newView: true,
	}
}

func newError(text string) SafetyError {
	return &safetyError{
		text:    text,
		fetch:   false,
		newView: false,
	}
}

type SafetyRules interface {
	// Security rules for proposed blocks
	PrepareBlockRules(block *protocols.PrepareBlock) SafetyError

	// Security rules for proposed votes
	PrepareVoteRules(vote *protocols.PrepareVote) SafetyError

	// Security rules for viewChange
	ViewChangeRules(vote *protocols.ViewChange) SafetyError
}

type baseSafetyRules struct {
	viewState *state.ViewState
	blockTree *types.BlockTree
}

// PrepareBlock rules
// 1.Less than local viewNumber drop
// 2.Synchronization greater than local viewNumber
// 3.Lost more than the time window
func (r *baseSafetyRules) PrepareBlockRules(block *protocols.PrepareBlock) SafetyError {
	if r.viewState.Epoch() != block.Epoch {
		return r.changeEpochBlockRules(block)
	}
	if r.viewState.ViewNumber() > block.ViewNumber {
		return newError(fmt.Sprintf("viewNumber too low(local:%d, msg:%d)", r.viewState.ViewNumber(), block.ViewNumber))
	}

	if r.viewState.ViewNumber() < block.ViewNumber {
		isQCChild := func() bool {
			return block.Block.NumberU64() == r.viewState.HighestQCBlock().NumberU64()+1 &&
				r.blockTree.FindBlockByHash(block.Block.ParentHash()) != nil
		}
		isLockChild := func() bool {
			return block.Block.ParentHash() == r.viewState.HighestLockBlock().Hash()
		}
		isFirstBlock := func() bool {
			return block.BlockIndex == 0
		}
		isNextView := func() bool {
			return r.viewState.ViewNumber()+1 == block.ViewNumber
		}
		if isNextView() && isFirstBlock() && (isQCChild() || isLockChild()) {
			return newViewError("need change view")
		}

		return newFetchError(fmt.Sprintf("viewNumber higher then local(local:%d, msg:%d)", r.viewState.ViewNumber(), block.ViewNumber))
	}

	if r.viewState.IsDeadline() {
		return newError(fmt.Sprintf("view's deadline is expire(over:%d)", time.Since(r.viewState.Deadline())))
	}
	return nil
}

func (r *baseSafetyRules) changeEpochBlockRules(block *protocols.PrepareBlock) SafetyError {
	if r.viewState.Epoch() > block.Epoch {
		return newError(fmt.Sprintf("epoch too low(local:%d, msg:%d)", r.viewState.Epoch(), block.Epoch))
	}
	if block.Block.ParentHash() != r.viewState.HighestQCBlock().Hash() {
		return newFetchError(fmt.Sprintf("epoch higher then local(local:%d, msg:%d)", r.viewState.Epoch(), block.Epoch))
	}
	return newViewError("new epoch, need change view")
}

// PrepareVote rules
// 1.Less than local viewNumber drop
// 2.Synchronization greater than local viewNumber
// 3.Lost more than the time window
func (r *baseSafetyRules) PrepareVoteRules(vote *protocols.PrepareVote) SafetyError {
	if r.viewState.Epoch() != vote.Epoch {
		return r.changeEpochVoteRules(vote)
	}
	if r.viewState.ViewNumber() > vote.ViewNumber {
		return newError(fmt.Sprintf("viewNumber too low(local:%d, msg:%d)", r.viewState.ViewNumber(), vote.ViewNumber))
	}

	if r.viewState.ViewNumber() < vote.ViewNumber {
		return newFetchError(fmt.Sprintf("viewNumber higher then local(local:%d, msg:%d)", r.viewState.ViewNumber(), vote.ViewNumber))
	}

	if r.viewState.IsDeadline() {
		return newError(fmt.Sprintf("view's deadline is expire(over:%d)", time.Since(r.viewState.Deadline())))
	}
	return nil
}

func (r *baseSafetyRules) changeEpochVoteRules(vote *protocols.PrepareVote) SafetyError {
	if r.viewState.Epoch() > vote.Epoch {
		return newError(fmt.Sprintf("epoch too low(local:%d, msg:%d)", r.viewState.Epoch(), vote.Epoch))
	}

	return newFetchError("new epoch, need fetch blocks")
}

// ViewChange rules
// 1.Less than local viewNumber drop
// 2.Synchronization greater than local viewNumber
func (r *baseSafetyRules) ViewChangeRules(viewChange *protocols.ViewChange) SafetyError {

	if r.viewState.Epoch() != viewChange.Epoch {
		return r.changeEpochViewChangeRules(viewChange)
	}
	if r.viewState.ViewNumber() > viewChange.ViewNumber {
		return newError(fmt.Sprintf("viewNumber too low(local:%d, msg:%d)", r.viewState.ViewNumber(), viewChange.ViewNumber))
	}

	if r.viewState.ViewNumber() < viewChange.ViewNumber {
		return newFetchError(fmt.Sprintf("viewNumber higher then local(local:%d, msg:%d)", r.viewState.ViewNumber(), viewChange.ViewNumber))
	}
	return nil
}

func (r *baseSafetyRules) changeEpochViewChangeRules(viewChange *protocols.ViewChange) SafetyError {
	if r.viewState.Epoch() > viewChange.Epoch {
		return newError(fmt.Sprintf("epoch too low(local:%d, msg:%d)", r.viewState.Epoch(), viewChange.Epoch))
	}

	return newFetchError("new epoch, need fetch blocks")
}

func NewSafetyRules(viewState *state.ViewState, blockTree *types.BlockTree) SafetyRules {
	return &baseSafetyRules{
		viewState: viewState,
		blockTree: blockTree,
	}
}
