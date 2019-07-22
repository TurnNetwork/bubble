package evidence

import (
	"math/big"
	"sort"

	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/protocols"
)

const (
	// IdentityLength is the expected length of the identity
	IdentityLength = 20
)

type NumberOrderPrepareBlock []*protocols.PrepareBlock
type NumberOrderPrepareVote []*protocols.PrepareVote
type NumberOrderViewChange []*protocols.ViewChange

type Identity [IdentityLength]byte

type PrepareBlockEvidence map[Identity]NumberOrderPrepareBlock
type PrepareVoteEvidence map[Identity]NumberOrderPrepareVote
type ViewChangeEvidence map[Identity]NumberOrderViewChange

// Bytes gets the string representation of the underlying identity.
func (id Identity) Bytes() []byte { return id[:] }

func (e PrepareBlockEvidence) Add(pb *protocols.PrepareBlock, id Identity) error {
	var l NumberOrderPrepareBlock

	if l = e[id]; l == nil {
		l = make(NumberOrderPrepareBlock, 0)
	}
	err := l.Add(pb)
	e[id] = l
	return err
}

func (e PrepareBlockEvidence) Clear(viewNumber uint64) {
	for k, v := range e {
		v.Remove(viewNumber)
		if v.Len() == 0 {
			delete(e, k)
		}
	}
}

func (opb *NumberOrderPrepareBlock) Add(pb *protocols.PrepareBlock) error {
	if ev := opb.find(pb.Epoch, pb.ViewNumber, pb.Block.NumberU64()); ev != nil {
		if ev.Block.Hash() != pb.Block.Hash() {
			a, b := pb, ev
			ha := new(big.Int).SetBytes(pb.Block.Hash().Bytes())
			hb := new(big.Int).SetBytes(ev.Block.Hash().Bytes())

			if ha.Cmp(hb) > 0 {
				a, b = ev, pb
			}
			return &DuplicatePrepareBlockEvidence{
				PrepareA: a,
				PrepareB: b,
			}
		}
	} else {
		*opb = append(*opb, pb)
		sort.Sort(*opb)
	}
	return nil
}

func (opb NumberOrderPrepareBlock) find(epoch uint64, viewNumber uint64, blockNumber uint64) *protocols.PrepareBlock {
	for _, v := range opb {
		if v.Epoch == epoch && v.ViewNumber == viewNumber && v.Block.NumberU64() == blockNumber {
			return v
		}
	}
	return nil
}

func (opb *NumberOrderPrepareBlock) Remove(viewNumber uint64) {
	i := 0

	for i < len(*opb) {
		if (*opb)[i].ViewNumber > viewNumber {
			break
		}
		(*opb)[i] = nil
		i++
	}
	if i == len(*opb) {
		*opb = (*opb)[:0]
	} else {
		*opb = append((*opb)[:0], (*opb)[i:]...)
	}
}

func (opb NumberOrderPrepareBlock) Len() int {
	return len(opb)
}

func (opb NumberOrderPrepareBlock) Less(i, j int) bool {
	return opb[i].ViewNumber < opb[j].ViewNumber
}

func (opb NumberOrderPrepareBlock) Swap(i, j int) {
	opb[i], opb[j] = opb[j], opb[i]
}

func (e PrepareVoteEvidence) Add(pv *protocols.PrepareVote, id Identity) error {
	var l NumberOrderPrepareVote

	if l = e[id]; l == nil {
		l = make(NumberOrderPrepareVote, 0)
	}
	err := l.Add(pv)
	e[id] = l
	return err
}

func (e PrepareVoteEvidence) Clear(viewNumber uint64) {
	for k, v := range e {
		v.Remove(viewNumber)
		if v.Len() == 0 {
			delete(e, k)
		}
	}
}

func (opb *NumberOrderPrepareVote) Remove(viewNumber uint64) {
	i := 0

	for i < len(*opb) {
		if (*opb)[i].ViewNumber > viewNumber {
			break
		}
		(*opb)[i] = nil
		i++
	}
	if i == len(*opb) {
		*opb = (*opb)[:0]
	} else {
		*opb = append((*opb)[:0], (*opb)[i:]...)
	}
}

func (opv *NumberOrderPrepareVote) Add(pv *protocols.PrepareVote) error {
	if ev := opv.find(pv.Epoch, pv.ViewNumber, pv.BlockNumber); ev != nil {
		if ev.BlockHash != pv.BlockHash {
			a, b := pv, ev
			ha := new(big.Int).SetBytes(pv.BlockHash.Bytes())
			hb := new(big.Int).SetBytes(ev.BlockHash.Bytes())

			if ha.Cmp(hb) > 0 {
				a, b = ev, pv
			}
			return &DuplicatePrepareVoteEvidence{
				VoteA: a,
				VoteB: b,
			}
		}
	} else {
		*opv = append(*opv, pv)
		sort.Sort(*opv)
	}
	return nil
}

func (opv NumberOrderPrepareVote) find(epoch uint64, viewNumber uint64, blockNumber uint64) *protocols.PrepareVote {
	for _, v := range opv {
		if v.Epoch == epoch && v.ViewNumber == viewNumber && v.BlockNumber == blockNumber {
			return v
		}
	}
	return nil
}

func (opv NumberOrderPrepareVote) Len() int {
	return len(opv)
}

func (opv NumberOrderPrepareVote) Less(i, j int) bool {
	return opv[i].ViewNumber < opv[j].ViewNumber
}

func (opv NumberOrderPrepareVote) Swap(i, j int) {
	opv[i], opv[j] = opv[j], opv[i]
}

func (e ViewChangeEvidence) Add(vc *protocols.ViewChange, id Identity) error {
	var l NumberOrderViewChange

	if l = e[id]; l == nil {
		l = make(NumberOrderViewChange, 0)
	}
	err := l.Add(vc)
	e[id] = l
	return err
}

func (e ViewChangeEvidence) Clear(viewNumber uint64) {
	for k, v := range e {
		v.Remove(viewNumber)
		if v.Len() == 0 {
			delete(e, k)
		}
	}
}

func (opb *NumberOrderViewChange) Remove(viewNumber uint64) {
	i := 0

	for i < len(*opb) {
		if (*opb)[i].ViewNumber > viewNumber {
			break
		}
		(*opb)[i] = nil
		i++
	}
	if i == len(*opb) {
		*opb = (*opb)[:0]
	} else {
		*opb = append((*opb)[:0], (*opb)[i:]...)
	}
}

func (ovc *NumberOrderViewChange) Add(vc *protocols.ViewChange) error {
	if ev := ovc.find(vc.Epoch, vc.ViewNumber, vc.BlockNumber); ev != nil {
		if ev.BlockHash != vc.BlockHash {
			a, b := vc, ev
			ha := new(big.Int).SetBytes(vc.BlockHash.Bytes())
			hb := new(big.Int).SetBytes(ev.BlockHash.Bytes())

			if ha.Cmp(hb) > 0 {
				a, b = ev, vc
			}
			return &DuplicateViewChangeEvidence{
				ViewA: a,
				ViewB: b,
			}
		}
	} else {
		*ovc = append(*ovc, vc)
		sort.Sort(*ovc)
	}
	return nil
}

func (ovc NumberOrderViewChange) find(epoch uint64, viewNumber uint64, blockNumber uint64) *protocols.ViewChange {
	for _, v := range ovc {
		if v.Epoch == epoch && v.ViewNumber == viewNumber && v.BlockNumber == blockNumber {
			return v
		}
	}
	return nil
}

func (ovc NumberOrderViewChange) Len() int {
	return len(ovc)
}

func (ovc NumberOrderViewChange) Less(i, j int) bool {
	return ovc[i].ViewNumber < ovc[j].ViewNumber
}

func (ovc NumberOrderViewChange) Swap(i, j int) {
	ovc[i], ovc[j] = ovc[j], ovc[i]
}
