package validator

import (
	"sync"

	"github.com/PlatONnetwork/PlatON-Go/common"
	cvm "github.com/PlatONnetwork/PlatON-Go/common/vm"
	"github.com/PlatONnetwork/PlatON-Go/consensus"
	mycrypto "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/crypto"
	"github.com/PlatONnetwork/PlatON-Go/core"
	"github.com/PlatONnetwork/PlatON-Go/core/cbfttypes"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/core/vm"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/event"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
)

func newValidators(nodes []discover.Node, validBlockNumber uint64) *cbfttypes.Validators {
	vds := &cbfttypes.Validators{
		Nodes:            make(cbfttypes.ValidateNodeMap, len(nodes)),
		ValidBlockNumber: validBlockNumber,
	}

	for i, node := range nodes {
		pubkey, err := node.ID.Pubkey()
		if err != nil {
			panic(err)
		}

		vds.Nodes[node.ID] = &cbfttypes.ValidateNode{
			Index:   i,
			Address: crypto.PubkeyToAddress(*pubkey),
			PubKey:  pubkey,
			NodeID:  node.ID,
		}
	}
	return vds
}

type StaticAgency struct {
	consensus.Agency

	validators *cbfttypes.Validators
}

func NewStaticAgency(nodes []discover.Node) consensus.Agency {
	return &StaticAgency{
		validators: newValidators(nodes, 0),
	}
}

func (d *StaticAgency) Sign(interface{}) error {
	return nil
}

func (d *StaticAgency) VerifySign(interface{}) error {
	return nil
}

func (d *StaticAgency) VerifyHeader(*types.Header) error {
	return nil
}

func (d *StaticAgency) GetLastNumber(blockNumber uint64) uint64 {
	return 0
}

func (d *StaticAgency) GetValidator(uint64) (*cbfttypes.Validators, error) {
	return d.validators, nil
}

func (d *StaticAgency) IsCandidateNode(nodeID discover.NodeID) bool {
	return false
}

type InnerAgency struct {
	consensus.Agency

	blocksPerNode         uint64
	defaultBlocksPerRound uint64
	offset                uint64
	blockchain            *core.BlockChain
	defaultValidators     *cbfttypes.Validators
}

func NewInnerAgency(nodes []discover.Node, chain *core.BlockChain, blocksPerNode, offset int) consensus.Agency {
	return &InnerAgency{
		blocksPerNode:         uint64(blocksPerNode),
		defaultBlocksPerRound: uint64(len(nodes) * blocksPerNode),
		offset:                uint64(offset),
		blockchain:            chain,
		defaultValidators:     newValidators(nodes, 0),
	}
}

func (ia *InnerAgency) Sign(interface{}) error {
	return nil
}

func (ia *InnerAgency) VerifySign(interface{}) error {
	return nil
}

func (ia *InnerAgency) VerifyHeader(*types.Header) error {
	return nil
}

func (ia *InnerAgency) GetLastNumber(blockNumber uint64) uint64 {
	var lastBlockNumber uint64
	if blockNumber <= ia.defaultBlocksPerRound {
		lastBlockNumber = ia.defaultBlocksPerRound
	} else {
		vds, err := ia.GetValidator(blockNumber)
		if err != nil {
			log.Error("Get validator fail", "blockNumber", blockNumber)
			return 0
		}

		if vds.ValidBlockNumber == 0 && blockNumber%ia.defaultBlocksPerRound == 0 {
			return blockNumber
		}

		// lastNumber = vds.ValidBlockNumber + ia.blocksPerNode * vds.Len() - 1
		lastBlockNumber = vds.ValidBlockNumber + ia.blocksPerNode*uint64(vds.Len()) - 1

		// May be `CurrentValidators ` had not updated, so we need to calcuate `lastBlockNumber`
		// via `blockNumber`.
		if lastBlockNumber < blockNumber {
			blocksPerRound := ia.blocksPerNode * uint64(vds.Len())
			baseNum := blockNumber - (blockNumber % blocksPerRound)
			lastBlockNumber = baseNum + blocksPerRound
		}
	}
	log.Debug("Get last block number", "blockNumber", blockNumber, "lastBlockNumber", lastBlockNumber)
	return lastBlockNumber
}

func (ia *InnerAgency) GetValidator(blockNumber uint64) (v *cbfttypes.Validators, err error) {
	//var lastBlockNumber uint64
	/*
		defer func() {
			log.Trace("Get validator",
				"lastBlockNumber", lastBlockNumber,
				"blocksPerNode", ia.blocksPerNode,
				"blockNumber", blockNumber,
				"validators", v,
				"error", err)
		}()*/

	if blockNumber <= ia.defaultBlocksPerRound {
		return ia.defaultValidators, nil
	}

	// Otherwise, get validators from inner contract.
	vdsCftNum := blockNumber - ia.offset - 1
	block := ia.blockchain.GetBlockByNumber(vdsCftNum)
	if block == nil {
		log.Error("Get the block fail, use default validators", "number", vdsCftNum)
		return ia.defaultValidators, nil
	}
	state, err := ia.blockchain.StateAt(block.Root())
	if err != nil {
		log.Error("Get the state fail, use default validators", "number", block.Number(), "hash", block.Hash(), "error", err)
		return ia.defaultValidators, nil
	}
	b := state.GetState(cvm.ValidatorInnerContractAddr, []byte(vm.CurrentValidatorKey))
	var vds vm.Validators
	err = rlp.DecodeBytes(b, &vds)
	if err != nil {
		log.Error("RLP decode fail, use default validators", "number", block.Number(), "error", err)
		return ia.defaultValidators, nil
	}
	var validators cbfttypes.Validators
	validators.Nodes = make(cbfttypes.ValidateNodeMap, len(vds.ValidateNodes))
	for _, node := range vds.ValidateNodes {
		pubkey, _ := node.NodeID.Pubkey()
		validators.Nodes[node.NodeID] = &cbfttypes.ValidateNode{
			Index:   int(node.Index),
			Address: node.Address,
			PubKey:  pubkey,
		}
	}
	validators.ValidBlockNumber = vds.ValidBlockNumber
	return &validators, nil
}

func (ia *InnerAgency) IsCandidateNode(nodeID discover.NodeID) bool {
	return true
}

// ValidatorPool a pool storing validators.
type ValidatorPool struct {
	agency consensus.Agency

	lock sync.RWMutex

	// Current node's public key
	nodeID discover.NodeID

	// A block number which validators switch point.
	switchPoint uint64

	prevValidators    *cbfttypes.Validators // Previous validators
	currentValidators *cbfttypes.Validators // Current validators

}

// NewValidatorPool new a validator pool.
func NewValidatorPool(agency consensus.Agency, blockNumber uint64, nodeID discover.NodeID) *ValidatorPool {
	pool := &ValidatorPool{
		agency: agency,
		nodeID: nodeID,
	}
	// FIXME: Check `GetValidator` return error
	if agency.GetLastNumber(blockNumber) == blockNumber {
		pool.prevValidators, _ = agency.GetValidator(blockNumber)
		pool.currentValidators, _ = agency.GetValidator(nextRound(blockNumber))
	} else {
		pool.currentValidators, _ = agency.GetValidator(blockNumber)
		pool.prevValidators = pool.currentValidators
	}
	pool.switchPoint = blockNumber
	return pool
}

// ShouldSwitch check if should switch validators at the moment.
func (vp *ValidatorPool) ShouldSwitch(blockNumber uint64) bool {
	return blockNumber == vp.agency.GetLastNumber(blockNumber)
}

// Update switch validators.
func (vp *ValidatorPool) Update(blockNumber uint64, eventMux *event.TypeMux) error {
	vp.lock.Lock()
	defer vp.lock.Unlock()

	// Only updated once
	if blockNumber <= vp.switchPoint {
		return nil
	}

	isValidatorBefore := vp.IsValidator(blockNumber, vp.nodeID)

	nds, err := vp.agency.GetValidator(nextRound(blockNumber))
	if err != nil {
		return err
	}
	vp.prevValidators = vp.currentValidators
	vp.currentValidators = nds
	vp.switchPoint = blockNumber

	isValidatorAfter := vp.IsValidator(blockNumber, vp.nodeID)

	if isValidatorBefore {
		// If we are still a consensus node, that adding
		// new validators as consensus peer, and removing
		// validators. Added as consensus peersis because
		// we need to keep connect with other validators
		// in the consensus stages. Also we are not needed
		// to keep connect with old validators.
		if isValidatorAfter {
			for _, nodeID := range vp.currentValidators.NodeList() {
				if node, _ := vp.prevValidators.FindNodeByID(nodeID); node == nil {
					eventMux.Post(cbfttypes.AddValidatorEvent{NodeID: nodeID})
					log.Trace("Post AddValidatorEvent", "nodeID", nodeID.String())
				}
			}

			for _, nodeID := range vp.prevValidators.NodeList() {
				if node, _ := vp.currentValidators.FindNodeByID(nodeID); node == nil {
					eventMux.Post(cbfttypes.RemoveValidatorEvent{NodeID: nodeID})
					log.Trace("Post RemoveValidatorEvent", "nodeID", nodeID.String())
				}
			}
		} else {
			for _, nodeID := range vp.prevValidators.NodeList() {
				eventMux.Post(cbfttypes.RemoveValidatorEvent{NodeID: nodeID})
				log.Trace("Post RemoveValidatorEvent", "nodeID", nodeID.String())
			}
		}
	} else {
		// We are become a consensus node, that adding all
		// validators as consensus peer except us. Added as
		// consensus peers is because we need to keep connecting
		// with other validators in the consensus stages.
		if isValidatorAfter {
			for _, nodeID := range vp.currentValidators.NodeList() {
				if vp.nodeID == nodeID {
					// Ignore myself
					continue
				}

				eventMux.Post(cbfttypes.AddValidatorEvent{NodeID: nodeID})
				log.Trace("Post AddValidatorEvent", "nodeID", nodeID.String())
			}
		}

		// We are still not a consensus node, just update validator list.
	}

	return nil
}

// GetValidatorByNodeID get the validator by node id.
func (vp *ValidatorPool) GetValidatorByNodeID(blockNumber uint64, nodeID discover.NodeID) (*cbfttypes.ValidateNode, error) {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.FindNodeByID(nodeID)
	}
	return vp.currentValidators.FindNodeByID(nodeID)
}

// GetValidatorByAddr get the validator by address.
func (vp *ValidatorPool) GetValidatorByAddr(blockNumber uint64, addr common.Address) (*cbfttypes.ValidateNode, error) {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.FindNodeByAddress(addr)
	}
	return vp.currentValidators.FindNodeByAddress(addr)
}

// GetValidatorByIndex get the validator by index.
func (vp *ValidatorPool) GetValidatorByIndex(blockNumber uint64, index uint32) (*cbfttypes.ValidateNode, error) {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.FindNodeByIndex(int(index))
	}
	return vp.currentValidators.FindNodeByIndex(int(index))
}

// GetNodeIDByIndex get the node id by index.
func (vp *ValidatorPool) GetNodeIDByIndex(blockNumber uint64, index int) discover.NodeID {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.NodeID(index)
	}
	return vp.currentValidators.NodeID(index)
}

// GetIndexByNodeID get the index by node id.
func (vp *ValidatorPool) GetIndexByNodeID(blockNumber uint64, nodeID discover.NodeID) (int, error) {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.Index(nodeID)
	}
	return vp.currentValidators.Index(nodeID)
}

// ValidatorList get the validator list.
func (vp *ValidatorPool) ValidatorList(blockNumber uint64) []discover.NodeID {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.NodeList()
	}
	return vp.currentValidators.NodeList()
}

// VerifyHeader verify block's header.
func (vp *ValidatorPool) VerifyHeader(header *types.Header) error {
	return vp.agency.VerifyHeader(header)
}

// IsValidator check if the node is validator.
func (vp *ValidatorPool) IsValidator(blockNumber uint64, nodeID discover.NodeID) bool {
	_, err := vp.GetValidatorByNodeID(blockNumber, nodeID)
	return err == nil
}

// IsCandidateNode check if the node is candidate node.
func (vp *ValidatorPool) IsCandidateNode(nodeID discover.NodeID) bool {
	return vp.agency.IsCandidateNode(nodeID)
}

// Len return number of validators.
func (vp *ValidatorPool) Len(blockNumber uint64) int {
	vp.lock.RLock()
	defer vp.lock.RUnlock()

	if blockNumber <= vp.switchPoint {
		return vp.prevValidators.Len()
	}
	return vp.currentValidators.Len()
}

// Verify verify signature.
func (vp *ValidatorPool) Verify(blockNumber uint64, validatorIndex uint32, msg, signature []byte) bool {
	validator, err := vp.GetValidatorByIndex(blockNumber, validatorIndex)
	if err != nil {
		return false
	}
	return validator.Verify(msg, signature)
}

// VerifyAggSig verify aggregation signature.
func (vp *ValidatorPool) VerifyAggSig(blockNumber uint64, validatorIndexes []uint32, msg, signature []byte) bool {
	vp.lock.RLock()
	validators := vp.currentValidators
	if blockNumber <= vp.switchPoint {
		validators = vp.prevValidators
	}

	nodeList, err := validators.NodeListByIndexes(validatorIndexes)
	if err != nil {
		return false
	}
	vp.lock.RUnlock()

	pub := &mycrypto.PublicKey{}
	for _, node := range nodeList {
		pub.Add(node.AggPubKey)
	}

	sig := &mycrypto.Signature{}
	err = sig.Recover(string(signature))
	if err != nil {
		return false
	}
	return sig.Verify(pub, string(msg))
}

func nextRound(blockNumber uint64) uint64 {
	return blockNumber + 1
}
