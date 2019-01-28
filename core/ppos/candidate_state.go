package pposm

import (
	"encoding/json"
	"errors"
	"fmt"
	_ "fmt"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/state"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/core/vm"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/params"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
	"math/big"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	// immediate elected candidate
	ImmediatePrefix     = "id"
	ImmediateListPrefix = "iL"
	// previous witness
	PreWitnessPrefix     = "Pwn"
	PreWitnessListPrefix = "PwL"
	// witness
	WitnessPrefix     = "wn"
	WitnessListPrefix = "wL"
	// next witness
	NextWitnessPrefix     = "Nwn"
	NextWitnessListPrefix = "NwL"
	// need refund
	DefeatPrefix     = "df"
	DefeatListPrefix = "dL"
)

var (
	// immediate elected candidate
	ImmediateBtyePrefix     = []byte(ImmediatePrefix)
	ImmediateListBtyePrefix = []byte(ImmediateListPrefix)
	// previous witness
	PreWitnessBytePrefix     = []byte(PreWitnessPrefix)
	PreWitnessListBytePrefix = []byte(PreWitnessListPrefix)
	// witness
	WitnessBtyePrefix     = []byte(WitnessPrefix)
	WitnessListBtyePrefix = []byte(WitnessListPrefix)
	// next witness
	NextWitnessBtyePrefix     = []byte(NextWitnessPrefix)
	NextWitnessListBytePrefix = []byte(NextWitnessListPrefix)
	// need refund
	DefeatBtyePrefix     = []byte(DefeatPrefix)
	DefeatListBtyePrefix = []byte(DefeatListPrefix)

	CandidateEncodeErr          = errors.New("Candidate encoding err")
	CandidateDecodeErr          = errors.New("Candidate decoding err")
	CandidateEmptyErr           = errors.New("Candidate is empty")
	ContractBalanceNotEnoughErr = errors.New("Contract's balance is not enough")
	CandidateOwnerErr           = errors.New("CandidateOwner Addr is illegal")
	DepositLowErr               = errors.New("Candidate deposit too low")
	WithdrawPriceErr            = errors.New("Withdraw Price err")
	WithdrawLowErr              = errors.New("Withdraw Price too low")
)

type CandidatePool struct {
	// min deposit allow threshold
	threshold *big.Int
	// min deposit limit percentage
	depositLimit uint64
	// allow immediate elected max count
	maxCount uint64
	// allow witness max count
	maxChair uint64
	// allow block interval for refunds
	RefundBlockNumber uint64

	// previous witness
	preOriginCandidates map[discover.NodeID]*types.Candidate
	// current witnesses
	originCandidates map[discover.NodeID]*types.Candidate
	// next witnesses
	nextOriginCandidates map[discover.NodeID]*types.Candidate
	// immediates
	immediateCandates map[discover.NodeID]*types.Candidate
	// refunds
	defeatCandidates map[discover.NodeID][]*types.Candidate

	// cache
	candidateCacheArr []*types.Candidate
	//lock              *sync.RWMutex
	lock *sync.Mutex
}

//var candidatePool *CandidatePool

// Initialize the global candidate pool object
func NewCandidatePool(configs *params.PposConfig) *CandidatePool {
	log.Debug("Build a New CandidatePool Info ...")
	if "" == strings.TrimSpace(configs.Candidate.Threshold) {
		configs.Candidate.Threshold = "1000000000000000000000000"
	}
	var threshold *big.Int
	if thd, ok := new(big.Int).SetString(configs.Candidate.Threshold, 10); !ok {
		threshold, _ = new(big.Int).SetString("1000000000000000000000000", 10)
	} else {
		threshold = thd
	}
	return &CandidatePool{
		threshold:            threshold,
		depositLimit:         configs.Candidate.DepositLimit,
		maxCount:             configs.Candidate.MaxCount,
		maxChair:             configs.Candidate.MaxChair,
		RefundBlockNumber:    configs.Candidate.RefundBlockNumber,
		preOriginCandidates:  make(map[discover.NodeID]*types.Candidate, 0),
		originCandidates:     make(map[discover.NodeID]*types.Candidate, 0),
		nextOriginCandidates: make(map[discover.NodeID]*types.Candidate, 0),
		immediateCandates:    make(map[discover.NodeID]*types.Candidate, 0),
		defeatCandidates:     make(map[discover.NodeID][]*types.Candidate, 0),
		candidateCacheArr:    make([]*types.Candidate, 0),
		//lock:                 &sync.RWMutex{},
		lock: &sync.Mutex{},
	}
	//return candidatePool
}

// flag:
// 0: only init previous witness and current witness and next witness
// 1：init previous witness and current witness and next witness and immediate
// 2: init all information
func (c *CandidatePool) initDataByState(state vm.StateDB, flag int) error {
	log.Info("init data by stateDB...")
	// loading previous witness
	var prewitnessIds []discover.NodeID
	c.preOriginCandidates = make(map[discover.NodeID]*types.Candidate, 0)
	if ids, err := getPreviousWitnessIdsState(state); nil != err {
		log.Error("Failed to decode previous witnessIds on initDataByState", " err", err)
		return err
	} else {
		prewitnessIds = ids
	}
	PrintObject("prewitnessIds", prewitnessIds)
	for _, witnessId := range prewitnessIds {
		//var can *types.Candidate
		if ca, err := getPreviousWitnessByState(state, witnessId); nil != err {
			log.Error("Failed to decode previous Candidate on initDataByState", "nodeId", witnessId.String(), "err", err)
			return CandidateDecodeErr
		} else {
			if nil != ca {
				PrintObject("Id:"+ witnessId.String()+", pre can", ca)
				c.preOriginCandidates[witnessId] = ca
			} else {
				delete(c.preOriginCandidates, witnessId)
			}
		}
	}

	// loading current witnesses
	var witnessIds []discover.NodeID
	c.originCandidates = make(map[discover.NodeID]*types.Candidate, 0)
	if ids, err := getWitnessIdsByState(state); nil != err {
		log.Error("Failed to decode current witnessIds on initDataByState", "err", err)
		return err
	} else {
		witnessIds = ids
	}
	PrintObject("current witnessIds", witnessIds)
	for _, witnessId := range witnessIds {
		//var can *types.Candidate
		if ca, err := getWitnessByState(state, witnessId); nil != err {
			log.Error("Failed to decode current Candidate on initDataByState", "nodeId", witnessId.String(), "err", err)
			return CandidateDecodeErr
		} else {
			if nil != ca {
				PrintObject("Id:"+ witnessId.String()+", cur can", ca)
				c.originCandidates[witnessId] = ca
			} else {
				delete(c.originCandidates, witnessId)
			}
		}
	}

	// loading next witnesses
	var nextWitnessIds []discover.NodeID
	c.nextOriginCandidates = make(map[discover.NodeID]*types.Candidate, 0)
	if ids, err := getNextWitnessIdsByState(state); nil != err {
		log.Error("Failed to decode nextWitnessIds on initDataByState", "err", err)
		return err
	} else {
		nextWitnessIds = ids
	}
	PrintObject("nextWitnessIds", nextWitnessIds)
	for _, witnessId := range nextWitnessIds {
		//fmt.Println("nextwitnessId = ", witnessId.String())
		//var can *types.Candidate
		if ca, err := getNextWitnessByState(state, witnessId); nil != err {
			log.Error("Failed to decode next Candidate on initDataByState", "nodeId", witnessId.String(), "err", err)
			return CandidateDecodeErr
		} else {
			if nil != ca {
				PrintObject("Id:"+ witnessId.String()+", next can", ca)
				c.nextOriginCandidates[witnessId] = ca
			} else {
				delete(c.nextOriginCandidates, witnessId)
			}
		}
	}

	if flag == 1 || flag == 2 {
		// loading immediate elected candidates
		var immediateIds []discover.NodeID
		c.immediateCandates = make(map[discover.NodeID]*types.Candidate, 0)
		if ids, err := getImmediateIdsByState(state); nil != err {
			log.Error("Failed to decode immediateIds on initDataByState", "err", err)
			return err
		} else {
			immediateIds = ids
		}

		// cache
		canCache := make([]*types.Candidate, 0)

		PrintObject("immediateIds", immediateIds)
		for _, immediateId := range immediateIds {
			//fmt.Println("immediateId = ", immediateId.String())
			//var can *types.Candidate
			if ca, err := getImmediateByState(state, immediateId); nil != err {
				log.Error("Failed to decode immediate Candidate on initDataByState", "nodeId", immediateId.String(), "err", err)
				return CandidateDecodeErr
			} else {
				if nil != ca {
					PrintObject("Id:"+ immediateId.String()+", im can", ca)
					c.immediateCandates[immediateId] = ca
					canCache = append(canCache, ca)
				} else {
					delete(c.immediateCandates, immediateId)
				}
			}
		}
		c.candidateCacheArr = canCache
	}

	if flag == 2 {
		// load refunds
		var defeatIds []discover.NodeID
		c.defeatCandidates = make(map[discover.NodeID][]*types.Candidate, 0)
		if ids, err := getDefeatIdsByState(state); nil != err {
			log.Error("Failed to decode defeatIds on initDataByState", "err", err)
			return err
		} else {
			defeatIds = ids
		}
		PrintObject("defeatIds", defeatIds)
		for _, defeatId := range defeatIds {
			//fmt.Println("defeatId = ", defeatId.String())
			//var canArr []*types.Candidate
			if arr, err := getDefeatsByState(state, defeatId); nil != err {
				log.Error("Failed to decode defeat's CandidateArr on initDataByState", "defeatId", defeatId.String(), "err", err)
				return CandidateDecodeErr
			} else {
				if nil != arr && len(arr) != 0 {
					PrintObject("Id:"+ defeatId.String()+", defeat canArr", arr)
					c.defeatCandidates[defeatId] = arr
				} else {
					delete(c.defeatCandidates, defeatId)
				}
			}
		}
	}
	return nil
}

// pledge Candidate
func (c *CandidatePool) SetCandidate(state vm.StateDB, nodeId discover.NodeID, can *types.Candidate) error {
	log.Info("Call SetCandidate start ...", "maxChair", c.maxChair, "maxCount", c.maxCount, "RefundBlockNumber", c.RefundBlockNumber)
	PrintObject("Call SetCandidate Info:", *can)
	// TODO
	if !c.checkFirstThreshold(can) {
		log.Info("Faided to SetCandidate", "err", DepositLowErr, "Deposit", can.Deposit.String(), "limit", c.threshold)
		return DepositLowErr
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	defer func() {
		log.Debug("Call SetCandidate Success view state again ...")
		c.initDataByState(state, 2)
	}()
	if err := c.initDataByState(state, 2); nil != err {
		log.Error("Failed to initDataByState on SetCandidate", " err", err)
		return err
	}

	if err := c.checkDeposit(can); nil != err {
		log.Error("Failed to checkDeposit on SetCandidate", "nodeId", nodeId.String(), " err", err)
		return err
	}

	c.immediateCandates[can.CandidateId] = can
	c.candidateCacheArr = make([]*types.Candidate, 0)
	// append to the cache array and then sort
	if len(c.immediateCandates) != 0 && len(c.candidateCacheArr) == 0 {
		for _, v := range c.immediateCandates {
			c.candidateCacheArr = append(c.candidateCacheArr, v)
		}
	}

	// Whether the current candidate is new
	// then append to cache array
	// sort cache array
	candidateSort(c.candidateCacheArr)

	// move the excessive of immediate elected candidate to refunds
	if len(c.candidateCacheArr) > int(c.maxCount) {
		// Intercepting the lost candidates to tmpArr
		tmpArr := (c.candidateCacheArr)[c.maxCount:]
		// Reserve elected candidates
		c.candidateCacheArr = (c.candidateCacheArr)[:c.maxCount]

		// handle tmpArr
		for _, tmpCan := range tmpArr {
			// delete the lost candidates from immediate elected candidates of trie
			if err := c.delImmediate(state, tmpCan.CandidateId); nil != err {
				log.Error("Failed to delImmediate on SetCandidate", "nodeId", tmpCan.CandidateId.String(), "err", err)
				return err
			}
			// append to refunds (defeat) trie
			if err := c.setDefeat(state, tmpCan.CandidateId, tmpCan); nil != err {
				log.Error("Failed to setDefeat on SetCandidate", "nodeId", tmpCan.CandidateId.String(), "err", err)
				return err
			}
		}

		// update index of refund (defeat) on trie
		if err := c.setDefeatIndex(state); nil != err {
			log.Error("Failed to setDefeatIndex on SetCandidate", "err", err)
			return err
		}
	}

	// cache id
	sortIds := make([]discover.NodeID, 0)

	// insert elected candidate to tire
	for _, can := range c.candidateCacheArr {
		if err := c.setImmediate(state, can.CandidateId, can); nil != err {
			log.Error("Failed to setImmediate on SetCandidate", "nodeId", can.CandidateId.String(), "err", err)
			return err
		}
		sortIds = append(sortIds, can.CandidateId)
	}

	// update index of immediate elected candidates on trie
	if err := c.setImmediateIndex(state, sortIds); nil != err {
		log.Error("Failed to setImmediateIndex on SetCandidate", "err", err)
		return err
	}

	log.Debug("Call SetCandidate successfully...")
	return nil
}

// Getting immediate candidate info by nodeId
func (c *CandidatePool) GetCandidate(state vm.StateDB, nodeId discover.NodeID) (*types.Candidate, error) {
	return c.getCandidate(state, nodeId)
}

// Getting immediate or reserve candidate info arr by nodeIds
func (c *CandidatePool) GetCandidateArr(state vm.StateDB, nodeIds ...discover.NodeID) (types.CandidateQueue, error) {
	return c.getCandidates(state, nodeIds...)
}

// candidate withdraw from immediates elected candidates
func (c *CandidatePool) WithdrawCandidate(state vm.StateDB, nodeId discover.NodeID, price, blockNumber *big.Int) error {
	log.Info("Call WithdrawCandidate...", "nodeId", nodeId.String(), "price", price.String())
	c.lock.Lock()
	defer c.lock.Unlock()
	defer func() {
		log.Debug("Call WithdrawCandidate SUCCESS VIEW state again ... ")
		c.initDataByState(state, 2)
	}()
	if err := c.initDataByState(state, 2); nil != err {
		log.Error("Failed to initDataByState on WithdrawCandidate", " err", err)
		return err
	}

	if price.Cmp(new(big.Int).SetUint64(0)) <= 0 {
		log.Error("Failed cmp price is invalid", "nodeId", nodeId.String(), " price", price.String())
		return WithdrawPriceErr
	}
	can, ok := c.immediateCandates[nodeId]
	if !ok || nil == can {
		log.Error("Failed to find current Candidate is empty", "nodeId", nodeId.String())
		return CandidateEmptyErr
	}

	// check withdraw price
	if can.Deposit.Cmp(price) < 0 {
		log.Error("Failed refund price must less or equal deposit", "nodeId", nodeId.String())
		return WithdrawPriceErr
	} else if can.Deposit.Cmp(price) == 0 { // full withdraw
		// delete current candidate from immediate elected candidates
		if err := c.delImmediate(state, nodeId); nil != err {
			log.Error("Failed to delImmediate on full withdraw", "nodeId", nodeId.String(), "err", err)
			return err
		}
		// update immediate id index
		if ids, err := c.getImmediateIndex(state); nil != err {
			log.Error("Failed to getImmediateIndex on full withdrawerr", "err", err)
			return err
		} else {
			//for i, id := range ids {
			for i := 0; i < len(ids); i++ {
				id := ids[i]
				if id == nodeId {
					ids = append(ids[:i], ids[i+1:]...)
					i--
				}
			}
			if err := c.setImmediateIndex(state, ids); nil != err {
				log.Error("Failed to setImmediateIndex on full withdrawerr", "err", err)
				return err
			}
		}

		// append to refund (defeat) trie
		if err := c.setDefeat(state, nodeId, can); nil != err {
			log.Error("Failed to setDefeat on full withdrawerr", "nodeId", nodeId.String(), "err", err)
			return err
		}
		// update index of defeat on trie
		if err := c.setDefeatIndex(state); nil != err {
			log.Error("Failed to setDefeatIndex on full withdrawerr", "err", err)
			return err
		}
	} else {
		// Only withdraw part of the refunds, need to reorder the immediate elected candidates
		// The remaining candiate price to update current candidate info

		if err := c.checkWithdraw(can.Deposit, price); nil != err {
			log.Error("Failed to checkWithdraw price invalid", "nodeId", nodeId.String(), " price", price.String(), "err", err)
			return err
		}

		canNew := &types.Candidate{
			Deposit:     new(big.Int).Sub(can.Deposit, price),
			BlockNumber: can.BlockNumber,
			TxIndex:     can.TxIndex,
			CandidateId: can.CandidateId,
			Host:        can.Host,
			Port:        can.Port,
			Owner:       can.Owner,
			From:        can.From,
			Extra:       can.Extra,
			Fee:         can.Fee,
		}

		// update current candidate
		if err := c.setImmediate(state, nodeId, canNew); nil != err {
			log.Error("Failed to setImmediate on a few of withdrawerr", "nodeId", nodeId.String(), "err", err)
			return err
		}

		// sort immediate
		c.candidateCacheArr = make([]*types.Candidate, 0)
		for _, can := range c.immediateCandates {
			c.candidateCacheArr = append(c.candidateCacheArr, can)
		}
		candidateSort(c.candidateCacheArr)
		ids := make([]discover.NodeID, 0)
		for _, can := range c.candidateCacheArr {
			ids = append(ids, can.CandidateId)
		}
		// update new index
		if err := c.setImmediateIndex(state, ids); nil != err {
			log.Error("Failed to setImmediateIndex on a few of withdrawerr", "err", err)
			return err
		}

		// the withdraw price to build a new refund into defeat on trie
		canDefeat := &types.Candidate{
			Deposit:     price,
			BlockNumber: blockNumber,
			TxIndex:     can.TxIndex,
			CandidateId: can.CandidateId,
			Host:        can.Host,
			Port:        can.Port,
			Owner:       can.Owner,
			From:        can.From,
			Extra:       can.Extra,
			Fee:         can.Fee,
		}
		// the withdraw
		if err := c.setDefeat(state, nodeId, canDefeat); nil != err {
			log.Error("Failed to setDefeat on a few of withdrawerr", "nodeId", nodeId.String(), "err", err)
			return err
		}
		// update index of defeat on trie
		if err := c.setDefeatIndex(state); nil != err {
			log.Error("Failed to setDefeatIndex on a few of withdrawerr", "err", err)
			return err
		}
	}
	log.Info("Call WithdrawCandidate SUCCESS !!!!!!!!!!!!")
	return nil
}

// Getting all immediate elected candidates array
func (c *CandidatePool) GetChosens(state vm.StateDB) []*types.Candidate {
	log.Debug("Call GetChosens getting immediate candidates ...")
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on WithdrawCandidate", "err", err)
		return nil
	}
	immediateIds, err := c.getImmediateIndex(state)
	if nil != err {
		log.Error("Failed to getImmediateIndex", "err", err)
		return nil
	}
	arr := make([]*types.Candidate, 0)
	for _, id := range immediateIds {
		arr = append(arr, c.immediateCandates[id])
	}
	return arr
}

// Getting all witness array
func (c *CandidatePool) GetChairpersons(state vm.StateDB) []*types.Candidate {
	log.Debug("Call GetChairpersons getting witnesses ...")
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 0); nil != err {
		log.Error("Failed to initDataByState on GetChairpersons", "err", err)
		return nil
	}
	witnessIds, err := c.getWitnessIndex(state)
	if nil != err {
		log.Error("Failed to getWitnessIndex on GetChairpersonserr", "err", err)
		return nil
	}
	arr := make([]*types.Candidate, 0)
	for _, id := range witnessIds {
		arr = append(arr, c.originCandidates[id])
	}
	return arr
}

// Getting all refund array by nodeId
func (c *CandidatePool) GetDefeat(state vm.StateDB, nodeId discover.NodeID) ([]*types.Candidate, error) {
	log.Debug("Call GetDefeat getting defeat arr: curr nodeId = " + nodeId.String())
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 2); nil != err {
		log.Error("Failed to initDataByState on GetDefeat", "err", err)
		return nil, err
	}

	defeat, ok := c.defeatCandidates[nodeId]
	if !ok {
		log.Error("Candidate is empty")
		return nil, nil
	}
	return defeat, nil
}

// Checked current candidate was defeat by nodeId
func (c *CandidatePool) IsDefeat(state vm.StateDB, nodeId discover.NodeID) (bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on IsDefeat", "err", err)
		return false, err
	}

	if _, ok := c.immediateCandates[nodeId]; ok {
		log.Error("Candidate is empty")
		return false, nil
	}

	if arr, ok := c.defeatCandidates[nodeId]; ok && len(arr) != 0 {
		return true, nil
	}

	return false, nil
}

// Getting owner's address of candidate info by nodeId
func (c *CandidatePool) GetOwner(state vm.StateDB, nodeId discover.NodeID) common.Address {
	log.Debug("Call GetOwner: curr nodeId = " + nodeId.String())
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 2); nil != err {
		log.Error("Failed to initDataByState on GetOwner", "err", err)
		return common.Address{}
	}
	pre_can, pre_ok := c.preOriginCandidates[nodeId]
	or_can, or_ok := c.originCandidates[nodeId]
	ne_can, ne_ok := c.nextOriginCandidates[nodeId]
	im_can, im_ok := c.immediateCandates[nodeId]
	canArr, de_ok := c.defeatCandidates[nodeId]

	if pre_ok {
		return pre_can.Owner
	}
	if or_ok {
		return or_can.Owner
	}
	if ne_ok {
		return ne_can.Owner
	}
	if im_ok {
		return im_can.Owner
	}
	if de_ok {
		if len(canArr) != 0 {
			return canArr[0].Owner
		}
	}
	return common.Address{}
}

// refund once
func (c *CandidatePool) RefundBalance(state vm.StateDB, nodeId discover.NodeID, blockNumber *big.Int) error {
	log.Info("Call RefundBalance:  curr nodeId = " + nodeId.String() + ",curr blocknumber:" + blockNumber.String())
	c.lock.Lock()
	defer c.lock.Unlock()
	defer func() {
		log.Debug("Call RefundBalance SUCCESS VIEW state again ... ")
		c.initDataByState(state, 2)
	}()
	if err := c.initDataByState(state, 2); nil != err {
		log.Error("Failed to initDataByState on RefundBalance", "err", err)
		return err
	}

	var canArr []*types.Candidate
	if defeatArr, ok := c.defeatCandidates[nodeId]; ok {
		canArr = defeatArr
	} else {
		log.Error("Failed to refundbalance candidate is empty", "nodeId", nodeId.String())
		return CandidateDecodeErr
	}
	// cache
	// Used for verification purposes, that is, the beneficiary in the pledge refund information of each nodeId should be the same
	var addr common.Address
	// Grand total refund amount for one-time
	amount := big.NewInt(0)
	// Transfer refund information that needs to be deleted
	delCanArr := make([]*types.Candidate, 0)

	contractBalance := state.GetBalance(common.CandidateAddr)
	//currentNum := new(big.Int).SetUint64(blockNumber)

	// Traverse all refund information belong to this nodeId
	//for index, can := range canArr {
	for index := 0; index < len(canArr); index++ {
		can := canArr[index]

		sub := new(big.Int).Sub(blockNumber, can.BlockNumber)
		log.Info("Check defeat detail", "nodeId:", nodeId.String(), "curr blocknumber:", blockNumber.String(), "setcandidate blocknumber:", can.BlockNumber.String(), " diff:", sub.String())
		if sub.Cmp(new(big.Int).SetUint64(c.RefundBlockNumber)) >= 0 { // allow refund
			delCanArr = append(delCanArr, can)
			canArr = append(canArr[:index], canArr[index+1:]...)
			index--
			// add up the refund price
			amount = new(big.Int).Add(amount, can.Deposit)
			//amount += can.Deposit.Uint64()
		} else {
			log.Warn("block height number had mismatch, No refunds allowed", "current block height", blockNumber.String(), "deposit block height", can.BlockNumber.String(), "nodeId", nodeId.String(), "allowed block interval", c.RefundBlockNumber)
			continue
		}

		if addr == common.ZeroAddr {
			addr = can.Owner
		} else {
			if addr != can.Owner {
				log.Info("Failed to refundbalance couse current nodeId had bind different owner address ", "nodeId", nodeId.String(), "addr1", addr.String(), "addr2", can.Owner)
				if len(canArr) != 0 {
					canArr = append(delCanArr, canArr...)
				} else {
					canArr = delCanArr
				}
				c.defeatCandidates[nodeId] = canArr
				log.Error("Failed to refundbalance Different beneficiary addresses under the same node", "nodeId", nodeId.String(), "addr1", addr.String(), "addr2", can.Owner)
				return CandidateOwnerErr
			}
		}

		// check contract account balance
		//if (contractBalance.Cmp(new(big.Int).SetUint64(amount))) < 0 {
		if (contractBalance.Cmp(amount)) < 0 {
			log.Error("Failed to refundbalance constract account insufficient balance ", "nodeId", nodeId.String(), "contract's balance", state.GetBalance(common.CandidateAddr).String(), "amount", amount.String())
			if len(canArr) != 0 {
				canArr = append(delCanArr, canArr...)
			} else {
				canArr = delCanArr
			}
			c.defeatCandidates[nodeId] = canArr
			return ContractBalanceNotEnoughErr
		}
	}

	// update the tire
	if len(canArr) == 0 {
		//delete(c.defeatCandidates, nodeId)
		if err := c.delDefeat(state, nodeId); nil != err {
			log.Error("RefundBalance failed to delDefeat", "nodeId", nodeId.String(), "err", err)
			return err
		}
		if ids, err := getDefeatIdsByState(state); nil != err {
			//for i, id := range ids {
			for i := 0; i < len(ids); i++ {
				id := ids[i]
				if id == nodeId {
					ids = append(ids[:i], ids[i+1:]...)
					i--
				}
			}
			if len(ids) != 0 {
				if value, err := rlp.EncodeToBytes(&ids); nil != err {
					log.Error("Failed to encode candidate ids on RefundBalance", "err", err)
					return CandidateEncodeErr
				} else {
					setDefeatIdsState(state, value)
				}
			} else {
				setDefeatIdsState(state, []byte{})
			}

		}
	} else {
		// If have some remaining, update that
		if arrVal, err := rlp.EncodeToBytes(canArr); nil != err {
			log.Error("Failed to encode candidate object on RefundBalance", "key", nodeId.String(), "err", err)
			canArr = append(delCanArr, canArr...)
			c.defeatCandidates[nodeId] = canArr
			return CandidateDecodeErr
		} else {
			// update the refund information
			setDefeatState(state, nodeId, arrVal)
			// remaining set back to defeat map
			c.defeatCandidates[nodeId] = canArr
		}
	}
	log.Info("Call RefundBalance to tansfer value：", "nodeId", nodeId.String(), "contractAddr", common.CandidateAddr.String(),
		"owner's addr", addr.String(), "Return the amount to be transferred:", amount.String())

	// sub contract account balance
	state.SubBalance(common.CandidateAddr, amount)
	// add owner balace
	state.AddBalance(addr, amount)
	log.Debug("Call RefundBalance success ...")
	return nil
}

// set immediate candidate extra value
func (c *CandidatePool) SetCandidateExtra(state vm.StateDB, nodeId discover.NodeID, extra string) error {
	log.Info("Call SetCandidateExtra:", "nodeId", nodeId.String(), "extra", extra)
	c.lock.Lock()
	defer c.lock.Unlock()
	defer func() {
		log.Debug("Call SetCandidateExtra Success view state again ...")
		c.initDataByState(state, 2)
	}()
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on SetCandidateExtra", "err", err)
		return err
	}
	if can, ok := c.immediateCandates[nodeId]; ok {
		// update current candidate info and update to tire
		can.Extra = extra
		if err := c.setImmediate(state, nodeId, can); nil != err {
			log.Error("Failed to setImmediate on SetCandidateExtra", "nodeId", nodeId.String(), "err", err)
			return err
		}
	} else {
		return CandidateEmptyErr
	}
	log.Debug("Call RefundBalance SUCCESS !!!!!! ")
	return nil
}

// Announce witness
func (c *CandidatePool) Election(state *state.StateDB) ([]*discover.Node, error) {
	log.Info("Call Election start ...", "maxChair", c.maxChair, "maxCount", c.maxCount, "RefundBlockNumber", c.RefundBlockNumber)

	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on Election", "err", err)
		return nil, err
	}

	// sort immediate candidates
	candidateSort(c.candidateCacheArr)

	// cache ids
	immediateIds := make([]discover.NodeID, 0)
	for _, can := range c.candidateCacheArr {
		immediateIds = append(immediateIds, can.CandidateId)
	}

	// a certain number of witnesses in front of the cache
	var nextWitIds []discover.NodeID
	// If the number of candidate selected does not exceed the number of witnesses
	if len(immediateIds) <= int(c.maxChair) {
		nextWitIds = make([]discover.NodeID, len(immediateIds))
		copy(nextWitIds, immediateIds)

	} else {
		// If the number of candidate selected exceeds the number of witnesses, the top N is extracted.
		nextWitIds = make([]discover.NodeID, c.maxChair)
		copy(nextWitIds, immediateIds)
	}
	log.Info("CHOOSE NEXT WITNESS'S IDS COUNT:", "len", len(nextWitIds))
	// cache map
	nextWits := make(map[discover.NodeID]*types.Candidate, 0)

	// copy witnesses information
	copyCandidateMapByIds(nextWits, c.immediateCandates, nextWitIds)
	// clear all old nextwitnesses information （If it is forked, the next round is no empty.）
	for nodeId, _ := range c.nextOriginCandidates {
		if err := c.delNextWitness(state, nodeId); nil != err {
			log.Error("failed to delNextWitness on election", "nodeId", nodeId.String(), "err", err)
			return nil, err
		}
	}

	// set up all new nextwitnesses information
	for nodeId, can := range nextWits {
		if err := c.setNextWitness(state, nodeId, can); nil != err {
			log.Error("failed to setNextWitness on election", "nodeId", nodeId.String(), "err", err)
			return nil, err
		}
	}
	// update new nextwitnesses index
	if err := c.setNextWitnessIndex(state, nextWitIds); nil != err {
		log.Error("failed to setNextWitnessIndex on election", "err", err)
		return nil, err
	}
	// replace the next round of witnesses
	c.nextOriginCandidates = nextWits
	arr := make([]*discover.Node, 0)
	for _, id := range nextWitIds {
		if can, ok := nextWits[id]; ok {
			if node, err := buildWitnessNode(can); nil != err {
				log.Error("Failed to build Node on GetWitness", "err", err, "nodeId", id.String())
				continue
			} else {
				arr = append(arr, node)
			}
		}
	}
	log.Info("Election next witness's node count:", "len", len(arr))
	log.Info("Call Election SUCCESS !!!!!!!")
	return arr, nil
}

// switch next witnesses to current witnesses
func (c *CandidatePool) Switch(state *state.StateDB) bool {
	log.Info("Call Switch start ...")
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 0); nil != err {
		log.Error("Failed to initDataByState on Switch", "err", err)
		return false
	}
	// clear all old previous witness on trie
	for nodeId, _ := range c.preOriginCandidates {
		if err := c.delPreviousWitness(state, nodeId); nil != err {
			log.Error("Failed to delPreviousWitness on Switch", "err", err)
			return false
		}
	}
	// set up new witnesses to previous witnesses on trie by current witnesses
	for nodeId, can := range c.originCandidates {
		if err := c.setPreviousWitness(state, nodeId, can); nil != err {
			log.Error("Failed to setPreviousWitness on Switch", "err", err)
			return false
		}
	}
	// update previous witness index by current witness index
	if ids, err := c.getWitnessIndex(state); nil != err {
		log.Error("Failed to getWitnessIndex on Switch", "err", err)
		return false
	} else {
		// replace witnesses index
		if err := c.setPreviousWitnessindex(state, ids); nil != err {
			log.Error("Failed to setPreviousWitnessindex on Switch", "err", err)
			return false
		}
	}

	// clear all old witnesses on trie
	for nodeId, _ := range c.originCandidates {
		if err := c.delWitness(state, nodeId); nil != err {
			log.Error("Failed to delWitness on Switch", "err", err)
			return false
		}
	}
	// set up new witnesses to current witnesses on trie by next witnesses
	for nodeId, can := range c.nextOriginCandidates {
		if err := c.setWitness(state, nodeId, can); nil != err {
			log.Error("Failed to setWitness on Switch", "err", err)
			return false
		}
	}
	// update current witness index by next witness index
	if ids, err := c.getNextWitnessIndex(state); nil != err {
		log.Error("Failed to getNextWitnessIndex on Switch", "err", err)
		return false
	} else {
		// replace witnesses index
		if err := c.setWitnessindex(state, ids); nil != err {
			log.Error("Failed to setWitnessindex on Switch", "err", err)
			return false
		}
	}
	// clear all old nextwitnesses information
	for nodeId, _ := range c.nextOriginCandidates {
		if err := c.delNextWitness(state, nodeId); nil != err {
			log.Error("failed to delNextWitness on election", "err", err)
			return false
		}
	}
	// clear next witness index
	if err := c.setNextWitnessIndex(state, make([]discover.NodeID, 0)); nil != err {
		log.Error("failed to setNextWitnessIndex clear next witness on Election")
	}
	log.Info("Call Switch SUCCESS !!!!!!!")
	return true
}

// Getting nodes of witnesses
// flag：-1: the previous round of witnesses  0: the current round of witnesses   1: the next round of witnesses
func (c *CandidatePool) GetWitness(state *state.StateDB, flag int) ([]*discover.Node, error) {
	log.Debug("Call GetWitness: flag = " + strconv.Itoa(flag))
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 0); nil != err {
		log.Error("Failed to initDataByState on GetWitness", "err", err)
		return nil, err
	}
	//var ids []discover.NodeID
	var witness map[discover.NodeID]*types.Candidate
	var indexArr []discover.NodeID
	if flag == -1 {
		witness = c.preOriginCandidates
		if ids, err := c.getPreviousWitnessIndex(state); nil != err {
			log.Error("Failed to getPreviousWitnessIndex on GetWitness", "err", err)
			return nil, err
		} else {
			indexArr = ids
		}
	} else if flag == 0 {
		witness = c.originCandidates
		if ids, err := c.getWitnessIndex(state); nil != err {
			log.Error("Failed to getWitnessIndex on GetWitness", "err", err)
			return nil, err
		} else {
			indexArr = ids
		}
	} else if flag == 1 {
		witness = c.nextOriginCandidates
		if ids, err := c.getNextWitnessIndex(state); nil != err {
			log.Error("Failed to getNextWitnessIndex on GetWitness", "err", err)
			return nil, err
		} else {
			indexArr = ids
		}
	}

	arr := make([]*discover.Node, 0)
	for _, id := range indexArr {
		if can, ok := witness[id]; ok {
			if node, err := buildWitnessNode(can); nil != err {
				log.Error("Failed to build Node on GetWitness", "err", err, "nodeId", id.String())
				return nil, err
			} else {
				arr = append(arr, node)
			}
		}
	}
	return arr, nil
}

// Getting previous and current and next witnesses
func (c *CandidatePool) GetAllWitness(state *state.StateDB) ([]*discover.Node, []*discover.Node, []*discover.Node, error) {
	log.Debug("Call GetAllWitness ...")
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.initDataByState(state, 0); nil != err {
		log.Error("Failed to initDataByState on GetAllWitness", "err", err)
		return nil, nil, nil, err
	}
	//var ids []discover.NodeID
	var prewitness, witness, nextwitness map[discover.NodeID]*types.Candidate
	prewitness = c.preOriginCandidates
	witness = c.originCandidates
	nextwitness = c.nextOriginCandidates
	// witness index
	var preIndex, curIndex, nextIndex []discover.NodeID

	if ids, err := c.getPreviousWitnessIndex(state); nil != err {
		log.Error("Failed to getPreviousWitnessIndex on GetAllWitness", "err", err)
		return nil, nil, nil, err
	} else {
		preIndex = ids
	}
	if ids, err := c.getWitnessIndex(state); nil != err {
		log.Error("Failed to getWitnessIndex on GetAllWitness", "err", err)
		return nil, nil, nil, err
	} else {
		curIndex = ids
	}
	if ids, err := c.getNextWitnessIndex(state); nil != err {
		log.Error("Failed to getNextWitnessIndex on GetAllWitness", "err", err)
		return nil, nil, nil, err
	} else {
		nextIndex = ids
	}
	preArr, curArr, nextArr := make([]*discover.Node, 0), make([]*discover.Node, 0), make([]*discover.Node, 0)
	for _, id := range preIndex {
		if can, ok := prewitness[id]; ok {
			if node, err := buildWitnessNode(can); nil != err {
				log.Error("Failed to build pre Node on GetAllWitness", "err", err, "nodeId", id.String())
				//continue
				return nil, nil, nil, err
			} else {
				preArr = append(preArr, node)
			}
		}
	}
	for _, id := range curIndex {
		if can, ok := witness[id]; ok {
			if node, err := buildWitnessNode(can); nil != err {
				log.Error("Failed to build cur Node on GetAllWitness", "err", err, "nodeId", id.String())
				//continue
				return nil, nil, nil, err
			} else {
				curArr = append(curArr, node)
			}
		}
	}
	for _, id := range nextIndex {
		if can, ok := nextwitness[id]; ok {
			if node, err := buildWitnessNode(can); nil != err {
				log.Error("Failed to build next Node on GetAllWitness", "err", err, "nodeId", id.String())
				//continue
				return nil, nil, nil, err
			} else {
				nextArr = append(nextArr, node)
			}
		}
	}
	return preArr, curArr, nextArr, nil
}

func (c *CandidatePool) GetRefundInterval() uint64 {
	log.Info("Call GetRefundInterval", "RefundBlockNumber", c.RefundBlockNumber)
	return c.RefundBlockNumber
}

func (c *CandidatePool) checkFirstThreshold(can *types.Candidate) bool {
	if can.Deposit.Cmp(c.threshold) < 0 {
		return false
	}
	return true
}

func (c *CandidatePool) checkDeposit(can *types.Candidate) error {
	if uint64(len(c.immediateCandates)) == c.maxCount {
		last := c.candidateCacheArr[len(c.candidateCacheArr)-1]
		lastDeposit := last.Deposit

		// y = 100 + x
		percentage := new(big.Int).Add(big.NewInt(100), big.NewInt(int64(c.depositLimit)))
		log.Debug("【Compared candidate Deposit】：", "First step:", percentage.String())
		// z = old * y
		tmp := new(big.Int).Mul(lastDeposit, percentage)
		log.Debug("【Compared candidate Deposit】：", "Second step:", tmp.String())
		// z/100 == old * (100 + x) / 100 == old * (y%)
		tmp = new(big.Int).Div(tmp, big.NewInt(100))
		log.Debug("【Compared candidate Deposit】：", "Third step： config file's limit value:", fmt.Sprint(c.depositLimit), " current candidate Deposit ：", can.Deposit.String(), "last's nodeId of Queue:", last.CandidateId.String(), " last's Deposit of Queue:", last.Deposit.String(), " Target Percentage Deposit:", tmp.String())

		if can.Deposit.Cmp(tmp) < 0 {
			log.Error(DepositLowErr.Error(), "depositLimit:", fmt.Sprint(c.depositLimit), "curr Deposit：", can.Deposit.String(), " last Deposit：", last.Deposit.String(), "target Deposit:", tmp.String())
			return DepositLowErr
		}
	}
	return nil
}

func (c *CandidatePool) checkWithdraw(source, price *big.Int) error {
	// y = old * x
	percentage := new(big.Int).Mul(source, big.NewInt(int64(c.depositLimit)))
	log.Debug("【Compared candidate Deposit】：", "First step:", percentage.String())
	// y/100 == old * (x/100) == old * x%
	tmp := new(big.Int).Div(percentage, big.NewInt(100))
	log.Debug("【Compared candidate Deposit】：", "Second step:  config file's limit value:", fmt.Sprint(c.depositLimit), " Current Refund's Origin Deposit:", source.String(), " Current Refund Value：", price.String(), " Target Percentage Deposit:", tmp.String())

	if price.Cmp(tmp) < 0 {
		log.Error(WithdrawLowErr.Error(), "depositLimit:", fmt.Sprint(c.depositLimit), "origin Deposit：", source.String(), " curr price：", price.String(), "target Deposit:", tmp.String())
		return WithdrawLowErr
	}
	return nil
}

func (c *CandidatePool) setImmediate(state vm.StateDB, candidateId discover.NodeID, can *types.Candidate) error {
	c.immediateCandates[candidateId] = can
	if value, err := rlp.EncodeToBytes(can); nil != err {
		log.Error("Failed to encode candidate object on setImmediate", "key", candidateId.String(), "err", err)
		return CandidateEncodeErr
	} else {
		// set immediate candidate input the trie
		setImmediateState(state, candidateId, value)
	}
	return nil
}

func (c *CandidatePool) getImmediateIndex(state vm.StateDB) ([]discover.NodeID, error) {
	return getImmediateIdsByState(state)
}

// deleted immediate candidate by nodeId (Automatically update the index)
func (c *CandidatePool) delImmediate(state vm.StateDB, candidateId discover.NodeID) error {

	// deleted immediate candidate by id on trie
	setImmediateState(state, candidateId, []byte{})
	// deleted immedidate candidate by id on map
	delete(c.immediateCandates, candidateId)
	return nil
}

func (c *CandidatePool) setImmediateIndex(state vm.StateDB, nodeIds []discover.NodeID) error {
	if len(nodeIds) == 0 {
		setImmediateIdsState(state, []byte{})
		return nil
	}
	if val, err := rlp.EncodeToBytes(nodeIds); nil != err {
		log.Error("Failed to encode ImmediateIds", "err", err)
		return err
	} else {
		setImmediateIdsState(state, val)
	}
	return nil
}

// setting refund information
func (c *CandidatePool) setDefeat(state vm.StateDB, candidateId discover.NodeID, can *types.Candidate) error {

	var defeatArr []*types.Candidate
	// append refund information
	if defeatArrTmp, ok := c.defeatCandidates[can.CandidateId]; ok {
		defeatArrTmp = append(defeatArrTmp, can)
		//c.defeatCandidates[can.CandidateId] = defeatArrTmp
		defeatArr = defeatArrTmp
	} else {
		defeatArrTmp = make([]*types.Candidate, 0)
		defeatArrTmp = append(defeatArr, can)
		//c.defeatCandidates[can.CandidateId] = defeatArrTmp
		defeatArr = defeatArrTmp
	}
	// setting refund information on trie
	if value, err := rlp.EncodeToBytes(&defeatArr); nil != err {
		log.Error("Failed to encode candidate object on setDefeat", "key", candidateId.String(), "err", err)
		return CandidateEncodeErr
	} else {
		setDefeatState(state, candidateId, value)
		c.defeatCandidates[can.CandidateId] = defeatArr
	}
	return nil
}

func (c *CandidatePool) delDefeat(state vm.StateDB, nodeId discover.NodeID) error {
	delete(c.defeatCandidates, nodeId)
	setDefeatState(state, nodeId, []byte{})

	return nil
}

// update refund index
func (c *CandidatePool) setDefeatIndex(state vm.StateDB) error {
	newdefeatIds := make([]discover.NodeID, 0)
	indexMap := make(map[string]discover.NodeID, 0)
	index := make([]string, 0)
	for id, _ := range c.defeatCandidates {
		indexMap[id.String()] = id
		index = append(index, id.String())
	}
	// sort id
	sort.Strings(index)

	for _, idStr := range index {
		id := indexMap[idStr]
		newdefeatIds = append(newdefeatIds, id)
	}

	if len(newdefeatIds) == 0 {
		setDefeatIdsState(state, []byte{})
		return nil
	}
	if value, err := rlp.EncodeToBytes(&newdefeatIds); nil != err {
		log.Error("Failed to encode candidate object on setDefeatIds", "err", err)
		return CandidateEncodeErr
	} else {
		setDefeatIdsState(state, value)
	}
	return nil
}

func (c *CandidatePool) delPreviousWitness(state vm.StateDB, candidateId discover.NodeID) error {
	// deleted previous witness by id on map
	delete(c.preOriginCandidates, candidateId)
	// delete previous witness by id on trie
	setPreviousWitnessState(state, candidateId, []byte{})
	return nil
}

func (c *CandidatePool) setPreviousWitness(state vm.StateDB, nodeId discover.NodeID, can *types.Candidate) error {
	c.preOriginCandidates[nodeId] = can
	if val, err := rlp.EncodeToBytes(can); nil != err {
		log.Error("Failed to encode Candidate on setPreviousWitness", "err", err)
		return err
	} else {
		setPreviousWitnessState(state, nodeId, val)
	}
	return nil
}

func (c *CandidatePool) setPreviousWitnessindex(state vm.StateDB, nodeIds []discover.NodeID) error {
	if len(nodeIds) == 0 {
		setPreviosWitnessIdsState(state, []byte{})
		return nil
	}
	if val, err := rlp.EncodeToBytes(nodeIds); nil != err {
		log.Error("Failed to encode Previous WitnessIds", "err", err)
		return err
	} else {
		setPreviosWitnessIdsState(state, val)
	}
	return nil
}

func (c *CandidatePool) getPreviousWitnessIndex(state vm.StateDB) ([]discover.NodeID, error) {
	return getPreviousWitnessIdsState(state)
}

func (c *CandidatePool) setWitness(state vm.StateDB, nodeId discover.NodeID, can *types.Candidate) error {
	c.originCandidates[nodeId] = can
	if val, err := rlp.EncodeToBytes(can); nil != err {
		log.Error("Failed to encode Candidate on setWitness", "err", err)
		return err
	} else {
		setWitnessState(state, nodeId, val)
	}
	return nil
}

func (c *CandidatePool) setWitnessindex(state vm.StateDB, nodeIds []discover.NodeID) error {
	if len(nodeIds) == 0 {
		setWitnessIdsState(state, []byte{})
		return nil
	}
	if val, err := rlp.EncodeToBytes(nodeIds); nil != err {
		log.Error("Failed to encode WitnessIds", "err", err)
		return err
	} else {
		setWitnessIdsState(state, val)
	}
	return nil
}

func (c *CandidatePool) delWitness(state vm.StateDB, candidateId discover.NodeID) error {
	// deleted witness by id on map
	delete(c.originCandidates, candidateId)
	// delete witness by id on trie
	setWitnessState(state, candidateId, []byte{})
	return nil
}

func (c *CandidatePool) getWitnessIndex(state vm.StateDB) ([]discover.NodeID, error) {
	return getWitnessIdsByState(state)
}

func (c *CandidatePool) setNextWitness(state vm.StateDB, nodeId discover.NodeID, can *types.Candidate) error {
	c.nextOriginCandidates[nodeId] = can
	if value, err := rlp.EncodeToBytes(can); nil != err {
		log.Error("Failed to encode candidate object on setImmediate", "key", nodeId.String(), "err", err)
		return CandidateEncodeErr
	} else {
		// setting next witness information on trie
		setNextWitnessState(state, nodeId, value)
	}
	return nil
}

func (c *CandidatePool) delNextWitness(state vm.StateDB, candidateId discover.NodeID) error {
	// deleted next witness by id on map
	delete(c.nextOriginCandidates, candidateId)
	// deleted next witness by id on trie
	setNextWitnessState(state, candidateId, []byte{})

	return nil
}

func (c *CandidatePool) setNextWitnessIndex(state vm.StateDB, nodeIds []discover.NodeID) error {
	if len(nodeIds) == 0 {
		setNextWitnessIdsState(state, []byte{})
		return nil
	}
	if value, err := rlp.EncodeToBytes(&nodeIds); nil != err {
		log.Error("Failed to encode candidate object on setDefeatIds", "err", err)
		return CandidateEncodeErr
	} else {
		setNextWitnessIdsState(state, value)
	}
	return nil
}

func (c *CandidatePool) getNextWitnessIndex(state vm.StateDB) ([]discover.NodeID, error) {
	return getNextWitnessIdsByState(state)
}

func (c *CandidatePool) getCandidate(state vm.StateDB, nodeId discover.NodeID) (*types.Candidate, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	log.Debug("Call Get Candidate ... ")
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on getCandidate", "err", err)
		return nil, err
	}
	if candidatePtr, ok := c.immediateCandates[nodeId]; ok {
		PrintObject("Call GetCandidate return：", *candidatePtr)
		return candidatePtr, nil
	}
	return nil, nil
}

func (c *CandidatePool) getCandidates(state vm.StateDB, nodeIds ...discover.NodeID) (types.CandidateQueue, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	log.Debug("Call Get Candidate Arr ...")
	if err := c.initDataByState(state, 1); nil != err {
		log.Error("Failed to initDataByState on getCandidates", "err", err)
		return nil, err
	}
	canArr := make(types.CandidateQueue, 0)
	tem := make(map[discover.NodeID]struct{}, 0)
	for _, nodeId := range nodeIds {
		if _, ok := tem[nodeId]; ok {
			continue
		}
		if candidatePtr, ok := c.immediateCandates[nodeId]; ok {
			canArr = append(canArr, candidatePtr)
			tem[nodeId] = struct{}{}
		}
	}
	return canArr, nil
}

func (c *CandidatePool) MaxChair() uint64 {
	return c.maxChair
}

func (c *CandidatePool) MaxCount() uint64 {
	return c.maxCount
}

func getPreviousWitnessIdsState(state vm.StateDB) ([]discover.NodeID, error) {
	var witnessIds []discover.NodeID
	//log.Debug("Call getPreviousWitnessIdsState DecodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", PreviousWitnessListKey(), len(PreviousWitnessListKey()), common.Bytes2Hex(PreviousWitnessListKey())))
	if valByte := state.GetState(common.CandidateAddr, PreviousWitnessListKey()); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getPreviousWitnessIdsState DecodeBytes", "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &witnessIds); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return witnessIds, nil
}

func setPreviosWitnessIdsState(state vm.StateDB, arrVal []byte) {
	//log.Debug("SETTING Call  setPreviosWitnessIdsState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", PreviousWitnessListKey(), len(PreviousWitnessListKey()), common.Bytes2Hex(PreviousWitnessListKey())))
	//log.Debug("SETTING Call  setPreviosWitnessIdsState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", arrVal, len(arrVal), common.Bytes2Hex(arrVal)))
	state.SetState(common.CandidateAddr, PreviousWitnessListKey(), arrVal)
}

func getPreviousWitnessByState(state vm.StateDB, id discover.NodeID) (*types.Candidate, error) {
	var can types.Candidate
	//log.Debug("Call getPreviousWitnessByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", PreviousWitnessKey(id), len(PreviousWitnessKey(id)), common.Bytes2Hex(PreviousWitnessKey(id))))
	if valByte := state.GetState(common.CandidateAddr, PreviousWitnessKey(id)); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getPreviousWitnessByState DecodeBytes", "nodeId", id.String(), "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &can); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return &can, nil
}

func setPreviousWitnessState(state vm.StateDB, id discover.NodeID, val []byte) {
	//log.Debug("SETTING Call  setPreviousWitnessState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", PreviousWitnessKey(id), len(PreviousWitnessKey(id)), common.Bytes2Hex(PreviousWitnessKey(id))))
	//log.Debug("SETTING Call  setPreviousWitnessState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", val, len(val), common.Bytes2Hex(val)))
	state.SetState(common.CandidateAddr, PreviousWitnessKey(id), val)
}

func getWitnessIdsByState(state vm.StateDB) ([]discover.NodeID, error) {
	var witnessIds []discover.NodeID
	//log.Debug("Call getWitnessIdsByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", WitnessListKey(), len(WitnessListKey()), common.Bytes2Hex(WitnessListKey())))
	if valByte := state.GetState(common.CandidateAddr, WitnessListKey()); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getWitnessIdsByState DecodeBytes", "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &witnessIds); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return witnessIds, nil
}

func setWitnessIdsState(state vm.StateDB, arrVal []byte) {
	//log.Debug("SETTING Call  setWitnessIdsState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", WitnessListKey(), len(WitnessListKey()), common.Bytes2Hex(WitnessListKey())))
	//log.Debug("SETTING Call  setWitnessIdsState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", arrVal, len(arrVal), common.Bytes2Hex(arrVal)))
	state.SetState(common.CandidateAddr, WitnessListKey(), arrVal)
}

func getWitnessByState(state vm.StateDB, id discover.NodeID) (*types.Candidate, error) {
	var can types.Candidate
	//log.Debug("Call getWitnessByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", WitnessKey(id), len(WitnessKey(id)), common.Bytes2Hex(WitnessKey(id))))
	if valByte := state.GetState(common.CandidateAddr, WitnessKey(id)); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getWitnessByState DecodeBytes", "nodeId", id.String(), "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &can); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return &can, nil
}

func setWitnessState(state vm.StateDB, id discover.NodeID, val []byte) {
	//log.Debug("SETTING Call  setWitnessState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", WitnessKey(id), len(WitnessKey(id)), common.Bytes2Hex(WitnessKey(id))))
	//log.Debug("SETTING Call  setWitnessState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", val, len(val), common.Bytes2Hex(val)))
	state.SetState(common.CandidateAddr, WitnessKey(id), val)
}

func getNextWitnessIdsByState(state vm.StateDB) ([]discover.NodeID, error) {
	var nextWitnessIds []discover.NodeID
	//log.Debug("Call getNextWitnessIdsByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", NextWitnessListKey(), len(NextWitnessListKey()), common.Bytes2Hex(NextWitnessListKey())))
	if valByte := state.GetState(common.CandidateAddr, NextWitnessListKey()); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getNextWitnessIdsByState DecodeBytes", "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &nextWitnessIds); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return nextWitnessIds, nil
}

func setNextWitnessIdsState(state vm.StateDB, arrVal []byte) {
	//log.Debug("SETTING Call  setNextWitnessIdsState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", NextWitnessListKey(), len(NextWitnessListKey()), common.Bytes2Hex(NextWitnessListKey())))
	//log.Debug("SETTING Call  setNextWitnessIdsState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", arrVal, len(arrVal), common.Bytes2Hex(arrVal)))
	state.SetState(common.CandidateAddr, NextWitnessListKey(), arrVal)
}

func getNextWitnessByState(state vm.StateDB, id discover.NodeID) (*types.Candidate, error) {
	var can types.Candidate
	//log.Debug("Call getNextWitnessByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", NextWitnessKey(id), len(NextWitnessKey(id)), common.Bytes2Hex(NextWitnessKey(id))))
	if valByte := state.GetState(common.CandidateAddr, NextWitnessKey(id)); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getNextWitnessByState DecodeBytes", "nodeId", id.String(), "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &can); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return &can, nil
}

func setNextWitnessState(state vm.StateDB, id discover.NodeID, val []byte) {
	//log.Debug("SETTING Call  setNextWitnessState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", NextWitnessKey(id), len(NextWitnessKey(id)), common.Bytes2Hex(NextWitnessKey(id))))
	//log.Debug("SETTING Call  setNextWitnessState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", val, len(val), common.Bytes2Hex(val)))
	state.SetState(common.CandidateAddr, NextWitnessKey(id), val)
}

func getImmediateIdsByState(state vm.StateDB) ([]discover.NodeID, error) {
	var immediateIds []discover.NodeID
	//log.Debug("Call getImmediateIdsByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", ImmediateListKey(), len(ImmediateListKey()), common.Bytes2Hex(ImmediateListKey())))
	if valByte := state.GetState(common.CandidateAddr, ImmediateListKey()); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getImmediateIdsByState DecodeBytes", "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &immediateIds); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return immediateIds, nil
}

func setImmediateIdsState(state vm.StateDB, arrVal []byte) {
	//log.Debug("SETTING Call  setImmediateIdsState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", ImmediateListKey(), len(ImmediateListKey()), common.Bytes2Hex(ImmediateListKey())))
	//log.Debug("SETTING Call  setImmediateIdsState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", arrVal, len(arrVal), common.Bytes2Hex(arrVal)))
	state.SetState(common.CandidateAddr, ImmediateListKey(), arrVal)
}

func getImmediateByState(state vm.StateDB, id discover.NodeID) (*types.Candidate, error) {
	var can types.Candidate
	//log.Debug("Call getImmediateByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", ImmediateKey(id), len(ImmediateKey(id)), common.Bytes2Hex(ImmediateKey(id))))
	if valByte := state.GetState(common.CandidateAddr, ImmediateKey(id)); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getImmediateByState DecodeBytes", "nodeId", id.String(), "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &can); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return &can, nil
}

func setImmediateState(state vm.StateDB, id discover.NodeID, val []byte) {
	//log.Debug("SETTING Call  setImmediateState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", ImmediateKey(id), len(ImmediateKey(id)), common.Bytes2Hex(ImmediateKey(id))))
	//log.Debug("SETTING Call  setImmediateState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", val, len(val), common.Bytes2Hex(val)))
	state.SetState(common.CandidateAddr, ImmediateKey(id), val)
}

func getDefeatIdsByState(state vm.StateDB) ([]discover.NodeID, error) {
	var defeatIds []discover.NodeID
	//log.Debug("Call getDefeatIdsByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", DefeatListKey(), len(DefeatListKey()), common.Bytes2Hex(DefeatListKey())))
	if valByte := state.GetState(common.CandidateAddr, DefeatListKey()); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getDefeatIdsByState DecodeBytes", "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &defeatIds); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return defeatIds, nil
}

func setDefeatIdsState(state vm.StateDB, arrVal []byte) {
	//log.Debug("SETTING Call  setDefeatIdsState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", DefeatListKey(), len(DefeatListKey()), common.Bytes2Hex(DefeatListKey())))
	//log.Debug("SETTING Call  setDefeatIdsState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", arrVal, len(arrVal), common.Bytes2Hex(arrVal)))
	state.SetState(common.CandidateAddr, DefeatListKey(), arrVal)
}

func getDefeatsByState(state vm.StateDB, id discover.NodeID) ([]*types.Candidate, error) {
	var canArr []*types.Candidate
	//log.Debug("Call getDefeatsByState DecodeBytes", "Key's content", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", DefeatKey(id), len(DefeatKey(id)), common.Bytes2Hex(DefeatKey(id))))
	if valByte := state.GetState(common.CandidateAddr, DefeatKey(id)); nil != valByte && len(valByte) != 0 {
		//log.Debug("Call getDefeatsByState DecodeBytes", "nodeId", id.String(), "[]byte context", fmt.Sprintf(" Pointer：%p ,Value：%+v  ,Len：%d,Content' Hash：%v", valByte, valByte, len(valByte), common.Bytes2Hex(valByte)))
		if err := rlp.DecodeBytes(valByte, &canArr); nil != err {
			return nil, err
		}
	} else {
		return nil, nil
	}
	return canArr, nil
}

func setDefeatState(state vm.StateDB, id discover.NodeID, val []byte) {
	//log.Debug("SETTING Call  setDefeatState EncodeBytes", "Key's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", DefeatKey(id), len(DefeatKey(id)), common.Bytes2Hex(DefeatKey(id))))
	//log.Debug("SETTING Call  setDefeatState EncodeBytes", "Value's content:", fmt.Sprintf(" Value：%+v  ,Len：%d,Content' Hash：%v", val, len(val), common.Bytes2Hex(val)))
	state.SetState(common.CandidateAddr, DefeatKey(id), val)
}

func copyCandidateMapByIds(target, source map[discover.NodeID]*types.Candidate, ids []discover.NodeID) {
	for _, id := range ids {
		target[id] = source[id]
	}
}

//func GetCandidatePtr() *CandidatePool {
//	return candidatePool
//}

func PrintObject(s string, obj interface{}) {
	objs, _ := json.Marshal(obj)
	log.Debug(s, "==", string(objs))
}

func buildWitnessNode(can *types.Candidate) (*discover.Node, error) {
	if nil == can {
		return nil, CandidateEmptyErr
	}
	ip := net.ParseIP(can.Host)
	// uint16
	var port uint16
	if portInt, err := strconv.Atoi(can.Port); nil != err {
		return nil, err
	} else {
		port = uint16(portInt)
	}
	return discover.NewNode(can.CandidateId, ip, port, port), nil
}

func compare(c, can *types.Candidate) int {
	// put the larger deposit in front
	if c.Deposit.Cmp(can.Deposit) > 0 {
		return 1
	} else if c.Deposit.Cmp(can.Deposit) == 0 {
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
func candidateSort(arr []*types.Candidate) {
	if len(arr) <= 1 {
		return
	}
	quickSort(arr, 0, len(arr)-1)
}
func quickSort(arr []*types.Candidate, left, right int) {
	if left < right {
		pivot := partition(arr, left, right)
		quickSort(arr, left, pivot-1)
		quickSort(arr, pivot+1, right)
	}
}
func partition(arr []*types.Candidate, left, right int) int {
	for left < right {
		for left < right && compare(arr[left], arr[right]) >= 0 {
			right--
		}
		if left < right {
			arr[left], arr[right] = arr[right], arr[left]
			left++
		}
		for left < right && compare(arr[left], arr[right]) >= 0 {
			left++
		}
		if left < right {
			arr[left], arr[right] = arr[right], arr[left]
			right--
		}
	}
	return left
}

func ImmediateKey(nodeId discover.NodeID) []byte {
	return immediateKey(nodeId.Bytes())
}
func immediateKey(key []byte) []byte {
	return append(append(common.CandidateAddr.Bytes(), ImmediateBtyePrefix...), key...)
}

func PreviousWitnessKey(nodeId discover.NodeID) []byte {
	return prewitnessKey(nodeId.Bytes())
}

func prewitnessKey(key []byte) []byte {
	return append(append(common.CandidateAddr.Bytes(), PreWitnessBytePrefix...), key...)
}

func WitnessKey(nodeId discover.NodeID) []byte {
	return witnessKey(nodeId.Bytes())
}
func witnessKey(key []byte) []byte {
	return append(append(common.CandidateAddr.Bytes(), WitnessBtyePrefix...), key...)
}

func NextWitnessKey(nodeId discover.NodeID) []byte {
	return nextWitnessKey(nodeId.Bytes())
}
func nextWitnessKey(key []byte) []byte {
	return append(append(common.CandidateAddr.Bytes(), NextWitnessBtyePrefix...), key...)
}

func DefeatKey(nodeId discover.NodeID) []byte {
	return defeatKey(nodeId.Bytes())
}
func defeatKey(key []byte) []byte {
	return append(append(common.CandidateAddr.Bytes(), DefeatBtyePrefix...), key...)
}

func ImmediateListKey() []byte {
	return append(common.CandidateAddr.Bytes(), ImmediateListBtyePrefix...)
}

func PreviousWitnessListKey() []byte {
	return append(common.CandidateAddr.Bytes(), PreWitnessListBytePrefix...)
}

func WitnessListKey() []byte {
	return append(common.CandidateAddr.Bytes(), WitnessListBtyePrefix...)
}

func NextWitnessListKey() []byte {
	return append(common.CandidateAddr.Bytes(), NextWitnessListBytePrefix...)
}

func DefeatListKey() []byte {
	return append(common.CandidateAddr.Bytes(), DefeatListBtyePrefix...)
}

// DEBUG
func TraversingStateDB(vmstate vm.StateDB) {
	log.Debug("【TraversingStateDB】start ...")
	/**
	key 1fc17e601a8c10a8c32000358358ecca6001adcb7bdd416fbc397c7bc3e18111
	value a045b3127bdb3fef53e3b1ef09af1be661288cfa7c2b0d09b5f27a6951375d6bc9
	key 6067c3ebcf658f6dfd6bc198dbead3d07c388e30bbcb3fc4b351646527c34010
	value a0725b3ebfd21899f174a09b1eef82e238e22b8c5de7254347fc9f94600e92aac7
	key 98ec7e3bf55c72137281890d90e63bb2d51fc370562c0c386db74359bf1c61af
	value a0b0ca14c4bd15393a0c14209c504886e428c2e4ca82698838e087a404ad985339
	key e8b020f2af4d1e15a9de1c664bdec1fad91737837557afa501aac5a004bd260c
	value a0b3e1f3803d057f97fad44a37599904c03962fb0aac2f0be5d6427c30ecd943b1
	*/
	if stateDB, ok := vmstate.(*state.StateDB); ok {

		/**
		obj
		*/
		/*so := stateDB.GetOrNewStateObject(common.CandidateAddr)
		if so == nil {
			return
		}

		// Otherwise load the valueKey from trie
		enc, err := so.GetCommittedState().getTrie(db).TryGet([]byte(key))
		if err != nil {
			self.setError(err)
			return []byte{}
		}
		if len(enc) > 0 {
			_, content, _, err := rlp.Split(enc)
			if err != nil {
				self.setError(err)
			}
			valueKey.SetBytes(content)

			//load value from db
			value = self.db.trie.GetKey(valueKey.Bytes())
			if err != nil {
				self.setError(err)
			}
		}*/

		objTrie := stateDB.StorageTrie(common.CandidateAddr)
		if objTrie == nil {
			return
		}

		//storageIt := trie.NewIterator(objTrie.NodeIterator(nil))
		//	for storageIt.Next() {
		storageIt := objTrie.NodeIterator(nil)
		for storageIt.Next(true) {
			if storageIt.Leaf() {

				//log.Debug("key", common.Bytes2Hex(storageIt.LeafKey()))
				//k := objTrie.GetKey(storageIt.LeafKey())
				//log.Debug("key context Hash:", string(k))
				//log.Debug("value", common.Bytes2Hex(storageIt.LeafBlob()))

				fmt.Println("key", common.Bytes2Hex(storageIt.LeafKey()))
				k := objTrie.GetKey(storageIt.LeafKey())
				fmt.Println("key context Hash:", string(k))
				fmt.Println("value", common.Bytes2Hex(storageIt.LeafBlob()))

				vl, _ := objTrie.TryGet(storageIt.LeafKey())
				fmt.Println("value2", common.Bytes2Hex(vl))
			}

		}

	}

	//handleFunc := func(valueKey common.Hash, value common.Hash) bool {
	//	log.Debug("【TraversingStateDB】", "key", valueKey, "value", value)
	//	return true
	//}
	//vmstate.ForEachStorage(common.CandidateAddr, handleFunc)

}
