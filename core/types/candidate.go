package types

import (
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"math/big"
)

type CanConditions map[discover.NodeID]*big.Int

type CandidateQueue []*Candidate

type KindCanQueue []CandidateQueue

func compare(cand CanConditions, c, can *Candidate) int {
	// put the larger deposit in front
	if cand[c.CandidateId].Cmp(cand[can.CandidateId]) > 0 /* c.Deposit.Cmp(can.Deposit) > 0*/ {
		return 1
	} else if cand[c.CandidateId].Cmp(cand[can.CandidateId]) == 0 /* c.Deposit.Cmp(can.Deposit) == 0 */ {
		// put the smaller blocknumber in front
		if c.BlockNumber.Cmp(can.BlockNumber) > 0 {
			return -1
		} else if c.BlockNumber.Cmp(can.BlockNumber) == 0 {
			// put the smaller tx'index in front
			if c.TxIndex > can.TxIndex {
				return -1
			} else if c.TxIndex == can.TxIndex {
				return 0
			} else {
				return 1
			}
		} else {
			return 1
		}
	} else {
		return -1
	}
}

// sorted candidates
func (arr CandidateQueue) CandidateSort(cand CanConditions) {
	if len(arr) <= 1 {
		return
	}
	arr.quickSort(cand, 0, len(arr)-1)
}
func (arr CandidateQueue) quickSort(cand CanConditions, left, right int) {
	if left < right {
		pivot := arr.partition(cand, left, right)
		arr.quickSort(cand, left, pivot-1)
		arr.quickSort(cand, pivot+1, right)
	}
}
func (arr CandidateQueue) partition(cand CanConditions, left, right int) int {
	for left < right {
		for left < right && compare(cand, arr[left], arr[right]) >= 0 {
			right--
		}
		if left < right {
			arr[left], arr[right] = arr[right], arr[left]
			left++
		}
		for left < right && compare(cand, arr[left], arr[right]) >= 0 {
			left++
		}
		if left < right {
			arr[left], arr[right] = arr[right], arr[left]
			right--
		}
	}
	return left
}

// candiate info
type Candidate struct {
	// Mortgage amount (margin)
	Deposit *big.Int
	// Current block height number at the time of the mortgage
	BlockNumber *big.Int
	// Current tx'index at the time of the mortgage
	TxIndex uint32
	// candidate's server nodeId
	CandidateId discover.NodeID
	Host        string
	Port        string
	// Mortgage beneficiary's account address
	Owner common.Address
	// The account address of initiating a mortgaged
	From  common.Address
	Extra string
	// brokerage   example: (fee/10000) * 100% == x%
	Fee uint64
	// Selected TicketId
	TicketId common.Hash
}

type CandidateAttach struct {
	// Sum Ticket age
	Epoch *big.Int `json:"epoch"`
}

func (ca *CandidateAttach) AddEpoch(number *big.Int) {
	ca.Epoch.Add(ca.Epoch, number)
}

func (ca *CandidateAttach) SubEpoch(number *big.Int) {
	if ca.Epoch.Cmp(number) >= 0 && number.Uint64() > 0 {
		ca.Epoch.Sub(ca.Epoch, number)
	}
}
