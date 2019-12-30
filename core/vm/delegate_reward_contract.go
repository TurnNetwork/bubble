package vm

import (
	"fmt"
	"math/big"

	"github.com/PlatONnetwork/PlatON-Go/x/xutil"

	"github.com/PlatONnetwork/PlatON-Go/common/vm"

	"github.com/PlatONnetwork/PlatON-Go/x/staking"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/log"

	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"

	"github.com/PlatONnetwork/PlatON-Go/params"
	"github.com/PlatONnetwork/PlatON-Go/x/plugin"
)

const (
	TxWithdrawDelegateReward       = 5000
	FuncNameWithdrawDelegateReward = "WithdrawDelegateReward"
	QueryDelegateReward            = 5100
	FuncNameDelegateReward         = "QueryDelegateReward"
)

type DelegateRewardContract struct {
	Plugin    *plugin.RewardMgrPlugin
	stkPlugin *plugin.StakingPlugin

	Contract *Contract
	Evm      *EVM
}

func (rc *DelegateRewardContract) RequiredGas(input []byte) uint64 {
	return params.DelegateRewardGas
}

func (rc *DelegateRewardContract) Run(input []byte) ([]byte, error) {
	return execPlatonContract(input, rc.FnSigns())
}

func (rc *DelegateRewardContract) FnSigns() map[uint16]interface{} {
	return map[uint16]interface{}{
		// Set
		TxWithdrawDelegateReward: rc.withdrawDelegateReward,

		// Get
		QueryDelegateReward: rc.getDelegateReward,
	}
}

func (rc *DelegateRewardContract) CheckGasPrice(gasPrice *big.Int, fcode uint16) error {
	return nil
}

func (rc *DelegateRewardContract) withdrawDelegateReward() ([]byte, error) {
	from := rc.Contract.CallerAddress
	txHash := rc.Evm.StateDB.TxHash()
	blockNum := rc.Evm.BlockNumber
	blockHash := rc.Evm.BlockHash
	state := rc.Evm.StateDB

	log.Debug("Call withdrawDelegateReward of DelegateRewardContract", "blockNumber", blockNum.Uint64(),
		"blockHash", blockHash.TerminalString(), "txHash", txHash.Hex(), "from", from.String())

	if !rc.Contract.UseGas(params.WithdrawDelegateRewardGas) {
		return nil, ErrOutOfGas
	}

	list, err := rc.stkPlugin.GetDelegatesInfo(blockHash, from)
	if err != nil {
		return txResultHandler(vm.DelegateRewardPoolAddr, rc.Evm, "withdrawDelegateReward", "",
			TxWithdrawDelegateReward, int(staking.ErrQueryDelegateInfo.Code)), err
	}
	if !rc.Contract.UseGas(params.WithdrawDelegateNodeGas * uint64(len(list))) {
		return nil, ErrOutOfGas
	}

	currentEpoch := xutil.CalculateEpoch(blockNum.Uint64())
	unCalEpoch := 0
	delegationInfoWithRewardPerList := make([]*plugin.DelegationInfoWithRewardPerList, 0)
	for _, stakingNode := range list {
		if uint64(stakingNode.Delegation.DelegateEpoch) == currentEpoch-1 {
			delegationInfoWithRewardPerList = append(delegationInfoWithRewardPerList, plugin.NewDelegationInfoWithRewardPerList(stakingNode, nil))
		} else {
			delegateRewardPerList, err := rc.Plugin.GetDelegateRewardPerList(blockHash, stakingNode.NodeID, stakingNode.StakeBlockNumber, uint64(stakingNode.Delegation.DelegateEpoch), currentEpoch-1)
			if err != nil {

			}
			unCalEpoch += len(delegateRewardPerList)
			delegationInfoWithRewardPerList = append(delegationInfoWithRewardPerList, plugin.NewDelegationInfoWithRewardPerList(stakingNode, delegateRewardPerList))
		}

	}

	if !rc.Contract.UseGas(params.WithdrawDelegateEpochGas * uint64(unCalEpoch)) {
		return nil, ErrOutOfGas
	}

	if txHash == common.ZeroHash {
		return nil, nil
	}

	reward, err := rc.Plugin.WithdrawDelegateReward(blockHash, blockNum.Uint64(), from, delegationInfoWithRewardPerList, state)
	if err != nil {
		//todo error checking
		return txResultHandler(vm.DelegateRewardPoolAddr, rc.Evm, FuncNameWithdrawDelegateReward, "", TxWithdrawDelegateReward, int(common.NoErr.Code)), nil
	}
	return txResultHandlerWithRes(vm.DelegateRewardPoolAddr, rc.Evm, FuncNameWithdrawDelegateReward, "", TxWithdrawDelegateReward, int(common.NoErr.Code), reward), nil
}

func (rc *DelegateRewardContract) getDelegateReward(nodeIDs []discover.NodeID) ([]byte, error) {
	state := rc.Evm.StateDB
	from := rc.Contract.CallerAddress

	blockNum := rc.Evm.BlockNumber
	blockHash := rc.Evm.BlockHash

	reward, err := rc.Plugin.GetDelegateReward(blockHash, blockNum.Uint64(), from, nodeIDs, state)
	if err != nil {

	}

	return callResultHandler(rc.Evm, fmt.Sprintf("getDelegateReward, account: %s", from.String()),
		reward, err), nil
}
