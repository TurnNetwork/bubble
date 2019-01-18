package pposm_test

import (
	"fmt"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/consensus/ethash"
	"github.com/PlatONnetwork/PlatON-Go/core"
	"github.com/PlatONnetwork/PlatON-Go/core/state"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/core/vm"
	"github.com/PlatONnetwork/PlatON-Go/ethdb"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/params"
	"math/big"
	"testing"

	"encoding/json"
	"errors"
	"github.com/PlatONnetwork/PlatON-Go/core/ppos"
)

func newChainState() (*state.StateDB, error) {
	var (
		db      = ethdb.NewMemDatabase()
		genesis = new(core.Genesis).MustCommit(db)
	)
	fmt.Println("genesis", genesis)
	// Initialize a fresh chain with only a genesis block
	blockchain, _ := core.NewBlockChain(db, nil, params.AllEthashProtocolChanges, ethash.NewFaker(), vm.Config{}, nil)

	var state *state.StateDB
	if statedb, err := blockchain.State(); nil != err {
		return nil, errors.New("reference statedb failed" + err.Error())
	} else {
		/*var isgenesis bool
		if blockchain.CurrentBlock().NumberU64() == blockchain.Genesis().NumberU64() {
			isgenesis = true
		}
		*/ /** test init candidatePool */ /*
			if pool, err := pposm.NewCandidatePool(*/ /*statedb,*/ /* &configs*/ /*, isgenesis*/ /*); nil != err {
			t.Log("init candidatePool err", err)
		}else{
			candidatePool = pool
		}*/
		state = statedb
	}
	return state, nil
}

func newPoolContext() *pposm.CandidatePoolContext {
	configs := &params.PposConfig{
		Candidate: &params.CandidateConfig{
			Threshold:         "10",
			DepositLimit:      10,
			MaxChair:          1,
			MaxCount:          3,
			RefundBlockNumber: 1,
		},
	}
	cContext := &pposm.CandidatePoolContext{
		configs,
	}
	return cContext
}

func printObject(title string, obj interface{}, t *testing.T) {
	objs, _ := json.Marshal(obj)
	t.Log(title, string(objs), "\n")
}

func TestInitCandidatePoolByConfig(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	//state.Commit(false)

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}

	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test GetCandidate */
	t.Log("test GetCandidate ...")
	can, _ := newPoolContext().GetCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"))
	t.Log("GetCandidate", can)

	/** test WithdrawCandidate */
	t.Log("test WithdrawCandidate ...")
	ok1 := newPoolContext().WithdrawCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(99)), new(big.Int).SetUint64(uint64(10)))
	t.Log("error", ok1)

	/** test WithdrawCandidate again */
	t.Log("test WithdrawCandidate again ...")
	ok2 := newPoolContext().WithdrawCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(10)), new(big.Int).SetUint64(uint64(11)))
	t.Log("error", ok2)

	/** test GetChosens */
	t.Log("test GetChosens ...")
	canArr := newPoolContext().GetChosens(state)
	printObject("Elected candidates", canArr, t)

	/** test GetChairpersons */
	t.Log("test GetChairpersons ...")
	canArr = newPoolContext().GetChairpersons(state)
	printObject("Witnesses", canArr, t)

	/** test GetDefeat */
	t.Log("test GetDefeat ...")
	defeatArr, _ := newPoolContext().GetDefeat(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("can be refund defeats", defeatArr, t)

	/** test IsDefeat */
	t.Log("test IsDefeat ...")
	flag, _ := newPoolContext().IsDefeat(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("isdefeat", flag, t)

	/** test Election */
	t.Log("test Election ...")
	_, err := newPoolContext().Election(state, big.NewInt(0))
	t.Log("whether election was successful", err)

	/** test RefundBalance */
	t.Log("test RefundBalance ...")
	err = newPoolContext().RefundBalance(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(11)))
	t.Log("err", err)

	/** test RefundBalance again */
	t.Log("test RefundBalance again ...")
	err = newPoolContext().RefundBalance(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012343"), new(big.Int).SetUint64(uint64(11)))
	t.Log("err", err)

	/** test GetOwner */
	t.Log("test GetOwner ...")
	addr := newPoolContext().GetOwner(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	t.Log("Benefit address", addr.String())

	/**  test GetWitness */
	t.Log("test GetWitness ...")
	nodeArr, _ := newPoolContext().GetWitness(state, 0)
	printObject("nodeArr", nodeArr, t)
}

func TestSetCandidate(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	//state.Commit(false)

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}

	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

}

func TestGetCandidate(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}

	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test GetCandidate */
	t.Log("test GetCandidate ...")
	can, _ := newPoolContext().GetCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("GetCandidate", can, t)

}

func TestWithdrawCandidate(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(99),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(99),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test GetCandidate */
	t.Log("test GetCandidate ...")
	can, _ := newPoolContext().GetCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("GetCandidate", can, t)

	/** test WithdrawCandidate */
	t.Log("test WithdrawCandidate ...")
	ok1 := newPoolContext().WithdrawCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(98)), new(big.Int).SetUint64(uint64(10)))
	t.Log("error", ok1)

	/** test GetCandidate */
	t.Log("test GetCandidate ...")
	can2, _ := newPoolContext().GetCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("GetCandidate", can2, t)
}

func TestGetChosens(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(99),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test GetChosens */
	t.Log("test GetChosens ...")
	canArr := newPoolContext().GetChosens(state)
	printObject("immediate elected candidates", canArr, t)

}

func TestGetElection(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(110),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate3 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012342"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate3.CandidateId, candidate3); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate4 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(110),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012343"),
		TxIndex:     4,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate4.CandidateId, candidate4); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test Election */
	t.Log("test Election ...")
	_, err := newPoolContext().Election(state, big.NewInt(0))
	t.Log("Whether election was successful err", err)

}

func TestGetWitness(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate3 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012342"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate3.CandidateId, candidate3); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate4 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(110),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012343"),
		TxIndex:     4,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate4.CandidateId, candidate4); nil != err {
		t.Error("SetCandidate err:", err)
	}

	/** test Election */
	t.Log("test Election ...")
	_, err := newPoolContext().Election(state, big.NewInt(0))
	t.Log("Whether election was successful err", err)

	/** test switch */
	t.Log("test Switch ...")
	flag := newPoolContext().Switch(state)
	t.Log("Switch was success ", flag)

	/** test GetChairpersons */
	t.Log("test GetChairpersons ...")
	canArr := newPoolContext().GetChairpersons(state)
	printObject("Witnesses", canArr, t)
}

func TestGetDefeat(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate3 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012342"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate3.CandidateId, candidate3); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate4 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(110),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012343"),
		TxIndex:     4,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate4.CandidateId, candidate4); nil != err {
		t.Error("SetCandidate err:", err)
	}

	arr := newPoolContext().GetChosens(state)
	by, _ := json.Marshal(arr)
	fmt.Println(string(by))

	/** test Election */
	t.Log("test Election ...")
	_, err := newPoolContext().Election(state, big.NewInt(0))
	t.Log("Whether election was successful err", err)

	/**  */
	printObject("candidatePool:", *newPoolContext(), t)
	/** test MaxChair */
	t.Log("test MaxChair:", newPoolContext().MaxChair())
	/**test Interval*/
	t.Log("test Interval:", newPoolContext().GetRefundInterval())

	/** test switch */
	t.Log("test Switch ...")
	flag := newPoolContext().Switch(state)
	t.Log("Switch was success ", flag)

	/** test GetChairpersons */
	t.Log("test GetChairpersons ...")
	canArr := newPoolContext().GetChairpersons(state)
	printObject("Witnesses", canArr, t)

	/** test WithdrawCandidate */
	t.Log("test WithdrawCandidate ...")
	ok1 := newPoolContext().WithdrawCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), big.NewInt(110000000000000), new(big.Int).SetUint64(uint64(10)))
	t.Log("error", ok1)

	/** test GetCandidate */
	t.Log("test GetCandidate ...")
	can2, _ := newPoolContext().GetCandidate(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("GetCandidate", can2, t)

	/** test GetDefeat */
	t.Log("test GetDefeat ...")
	defeatArr, _ := newPoolContext().GetDefeat(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	printObject("can be refund defeats", defeatArr, t)

	/** test IsDefeat */
	t.Log("test IsDefeat ...")
	flag, _ = newPoolContext().IsDefeat(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	t.Log("isdefeat", flag)

	/** test RefundBalance */
	t.Log("test RefundBalance ...")
	err = newPoolContext().RefundBalance(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(11)))
	t.Log("RefundBalance err", err)

	/** test RefundBalance again */
	t.Log("test RefundBalance again ...")
	err = newPoolContext().RefundBalance(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"), new(big.Int).SetUint64(uint64(11)))
	t.Log("RefundBalance again err", err)

	/** test GetOwner */
	t.Log("test GetOwner ...")
	addr := newPoolContext().GetOwner(state, discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"))
	t.Log("Benefit address", addr.String())

	/**  test GetWitness */
	t.Log("test GetWitness ...")
	nodeArr, _ := newPoolContext().GetWitness(state, 0)
	printObject("nodeArr", nodeArr, t)
}

func TestTraversingStateDB(t *testing.T) {
	var state *state.StateDB
	if st, err := newChainState(); nil != err {
		t.Error("Getting stateDB err", err)
	} else {
		state = st
	}

	candidate := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(100),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345"),
		TxIndex:     6,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x12"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate.CandidateId, candidate); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate2 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(99),
		BlockNumber: new(big.Int).SetUint64(7),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012341"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate2.CandidateId, candidate2); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate3 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(99),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012342"),
		TxIndex:     5,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate3.CandidateId, candidate3); nil != err {
		t.Error("SetCandidate err:", err)
	}

	candidate4 := &types.Candidate{
		Deposit:     new(big.Int).SetUint64(110),
		BlockNumber: new(big.Int).SetUint64(6),
		CandidateId: discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012343"),
		TxIndex:     4,
		Host:        "10.0.0.1",
		Port:        "8548",
		Owner:       common.HexToAddress("0x15"),
	}
	t.Log("Set New Candidate ...")
	/** test SetCandidate */
	if err := newPoolContext().SetCandidate(state, candidate4.CandidateId, candidate4); nil != err {
		t.Error("SetCandidate err:", err)
	}
	//state.Commit(true)

	//state.Finalise(false)

	pposm.TraversingStateDB(state)
}
