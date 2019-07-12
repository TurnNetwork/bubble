package plugin

import (
	"encoding/hex"
	"errors"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/consensus"
	"github.com/PlatONnetwork/PlatON-Go/common/vm"
	"github.com/PlatONnetwork/PlatON-Go/core/snapshotdb"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/life/utils"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
	"github.com/PlatONnetwork/PlatON-Go/x/staking"
	"github.com/PlatONnetwork/PlatON-Go/x/xcom"
	"github.com/PlatONnetwork/PlatON-Go/x/xutil"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/util"
	"math/big"
	"sync"
)

var (
	// Identifies the prefix of the current round
	curAbnormalPrefix = []byte("SlashCb")
	// Identifies the prefix of the previous round
	preAbnormalPrefix = []byte("SlashPb")

	errMutiSignVerify	= errors.New("multi-sign verification failed")
	errSlashExist		= errors.New("punishment has been implemented")

	once = sync.Once{}
)

type SlashingPlugin struct {
	db				snapshotdb.DB
	decodeEvidence 	func(data string) (consensus.Evidences, error)
}

var slsh *SlashingPlugin

func SlashInstance() *SlashingPlugin {
	once.Do(func() {
		slsh = &SlashingPlugin{
			db:snapshotdb.Instance(),
		}
	})
	return slsh
}

//func ClearSlashPlugin() error {
//	if nil == slsh {
//		return common.NewSysError("the SlashPlugin already be nil")
//	}
//	slsh = nil
//	return nil
//}

func (sp *SlashingPlugin) SetDecodeEvidenceFun(f func(data string) (consensus.Evidences, error)) {
	sp.decodeEvidence = f
}

func (sp *SlashingPlugin) BeginBlock(blockHash common.Hash, header *types.Header, state xcom.StateDB) error {
	// If it is the 230th block of each round, it will punish the node with abnormal block rate.
	if header.Number.Uint64() > xcom.GetEcModInstance().Staking.ConsensusSize && xutil.IsElection(header.Number.Uint64()) {
		log.Debug("slashingPlugin Ranking block amount", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "consensusSize", xcom.ConsensusSize, "electionDistance", xcom.ElectionDistance)
		if result, err := sp.GetPreNodeAmount(); nil != err {
			return err
		} else {
			if nil == result {
				log.Error("slashingPlugin GetPreNodeAmount is nil", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()))
				return common.NewBizError("block rate data not found")
			}
			validatorList, err := stk.GetCandidateONRound(blockHash, header.Number.Uint64(), PreviousRound, QueryStartIrr)
			if nil != err {
				return err
			}
			for _, validator := range validatorList {
				nodeId := validator.NodeId
				amount, success := result[nodeId]
				isSlash := false
				var rate uint32
				isDelete := false
				if success {
					// Start to punish nodes with abnormal block rate
					log.Debug("slashingPlugin node block amount", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(nodeId.Bytes()), "amount", amount)
					if isAbnormal(amount) {
						log.Debug("Slashing anomalous nodes", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(nodeId.Bytes()))
						if amount <= xcom.GetEcModInstance().Slashing.BlockAmountLow && amount > xcom.GetEcModInstance().Slashing.BlockAmountHigh {
							isSlash = true
							rate = xcom.GetEcModInstance().Slashing.BlockAmountLowSlashing
						} else if amount <= xcom.GetEcModInstance().Slashing.BlockAmountHigh {
							isSlash = true
							isDelete = true
							rate = xcom.GetEcModInstance().Slashing.BlockAmountHighSlashing
						}
					}
				} else {
					isSlash = true
					rate = xcom.GetEcModInstance().Slashing.BlockAmountHighSlashing
				}
				if isSlash && rate > 0 {
					// If there is no record of the node, it means that there is no block, then the penalty is directly
					if err := stk.SlashCandidates(state, blockHash, header.Number.Uint64(), nodeId, calcSlashAmount(validator, rate), isDelete, staking.LowRatio, common.ZeroAddr); nil != err {
						log.Error("slashingPlugin SlashCandidates failed", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(nodeId.Bytes()), "err", err)
						return err
					}
				}
			}
		}
	}
	return nil
}

func (sp *SlashingPlugin) EndBlock(blockHash common.Hash, header *types.Header, state xcom.StateDB) error {
	return nil
}

func (sp *SlashingPlugin) Confirmed(block *types.Block) error {
	// If it is the first block in each round, switch the number of blocks in the upper and lower rounds.
	log.Debug("slashingPlugin Confirmed", "blockNumber", block.NumberU64(), "blockHash", hex.EncodeToString(block.Hash().Bytes()), "consensusSize", xcom.ConsensusSize)
	if (block.NumberU64() % xcom.GetEcModInstance().Staking.ConsensusSize == 1) && block.NumberU64() > 1 {
		if err := sp.switchEpoch(block.Hash()); nil != err {
			log.Error("slashingPlugin switchEpoch fail", "blockNumber", block.NumberU64(), "blockHash", hex.EncodeToString(block.Hash().Bytes()), "err", err)
			return err
		}
	}
	if err := sp.setBlockAmount(block.Hash(), block.Header()); nil != err {
		log.Error("slashingPlugin setBlockAmount fail", "blockNumber", block.NumberU64(), "blockHash", hex.EncodeToString(block.Hash().Bytes()), "err", err)
		return err
	}
	return nil
}

func (sp *SlashingPlugin) getBlockAmount(blockHash common.Hash, header *types.Header) (uint32, error) {
	log.Debug("slashingPlugin getBlockAmount", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()))
	nodeId, err := parseNodeId(header)
	if nil != err {
		return 0, err
	}
	value, err := sp.db.GetBaseDB(curKey(nodeId.Bytes()))
	if nil != err && err != snapshotdb.ErrNotFound {
		return 0, err
	}
	var amount uint32
	if err == snapshotdb.ErrNotFound {
		amount = 0
	} else {
		if err := rlp.DecodeBytes(value, &amount); nil != err {
			return 0, err
		}
	}
	return amount, nil
}

func (sp *SlashingPlugin) setBlockAmount(blockHash common.Hash, header *types.Header) error {
	log.Debug("slashingPlugin setBlockAmount", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()))
	nodeId, err := parseNodeId(header)
	if nil != err {
		return err
	}
	if value, err := sp.getBlockAmount(blockHash, header); nil != err {
		return err
	} else {
		value++
		if enValue, err := rlp.EncodeToBytes(value); nil != err {
			return err
		} else {
			if err := sp.db.PutBaseDB(curKey(nodeId.Bytes()), enValue); nil != err {
				return err
			}
			log.Debug("slashingPlugin setBlockAmount success", "blockNumber", header.Number.Uint64(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(nodeId.Bytes()), "value", value)
		}
	}
	return nil
}

func (sp *SlashingPlugin) switchEpoch(blockHash common.Hash) error {
	log.Debug("slashingPlugin switchEpoch", "blockHash", hex.EncodeToString(blockHash.Bytes()))
	preCount := 0
	err := sp.db.WalkBaseDB(util.BytesPrefix(preAbnormalPrefix), func(num *big.Int, iter iterator.Iterator) error {
		for iter.Next() {
			if err := sp.db.DelBaseDB(iter.Key()); nil != err {
				return err
			}
			preCount++
		}
		return nil
	})
	if nil != err {
		return err
	}
	curCount := 0
	err = sp.db.WalkBaseDB(util.BytesPrefix(curAbnormalPrefix), func(num *big.Int, iter iterator.Iterator) error {
		for iter.Next() {
			key := iter.Key()
			if err := sp.db.DelBaseDB(key); nil != err {
				return err
			}
			key = preKey(key[len(curAbnormalPrefix):])
			if err := sp.db.PutBaseDB(key, iter.Value()); nil != err {
				return err
			}
			curCount++
		}
		return nil
	})
	if nil != err {
		return err
	}
	log.Info("slashingPlugin switchEpoch success", "blockHash", hex.EncodeToString(blockHash.Bytes()), "preCount", preCount, "curCount", curCount)
	return nil
}

// Get the consensus rate of all nodes in the previous round
func (sp *SlashingPlugin) GetPreNodeAmount() (map[discover.NodeID]uint32, error) {
	result := make(map[discover.NodeID]uint32)
	err := sp.db.WalkBaseDB(util.BytesPrefix(preAbnormalPrefix), func(num *big.Int, iter iterator.Iterator) error {
		for iter.Next() {
			key := iter.Key()
			value := iter.Value()
			var amount uint32
			if err := rlp.DecodeBytes(value, &amount); nil != err {
				log.Error("slashingPlugin rlp block amount fail", "value", value, "err", err)
				return err
			}
			log.Debug("slashingPlugin GetPreEpochAnomalyNode", "key", hex.EncodeToString(key), "value", amount)
			nodeId, err := getNodeId(preAbnormalPrefix, key)
			if nil != err {
				return err
			}
			result[nodeId] = amount
		}
		return nil
	})
	if nil != err {
		return nil, err
	}
	return result, nil
}

func (sp *SlashingPlugin) Slash(data string, blockHash common.Hash, blockNumber uint64, stateDB xcom.StateDB, caller common.Address) error {
	evidences, err := sp.decodeEvidence(data)
	if nil != err {
		log.Error("Slash failed", "data", data, "err", err)
		return err
	}
	if len(evidences) == 0 {
		log.Error("slashingPlugin decodeEvidence len 0", "blockNumber", blockNumber, "blockHash", hex.EncodeToString(blockHash.Bytes()), "data", data)
		return common.NewBizError(errMutiSignVerify.Error())
	}
	for _, evidence := range evidences {
		if err := evidence.Validate(); nil != err {
			return err
		}
		if value := sp.getSlashResult(evidence.Address(), evidence.BlockNumber(), uint32(evidence.Type()), stateDB); len(value) > 0 {
			log.Error("Execution slashing failed", "blockNumber", evidence.BlockNumber(), "evidenceHash", hex.EncodeToString(evidence.Hash()), "addr", hex.EncodeToString(evidence.Address().Bytes()), "type", evidence.Type())
			return common.NewBizError(errSlashExist.Error())
		}
		if candidate, err := stk.GetCandidateInfo(blockHash, evidence.Address()); nil != err {
			return common.NewBizError(err.Error())
		} else {
			if nil == candidate {
				log.Error("slashingPlugin GetCandidateInfo is nil", "blockNumber", blockNumber, "blockHash", hex.EncodeToString(blockHash.Bytes()), "addr", hex.EncodeToString(evidence.Address().Bytes()), "type", evidence.Type())
				return common.NewBizError(errMutiSignVerify.Error())
			}
			if err := stk.SlashCandidates(stateDB, blockHash, blockNumber, candidate.NodeId, calcSlashAmount(candidate, xcom.GetEcModInstance().Slashing.DuplicateSignLowSlashing), true, staking.DoubleSign, caller); nil != err {
				log.Error("slashingPlugin SlashCandidates failed", "blockNumber", blockNumber, "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(candidate.NodeId.Bytes()), "err", err)
				return err
			}
			sp.putSlashResult(evidence.Address(), evidence.BlockNumber(), uint32(evidence.Type()), stateDB)
			log.Info("Slash Multi-sign success", "currentBlockNumber", blockNumber, "mutiSignBlockNumber", evidence.BlockNumber(), "blockHash", hex.EncodeToString(blockHash.Bytes()), "nodeId", hex.EncodeToString(candidate.NodeId.Bytes()), "etype", evidence.Type(), "txHash", hex.EncodeToString(stateDB.TxHash().Bytes()))
		}
	}
	return nil
}

func (sp *SlashingPlugin) CheckMutiSign(addr common.Address, blockNumber uint64, etype uint32, stateDB xcom.StateDB) (bool, []byte, error) {
	if value := sp.getSlashResult(addr, blockNumber, etype, stateDB); len(value) > 0 {
		log.Info("CheckMutiSign exist", "blockNumber", blockNumber, "addr", hex.EncodeToString(addr.Bytes()), "type", etype, "txHash", hex.EncodeToString(value))
		return true, value, nil
	}
	return false, nil, nil
}

func (sp *SlashingPlugin) putSlashResult(addr common.Address, blockNumber uint64, etype uint32, stateDB xcom.StateDB) {
	stateDB.SetState(vm.SlashingContractAddr, mutiSignKey(addr, blockNumber, etype), stateDB.TxHash().Bytes())
}

func (sp *SlashingPlugin) getSlashResult(addr common.Address, blockNumber uint64, etype uint32, stateDB xcom.StateDB) []byte {
	return stateDB.GetState(vm.SlashingContractAddr, mutiSignKey(addr, blockNumber, etype))
}

// Multi-signed result key format addr+blockNumber+_+etype
func mutiSignKey(addr common.Address, blockNumber uint64, etype uint32) []byte {
	value := append(addr.Bytes(), utils.Uint64ToBytes(blockNumber)...)
	value = append(value, []byte("_")...)
	value = append(value, utils.Uint64ToBytes(uint64(etype))...)
	return value
}

func curKey(key []byte) []byte {
	return append(curAbnormalPrefix, key...)
}

func preKey(key []byte) []byte {
	return append(preAbnormalPrefix, key...)
}

func getNodeId(prefix []byte, key []byte) (discover.NodeID, error) {
	key = key[len(prefix):]
	nodeId, err := discover.BytesID(key)
	if nil != err {
		return discover.NodeID{}, err
	}
	return nodeId, nil
}

func isAbnormal(amount uint32) bool {
	if uint64(amount) < (xcom.GetEcModInstance().Staking.ConsensusSize / xcom.GetEcModInstance().Staking.ConsValidatorNum) {
		return true
	}
	return false
}

func parseNodeId(header *types.Header) (discover.NodeID, error) {
	log.Debug("extra parseNodeId", "extra", hex.EncodeToString(header.Extra), "sealHash", hex.EncodeToString(header.SealHash().Bytes()))
	sign := header.Extra[32:97]
	pk, err := crypto.SigToPub(header.SealHash().Bytes(), sign)
	if nil != err {
		return discover.NodeID{}, err
	}
	return discover.PubkeyID(pk), nil
}

func calcSlashAmount(candidate *staking.Candidate, rate uint32) *big.Int {
	sumAmount := new(big.Int)
	sumAmount.Add(candidate.Released, candidate.ReleasedHes)
	sumAmount.Add(sumAmount, candidate.RestrictingPlan)
	sumAmount.Add(sumAmount, candidate.RestrictingPlanHes)
	return sumAmount.Div(sumAmount, new(big.Int).SetUint64(uint64(rate)))
}