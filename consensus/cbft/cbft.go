// Package bft implements the BFT consensus engine.
package cbft

import (
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/hexutil"
	"github.com/PlatONnetwork/PlatON-Go/consensus"
	"github.com/PlatONnetwork/PlatON-Go/core"
	"github.com/PlatONnetwork/PlatON-Go/core/cbfttypes"
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
	"math/big"
	"sync"
	"time"
)

var (
	errSign                = errors.New("sign error")
	errUnauthorizedSigner  = errors.New("unauthorized signer")
	errIllegalBlock        = errors.New("illegal block")
	errDuplicatedBlock     = errors.New("duplicated block")
	errBlockNumber         = errors.New("error block number")
	errUnknownBlock        = errors.New("unknown block")
	errFutileBlock         = errors.New("futile block")
	errGenesisBlock        = errors.New("cannot handle genesis block")
	errHighestLogicalBlock = errors.New("cannot find a logical block")
	errListConfirmedBlocks = errors.New("list confirmed blocks error")
	errMissingSignature    = errors.New("extra-data 65 byte signature suffix missing")
	extraSeal              = 65
	windowSize             = uint64(20)

	//periodMargin is a percentum for period margin
	periodMargin = uint64(20)

	//maxPingLatency is the time in milliseconds between Ping and Pong
	maxPingLatency = int64(5000)

	//maxAvgLatency is the time in milliseconds between two peers
	maxAvgLatency = int64(2000)
)

type Cbft struct {
	config                *params.CbftConfig
	dpos                  *dpos
	rotating              *rotating
	blockSignOutCh        chan *cbfttypes.BlockSignature //a channel to send block signature
	cbftResultOutCh       chan *cbfttypes.CbftResult     //a channel to send consensus result
	highestLogicalBlockCh chan *types.Block
	closeOnce             sync.Once
	exitCh                chan chan error
	txPool                *core.TxPool
	blockExtMap           map[common.Hash]*BlockExt //store all received blocks and signs
	dataReceiveCh         chan interface{}          //a channel to receive data from miner
	blockChain            *core.BlockChain          //the block chain
	highestLogical        *BlockExt                 //local node's new block will base on it
	highestConfirmed      *BlockExt                 //highest confirmed block, it will be written to chain
	signedSet             map[uint64]struct{}       //all block numbers signed by local node
	lock                  sync.RWMutex
	consensusCache        *Cache //cache for cbft consensus

	netLatencyMap map[discover.NodeID]*list.List
}

var cbft *Cbft

// New creates a concurrent BFT consensus engine
func New(config *params.CbftConfig, blockSignatureCh chan *cbfttypes.BlockSignature, cbftResultCh chan *cbfttypes.CbftResult, highestLogicalBlockCh chan *types.Block) *Cbft {
	initialNodesID := make([]discover.NodeID, 0, len(config.InitialNodes))
	for _, n := range config.InitialNodes {
		initialNodesID = append(initialNodesID, n.ID)
	}
	_dpos := newDpos(initialNodesID)
	cbft = &Cbft{
		config:                config,
		dpos:                  _dpos,
		rotating:              newRotating(_dpos, config.Duration),
		blockSignOutCh:        blockSignatureCh,
		cbftResultOutCh:       cbftResultCh,
		highestLogicalBlockCh: highestLogicalBlockCh,

		blockExtMap:   make(map[common.Hash]*BlockExt),
		signedSet:     make(map[uint64]struct{}),
		dataReceiveCh: make(chan interface{}, 250),
		netLatencyMap: make(map[discover.NodeID]*list.List),
	}

	flowControl = NewFlowControl()

	go cbft.dataReceiverLoop()

	return cbft
}

// BlockExt is an extension from Block
type BlockExt struct {
	block       *types.Block
	isLinked    bool
	isSigned    bool
	isStored    bool
	isConfirmed bool
	number      uint64
	signs       []*common.BlockConfirmSign //all signs for block
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
		if len(ext.signs) >= cbft.getThreshold() {
			ext.isConfirmed = true
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
func (ext *BlockExt) findParent() *BlockExt {
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

// isFork checks if new branch is forked from old branch
// newFork[0] == oldFork[0], newFork[len(newFork)-1] is the new confirmed block
func (cbft *Cbft) isFork(newFork []*BlockExt, oldFork []*BlockExt) (bool, *BlockExt) {
	for i := 1; i < len(newFork); i++ {
		if oldFork[i].isConfirmed {
			return false, oldFork[i]
		}
	}
	return true, nil
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
func (ext *BlockExt) findChildren() []*BlockExt {
	if ext.block == nil {
		return nil
	}
	children := make([]*BlockExt, 0)

	for _, child := range cbft.blockExtMap {
		if child.block != nil && child.block.ParentHash() == ext.block.Hash() {
			if child.block.NumberU64()-1 == ext.block.NumberU64() {
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

// saveBlock saves block in memory
func (cbft *Cbft) saveBlock(hash common.Hash, ext *BlockExt) {
	cbft.blockExtMap[hash] = ext
	log.Debug("total blocks in memory", "totalBlocks", len(cbft.blockExtMap))
}

// isAncestor checks if a block is another's ancestor
func (lower *BlockExt) isAncestor(higher *BlockExt) bool {
	if higher.block == nil || lower.block == nil {
		return false
	}
	generations := higher.block.NumberU64() - lower.block.NumberU64()

	for i := uint64(0); i < generations; i++ {
		parent := higher.findParent()
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

// findNewHighestConfirmed finds a new highest confirmed blockExt from ext start; If there are multiple highest confirmed blockExts, return the first.
func (cbft *Cbft) findNewHighestConfirmed(ext *BlockExt) *BlockExt {
	log.Debug("find first, highest confirmed block ")
	found := ext
	if !found.isConfirmed {
		found = nil
	}
	//each child has non-nil block
	children := ext.findChildren()
	if children != nil {
		for _, child := range children {
			current := cbft.findNewHighestConfirmed(child)
			if current != nil && current.isConfirmed && (found == nil || current.block.NumberU64() > found.block.NumberU64()) {
				found = current
			}
		}
	}
	return found
}

// findHighest finds the highest block from ext start; If there are multiple highest blockExts, return the one that has most signs
func (cbft *Cbft) findHighest(ext *BlockExt) *BlockExt {
	log.Debug("find highest block")
	highest := ext
	//each child has non-nil block
	children := ext.findChildren()
	if children != nil {
		for _, child := range children {
			current := cbft.findHighest(child)
			if current.block.NumberU64() > highest.block.NumberU64() || (current.block.NumberU64() == highest.block.NumberU64() && len(current.signs) > len(highest.signs)) {
				highest = current
			}
		}
	}
	return highest
}

// findHighestSignedByLocal finds the highest block signed by local node from ext start; If there are multiple highest blockExts, return the one that has most signs
func (cbft *Cbft) findHighestSignedByLocal(ext *BlockExt) *BlockExt {
	log.Debug("find highest block has most signs")
	highest := ext

	if !highest.isSigned {
		highest = nil
	}
	//each child has non-nil block
	children := ext.findChildren()
	if children != nil {
		for _, child := range children {
			current := cbft.findHighestSignedByLocal(child)
			if current != nil && current.isSigned {
				if highest == nil {
					highest = current
				} else if current.block.NumberU64() > highest.block.NumberU64() || (current.block.NumberU64() == highest.block.NumberU64() && len(current.signs) > len(highest.signs)) {
					highest = current
				}
			}
		}
	}
	return highest
}

// handleBlockAndDescendant executes the block's transactions and its descendant, and sign the block and its descendant if possible
func (cbft *Cbft) handleBlockAndDescendant(ext *BlockExt, parent *BlockExt, signIfPossible bool) {
	log.Debug("handle block recursively", "hash", ext.block.Hash(), "number", ext.block.NumberU64(), "signIfPossible", signIfPossible)

	cbft.executeBlockAndDescendant(ext, parent)

	if ext.findChildren() == nil {
		if signIfPossible {
			if _, signed := cbft.signedSet[ext.block.NumberU64()]; !signed {
				cbft.sign(ext)
			}
		}
	} else {
		highest := cbft.findHighest(ext)
		//logicalExts := cbft.backTrackLogicals(highest, ext)
		logicalExts := cbft.backTrackBlocks(highest, ext, false)
		for _, logical := range logicalExts {
			if _, signed := cbft.signedSet[logical.block.NumberU64()]; !signed {
				cbft.sign(logical)
			}
		}
	}
}

// executeBlockAndDescendant executes the block's transactions and its descendant
func (cbft *Cbft) executeBlockAndDescendant(ext *BlockExt, parent *BlockExt) {
	log.Debug("execute block recursively", "hash", ext.block.Hash(), "number", ext.block.NumberU64())
	if ext.isLinked == false {
		cbft.execute(ext, parent)
		ext.isLinked = true
	}
	//each child has non-nil block
	children := ext.findChildren()
	if children != nil {
		for _, child := range children {
			cbft.executeBlockAndDescendant(child, ext)
		}
	}
}

// sign signs a block
func (cbft *Cbft) sign(ext *BlockExt) {
	sealHash := sealHash(ext.block.Header())
	signature, err := cbft.signFn(sealHash.Bytes())
	if err == nil {
		log.Debug("Sign block ", "Hash", ext.block.Hash(), "number", ext.block.NumberU64(), "sealHash", sealHash, "signature", hexutil.Encode(signature))

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
		}
		cbft.blockSignOutCh <- blockSign
	} else {
		panic("sign a block error")
	}
}

// execute executes the block's transactions based on its parent
// if success then save the receipts and state to consensusCache
func (cbft *Cbft) execute(ext *BlockExt, parent *BlockExt) {
	state, err := cbft.consensusCache.MakeStateDB(parent.block)
	if err != nil {
		log.Error("execute block error, cannot make state based on parent")
		return
	}

	//to execute
	receipts, err := cbft.blockChain.ProcessDirectly(ext.block, state, parent.block)
	if err == nil {
		//save the receipts and state to consensusCache
		cbft.consensusCache.WriteReceipts(ext.block.Hash(), receipts, ext.block.NumberU64())
		cbft.consensusCache.WriteStateDB(ext.block.Root(), state, ext.block.NumberU64())
	} else {
		log.Error("execute a block error", err)
	}
}

// backTrackBlocks return blocks from start to end, these blocks are in a same tree branch.
// The result is sorted by block number from lower to higher.
func (cbft *Cbft) backTrackBlocks(start *BlockExt, end *BlockExt, includeEnd bool) []*BlockExt {
	log.Debug("back track blocks", "startHash", start.block.Hash(), "startParentHash", end.block.ParentHash(), "endHash", start.block.Hash())

	found := false
	logicalExts := make([]*BlockExt, 1)
	logicalExts[0] = start

	for {
		parent := start.findParent()
		if parent == nil {
			break
		} else if parent.block.Hash() == end.block.Hash() && parent.block.NumberU64() == end.block.NumberU64() {
			log.Debug("ending of back track block ")
			if includeEnd {
				logicalExts = append(logicalExts, parent)
			}
			found = true
			break
		} else {
			log.Debug("found new block", "Hash", parent.block.Hash(), "ParentHash", parent.block.ParentHash(), "number", parent.block.NumberU64())
			logicalExts = append(logicalExts, parent)
			start = parent
		}
	}

	if found {
		//sorted by block number from lower to higher
		if len(logicalExts) > 1 {
			reverse(logicalExts)
		}
		return logicalExts
	} else {
		return nil
	}
}

func reverse(s []*BlockExt) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// backTrackTillStored return blocks from the new confirmed one till the already confirmed one(including), these blocks are in a same tree branch.
// The result is sorted by block number from lower to higher.
func (cbft *Cbft) backTrackTillStored(newConfirmed *BlockExt) []*BlockExt {

	log.Debug("found new block to store", "Hash", newConfirmed.block.Hash(), "ParentHash", newConfirmed.block.ParentHash(), "number", newConfirmed.block.NumberU64())

	existMap := make(map[common.Hash]struct{})

	foundExts := make([]*BlockExt, 1)
	foundExts[0] = newConfirmed

	existMap[newConfirmed.block.Hash()] = struct{}{}
	foundRoot := false
	for {
		parent := newConfirmed.findParent()
		if parent == nil {
			break
		}

		foundExts = append(foundExts, parent)

		if parent.isStored {
			foundRoot = true
			break
		} else {
			log.Debug("found new block to store", "Hash", parent.block.Hash(), "ParentHash", parent.block.ParentHash(), "number", parent.block.NumberU64())
			if _, exist := existMap[parent.block.Hash()]; exist {
				log.Error("get into a loop when finding new block to store")
				return nil
			}

			newConfirmed = parent
		}
	}

	if !foundRoot {
		log.Error("cannot lead to a already store block")
		return nil
	}

	//sorted by block number from lower to higher
	if len(foundExts) > 1 {
		reverse(foundExts)
	}
	return foundExts
}

// SetPrivateKey sets local's private key by the backend.go
func (cbft *Cbft) SetPrivateKey(privateKey *ecdsa.PrivateKey) {
	cbft.config.PrivateKey = privateKey
	cbft.config.NodeID = discover.PubkeyID(&privateKey.PublicKey)
}

func SetConsensusCache(cache *Cache) {
	cbft.consensusCache = cache
}

// setHighestLogical sets highest logical block and send it to the highestLogicalBlockCh
func setHighestLogical(highestLogical *BlockExt) {
	cbft.highestLogical = highestLogical
	cbft.highestLogicalBlockCh <- highestLogical.block
}

// SetBackend sets blockChain and txPool into cbft
func SetBackend(blockChain *core.BlockChain, txPool *core.TxPool) {
	log.Debug("call SetBackend()")

	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	cbft.blockChain = blockChain
	cbft.dpos.SetStartTimeOfEpoch(blockChain.Genesis().Time().Int64())

	currentBlock := blockChain.CurrentBlock()

	genesisParentHash := bytes.Repeat([]byte{0x00}, 32)
	if bytes.Equal(currentBlock.ParentHash().Bytes(), genesisParentHash) && currentBlock.Number() == nil {
		currentBlock.Header().Number = big.NewInt(0)
	}

	log.Debug("init cbft.highestLogicalBlock", "Hash", currentBlock.Hash(), "number", currentBlock.NumberU64())

	confirmedBlock := NewBlockExt(currentBlock, currentBlock.NumberU64())
	confirmedBlock.isLinked = true
	confirmedBlock.isStored = true
	confirmedBlock.isConfirmed = true
	confirmedBlock.number = currentBlock.NumberU64()

	cbft.saveBlock(currentBlock.Hash(), confirmedBlock)

	cbft.highestConfirmed = confirmedBlock
	//cbft.highestLogical = confirmedBlock
	setHighestLogical(confirmedBlock)

	txPool = txPool
}

// BlockSynchronisation reset the cbft env, such as cbft.highestLogical, cbft.highestConfirmed.
// This function is invoked after that local has synced new blocks from other node.
func BlockSynchronisation() {
	log.Debug("call BlockSynchronisation()")
	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	currentBlock := cbft.blockChain.CurrentBlock()

	if currentBlock.NumberU64() > cbft.highestConfirmed.block.NumberU64() {
		log.Debug("found higher highestConfirmed block")

		confirmedBlock := NewBlockExt(currentBlock, currentBlock.NumberU64())
		confirmedBlock.isLinked = true
		confirmedBlock.isStored = true
		confirmedBlock.isConfirmed = true
		confirmedBlock.number = currentBlock.NumberU64()

		cbft.slideWindow(confirmedBlock)

		cbft.saveBlock(currentBlock.Hash(), confirmedBlock)

		cbft.highestConfirmed = confirmedBlock

		highestLogical := cbft.findHighestSignedByLocal(confirmedBlock)
		if highestLogical == nil {
			highestLogical = cbft.findHighest(confirmedBlock)
		}

		if highestLogical == nil {
			log.Warn("cannot find a logical block")
			return
		}

		setHighestLogical(highestLogical)

		children := confirmedBlock.findChildren()
		for _, child := range children {
			cbft.handleBlockAndDescendant(child, confirmedBlock, true)
		}
	}
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
					log.Error("Received wrong data type")
				}
			}
		}
	}
}

// slideWindow slides the blocks window in memory,
// the newConfirmed will reserved in blockExtMap, but it's signs will be removed from cbft.signedSet.
func (cbft *Cbft) slideWindow(newConfirmed *BlockExt) {
	for hash, ext := range cbft.blockExtMap {
		if ext.number <= newConfirmed.block.NumberU64()-windowSize {
			if ext.block == nil {
				log.Debug("delete blockExt(only signs) from blockExtMap", "Hash", hash)
				delete(cbft.blockExtMap, hash)
			} else if ext.block.Hash() != newConfirmed.block.Hash() {
				log.Debug("delete blockExt from blockExtMap", "Hash", hash, "number", ext.block.NumberU64())
				delete(cbft.blockExtMap, hash)
			}
		}
	}

	for number, _ := range cbft.signedSet {
		if number <= cbft.highestConfirmed.block.NumberU64()-windowSize {
			log.Debug("delete number from signedSet", "number", number)
			delete(cbft.signedSet, number)
		}
	}

	log.Debug("remaining blocks in memory", "remainingBlocks", len(cbft.blockExtMap))
}

// handleNewConfirmed handles the new confirmed block.
// 1. If new confirmed block is higher than cbft.highestConfirmed, and is its descendant, then the new confirmed block should be saved into chain.
// In this case, its parent or ancestor will be confirmed indirectly even their's signs are not enough.
// 2. The new confirmed block is lower than cbft.highestConfirmed, then it maybe fork the original chain.
// In this case, the transactions including in the forked blocks and excluding from the new fork blocks should be recovered into pending queue of tx pool.
func (cbft *Cbft) handleNewConfirmed(newConfirmed *BlockExt) error {
	if newConfirmed.block.NumberU64() > cbft.highestConfirmed.block.NumberU64() {
		if cbft.highestConfirmed.isAncestor(newConfirmed) {
			//cbft.highestConfirmed is the new confirmed block's ancestor
			blocksToStore := cbft.backTrackTillStored(newConfirmed)
			if blocksToStore == nil {
				return errListConfirmedBlocks
			}
			return cbft.handleNewConfirmedContinue(newConfirmed, blocksToStore[1:])
		} else {
			//the new confirmed is higher, but but it is not a descendant of cbft.highestConfirmed
			log.Warn("consensus error, new confirmed block is higher, but it is not a descendant of the cbft.highestConfirmed")
			return nil
		}
	} else if newConfirmed.block.NumberU64() == cbft.highestConfirmed.block.NumberU64() {
		log.Warn("consensus error, new confirmed block is as high as cbft.highestConfirmed")
		return nil
	} else {
		log.Warn("new confirmed block is lower than cbft.highestConfirmed, then it maybe fork the original chain.")
		newFork := cbft.backTrackTillStored(newConfirmed)
		if len(newFork) <= 1 {
			log.Error("new fork error")
			return nil
		}

		//newForm[0] is a confirmed and stored block
		oldFork := cbft.backTrackBlocks(cbft.highestConfirmed, newFork[0], true)
		if len(newFork) >= len(oldFork) {
			log.Error("new fork chain must shorter than the original")
			return nil
		}

		if isFork, cause := cbft.isFork(newFork, oldFork); !isFork {
			log.Warn("consensus success, but cannot fork to new confirmed", "causeHash", cause.block.Hash(), "causeNumber", cause.block.NumberU64())
			return nil
		} else {
			err := cbft.handleNewConfirmedContinue(newConfirmed, newFork[1:])
			if err == nil {
				log.Warn("chain forks to new confirmed", "causeHash", cause.block.Hash(), "causeNumber", cause.block.NumberU64())
				return nil
			} else {
				log.Error("chain forks error", "err", err)
				return nil
			}
		}
	}
}

func (cbft *Cbft) handleNewConfirmedContinue(newConfirmed *BlockExt, blocksToStore []*BlockExt) error {
	log.Debug("call handleNewConfirmedContinue()", "blockCount", len(blocksToStore))

	cbft.storeBlocks(blocksToStore)

	cbft.highestConfirmed = newConfirmed
	highestLogical := cbft.findHighestSignedByLocal(newConfirmed)
	if highestLogical == nil {
		highestLogical = cbft.findHighest(newConfirmed)
	}
	if highestLogical == nil {
		return errHighestLogicalBlock
	}

	setHighestLogical(highestLogical)

	//free memory
	cbft.slideWindow(newConfirmed)
	return nil
}

// signReceiver handles the received block signature
func (cbft *Cbft) signReceiver(sig *cbfttypes.BlockSignature) error {
	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	log.Debug("=== call handleNewConfirmedContinue() ===", "Hash", sig.Hash, "number", sig.Number.Uint64())

	if sig.Number.Uint64() <= cbft.highestConfirmed.number {
		log.Warn("block sign is too late")
		return nil
	}

	ext := cbft.findBlockExt(sig.Hash)
	if ext == nil {
		log.Debug("have not received the corresponding block")
		//the block is nil
		ext = NewBlockExt(nil, sig.Number.Uint64())
		ext.isLinked = false

		cbft.saveBlock(sig.Hash, ext)
	} else if ext.isStored {
		// Receive the signature of the confirmed block and throw it away directly.
		log.Debug("received a highestConfirmed block's signature, just discard it")
		return nil
	}

	cbft.collectSign(ext, sig.Signature)

	log.Debug("count signatures", "signCount", len(ext.signs))

	if ext.isConfirmed && ext.isLinked {
		return cbft.handleNewConfirmed(ext)
	}

	log.Debug("=== end to handle new signature ===", "Hash", sig.Hash, "number", sig.Number.Uint64())

	return nil
}

//blockReceiver handles the new block
func (cbft *Cbft) blockReceiver(block *types.Block) error {

	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	log.Debug("=== call blockReceiver() ===", "Hash", block.Hash(), "number", block.Number().Uint64(), "ParentHash", block.ParentHash())

	if block.NumberU64() <= cbft.highestConfirmed.block.NumberU64() {
		log.Warn("Received block is lower than the highestConfirmed block")
		return nil
	}

	if block.NumberU64() <= 0 {
		return errGenesisBlock
	}

	if block.NumberU64() <= cbft.highestConfirmed.number {
		return errBlockNumber
	}
	//recover the producer's NodeID
	producerNodeID, sign, err := ecrecover(block.Header())
	if err != nil {
		return err
	}

	curTime := toMilliseconds(time.Now())

	keepIt := cbft.shouldKeepIt(curTime, producerNodeID)
	log.Debug("check if block should be kept", "result", keepIt, "producerNodeID", hex.EncodeToString(producerNodeID.Bytes()[:8]))
	if !keepIt {
		return errIllegalBlock
	}

	//to check if there's a existing blockExt for received block
	//sometime we'll receive the block's sign before the block self.
	ext := cbft.findBlockExt(block.Hash())
	if ext == nil {
		ext = NewBlockExt(block, block.NumberU64())
		//default
		ext.isLinked = false

		cbft.saveBlock(block.Hash(), ext)

	} else if ext.block == nil {
		//received its sign before.
		ext.block = block
	} else {
		return errDuplicatedBlock
	}

	//collect the block's sign of producer
	cbft.collectSign(ext, common.NewBlockConfirmSign(sign))

	parent := ext.findParent()
	if parent != nil && parent.isLinked {
		inTurn := cbft.inTurnVerify(curTime, producerNodeID)
		log.Debug("check if block is in turn", "result", inTurn, "producerNodeID", hex.EncodeToString(producerNodeID.Bytes()[:8]))

		passed := flowControl.control(producerNodeID, curTime)
		log.Debug("check if block is allowed by flow control", "result", passed, "producerNodeID", hex.EncodeToString(producerNodeID.Bytes()[:8]))

		signIfPossible := inTurn && passed && cbft.highestConfirmed.isAncestor(ext)

		cbft.handleBlockAndDescendant(ext, parent, signIfPossible)

		newConfirmed := cbft.findNewHighestConfirmed(ext)
		if newConfirmed != nil {
			log.Debug("found new highest confirmed block")
			// cbft.handleNewConfirmed will reset highest logical block
			return cbft.handleNewConfirmed(newConfirmed)
		} else {
			//reset highest logical block
			if ext.isSigned {
				setHighestLogical(ext)
			}
		}
	} else {
		log.Warn("cannot find block's parent, just keep it")
	}

	log.Debug("=== end to handle block ===", "Hash", block.Hash(), "number", block.Number().Uint64())
	return nil
}

// ShouldSeal checks if it's local's turn to package new block at current time.
func (cbft *Cbft) ShouldSeal() (bool, error) {
	log.Trace("call ShouldSeal()")
	return cbft.inTurn(), nil
}

// ConsensusNodes returns all consensus nodes.
func (cbft *Cbft) ConsensusNodes() ([]discover.NodeID, error) {
	log.Trace("call ConsensusNodes()", "dposNodeCount", len(cbft.dpos.primaryNodeList))
	return cbft.dpos.primaryNodeList, nil
}

// CheckConsensusNode check if the nodeID is a consensus node.
func (cbft *Cbft) CheckConsensusNode(nodeID discover.NodeID) (bool, error) {
	log.Trace("call CheckConsensusNode()", "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]))
	return cbft.dpos.NodeIndex(nodeID) >= 0, nil
}

// IsConsensusNode check if local is a consensus node.
func (cbft *Cbft) IsConsensusNode() (bool, error) {
	log.Trace("call IsConsensusNode()")
	return cbft.dpos.NodeIndex(cbft.config.NodeID) >= 0, nil
}

// Author implements consensus.Engine, returning the Ethereum address recovered
// from the signature in the header's extra-data section.
func (cbft *Cbft) Author(header *types.Header) (common.Address, error) {
	log.Trace("call Author()", "Hash", header.Hash(), "number", header.Number.Uint64())
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (cbft *Cbft) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	log.Trace("call VerifyHeader()", "Hash", header.Hash(), "number", header.Number.Uint64(), "seal", seal)

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
	log.Trace("call VerifySeal()", "Hash", header.Hash(), "number", header.Number.String())

	return cbft.verifySeal(chain, header, nil)
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (b *Cbft) Prepare(chain consensus.ChainReader, header *types.Header) error {
	log.Debug("call Prepare()", "Hash", header.Hash(), "number", header.Number.Uint64())

	cbft.lock.RLock()
	defer cbft.lock.RUnlock()

	// Check the parent block
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
	log.Debug("call Finalize()", "Hash", header.Hash(), "number", header.Number.Uint64(), "txs", len(txs), "receipts", len(receipts))
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)
	return types.NewBlock(header, txs, nil, receipts), nil
}

//to sign the block, and store the sign to header.Extra[32:], send the sign to chanel to broadcast to other consensus nodes
func (cbft *Cbft) Seal(chain consensus.ChainReader, block *types.Block, sealResultCh chan<- *types.Block, stopCh <-chan struct{}) error {
	cbft.lock.Lock()
	defer cbft.lock.Unlock()

	log.Debug("call Seal()", "number", block.NumberU64(), "parentHash", block.ParentHash())

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

	curExt := NewBlockExt(sealedBlock, sealedBlock.NumberU64())

	//this block is produced by local node, so need not execute in cbft.
	curExt.isLinked = true

	//collect the sign
	cbft.collectSign(curExt, common.NewBlockConfirmSign(sign))

	//save the block to cbft.blockExtMap
	cbft.saveBlock(sealedBlock.Hash(), curExt)

	log.Debug("seal complete", "Hash", sealedBlock.Hash(), "number", block.NumberU64())

	if len(cbft.dpos.primaryNodeList) == 1 {
		//only one consensus node, so, each block is highestConfirmed. (lock is needless)
		return cbft.handleNewConfirmed(curExt)
	}

	//reset cbft.highestLogicalBlockExt cause this block is produced by myself
	setHighestLogical(curExt)

	go func() {
		select {
		case <-stopCh:
			return
		case sealResultCh <- sealedBlock:
		default:
			log.Warn("Sealing result is not ready by miner", "sealHash", cbft.SealHash(header))
		}
	}()
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
	log.Debug("call SealHash()", "Hash", header.Hash(), "number", header.Number.Uint64())
	return sealHash(header)
}

// Close implements consensus.Engine. It's a noop for cbft as there is are no background threads.
func (cbft *Cbft) Close() error {
	log.Trace("call Close()")

	var err error
	cbft.closeOnce.Do(func() {
		// Short circuit if the exit channel is not allocated.
		if cbft.exitCh == nil {
			return
		}
		errc := make(chan error)
		cbft.exitCh <- errc
		err = <-errc
		close(cbft.exitCh)
	})
	return err
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
	log.Debug("call OnBlockSignature()", "Hash", rcvSign.Hash, "number", rcvSign.Number, "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]), "signHash", rcvSign.SignHash)

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
	log.Trace("call OnNewBlock()", "Hash", rcvBlock.Hash(), "number", rcvBlock.NumberU64(), "ParentHash", rcvBlock.ParentHash())

	cbft.dataReceiveCh <- rcvBlock
	return nil
}

// OnPong is called by protocol handler when it received a new Pong message by P2P.
func (cbft *Cbft) OnPong(nodeID discover.NodeID, netLatency int64) error {
	log.Trace("call OnPong()", "nodeID", hex.EncodeToString(nodeID.Bytes()[:8]), "netLatency", netLatency)
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
	cbft.lock.RLock()
	defer cbft.lock.RUnlock()

	log.Debug("call HighestLogicalBlock() ...")

	return cbft.highestLogical.block
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
		ext.isStored = true
		log.Debug("send to channel", "Hash", ext.block.Hash(), "number", ext.block.NumberU64(), "signCount", len(ext.signs))
		cbft.cbftResultOutCh <- cbftResult
	}
}

// inTurn return if it is local's turn to package new block.
func (cbft *Cbft) inTurn() bool {
	curTime := toMilliseconds(time.Now())
	inturn := cbft.calTurn(curTime, cbft.config.NodeID)
	log.Debug("inTurn", "result", inturn)
	return inturn

}

// inTurnVerify verifies the time is in the time-window of the nodeID to package new block.
func (cbft *Cbft) inTurnVerify(curTime int64, nodeID discover.NodeID) bool {
	latency := cbft.avgLatency(nodeID)
	if latency >= maxAvgLatency {
		log.Debug("inTurnVerify, return false cause of net latency", "result", false, "latency", latency)
		return false
	}
	inTurnVerify := cbft.calTurn(curTime-latency, nodeID)
	log.Debug("inTurnVerify", "result", inTurnVerify, "latency", latency)
	return inTurnVerify
}

//shouldKeepIt verifies the time is legal to package new block for the nodeID.
func (cbft *Cbft) shouldKeepIt(curTime int64, nodeID discover.NodeID) bool {
	offset := 1000 * (cbft.config.Duration/2 - 1)
	keepIt := cbft.calTurn(curTime-offset, nodeID)
	if !keepIt {
		keepIt = cbft.calTurn(curTime+offset, nodeID)
	}
	log.Debug("shouldKeepIt", "result", keepIt, "offset", offset)
	return keepIt
}

func (cbft *Cbft) calTurn(curTime int64, nodeID discover.NodeID) bool {
	nodeIdx := cbft.dpos.NodeIndex(nodeID)
	startEpoch := cbft.dpos.StartTimeOfEpoch() * 1000

	if nodeIdx >= 0 {
		durationPerNode := cbft.config.Duration * 1000
		durationPerTurn := durationPerNode * int64(len(cbft.dpos.primaryNodeList))

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

func (cbft *Cbft) getThreshold() int {
	trunc := len(cbft.dpos.primaryNodeList) * 2 / 3
	return int(trunc + 1)
}

func toMilliseconds(t time.Time) int64 {
	return t.UnixNano() / 1e6
}
