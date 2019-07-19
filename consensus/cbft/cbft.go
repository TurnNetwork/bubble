package cbft

import (
	"bytes"
	"crypto/ecdsa"

	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/fetcher"

	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/consensus"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/evidence"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/executor"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/protocols"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/rules"
	cstate "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/state"
	ctypes "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/types"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/validator"
	"github.com/PlatONnetwork/PlatON-Go/consensus/cbft/wal"

	"github.com/PlatONnetwork/PlatON-Go/core/cbfttypes"

	"github.com/PlatONnetwork/PlatON-Go/core/state"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/event"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/node"
	"github.com/PlatONnetwork/PlatON-Go/p2p"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/params"
	"github.com/PlatONnetwork/PlatON-Go/rpc"
)

const cbftVersion = 1

type Config struct {
	sys    *params.CbftConfig
	option *OptionsConfig
}

type Cbft struct {
	config           Config
	eventMux         *event.TypeMux
	closeOnce        sync.Once
	exitCh           chan struct{}
	txPool           consensus.TxPoolReset
	blockChain       consensus.ChainReader
	blockCacheWriter consensus.BlockCacheWriter
	peerMsgCh        chan *ctypes.MsgInfo
	syncMsgCh        chan *ctypes.MsgInfo
	evPool           evidence.EvidencePool
	log              log.Logger

	// Async call channel
	asyncCallCh chan func()

	fetcher *fetcher.Fetcher
	// Control the current view state
	state cstate.ViewState

	// Block asyncExecutor, the block responsible for executing the current view
	asyncExecutor executor.AsyncBlockExecutor

	// Verification security rules for proposed blocks and viewchange
	safetyRules rules.SafetyRules

	// Determine when to allow voting
	voteRules rules.VoteRules

	// Validator pool
	validatorPool *validator.ValidatorPool

	// Store blocks that are not committed
	blockTree ctypes.BlockTree

	// wal
	nodeServiceContext *node.ServiceContext
	wal                wal.Wal
	stateMu            sync.Mutex
	viewMu             sync.Mutex
}

func New(sysConfig *params.CbftConfig, optConfig *OptionsConfig, eventMux *event.TypeMux, ctx *node.ServiceContext) *Cbft {
	cbft := &Cbft{
		config:             Config{sysConfig, optConfig},
		eventMux:           eventMux,
		exitCh:             make(chan struct{}),
		peerMsgCh:          make(chan *ctypes.MsgInfo, optConfig.PeerMsgQueueSize),
		syncMsgCh:          make(chan *ctypes.MsgInfo, optConfig.PeerMsgQueueSize),
		log:                log.New(),
		asyncCallCh:        make(chan func(), optConfig.PeerMsgQueueSize),
		nodeServiceContext: ctx,
	}

	if evPool, err := evidence.NewEvidencePool(); err == nil {
		cbft.evPool = evPool
	} else {
		return nil
	}

	//todo init safety rules, vote rules, state, asyncExecutor
	cbft.safetyRules = rules.NewSafetyRules(&cbft.state)
	cbft.voteRules = rules.NewVoteRules(&cbft.state)

	return cbft
}

func (cbft *Cbft) Start(chain consensus.ChainReader, blockCacheWriter consensus.BlockCacheWriter, txPool consensus.TxPoolReset, agency consensus.Agency) error {
	cbft.blockChain = chain
	cbft.txPool = txPool
	cbft.asyncExecutor = executor.NewAsyncExecutor(blockCacheWriter.Execute)
	cbft.validatorPool = validator.NewValidatorPool(agency, chain.CurrentHeader().Number.Uint64(), cbft.config.sys.NodeID)

	//Initialize block tree
	block := chain.GetBlock(chain.CurrentHeader().Hash(), chain.CurrentHeader().Number.Uint64())

	cbft.blockTree.InsertQCBlock(block, nil)

	//Initialize view state
	cbft.state.SetHighestExecutedBlock(block)
	cbft.state.SetHighestQCBlock(block)
	cbft.state.SetHighestLockBlock(block)
	cbft.state.SetHighestCommitBlock(block)

	// load wal state
	if err := cbft.LoadWal(); err != nil {
		return err
	}

	go cbft.receiveLoop()
	return nil
}

// Entrance: The messages related to the consensus are entered from here.
// The message sent from the peer node is sent to the CBFT message queue and
// there is a loop that will distribute the incoming message.
func (cbft *Cbft) ReceiveMessage(msg *ctypes.MsgInfo) {
	select {
	case cbft.peerMsgCh <- msg:
		cbft.log.Debug("Received message from peer", "peer", msg.PeerID, "msgType", reflect.TypeOf(msg.Msg), "msgHash", msg.Msg.MsgHash().TerminalString(), "BHash", msg.Msg.BHash().TerminalString())
	case <-cbft.exitCh:
		cbft.log.Error("Cbft exit")
	}
}

func (cbft *Cbft) LoadWal() error {
	// init wal and load wal state
	var err error
	if cbft.wal, err = wal.NewWal(cbft.nodeServiceContext, ""); err != nil {
		return err
	}
	//cbft.wal = &emptyWal{}

	if err = cbft.wal.LoadChainState(cbft.recoveryChainState); err != nil {
		return err
	}

	if err = cbft.wal.Load(cbft.recoveryMsg); err != nil {
		return err
	}
	return nil
}

//Receive all consensus related messages, all processing logic in the same goroutine
func (cbft *Cbft) receiveLoop() {
	// channel Divided into read-only type, writable type
	// Read-only is the channel that gets the current CBFT status.
	// Writable type is the channel that affects the consensus state

	for {
		select {
		case msg := <-cbft.peerMsgCh:
			cbft.handleConsensusMsg(msg)
		case msg := <-cbft.syncMsgCh:
			cbft.handleSyncMsg(msg)

		case fn := <-cbft.asyncCallCh:
			fn()
		default:
		}

		// read-only channel
		select {}
	}
}

//Handling consensus messages, there are three main types of messages. prepareBlock, prepareVote, viewchagne
func (cbft *Cbft) handleConsensusMsg(info *ctypes.MsgInfo) {
	msg, id := info.Msg, info.PeerID
	var err error

	switch msg := msg.(type) {
	case *protocols.PrepareBlock:
		err = cbft.OnPrepareBlock(id, msg)
	case *protocols.PrepareVote:
		err = cbft.OnPrepareVote(id, msg)
	case *protocols.ViewChange:
		err = cbft.OnViewChange(id, msg)
	}

	if err != nil {
		cbft.log.Error("Handle msg Failed", "error", err, "type", reflect.TypeOf(msg), "peer", id)
	}
}

// Behind the node will be synchronized by synchronization message
func (cbft *Cbft) handleSyncMsg(info *ctypes.MsgInfo) {
	msg, id := info.Msg, info.PeerID

	if cbft.fetcher.MatchTask(id, msg) {
		return
	}

	var err error
	switch msg.(type) {
	}

	if err != nil {
		cbft.log.Error("Handle msg Failed", "error", err, "type", reflect.TypeOf(msg), "peer", id)
	}
}

func (cbft *Cbft) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (cbft *Cbft) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return cbft.validatorPool.VerifyHeader(header)
}

func (Cbft) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	panic("implement me")
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (cbft *Cbft) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	cbft.log.Trace("Verify seal", "hash", header.Hash(), "number", header.Number)
	if header.Number.Uint64() == 0 {
		return errors.New("unknown block")
	}
	return nil
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header of running the transactions on top.
func (cbft *Cbft) Prepare(chain consensus.ChainReader, header *types.Header) error {
	cbft.log.Debug("Prepare", "hash", header.Hash(), "number", header.Number.Uint64())

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

// Finalize implements consensus.Engine, no block
// rewards given, and returns the final block.
func (cbft *Cbft) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, receipts []*types.Receipt) (*types.Block, error) {
	cbft.log.Debug("Finalize block", "hash", header.Hash(), "number", header.Number, "txs", len(txs), "receipts", len(receipts))
	header.Root = state.IntermediateRoot(true)
	return types.NewBlock(header, txs, receipts), nil
}

func (cbft *Cbft) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	cbft.log.Info("Seal block", "number", block.Number(), "parentHash", block.ParentHash())
	if block.NumberU64() == 0 {
		return errors.New("unknown block")
	}

	// TODO signature block

	cbft.asyncCallCh <- func() {
		cbft.OnSeal(block, results, stop)
	}
	return nil
}

func (cbft *Cbft) OnSeal(block *types.Block, results chan<- *types.Block, stop <-chan struct{}) {
	// TODO: check is turn to seal block

	if cbft.state.HighestExecutedBlock().Hash() != block.ParentHash() {
		cbft.log.Warn("Futile block cause highest executed block changed", "nubmer", block.Number(), "parentHash", block.ParentHash())
		return
	}

	me, _ := cbft.validatorPool.GetValidatorByNodeID(cbft.state.HighestQCBlock().NumberU64(), cbft.config.sys.NodeID)

	// TODO: seal process
	prepareBlock := &protocols.PrepareBlock{
		Epoch:         cbft.state.Epoch(),
		ViewNumber:    cbft.state.ViewNumber(),
		Block:         block,
		BlockIndex:    cbft.state.NumViewBlocks(),
		ProposalIndex: uint32(me.Index),
		ProposalAddr:  me.Address,
	}

	if cbft.state.NumViewBlocks() == 0 {
		parentBlock, parentQC := cbft.blockTree.FindBlockAndQC(block.ParentHash(), block.NumberU64()-1)
		if parentBlock == nil {
			cbft.log.Error("Can not find parent block", "number", block.Number(), "parentHash", block.ParentHash())
			return
		}
		prepareBlock.PrepareQC = parentQC
	}

	// TODO: add viewchange qc

	// TODO: signature block

	cbft.state.AddPrepareBlock(prepareBlock)
	cbft.state.SetHighestExecutedBlock(block)

	// TODO: single node process
	if cbft.validatorPool.Len(cbft.state.HighestQCBlock().NumberU64()) == 1 {
		cbft.state.SetHighestQCBlock(block)
		cbft.state.SetHighestLockBlock(block)

		// TODO: signature prepare qc
		var qc ctypes.QuorumCert
		cbft.commitBlock(block, &qc)
		cbft.state.SetHighestCommitBlock(block)
	}

	// TODO: broadcast block

	go func() {
		select {
		case <-stop:
			return
		case results <- block:
		default:
			cbft.log.Warn("Sealing result channel is not ready by miner", "sealHash", block.Header().SealHash())
		}
	}()
}

// SealHash returns the hash of a block prior to it being sealed.
func (cbft *Cbft) SealHash(header *types.Header) common.Hash {
	cbft.log.Debug("Seal hash", "hash", header.Hash(), "number", header.Number)
	return header.SealHash()
}

func (Cbft) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{}
}

func (cbft *Cbft) Protocols() []p2p.Protocol {
	panic("implement me")
}

func (cbft *Cbft) NextBaseBlock() *types.Block {
	result := make(chan *types.Block, 1)
	cbft.asyncCallCh <- func() {
		block := cbft.state.HighestExecutedBlock()
		cbft.log.Debug("Base block", "hash", block.Hash(), "number", block.Number())
		result <- block
	}
	return <-result
}

func (Cbft) InsertChain(block *types.Block, errCh chan error) {
	panic("implement me")
}

// HashBlock check if the specified block exists in block tree.
func (cbft *Cbft) HasBlock(hash common.Hash, number uint64) bool {
	if cbft.state.HighestExecutedBlock().NumberU64() >= number {
		return true
	}
	return false
}

func (Cbft) Status() string {
	panic("implement me")
}

// GetBlockByHash get the specified block by hash.
func (cbft *Cbft) GetBlockByHash(hash common.Hash) *types.Block {
	result := make(chan *types.Block, 1)
	cbft.asyncCallCh <- func() {
		block := cbft.blockTree.FindBlockByHash(hash)
		result <- block
	}
	return <-result
}

// CurrentBlock get the current lock block.
func (cbft *Cbft) CurrentBlock() *types.Block {
	return cbft.state.HighestLockBlock()
}

func (cbft *Cbft) FastSyncCommitHead() <-chan error {
	result := make(chan error, 1)

	cbft.asyncCallCh <- func() {
		currentBlock := cbft.blockChain.GetBlock(cbft.blockChain.CurrentHeader().Hash(), cbft.blockChain.CurrentHeader().Number.Uint64())

		// TODO: update view
		cbft.state.SetHighestExecutedBlock(currentBlock)
		cbft.state.SetHighestQCBlock(currentBlock)
		cbft.state.SetHighestLockBlock(currentBlock)
		cbft.state.SetHighestCommitBlock(currentBlock)

		result <- nil
	}
	return result
}

func (cbft *Cbft) Close() error {
	cbft.log.Info("Close cbft consensus")
	cbft.closeOnce.Do(func() {
		// Short circuit if the exit channel is not allocated.
		if cbft.exitCh == nil {
			return
		}
		close(cbft.exitCh)
	})
	if cbft.asyncExecutor != nil {
		cbft.asyncExecutor.Stop()
	}
	return nil
}

func (cbft *Cbft) ConsensusNodes() ([]discover.NodeID, error) {
	return cbft.validatorPool.ValidatorList(cbft.state.HighestQCBlock().NumberU64()), nil
}

// ShouldSeal check if we can seal block.
func (cbft *Cbft) ShouldSeal(curTime time.Time) (bool, error) {
	result := make(chan error, 1)
	// FIXME: should use independent channel?
	cbft.asyncCallCh <- func() {
		cbft.OnShouldSeal(result)
	}
	select {
	case err := <-result:
		return err == nil, err
	case <-time.After(2 * time.Millisecond):
		return false, errors.New("CBFT engine busy")
	}
}

func (cbft *Cbft) OnShouldSeal(result chan error) {
	currentExecutedBlockNumber := cbft.state.HighestExecutedBlock().NumberU64()
	if !cbft.validatorPool.IsValidator(currentExecutedBlockNumber, cbft.config.sys.NodeID) {
		result <- errors.New("current node not a validator")
		return
	}

	numValidators := cbft.validatorPool.Len(currentExecutedBlockNumber)
	currentProposer := cbft.state.ViewNumber() % uint64(numValidators)
	validator, _ := cbft.validatorPool.GetValidatorByNodeID(currentExecutedBlockNumber, cbft.config.sys.NodeID)
	if currentProposer != uint64(validator.Index) {
		result <- errors.New("current node not the proposer")
		return
	}

	if cbft.state.NumViewBlocks() >= cbft.config.sys.Amount {
		result <- errors.New("produce block over limit")
		return
	}
	result <- nil
}

func (cbft *Cbft) CalcBlockDeadline(timePoint time.Time) time.Time {
	// FIXME: condition race
	produceInterval := time.Duration(cbft.config.sys.Period/uint64(cbft.config.sys.Amount)) * time.Millisecond
	if cbft.state.Deadline().Sub(timePoint) > produceInterval {
		return timePoint.Add(produceInterval)
	}
	return cbft.state.Deadline()
}

func (cbft *Cbft) CalcNextBlockTime(blockTime time.Time) time.Time {
	// FIXME: condition race
	produceInterval := time.Duration(cbft.config.sys.Period/uint64(cbft.config.sys.Amount)) * time.Millisecond
	if time.Now().Sub(blockTime) < produceInterval {
		// TODO: add network latency
		return time.Now().Add(time.Now().Sub(blockTime))
	}
	return time.Now()
}

func (cbft *Cbft) IsConsensusNode() bool {
	return cbft.validatorPool.IsValidator(cbft.state.HighestQCBlock().NumberU64(), cbft.config.sys.NodeID)
}

func (cbft *Cbft) GetBlock(hash common.Hash, number uint64) *types.Block {
	result := make(chan *types.Block, 1)
	cbft.asyncCallCh <- func() {
		block, _ := cbft.blockTree.FindBlockAndQC(hash, number)
		result <- block
	}
	return <-result
}

func (cbft *Cbft) GetBlockWithoutLock(hash common.Hash, number uint64) *types.Block {
	block, _ := cbft.blockTree.FindBlockAndQC(hash, number)
	return block
}

func (Cbft) SetPrivateKey(privateKey *ecdsa.PrivateKey) {
	panic("implement me")
}

func (Cbft) IsSignedBySelf(sealHash common.Hash, signature []byte) bool {
	panic("implement me")
}

func (Cbft) Evidences() string {
	panic("implement me")
}

func (Cbft) TracingSwitch(flag int8) {
	panic("implement me")
}

func (cbft *Cbft) OnPong(nodeID discover.NodeID, netLatency int64) error {
	panic("need to be improved")
	return nil
}

func (cbft *Cbft) Config() *Config {
	panic("need to be improved")
	return nil
}

func (cbft *Cbft) commitBlock(block *types.Block, qc *ctypes.QuorumCert) {
	extra, err := ctypes.EncodeExtra(byte(cbftVersion), qc)
	if err != nil {
		cbft.log.Error("Encode extra error", "nubmer", block.Number(), "hash", block.Hash(), "cbftVersion", cbftVersion)
		return
	}

	cbft.log.Debug("Send consensus result to worker", "number", block.Number(), "hash", block.Hash())
	cbft.eventMux.Post(cbfttypes.CbftResult{
		Block:     block,
		ExtraData: extra,
		SyncState: nil,
	})
}
