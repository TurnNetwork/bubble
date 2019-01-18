// Package bft implements the BFT consensus engine.
package cbft

import (
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/hexutil"
	"github.com/PlatONnetwork/PlatON-Go/consensus"
	"github.com/PlatONnetwork/PlatON-Go/core"
	"github.com/PlatONnetwork/PlatON-Go/core/cbfttypes"
	"github.com/PlatONnetwork/PlatON-Go/core/ppos"
	"github.com/PlatONnetwork/PlatON-Go/core/state"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/crypto/sha3"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/params"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
	"github.com/PlatONnetwork/PlatON-Go/rpc"
	"bytes"
	"container/list"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math"
	"math/big"
	"sync"
	"time"
)

const (
	former int32 = iota
	current
	next
	all
)

var (
	errSign                = errors.New("sign error")
	errUnauthorizedSigner  = errors.New("unauthorized signer")
	errIllegalBlock        = errors.New("illegal block")
	lateBlock              = errors.New("block is late")
	errDuplicatedBlock     = errors.New("duplicated block")
	errBlockNumber         = errors.New("error block number")
	errUnknownBlock        = errors.New("unknown block")
	errFutileBlock         = errors.New("futile block")
	errGenesisBlock        = errors.New("cannot handle genesis block")
	errHighestLogicalBlock = errors.New("cannot find a logical block")
	errListConfirmedBlocks = errors.New("list confirmed blocks error")
	errMissingSignature    = errors.New("extra-data 65 byte signature suffix missing")
	extraSeal              = 65
	windowSize             = 10

	//periodMargin is a percentum for period margin
	periodMargin = uint64(20)

	//maxPingLatency is the time in milliseconds between Ping and Pong
	maxPingLatency = int64(5000)

	//maxAvgLatency is the time in milliseconds between two peers
	maxAvgLatency = int64(2000)
)

type Cbft struct {
	config                *params.CbftConfig
	ppos                  *ppos
	rotating              *rotating
	blockSignOutCh        chan *cbfttypes.BlockSignature //a channel to send block signature
	cbftResultOutCh       chan *cbfttypes.CbftResult     //a channel to send consensus result
	highestLogicalBlockCh chan *types.Block
	closeOnce             sync.Once
	exitCh                chan struct{}
	txPool                *core.TxPool
	blockExtMap           map[common.Hash]*BlockExt //store all received blocks and signs
	dataReceiveCh         chan interface{}          //a channel to receive data from miner
	blockChain            *core.BlockChain          //the block chain
	highestLogical        *BlockExt                 //highest block in logical path, local packages new block will base on it
	highestConfirmed      *BlockExt                 //highest confirmed block in logical path
	rootIrreversible      *BlockExt                 //the latest block has stored in chain
	signedSet             map[uint64]struct{}       //all block numbers signed by local node
	lock                  sync.RWMutex
	blockChainCache       *core.BlockChainCache
	blockSyncedCh         chan struct{}

	netLatencyMap  map[discover.NodeID]*list.List
	netLatencyLock sync.Mutex
}

var cbft *Cbft

// New creates a concurrent BFT consensus engine
func New(config *params.CbftConfig, blockSignatureCh chan *cbfttypes.BlockSignature, cbftResultCh chan *cbfttypes.CbftResult, highestLogicalBlockCh chan *types.Block) *Cbft {
	pposm.PrintObject("Get ppos config：", *config)
	_ppos := newPpos(/*config.InitialNodes, */config)

	cbft = &Cbft{
		config:                config,
		ppos:                  _ppos,
		rotating:              newRotating(_ppos, config.Duration),
		blockSignOutCh:        blockSignatureCh,
		cbftResultOutCh:       cbftResultCh,
		highestLogicalBlockCh: highestLogicalBlockCh,
		exitCh:                make(chan struct{}),
		blockExtMap:           make(map[common.Hash]*BlockExt),
		signedSet:             make(map[uint64]struct{}),
		dataReceiveCh:         make(chan interface{}, 250),
		blockSyncedCh:         make(chan struct{}),
		netLatencyMap:         make(map[discover.NodeID]*list.List),
	}

	flowControl = NewFlowControl()

	go cbft.dataReceiverLoop()

	return cbft
}

// BlockExt is an extension from Block
type BlockExt struct {
	block       *types.Block
	inTree      bool
	inTurn      bool
	isExecuted  bool
	isSigned    bool
	isConfirmed bool
	number      uint64
	signs       []*common.BlockConfirmSign //all signs for block
	parent      *BlockExt
	children    []*BlockExt
}

// New creates a BlockExt object
func NewBlockExt(block *types.Block, blockNum uint64) *BlockExt {
	return &BlockExt{
		block:  block,
		number: blockNum,
		signs:  make([]*common.BlockConfirmSign, 0),
	}
}

var flowControl *FlowControl

// FlowControl is a rectifier for sequential blocks
type FlowControl struct {
	nodeID      discover.NodeID
	lastTime    int64
	maxInterval int64
	minInterval int64
}

func NewFlowControl() *FlowControl {
	return &FlowControl{
		nodeID:      discover.NodeID{},
		maxInterval: int64(cbft.config.Period*1000 + cbft.config.Period*1000*periodMargin/100),
		minInterval: int64(cbft.config.Period*1000 - cbft.config.Period*1000*periodMargin/100),
	}
}

// control checks if the block is received at a proper rate
func (flowControl *FlowControl) control(nodeID discover.NodeID, curTime int64) bool {
	passed := false
	if flowControl.nodeID == nodeID {
		differ := curTime - flowControl.lastTime
		if differ >= flowControl.minInterval && differ <= flowControl.maxInterval {
			passed = true
		} else {
			passed = false
		}
	} else {
		passed = true
	}
	flowControl.nodeID = nodeID
	flowControl.lastTime = curTime

	return passed
}

// findBlockExt finds BlockExt in cbft.blockExtMap
func (cbft *Cbft) findBlockExt(hash common.Hash) *BlockExt {
	if v, ok := cbft.blockExtMap[hash]; ok {
		return v
	}
	return nil
}

//collectSign collects all signs for a block
func (cbft *Cbft) collectSign(ext *BlockExt, sign *common.BlockConfirmSign) {
	if sign != nil {
		ext.signs = append(ext.signs, sign)
		blockNumber := big.NewInt((int64(ext.number)))
		parentNumber := new(big.Int).Sub(blockNumber, common.Big1)
		//if ext.isLinked && ext.block != nil {
		if ext.inTree { // ext.block != nil is unnecessary
			if len(ext.signs) >= cbft.getThreshold(parentNumber, ext.block.ParentHash(), blockNumber) {
				ext.isConfirmed = true
			}
		}
	}
}

// isParent checks if a block is another's parent
func (parent *BlockExt) isParent(child *types.Block) bool {
	if parent.block != nil && parent.block.NumberU64()+1 == child.NumberU64() && parent.block.Hash() == child.ParentHash() {
		return true
	}
	return false
}

// findParent finds ext's parent with non-nil block
func (cbft *Cbft) findParent(ext *BlockExt) *BlockExt {
	if ext.block == nil {
		return nil
	}
	parent := cbft.findBlockExt(ext.block.ParentHash())
	if parent != nil {
		if parent.block == nil {
			log.Warn("parent block has not received")
		} else if parent.block.NumberU64()+1 == ext.block.NumberU64() {
			return parent
		} else {
			log.Warn("data error, parent block hash is not mapping to number")
		}
	}
	return nil
}

// collectTxs collects exts's transactions
func (cbft *Cbft) collectTxs(exts []*BlockExt) types.Transactions {
	txs := make([]*types.Transaction, 0)
	for _, ext := range exts {
		copy(txs, ext.block.Transactions())
	}
	return types.Transactions(txs)
}

// findChildren finds current blockExt's all children with non-nil block
func (cbft *Cbft) findChildren(parent *BlockExt) []*BlockExt {
	if parent.block == nil {
		return nil
	}
	children := make([]*BlockExt, 0)

	for _, child := range cbft.blockExtMap {
		if child.block != nil && child.block.ParentHash() == parent.block.Hash() {
			if child.block.NumberU64()-1 == parent.block.NumberU64() {
				children = append(children, child)
			} else {
				log.Warn("data error, child block hash is not mapping to number")
			}
		}
	}

	if len(children) == 0 {
		return nil
	} else {
		return children
	}
}

// saveBlockExt saves block in memory
func (cbft *Cbft) saveBlockExt(hash common.Hash, ext *BlockExt) {
	cbft.blockExtMap[hash] = ext
	log.Debug("save block in memory", "RoutineID", common.CurrentGoRoutineID(), "hash", hash, "number", ext.number, "totalBlocks", len(cbft.blockExtMap))
}

// isAncestor checks if a block is another's ancestor
func (lower *BlockExt) isAncestor(higher *BlockExt) bool {

	if higher == nil || higher.block == nil || lower == nil || lower.block == nil {
		return false
	}
	generations := higher.block.NumberU64() - lower.block.NumberU64()
	if generations <= 0 {
		return false
	}

	for i := uint64(0); i < generations; i++ {
		parent := higher.parent
		if parent != nil {
			higher = parent
		} else {
			return false
		}
	}

	if lower.block.Hash() == higher.block.Hash() && lower.block.NumberU64() == higher.block.NumberU64() {
		return true
	}
	return false
}

// findHighest finds the highest block from current start; If there are multiple highest blockExts, return the one that has most signs
func (cbft *Cbft) findHighest(current *BlockExt) *BlockExt {
	highest := current
	for _, child := range current.children {
		current := cbft.findHighest(child)
		if current.block.NumberU64() > highest.block.NumberU64() || (current.block.NumberU64() == highest.block.NumberU64() && len(current.signs) > len(highest.signs)) {
			highest = current
		}
	}
	return highest
}

// findHighestLogical finds a logical path and return the highest block.
// the precondition is cur is a logical block, so, findHighestLogical will return cur if the path only has one block.
func (cbft *Cbft) findHighestLogical(cur *BlockExt) *BlockExt {
	lastClosestConfirmed := cbft.findLastClosestConfirmedIncludingSelf(cur)
	if lastClosestConfirmed == nil {
		return cbft.findHighest(cur)
	} else {
		return cbft.findHighest(lastClosestConfirmed)
	}
}

// findLastClosestConfirmedIncludingSelf return the last found block by call findClosestConfirmedExcludingSelf in a circular manner
func (cbft *Cbft) findLastClosestConfirmedIncludingSelf(cur *BlockExt) *BlockExt {
	var lastClosestConfirmed *BlockExt
	for {
		lastClosestConfirmed = cbft.findClosestConfirmedExcludingSelf(cur)
		if lastClosestConfirmed == nil || lastClosestConfirmed.block.Hash() == cur.block.Hash() {
			break
		} else {
			cur = lastClosestConfirmed
		}
	}
	if lastClosestConfirmed != nil {
		return lastClosestConfirmed
	} else if cur.isConfirmed {
		return cur
	} else {
		return nil
	}
}

// findClosestConfirmedIncludingSelf returns the closest confirmed block in current's descendant (including current itself).
// return nil if there's no confirmed in current's descendant.
func (cbft *Cbft) findClosestConfirmedIncludingSelf(current *BlockExt) *BlockExt {
	closest := current
	if current.inTree && current.isExecuted && !current.isConfirmed {
		closest = nil
	}
	for _, child := range current.children {
		temp := cbft.findClosestConfirmedIncludingSelf(child)
		if closest == nil || (temp != nil && temp.inTree && temp.isExecuted && temp.isConfirmed && temp.number < closest.number) {
			closest = temp
		}
	}
	return closest
}

// findClosestConfirmedExcludingSelf returns the closest confirmed block in current's descendant (excluding current itself).
// return nil if there's no confirmed in current's descendant.
func (cbft *Cbft) findClosestConfirmedExcludingSelf(current *BlockExt) *BlockExt {
	var closest *BlockExt
	for _, child := range current.children {
		if child != nil && child.inTree && child.isExecuted && child.isConfirmed {
			return child
		} else {
			temp := cbft.findClosestConfirmedIncludingSelf(child)
			if closest == nil || (temp != nil && temp.number < closest.number) {
				closest = temp
			}
		}
	}
	return closest
}

// signLogicalAndDescendant signs logical block go along the logical path from current block, and will not sign the block if there's another same number block has been signed.
func (cbft *Cbft) signLogicalAndDescendant(current *BlockExt) {
	log.Debug("sign logical block and its descendant", "RoutineID", common.CurrentGoRoutineID(), "hash", current.block.Hash(), "number", current.block.NumberU64())
	highestLogical := cbft.findHighestLogical(current)

	logicalBlocks := cbft.backTrackBlocks(highestLogical, current, true)

	//var highestConfirmed *BlockExt
	for _, logical := range logicalBlocks {
		if logical.inTurn && !logical.isSigned {
			if _, signed := cbft.signedSet[logical.block.NumberU64()]; !signed {
				cbft.sign(logical)
				log.Debug("reset TxPool after block signed", "RoutineID", common.CurrentGoRoutineID(), "hash", logical.block.Hash(), "number", logical.number)
				cbft.txPool.Reset(logical.block)
			}
		}

		/*if logical.isConfirmed {
			highestConfirmed = logical
		}*/
	}
	/*log.Trace("reset highest logical", "hash", highestLogical.block.Hash(), "number", highestLogical.block.NumberU64())
	cbft.setHighestLogical(highestLogical)
	if highestConfirmed != nil {
		log.Trace("reset highest confirmed", "hash", highestConfirmed.block.Hash(), "number", highestConfirmed.block.NumberU64())
		cbft.highestConfirmed = highestConfirmed
	}*/
}

// executeBlockAndDescendant executes the block's transactions and its descendant
func (cbft *Cbft) executeBlockAndDescendant(current *BlockExt, parent *BlockExt) error {
	log.Debug("execute block", "RoutineID", common.CurrentGoRoutineID(), "hash", current.block.Hash(), "number", current.block.NumberU64())
	if !current.isExecuted {
		if err := cbft.execute(current, parent); err != nil {
			current.inTree = false
			current.isExecuted = false
			//remove bad block from tree and map
			cbft.removeBadBlock(current)
			log.Error("execute block error", "hash", current.block.Hash(), "number", current.block.NumberU64())
			return errors.New("execute block error")
		} else {
			current.inTree = true
			current.isExecuted = true
		}
	}

	for _, child := range current.children {
		if err := cbft.executeBlockAndDescendant(child, current); err != nil {
			//remove bad block from tree and map
			cbft.removeBadBlock(child)
			return err
		}
	}
	return nil
}

// sign signs a block
func (cbft *Cbft) sign(ext *BlockExt) {
	sealHash := sealHash(ext.block.Header())
	if signature, err := cbft.signFn(sealHash.Bytes()); err == nil {
		log.Debug("Sign block ", "RoutineID", common.CurrentGoRoutineID(), "hash", ext.block.Hash(), "number", ext.block.NumberU64(), "sealHash", sealHash, "signature", hexutil.Encode(signature[:8]))

		sign := common.NewBlockConfirmSign(signature)
		ext.isSigned = true

		cbft.collectSign(ext, sign)

		//save this block number
		cbft.signedSet[ext.block.NumberU64()] = struct{}{}

		blockHash := ext.block.Hash()

		//send the BlockSignature to channel
		blockSign := &cbfttypes.BlockSignature{
			SignHash:  sealHash,
			Hash:      blockHash,
			Number:    ext.block.Number(),
			Signature: sign,
			ParentHash: ext.block.ParentHash(),
		}
		cbft.blockSignOutCh <- blockSign
	} else {
		panic("sign block fatal error")
	}
}

// execute executes the block's transactions based on its parent
// if success then save the receipts and state to consensusCache
func (cbft *Cbft) execute(ext *BlockExt, parent *BlockExt) error {
	state, err := cbft.blockChainCache.MakeStateDB(parent.block)

	if err != nil {
		log.Error("execute block error, cannot make state based on parent", "hash", ext.block.Hash(), "Number", ext.block.NumberU64(), "ParentHash", parent.block.Hash(), "err", err)
		return errors.New("execute block error")
	}

	//to execute
	receipts, err := cbft.blockChain.ProcessDirectly(ext.block, state, parent.block)
	if err == nil {
		//save the receipts and state to consensusCache
		stateIsNil := state == nil
		log.Debug("execute block success", "hash", ext.block.Hash(), "number", ext.block.NumberU64(), "ParentHash", parent.block.Hash(), "lenReceipts", len(receipts), "stateIsNil", stateIsNil, "root", ext.block.Root())
		sealHash := sealHash(ext.block.Header())
		cbft.blockChainCache.WriteReceipts(sealHash, receipts, ext.block.NumberU64())
		cbft.blockChainCache.WriteStateDB(sealHash, state, ext.block.NumberU64())
	} else {
		log.Error("execute block error", "hash", ext.block.Hash(), "number", ext.block.NumberU64(), "ParentHash", parent.block.Hash(), "err", err)
		return errors.New("execute block error")
	}
	return nil
}

// backTrackBlocks return blocks from start to end, these blocks are in a same tree branch.
// The result is sorted by block number from lower to higher.
func (cbft *Cbft) backTrackBlocks(start *BlockExt, end *BlockExt, includeEnd bool) []*BlockExt {
	log.Trace("back track blocks", "RoutineID", common.CurrentGoRoutineID(), "startHash", start.block.Hash(), "startParentHash", end.block.ParentHash(), "endHash", start.block.Hash())

	result := make([]*BlockExt, 0)

	if start.block.Hash() == end.block.Hash() && includeEnd {
		result = append(result, start)
	} else if start.block.NumberU64() > end.block.NumberU64() {
		found := false
		result = append(result, start)

		for {
			parent := start.parent
			if parent == nil {
				break
			} else if parent.block.Hash() == end.block.Hash() && parent.block.NumberU64() == end.block.NumberU64() {
				//log.Debug("ending of back track block ")
				if includeEnd {
					result = append(result, parent)
				}
				found = true
				break
			} else {
				//log.Debug("found new block", "hash", parent.block.Hash(), "ParentHash", parent.block.ParentHash(), "number", parent.block.NumberU64())
				result = append(result, parent)
				start = parent
			}
		}

		if found {
			//sorted by block number from lower to higher
			if len(result) > 1 {
				reverse(result)
			}
		} else {
			result = nil
		}
	}

	for _, logical := range result {
		log.Debug("found new block", "hash", logical.block.Hash(), "number", logical.block.NumberU64())
	}

	return result
}

func reverse(s []*BlockExt) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// SetPrivateKey sets local's private key by the backend.go
func (cbft *Cbft) SetPrivateKey(privateKey *ecdsa.PrivateKey) {
	cbft.config.PrivateKey = privateKey
	cbft.config.NodeID = discover.PubkeyID(&privateKey.PublicKey)
}

func SetBlockChainCache(blockChainCache *core.BlockChainCache) {
	cbft.blockChainCache = blockChainCache
}

// setHighestLogical sets highest logical block and send it to the highestLogicalBlockCh
func (cbft *Cbft) setHighestLogical(highestLogical *BlockExt) {
	cbft.highestLogical = highestLogical
	cbft.highestLogicalBlockCh <- highestLogical.block
}

// SetBackend sets blockChain and txPool into cbft
func SetBackend(blockChain *core.BlockChain, txPool *core.TxPool) {
	log.Debug("call SetBackend()")

	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	cbft.blockChain = blockChain
	cbft.ppos.SetStartTimeOfEpoch(blockChain.Genesis().Time().Int64())

	currentBlock := blockChain.CurrentBlock()

	genesisParentHash := bytes.Repeat([]byte{0x00}, 32)
	if bytes.Equal(currentBlock.ParentHash().Bytes(), genesisParentHash) && currentBlock.Number() == nil {
		currentBlock.Header().Number = big.NewInt(0)
	}

	log.Debug("init cbft.highestLogicalBlock", "hash", currentBlock.Hash(), "number", currentBlock.NumberU64())

	current := NewBlockExt(currentBlock, currentBlock.NumberU64())
	current.inTree = true
	current.isExecuted = true
	current.isSigned = true
	current.isConfirmed = true
	current.number = currentBlock.NumberU64()

	cbft.saveBlockExt(currentBlock.Hash(), current)

	cbft.highestConfirmed = current

	//cbft.highestLogical = current
	cbft.setHighestLogical(current)

	cbft.rootIrreversible = current

	cbft.txPool = txPool
}

func SetDopsOption(blockChain *core.BlockChain) {
	cbft.ppos.SetCandidateContextOption(blockChain, cbft.config.InitialNodes)
}

// BlockSynchronisation reset the cbft env, such as cbft.highestLogical, cbft.highestConfirmed.
// This function is invoked after that local has synced new blocks from other node.
func (cbft *Cbft) OnBlockSynced() {
	log.Debug("call OnBlockSynced(）", "GoRoutineID", common.CurrentGoRoutineID())
	cbft.dataReceiveCh <- &cbfttypes.BlockSynced{}
}

func (cbft *Cbft) blockSynced() {
	log.Debug("=== call blockSynced() ===\n",
		"GoRoutineID", common.CurrentGoRoutineID(),
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)

	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	currentBlock := cbft.blockChain.CurrentBlock()

	if currentBlock.NumberU64() > cbft.rootIrreversible.number {
		log.Debug("chain has a higher irreversible block", "hash", currentBlock.Hash(), "number", currentBlock.NumberU64())

		newRoot := NewBlockExt(currentBlock, currentBlock.NumberU64())
		newRoot.inTree = true
		newRoot.isExecuted = true
		newRoot.isSigned = true
		newRoot.isConfirmed = true
		newRoot.number = currentBlock.NumberU64()
		newRoot.parent = nil

		//save the root in BlockExtMap
		cbft.saveBlockExt(newRoot.block.Hash(), newRoot)

		//reset the new root irreversible
		cbft.rootIrreversible = newRoot
		log.Debug("cbft.rootIrreversible", "hash", cbft.rootIrreversible.block.Hash(), "number", cbft.rootIrreversible.block.NumberU64())

		//reorg the block tree
		cbft.buildChildNode(newRoot)

		//the new root's children should re-execute base on new state
		for _, child := range newRoot.children {
			if err := cbft.executeBlockAndDescendant(child, newRoot); err != nil {
				//remove bad block from tree and map
				cbft.removeBadBlock(child)
				log.Error("execute the block error, remove it", "err", err)
				break
			}
		}

		//there are some redundancy code for newRoot, but these codes are necessary for other logical blocks
		cbft.signLogicalAndDescendant(newRoot)

		//reset logical path
		highestLogical := cbft.findHighestLogical(newRoot)
		cbft.setHighestLogical(highestLogical)

		//reset highest confirmed block
		cbft.highestConfirmed = cbft.findLastClosestConfirmedIncludingSelf(newRoot)

		if cbft.highestConfirmed != nil {
			log.Debug("cbft.highestConfirmed", "hash", newRoot.block.Hash(), "number", newRoot.block.NumberU64())
		} else {
			log.Debug("cbft.highestConfirmed is null")
		}

		if !cbft.flushReadyBlock() {
			//remove all other blocks those their numbers are too low
			cbft.cleanByNumber(cbft.rootIrreversible.number)
		}

		log.Debug("reset TxPool after block synced", "RoutineID", common.CurrentGoRoutineID(), "hash", newRoot.block.Hash(), "number", newRoot.number)
		cbft.txPool.Reset(currentBlock)
	}

	log.Debug("=== end of blockSynced() ===\n",
		"RoutineID", common.CurrentGoRoutineID(),
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)
}

// dataReceiverLoop is the main loop that handle the data from worker, or eth protocol's handler
// the new blocks packed by local in worker will be handled here; the other blocks and signs received by P2P will be handled here.
func (cbft *Cbft) dataReceiverLoop() {
	for {
		select {
		case v := <-cbft.dataReceiveCh:
			sign, ok := v.(*cbfttypes.BlockSignature)
			if ok {
				err := cbft.signReceiver(sign)
				if err != nil {
					log.Error("Error", "msg", err)
				}
			} else {
				block, ok := v.(*types.Block)
				if ok {
					err := cbft.blockReceiver(block)
					if err != nil {
						log.Error("Error", "msg", err)
					}
				} else {
					_, ok := v.(*cbfttypes.BlockSynced)
					if ok {
						cbft.blockSynced()
					} else {
						log.Error("Received wrong data type")
					}
				}
			}
		case <-cbft.exitCh:
			return
		}
	}
}

// buildIntoTree inserts current BlockExt to the tree structure
func (cbft *Cbft) buildIntoTree(current *BlockExt) {
	parent := cbft.findParent(current)
	if parent != nil {
		//catch up with parent
		parent.children = append(parent.children, current)
		current.parent = parent
		current.inTree = parent.inTree
	} else {
		log.Warn("cannot find parent block", "hash", current.block.Hash(), "number", current.block.NumberU64())
	}

	cbft.buildChildNode(current)
}

func (cbft *Cbft) buildChildNode(current *BlockExt) {
	children := cbft.findChildren(current)
	if len(children) > 0 {
		current.children = children
		for _, child := range children {
			//child should catch up with current
			child.parent = current
		}
		cbft.setDescendantInTree(current)
	}
}

func (cbft *Cbft) setDescendantInTree(child *BlockExt) {
	log.Debug("set descendant inTree attribute", "RoutineID", common.CurrentGoRoutineID(), "hash", child.block.Hash(), "number", child.number)
	for _, grandchild := range child.children {
		grandchild.inTree = child.inTree
		cbft.setDescendantInTree(grandchild)
	}
}

// removeBadBlock removes bad block executed error from the tree structure and cbft.blockExtMap.
func (cbft *Cbft) removeBadBlock(badBlock *BlockExt) {
	tailorTree(badBlock)
	for _, child := range badBlock.children {
		child.parent = nil
	}
	delete(cbft.blockExtMap, badBlock.block.Hash())
}

// signReceiver handles the received block signature
func (cbft *Cbft) signReceiver(sig *cbfttypes.BlockSignature) error {
	log.Debug("=== call signReceiver() ===\n",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", sig.Hash,
		"number", sig.Number.Uint64(),
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)
	cbft.lock.Lock()
	defer cbft.lock.Unlock()
	if sig.Number.Uint64() <= cbft.rootIrreversible.number {
		log.Warn("block sign is too late")
		return nil
	}

	current := cbft.findBlockExt(sig.Hash)
	if current == nil {
		log.Warn("have not received the corresponding block")
		//the block is nil
		current = NewBlockExt(nil, sig.Number.Uint64())
		current.inTree = false
		current.isExecuted = false
		current.isSigned = false
		current.isConfirmed = false

		cbft.saveBlockExt(sig.Hash, current)
	}

	cbft.collectSign(current, sig.Signature)

	var hashLog interface{}
	if current.block != nil {
		hashLog = current.block.Hash()
	} else {
		hashLog = "hash is nil"
	}

	log.Debug("count signatures",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", hashLog,
		"number", current.number,
		"signCount", len(current.signs),
		"inTree", current.inTree,
		"isExecuted", current.isExecuted,
		"isConfirmed", current.isConfirmed,
		"isSigned", current.isSigned)

	if current.inTree && current.isConfirmed {
		//the current is new highestConfirmed on the same logical path
		if current.number > cbft.highestConfirmed.number && cbft.highestConfirmed.isAncestor(current) {
			cbft.highestConfirmed = current
			newHighestLogical := cbft.findHighestLogical(current)
			cbft.setHighestLogical(newHighestLogical)
		} else if current.number < cbft.highestConfirmed.number && !current.isAncestor(cbft.highestConfirmed) {
			//only this case may cause a new fork
			cbft.checkFork(current)
		}
		cbft.flushReadyBlock()
	}

	log.Debug("=== end of signReceiver()  ===\n",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", hashLog,
		"number", current.number,
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)
	return nil
}

//blockReceiver handles the new block
func (cbft *Cbft) blockReceiver(block *types.Block) error {
	log.Debug("=== call blockReceiver() ===\n",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", block.Hash(),
		"number", block.NumberU64(),
		"parentHash", block.ParentHash(),
		"ReceiptHash", block.ReceiptHash(),
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)

	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	if block.NumberU64() <= 0 {
		return errGenesisBlock
	}

	if block.NumberU64() <= cbft.rootIrreversible.number {
		return lateBlock
	}

	//recover the producer's NodeID
	producerID, sign, err := ecrecover(block.Header())
	if err != nil {
		return err
	}

	curTime := toMilliseconds(time.Now())

	// TODO
	//isLegal := cbft.isLegal(curTime, producerID)
	blockNumber := block.Number()
	parentNumber := new(big.Int).Sub(blockNumber, common.Big1)
	isLegal := cbft.isLegal(parentNumber, block.ParentHash(), blockNumber, curTime, producerID)
	if !isLegal {
		log.Warn("illegal block",
			"RoutineID", common.CurrentGoRoutineID(),
			"hash", block.Hash(),
			"number", block.NumberU64(),
			"parentHash", block.ParentHash(),
			"curTime", curTime,
			"producerID", producerID)
		return errIllegalBlock
	}

	//to check if there's a existing blockExt for received block
	//sometime we'll receive the block's sign before the block self.
	ext := cbft.findBlockExt(block.Hash())
	if ext == nil {
		ext = NewBlockExt(block, block.NumberU64())
		//default
		ext.inTree = false
		ext.isExecuted = false
		ext.isSigned = false
		ext.isConfirmed = false

		cbft.saveBlockExt(block.Hash(), ext)

	} else if ext.block == nil {
		//received its sign before.
		ext.block = block
	} else {
		return errDuplicatedBlock
	}

	//make tree node
	cbft.buildIntoTree(ext)

	//collect the block's sign of producer
	cbft.collectSign(ext, common.NewBlockConfirmSign(sign))

	log.Debug("count signatures",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", ext.block.Hash(),
		"number", ext.number,
		"signCount", len(ext.signs),
		"inTree", ext.inTree,
		"isExecuted", ext.isExecuted,
		"isConfirmed", ext.isConfirmed,
		"isSigned", ext.isSigned)

	if ext.inTree {
		if err := cbft.executeBlockAndDescendant(ext, ext.parent); err != nil {
			return err
		}

		// TODO
		//inTurn := cbft.inTurnVerify(curTime, producerID)
		blockNumber := block.Number()
		parentNumber := new(big.Int).Sub(blockNumber, common.Big1)
		inTurn := cbft.inTurnVerify(parentNumber, block.ParentHash(), blockNumber, curTime, producerID)
		if !inTurn {
			log.Warn("not in turn",
				"RoutineID", common.CurrentGoRoutineID(),
				"hash", block.Hash(),
				"number", block.NumberU64(),
				"parentHash", block.ParentHash(),
				"curTime", curTime,
				"producerID", producerID)
		}

		ext.inTurn = inTurn

		//flowControl := flowControl.control(producerID, curTime)
		flowControl := true
		highestConfirmedIsAncestor := cbft.highestConfirmed.isAncestor(ext)

		isLogical := inTurn && flowControl && highestConfirmedIsAncestor

		log.Debug("check if block is logical", "RoutineID", common.CurrentGoRoutineID(), "result", isLogical, "hash", ext.block.Hash(), "number", ext.number, "inTurn", inTurn, "flowControl", flowControl, "highestConfirmedIsAncestor", highestConfirmedIsAncestor)

		/*
			if isLogical {
				cbft.signLogicalAndDescendant(ext)
			}
			//rearrange logical path from cbft.rootIrreversible each time
			newHighestLogical := cbft.findHighestLogical(cbft.rootIrreversible)
			if newHighestLogical != nil {
				cbft.setHighestLogical(newHighestLogical)
			}

			newHighestConfirmed := cbft.findLastClosestConfirmedIncludingSelf(cbft.rootIrreversible)
			if newHighestConfirmed != nil && newHighestConfirmed.block.Hash() != cbft.highestConfirmed.block.Hash() {
				//fork
				if newHighestConfirmed.number < cbft.highestConfirmed.number  ||  (newHighestConfirmed.number > cbft.highestConfirmed.number && !cbft.highestConfirmed.isAncestor(newHighestConfirmed)) {
					//only this case may cause a new fork
					cbft.checkFork(closestConfirmed)
				}

				cbft.highestConfirmed = newHighestConfirmed
			}
		*/

		if isLogical {
			cbft.signLogicalAndDescendant(ext)
			//rearrange logical path
			newHighestLogical := cbft.findHighestLogical(cbft.highestConfirmed)
			if newHighestLogical != nil {
				cbft.setHighestLogical(newHighestLogical)
			}

			newHighestConfirmed := cbft.findLastClosestConfirmedIncludingSelf(cbft.highestConfirmed)
			if newHighestConfirmed != nil {
				cbft.highestConfirmed = newHighestConfirmed
			}
		} else {
			closestConfirmed := cbft.findClosestConfirmedIncludingSelf(ext)

			//if closestConfirmed != nil && closestConfirmed.number < cbft.highestConfirmed.number && !closestConfirmed.isAncestor(cbft.highestConfirmed){
			if closestConfirmed != nil && closestConfirmed.number < cbft.highestConfirmed.number {
				//only this case may cause a new fork
				cbft.checkFork(closestConfirmed)
			}
		}

		cbft.flushReadyBlock()
	}
	log.Debug("=== end of blockReceiver() ===\n",
		"RoutineID", common.CurrentGoRoutineID(),
		"hash", block.Hash(),
		"number", block.NumberU64(),
		"parentHash", block.ParentHash(),
		"ReceiptHash", block.ReceiptHash(),
		"highestLogicalHash", cbft.highestLogical.block.Hash(),
		"highestLogicalNumber", cbft.highestLogical.number,
		"highestConfirmedHash", cbft.highestConfirmed.block.Hash(),
		"highestConfirmedNumber", cbft.highestConfirmed.number,
		"rootIrreversibleHash", cbft.rootIrreversible.block.Hash(),
		"rootIrreversibleNumber", cbft.rootIrreversible.number)
	return nil
}

// forked returns the blocks forked from original branch
// original[0] == newFork[0] == cbft.rootIrreversible, len(origPath) > len(newPath)
func (cbft *Cbft) forked(origPath []*BlockExt, newPath []*BlockExt) (oldTress, newTress []*BlockExt) {
	for i := 0; i < len(newPath); i++ {
		if newPath[i].block.Hash() != origPath[i].block.Hash() {
			return origPath[i:], newPath[i:]
		}
	}
	return nil, nil
}

func extraBlocks(exts []*BlockExt) []*types.Block {
	blocks := make([]*types.Block, len(exts))
	for idx, ext := range exts {
		blocks[idx] = ext.block
	}
	return blocks
}

// checkFork checks if the logical path is changed cause the newConfirmed, if changed, this is a new fork.
func (cbft *Cbft) checkFork(newConfirmed *BlockExt) {
	newHighestConfirmed := cbft.findLastClosestConfirmedIncludingSelf(newConfirmed)
	if newHighestConfirmed != nil && newHighestConfirmed.block.Hash() != cbft.highestConfirmed.block.Hash() {
		//fork
		log.Debug("the block chain in memory forked", "RoutineID", common.CurrentGoRoutineID(), "newHighestConfirmedHash", newHighestConfirmed.block.Hash(), "newHighestConfirmedNumber", newHighestConfirmed.number)
		newHighestLogical := cbft.findHighestLogical(newHighestConfirmed)
		newPath := cbft.backTrackBlocks(newHighestLogical, cbft.rootIrreversible, true)

		origPath := cbft.backTrackBlocks(cbft.highestLogical, cbft.rootIrreversible, true)

		oldTress, newTress := cbft.forked(origPath, newPath)

		cbft.txPool.ForkedReset(extraBlocks(oldTress), extraBlocks(newTress))

		//forkFrom to lower block
		cbft.highestConfirmed = newHighestConfirmed
		cbft.setHighestLogical(newHighestLogical)
		log.Warn("chain is forked")
	}
}

// flushReadyBlock finds ready blocks and flush them to chain
func (cbft *Cbft) flushReadyBlock() bool {
	log.Debug("check if there's any block ready to flush to chain", "RoutineID", common.CurrentGoRoutineID(), "highestConfirmedNumber", cbft.highestConfirmed.number, "rootIrreversibleNumber", cbft.rootIrreversible.number)

	fallCount := int(cbft.highestConfirmed.number - cbft.rootIrreversible.number)
	var newRoot *BlockExt
	if fallCount == 1 && cbft.rootIrreversible.isParent(cbft.highestConfirmed.block) {
		cbft.storeBlocks([]*BlockExt{cbft.highestConfirmed})
		newRoot = cbft.highestConfirmed
	} else if fallCount > windowSize {
		//find the completed path from root to highest logical
		logicalBlocks := cbft.backTrackBlocks(cbft.highestConfirmed, cbft.rootIrreversible, false)
		total := len(logicalBlocks)
		toFlushs := logicalBlocks[:total-windowSize]

		logicalBlocks = logicalBlocks[total-windowSize:]

		for _, confirmed := range logicalBlocks {
			if confirmed.isConfirmed {
				toFlushs = append(toFlushs, confirmed)
			} else {
				break
			}
		}

		cbft.storeBlocks(toFlushs)

		for _, confirmed := range toFlushs {
			log.Debug("blocks should be flushed to chain  ", "RoutineID", common.CurrentGoRoutineID(), "hash", confirmed.block.Hash(), "number", confirmed.number)
		}

		newRoot = toFlushs[len(toFlushs)-1]
	}
	if newRoot != nil {
		// blocks[0] == cbft.rootIrreversible
		oldRoot := cbft.rootIrreversible
		log.Debug("blockExt tree reorged, root info", "RoutineID", common.CurrentGoRoutineID(), "origHash", oldRoot.block.Hash(), "origNumber", oldRoot.number, "newHash", newRoot.block.Hash(), "newNumber", newRoot.number)
		//cut off old tree from new root,
		tailorTree(newRoot)

		//set the new root as cbft.rootIrreversible
		cbft.rootIrreversible = newRoot

		//remove all blocks referenced in old tree after being cut off
		cbft.cleanByTailoredTree(oldRoot)

		//remove all other blocks those their numbers are too low
		cbft.cleanByNumber(cbft.rootIrreversible.number)
		return true
	}
	return false

	/*if exceededCount := cbft.highestConfirmed.number - cbft.rootIrreversible.number; exceededCount > 0 {
		//find the completed path from root to highest logical
		logicalBlocks := cbft.backTrackBlocks(cbft.highestConfirmed, cbft.rootIrreversible, false)

		total := len(logicalBlocks)

		var newRoot *BlockExt

		if total > 20 {
			forced := logicalBlocks[:total-20]
			log.Warn("force to flush blocks to chain", "blockCount", len(forced))

			cbft.storeBlocks(forced)

			newRoot = forced[len(forced)-1]
			logicalBlocks = logicalBlocks[total-20:]
		}

		count := 0
		for _, confirmed := range logicalBlocks {
			if confirmed.isConfirmed {
				newRoot = confirmed
				log.Debug("find confirmed block that can be flushed to chain  ", "hash", newRoot.block.Hash(), "number", newRoot.number)
				count++
			} else {
				break
			}
		}
		if count > 0 {
			cbft.storeBlocks(logicalBlocks[:count])
		}
		if newRoot != nil {
			// blocks[0] == cbft.rootIrreversible
			oldRoot := cbft.rootIrreversible
			log.Debug("oldRoot", "hash", oldRoot.block.Hash(), "number", oldRoot.number)
			log.Debug("newRoot", "hash", newRoot.block.Hash(), "number", newRoot.number)
			//cut off old tree from new root,
			tailorTree(newRoot)

			//set the new root as cbft.rootIrreversible
			cbft.rootIrreversible = newRoot

			//remove all blocks referenced in old tree after being cut off
			cbft.cleanByTailoredTree(oldRoot)

			//remove all other blocks those their numbers are too low
			cbft.cleanByNumber(cbft.rootIrreversible.number)
		}
	}*/
}

// tailorTree tailors the old tree from new root
func tailorTree(newRoot *BlockExt) {
	for i := 0; i < len(newRoot.parent.children); i++ {
		//remove newRoot from its parent's children list
		if newRoot.parent.children[i].block.Hash() == newRoot.block.Hash() {
			newRoot.parent.children = append(newRoot.parent.children[:i], newRoot.parent.children[i+1:]...)
			break
		}
	}
	newRoot.parent = nil
}

// cleanByTailoredTree removes all blocks in the tree which has been tailored.
func (cbft *Cbft) cleanByTailoredTree(root *BlockExt) {
	log.Trace("call cleanByTailoredTree()", "RoutineID", common.CurrentGoRoutineID(), "rootHash", root.block.Hash(), "rootNumber", root.block.NumberU64())
	if len(root.children) > 0 {
		for _, child := range root.children {
			cbft.cleanByTailoredTree(child)
			log.Debug("remove block in memory", "RoutineID", common.CurrentGoRoutineID(), "hash", root.block.Hash(), "number", root.block.NumberU64())
			delete(cbft.blockExtMap, root.block.Hash())
			delete(cbft.signedSet, root.block.NumberU64())
		}
	} else {
		log.Debug("remove block in memory", "RoutineID", common.CurrentGoRoutineID(), "hash", root.block.Hash(), "number", root.block.NumberU64())
		delete(cbft.blockExtMap, root.block.Hash())
	}
}

// cleanByNumber removes all blocks lower than upperLimit in BlockExtMap.
func (cbft *Cbft) cleanByNumber(upperLimit uint64) {
	log.Trace("call cleanByNumber()", "RoutineID", common.CurrentGoRoutineID(), "upperLimit", upperLimit)
	for hash, ext := range cbft.blockExtMap {
		if ext.number < upperLimit {
			log.Debug("remove block in memory", "RoutineID", common.CurrentGoRoutineID(), "hash", hash, "number", ext.number)
			delete(cbft.blockExtMap, hash)
		}
	}
	for number, _ := range cbft.signedSet {
		if number < upperLimit {
			delete(cbft.signedSet, number)
		}
	}
}

// Author implements consensus.Engine, returning the Ethereum address recovered
// from the signature in the header's extra-data section.
func (cbft *Cbft) Author(header *types.Header) (common.Address, error) {
	log.Trace("call Author()", "hash", header.Hash(), "number", header.Number.Uint64())
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (cbft *Cbft) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	log.Trace("call VerifyHeader()", "hash", header.Hash(), "number", header.Number.Uint64(), "seal", seal)

	if header.Number == nil {
		return errUnknownBlock
	}

	if len(header.Extra) < extraSeal {
		return errMissingSignature
	}
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (cbft *Cbft) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	log.Trace("call VerifyHeaders()", "Headers count", len(headers))

	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for _, header := range headers {
			err := cbft.VerifyHeader(chain, header, false)

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (cbft *Cbft) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (cbft *Cbft) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	log.Trace("call VerifySeal()", "hash", header.Hash(), "number", header.Number.String())

	return cbft.verifySeal(chain, header, nil)
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (b *Cbft) Prepare(chain consensus.ChainReader, header *types.Header) error {
	log.Debug("call Prepare()", "RoutineID", common.CurrentGoRoutineID(), "hash", header.Hash(), "number", header.Number.Uint64())

	cbft.lock.RLock()
	defer cbft.lock.RUnlock()

	if cbft.highestLogical.block == nil || header.ParentHash != cbft.highestLogical.block.Hash() || header.Number.Uint64()-1 != cbft.highestLogical.block.NumberU64() {
		return consensus.ErrUnknownAncestor
	}

	header.Difficulty = big.NewInt(2)

	//header.Extra[0:31] to store block's version info etc. and right pad with 0x00;
	//header.Extra[32:] to store block's sign of producer, the length of sign is 65.
	if len(header.Extra) < 32 {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, 32-len(header.Extra))...)
	}
	header.Extra = header.Extra[:32]

	//init header.Extra[32: 32+65]
	header.Extra = append(header.Extra, make([]byte, consensus.ExtraSeal)...)
	return nil
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given, and returns the final block.
func (cbft *Cbft) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	log.Debug("call Finalize()", "RoutineID", common.CurrentGoRoutineID(), "hash", header.Hash(), "number", header.Number.Uint64(), "txs", len(txs), "receipts", len(receipts))
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)
	return types.NewBlock(header, txs, nil, receipts), nil
}

//to sign the block, and store the sign to header.Extra[32:], send the sign to chanel to broadcast to other consensus nodes
func (cbft *Cbft) Seal(chain consensus.ChainReader, block *types.Block, sealResultCh chan<- *types.Block, stopCh <-chan struct{}) error {
	log.Debug("call Seal()", "RoutineID", common.CurrentGoRoutineID(), "number", block.NumberU64(), "parentHash", block.ParentHash())
	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	header := block.Header()
	number := block.NumberU64()

	if number == 0 {
		return errUnknownBlock
	}

	if !cbft.highestLogical.isParent(block) {
		log.Error("Futile block cause highest logical block changed", "parentHash", block.ParentHash())
		return errFutileBlock
	}

	// sign the seal hash
	sign, err := cbft.signFn(sealHash(header).Bytes())
	if err != nil {
		return err
	}

	//store the sign in  header.Extra[32:]
	copy(header.Extra[len(header.Extra)-extraSeal:], sign[:])

	sealedBlock := block.WithSeal(header)

	current := NewBlockExt(sealedBlock, sealedBlock.NumberU64())

	//this block is produced by local node, so need not execute in cbft.
	current.inTree = true
	current.isExecuted = true
	current.isSigned = true

	//save the block to cbft.blockExtMap
	cbft.saveBlockExt(sealedBlock.Hash(), current)

	//collect the sign
	cbft.collectSign(current, common.NewBlockConfirmSign(sign))

	//log this signed block's number
	cbft.signedSet[sealedBlock.NumberU64()] = struct{}{}

	//build tree node
	cbft.buildIntoTree(current)

	log.Debug("seal complete", "RoutineID", common.CurrentGoRoutineID(), "hash", sealedBlock.Hash(), "number", block.NumberU64())

	// SetNodeCache
	blockNumber := current.block.Number()
	parentNumber := new(big.Int).Sub(blockNumber, common.Big1)
	sealhash := cbft.SealHash(current.block.Header())
	state := cbft.blockChainCache.ReadStateDB(sealhash)
	log.Debug("setNodeCache", "parentNumber", parentNumber, "parentHash", current.block.ParentHash(), "blockNumber", blockNumber, "blockHash", current.block.Hash())
	if state != nil {
		cbft.ppos.SetNodeCache(state, parentNumber, blockNumber, block.ParentHash(), current.block.Hash())
	} else {
		log.Error("setNodeCache error")
	}

	consensusNodes := cbft.ConsensusNodes(parentNumber, current.block.ParentHash(), blockNumber)

	if consensusNodes != nil && len(consensusNodes) == 1 {
		log.Debug("single node Mode")
		//only one consensus node, so, each block is highestConfirmed. (lock is needless)
		current.isConfirmed = true
		current.isSigned = true
		cbft.setHighestLogical(current)
		cbft.highestConfirmed = current
		cbft.flushReadyBlock()
		log.Debug("reset TxPool after block sealed", "RoutineID", common.CurrentGoRoutineID(), "hash", current.block.Hash(), "number", current.number)
		cbft.txPool.Reset(current.block)
		return nil
	}

	//reset cbft.highestLogicalBlockExt cause this block is produced by myself
	cbft.setHighestLogical(current)

	go func() {
		select {
		case <-stopCh:
			return
		case sealResultCh <- sealedBlock:
		default:
			log.Warn("Sealing result is not ready by miner", "sealHash", sealHash(header))
		}
	}()

	log.Debug("reset TxPool after block sealed", "RoutineID", common.CurrentGoRoutineID(), "hash", current.block.Hash(), "number", current.number)
	cbft.txPool.Reset(current.block)
	return nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (b *Cbft) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	log.Trace("call CalcDifficulty()", "time", time, "parentHash", parent.Hash(), "parentNumber", parent.Number.Uint64())
	return big.NewInt(2)
}

// SealHash returns the hash of a block prior to it being sealed.
func (b *Cbft) SealHash(header *types.Header) common.Hash {
	log.Debug("call SealHash()", "RoutineID", common.CurrentGoRoutineID(), "hash", header.Hash(), "number", header.Number.Uint64())
	return sealHash(header)
}

// Close implements consensus.Engine. It's a noop for cbft as there is are no background threads.
func (cbft *Cbft) Close() error {
	log.Trace("call Close()")
	cbft.closeOnce.Do(func() {
		// Short circuit if the exit channel is not allocated.
		if cbft.exitCh == nil {
			return
		}
		cbft.exitCh <- struct{}{}
		close(cbft.exitCh)
	})
	return nil
}

// APIs implements consensus.Engine, returning the user facing RPC API to allow
// controlling the signer voting.
func (cbft *Cbft) APIs(chain consensus.ChainReader) []rpc.API {
	log.Trace("call APIs()")

	return []rpc.API{{
		Namespace: "cbft",
		Version:   "1.0",
		Service:   &API{chain: chain, cbft: cbft},
		Public:    false,
	}}
}

// OnBlockSignature is called by by protocol handler when it received a new block signature by P2P.
func (cbft *Cbft) OnBlockSignature(chain consensus.ChainReader, nodeID discover.NodeID, rcvSign *cbfttypes.BlockSignature) error {
	log.Debug("call OnBlockSignature()", "GoRoutineID", common.CurrentGoRoutineID(), "hash", rcvSign.Hash, "number", rcvSign.Number, "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]), "signHash", rcvSign.SignHash, "cbft.dataReceiveCh.len", len(cbft.dataReceiveCh))
	ok, err := verifySign(nodeID, rcvSign.SignHash, rcvSign.Signature[:])
	if err != nil {
		log.Error("verify sign error", "errors", err)
		return err
	}

	if !ok {
		log.Error("unauthorized signer")
		return errUnauthorizedSigner
	}
	cbft.dataReceiveCh <- rcvSign
	return nil
}

// OnNewBlock is called by protocol handler when it received a new block by P2P.
func (cbft *Cbft) OnNewBlock(chain consensus.ChainReader, rcvBlock *types.Block) error {
	log.Debug("call OnNewBlock()", "GoRoutineID", common.CurrentGoRoutineID(), "hash", rcvBlock.Hash(), "number", rcvBlock.NumberU64(), "ParentHash", rcvBlock.ParentHash(), "cbft.dataReceiveCh.len", len(cbft.dataReceiveCh))
	cbft.dataReceiveCh <- rcvBlock
	return nil
}

// OnPong is called by protocol handler when it received a new Pong message by P2P.
func (cbft *Cbft) OnPong(nodeID discover.NodeID, netLatency int64) error {
	log.Trace("call OnPong()", "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]), "netLatency", netLatency)

	cbft.netLatencyLock.Lock()
	defer cbft.netLatencyLock.Unlock()

	if netLatency >= maxPingLatency {
		return nil
	}

	latencyList, exist := cbft.netLatencyMap[nodeID]
	if !exist {
		cbft.netLatencyMap[nodeID] = list.New()
		cbft.netLatencyMap[nodeID].PushBack(netLatency)
	} else {
		if latencyList.Len() > 5 {
			e := latencyList.Front()
			cbft.netLatencyMap[nodeID].Remove(e)
		}
		cbft.netLatencyMap[nodeID].PushBack(netLatency)
	}
	return nil
}

// avgLatency statistics the net latency between local and other peers.
func (cbft *Cbft) avgLatency(nodeID discover.NodeID) int64 {
	if latencyList, exist := cbft.netLatencyMap[nodeID]; exist {
		sum := int64(0)
		counts := int64(0)
		for e := latencyList.Front(); e != nil; e = e.Next() {
			if latency, ok := e.Value.(int64); ok {
				counts++
				sum += latency
			}
		}
		if counts > 0 {
			return sum / counts
		}
	}
	return cbft.config.MaxLatency
}

// HighestLogicalBlock returns the cbft.highestLogical.block.
func (cbft *Cbft) HighestLogicalBlock() *types.Block {
	log.Debug("call HighestLogicalBlock() ...")
	cbft.lock.RLock()
	defer cbft.lock.RUnlock()

	if cbft.highestLogical == nil {
		return nil
	} else {
		return cbft.highestLogical.block
	}
}

// HighestConfirmedBlock returns the cbft.highestConfirmed.block.
func (cbft *Cbft) HighestConfirmedBlock() *types.Block {
	log.Debug("call HighestConfirmedBlock() ...")
	cbft.lock.RLock()
	defer cbft.lock.RUnlock()
	if cbft.highestConfirmed == nil {
		return nil
	} else {
		return cbft.highestConfirmed.block
	}
}

// GetBlock returns the block in blockExtMap.
func (cbft *Cbft) GetBlock(hash common.Hash, number uint64) *types.Block {
	cbft.lock.RLock()
	defer cbft.lock.RUnlock()

	ext := cbft.blockExtMap[hash]
	if ext != nil && ext.block != nil && ext.number == number {
		return ext.block
	}
	return nil
}

// IsSignedBySelf returns if the block is signed by local.
func IsSignedBySelf(sealHash common.Hash, signature []byte) bool {
	ok, err := verifySign(cbft.config.NodeID, sealHash, signature)
	if err != nil {
		log.Error("verify sign error", "errors", err)
		return false
	}
	return ok
}

// storeBlocks sends the blocks to cbft.cbftResultOutCh, the receiver will write them into chain
func (cbft *Cbft) storeBlocks(blocksToStore []*BlockExt) {
	for _, ext := range blocksToStore {
		cbftResult := &cbfttypes.CbftResult{
			Block:             ext.block,
			BlockConfirmSigns: ext.signs,
		}
		log.Debug("send to channel", "RoutineID", common.CurrentGoRoutineID(), "hash", ext.block.Hash(), "number", ext.block.NumberU64(), "signCount", len(ext.signs))
		cbft.cbftResultOutCh <- cbftResult
	}
}

// inTurn return if it is local's turn to package new block.
//func (cbft *Cbft) inTurn() bool {
//	curTime := toMilliseconds(time.Now())
//	inturn := cbft.calTurn(curTime, cbft.config.NodeID)
//	log.Debug("inTurn", "result", inturn)
//	return inturn
//}

func (cbft *Cbft) inTurn(parentNumber *big.Int, parentHash common.Hash, commitNumber *big.Int) bool {
	curTime := toMilliseconds(time.Now())
	log.Debug("ShouldSeal InTurn: ", "parentNumber", parentNumber, "parentHash", parentHash, "commitNumber", commitNumber, "currentNodeID", cbft.config.NodeID)
	inturn := cbft.calTurn(parentNumber, parentHash, commitNumber, curTime-300, cbft.config.NodeID, current)
	log.Debug("first inturn: ", "result", inturn)
	if inturn {
		inturn = cbft.calTurn(parentNumber, parentHash, commitNumber, curTime+600, cbft.config.NodeID, current)
		log.Debug("second inturn: ", "result", inturn)
	}
	log.Debug("ShouldSeal InTurn", "result", inturn)
	return inturn
}

// inTurnVerify verifies the time is in the time-window of the nodeID to package new block.
//func (cbft *Cbft) inTurnVerify(curTime int64, nodeID discover.NodeID) bool {
//	latency := cbft.avgLatency(nodeID)
//	if latency >= maxAvgLatency {
//		log.Debug("inTurnVerify, return false cause of net latency", "result", false, "latency", latency)
//		return false
//	}
//	inTurnVerify := cbft.calTurn(curTime-latency, nodeID)
//	log.Debug("inTurnVerify", "result", inTurnVerify, "latency", latency)
//	return inTurnVerify
//}
func (cbft *Cbft) inTurnVerify(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int, curTime int64, nodeID discover.NodeID) bool {
	latency := cbft.avgLatency(nodeID)
	if latency >= maxAvgLatency {
		log.Debug("inTurnVerify, return false cause of net latency", "result", false, "latency", latency)
		return false
	}
	inTurnVerify := cbft.calTurn(parentNumber, parentHash, blockNumber, curTime-latency, nodeID, all)
	log.Debug("inTurnVerify", "result", inTurnVerify, "latency", latency)
	return inTurnVerify
}

//shouldKeepIt verifies the time is legal to package new block for the nodeID.
//func (cbft *Cbft) isLegal(curTime int64, producerID discover.NodeID) bool {
//	offset := 1000 * (cbft.config.Duration/2 - 1)
//	isLegal := cbft.calTurn(curTime-offset, producerID)
//	if !isLegal {
//		isLegal = cbft.calTurn(curTime+offset, producerID)
//	}
//	return isLegal
//}
func (cbft *Cbft) isLegal(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int, curTime int64, producerID discover.NodeID) bool {
	offset := 1000 * (cbft.config.Duration/2 - 1)
	isLegal := cbft.calTurn(parentNumber, parentHash, blockNumber, curTime-offset, producerID, all)
	if !isLegal {
		isLegal = cbft.calTurn(parentNumber, parentHash, blockNumber, curTime+offset, producerID, all)
	}
	return isLegal
}

//func (cbft *Cbft) calTurn(curTime int64, nodeID discover.NodeID) bool {
//	nodeIdx := cbft.dpos.NodeIndex(nodeID)
//	startEpoch := cbft.dpos.StartTimeOfEpoch() * 1000
//
//	if nodeIdx >= 0 {
//		durationPerNode := cbft.config.Duration * 1000
//		durationPerTurn := durationPerNode * int64(len(cbft.dpos.primaryNodeList))
//
//		min := nodeIdx * (durationPerNode)
//
//		value := (curTime - startEpoch) % durationPerTurn
//
//		max := (nodeIdx + 1) * durationPerNode
//
//		log.Debug("calTurn", "idx", nodeIdx, "min", min, "value", value, "max", max, "curTime", curTime, "startEpoch", startEpoch)
//
//		if value > min && value < max {
//			return true
//		}
//	}
//	return false
//}

func (cbft *Cbft) calTurn(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int, curTime int64, nodeID discover.NodeID, round int32) bool {
	nodeIdx := cbft.ppos.BlockProducerIndex(parentNumber, parentHash, blockNumber, nodeID, round)
	startEpoch := cbft.ppos.StartTimeOfEpoch() * 1000

	if nodeIdx >= 0 {
		durationPerNode := cbft.config.Duration * 1000

		consensusNodes := cbft.ConsensusNodes(parentNumber, parentHash, blockNumber)
		if consensusNodes == nil || len(consensusNodes) <= 0 {
			log.Error("calTurn consensusNodes is emtpy~")
			return false
		}

		durationPerTurn := durationPerNode * int64(len(consensusNodes))

		min := nodeIdx * (durationPerNode)

		value := (curTime - startEpoch) % durationPerTurn

		max := (nodeIdx + 1) * durationPerNode

		log.Debug("calTurn", "idx", nodeIdx, "min", min, "value", value, "max", max, "curTime", curTime, "startEpoch", startEpoch)

		if value > min && value < max {
			return true
		}
	}
	return false
}

// producer's signature = header.Extra[32:]
// public key can be recovered from signature, the length of public key is 65,
// the length of NodeID is 64, nodeID = publicKey[1:]
func ecrecover(header *types.Header) (discover.NodeID, []byte, error) {
	var nodeID discover.NodeID
	if len(header.Extra) < extraSeal {
		return nodeID, []byte{}, errMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]
	sealHash := sealHash(header)

	pubkey, err := crypto.Ecrecover(sealHash.Bytes(), signature)
	if err != nil {
		return nodeID, []byte{}, err
	}

	nodeID, err = discover.BytesID(pubkey[1:])
	if err != nil {
		return nodeID, []byte{}, err
	}
	return nodeID, signature, nil
}

// verify sign, check the sign is from the right node.
func verifySign(expectedNodeID discover.NodeID, sealHash common.Hash, signature []byte) (bool, error) {
	pubkey, err := crypto.SigToPub(sealHash.Bytes(), signature)

	if err != nil {
		return false, err
	}

	nodeID := discover.PubkeyID(pubkey)
	if bytes.Equal(nodeID.Bytes(), expectedNodeID.Bytes()) {
		return true, nil
	}
	return false, nil
}

// seal hash, only include from byte[0] to byte[32] of header.Extra
func sealHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[0:32],
		header.MixDigest,
		header.Nonce,
	})
	hasher.Sum(hash[:0])

	return hash
}

func (cbft *Cbft) verifySeal(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	return nil
}

func (cbft *Cbft) signFn(headerHash []byte) (sign []byte, err error) {
	return crypto.Sign(headerHash, cbft.config.PrivateKey)
}

//func (cbft *Cbft) getThreshold() int {
//	trunc := len(cbft.dpos.primaryNodeList) * 2 / 3
//	return int(trunc + 1)
//}

func (cbft *Cbft) getThreshold(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int) int {
	consensusNodes := cbft.ConsensusNodes(parentNumber, parentHash, blockNumber)
	if consensusNodes != nil {
		trunc := len(consensusNodes) * 2 / 3
		return int(trunc + 1)
	}
	return math.MaxInt16
}

func toMilliseconds(t time.Time) int64 {
	return t.UnixNano() / 1e6
}

func (cbft *Cbft) ShouldSeal(parentNumber *big.Int, parentHash common.Hash, commitNumber *big.Int) bool {
	log.Debug("Call ShouldSeal: ", "parentNumber", parentNumber, "parentHash", parentHash, "commitNumber", commitNumber)
	return cbft.inTurn(parentNumber, parentHash, commitNumber)
}

func (cbft *Cbft) CurrentNodes(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int) []*discover.Node {
	return cbft.ppos.getCurrentNodes(parentNumber, parentHash, blockNumber)
}

func (cbft *Cbft) IsCurrentNode(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int) bool {
	currentNodes := cbft.ppos.getCurrentNodes(parentNumber, parentHash, blockNumber)
	nodeID := cbft.GetOwnNodeID()
	for _, n := range currentNodes {
		if nodeID == n.ID {
			return true
		}
	}
	return false
}

func (cbft *Cbft) ConsensusNodes(parentNumber *big.Int, parentHash common.Hash, blockNumber *big.Int) []discover.NodeID {
	return cbft.ppos.consensusNodes(parentNumber, parentHash, blockNumber)
}

// wether nodeID in former or current or next
//func (cbft *Cbft) CheckConsensusNode(nodeID discover.NodeID) (bool, error) {
//	log.Debug("call CheckConsensusNode()", "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]))
//	return cbft.ppos.AnyIndex(nodeID) >= 0, nil
//}

// wether nodeID in former or current or next
//func (cbft *Cbft) IsConsensusNode() (bool, error) {
//	log.Debug("call IsConsensusNode()")
//	return cbft.ppos.AnyIndex(cbft.config.NodeID) >= 0, nil
//}

func (cbft *Cbft) Election(state *state.StateDB, blockNumber *big.Int) ([]*discover.Node, error) {
	return cbft.ppos.Election(state, blockNumber)
}

func (cbft *Cbft) Switch(state *state.StateDB) bool {
	return cbft.ppos.Switch(state)
}

func (cbft *Cbft) GetWitness(state *state.StateDB, flag int) ([]*discover.Node, error) {
	return cbft.ppos.GetWitness(state, flag)
}

func (cbft *Cbft) GetOwnNodeID() discover.NodeID {
	return cbft.config.NodeID
}

func (cbft *Cbft) SetNodeCache(state *state.StateDB, parentNumber, currentNumber *big.Int, parentHash, currentHash common.Hash) error {
	log.Info("cbft SetNodeCache", "parentNumber", parentNumber, "parentHash", parentHash, "currentNumber", currentNumber, "currentHash", currentHash)
	return cbft.ppos.SetNodeCache(state, parentNumber, currentNumber, parentHash, currentHash)
}