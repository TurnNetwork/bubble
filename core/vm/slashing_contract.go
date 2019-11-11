package vm

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/PlatONnetwork/PlatON-Go/common/consensus"

	"github.com/PlatONnetwork/PlatON-Go/common/vm"

	"github.com/PlatONnetwork/PlatON-Go/common/hexutil"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/params"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/x/plugin"
)

const (
	TxReportDuplicateSign = 3000
	CheckDuplicateSign    = 3001
)

type SlashingContract struct {
	Plugin   *plugin.SlashingPlugin
	Contract *Contract
	Evm      *EVM
}

func (sc *SlashingContract) RequiredGas(input []byte) uint64 {
	return params.SlashingGas
}

func (sc *SlashingContract) Run(input []byte) ([]byte, error) {
	return execPlatonContract(input, sc.FnSigns())
}

func (sc *SlashingContract) FnSigns() map[uint16]interface{} {
	return map[uint16]interface{}{
		// Set
		TxReportDuplicateSign: sc.reportDuplicateSign,
		// Get
		CheckDuplicateSign: sc.checkDuplicateSign,
	}
}

func (sc *SlashingContract) CheckGasPrice(gasPrice *big.Int, fcode uint16) error {
	return nil
}

// Report the double signing behavior of the node
func (sc *SlashingContract) reportDuplicateSign(dupType uint8, data string) ([]byte, error) {

	txHash := sc.Evm.StateDB.TxHash()
	blockNumber := sc.Evm.BlockNumber
	blockHash := sc.Evm.BlockHash
	from := sc.Contract.CallerAddress

	if !sc.Contract.UseGas(params.ReportDuplicateSignGas) {
		return nil, ErrOutOfGas
	}

	if !sc.Contract.UseGas(params.DuplicateEvidencesGas) {
		return nil, ErrOutOfGas
	}
	if txHash == common.ZeroHash {
		return nil, nil
	}

	log.Debug("Call reportDuplicateSign", "blockNumber", blockNumber, "blockHash", blockHash.Hex(),
		"TxHash", txHash.Hex(), "from", from.Hex())
	evidence, err := sc.Plugin.DecodeEvidence(consensus.EvidenceType(dupType), data)
	if nil != err {
		return txResultHandler(vm.SlashingContractAddr, sc.Evm, "reportDuplicateSign",
			common.InvalidParameter.Wrap(err.Error()).Error(),
			TxReportDuplicateSign, int(common.InvalidParameter.Code)), nil
	}
	if err := sc.Plugin.Slash(evidence, blockHash, blockNumber.Uint64(), sc.Evm.StateDB, from); nil != err {
		if bizErr, ok := err.(*common.BizError); ok {
			return txResultHandler(vm.SlashingContractAddr, sc.Evm, "reportDuplicateSign",
				bizErr.Error(), TxReportDuplicateSign, int(bizErr.Code)), nil
		} else {
			return nil, err
		}
	}
	return txResultHandler(vm.SlashingContractAddr, sc.Evm, "",
		"", TxReportDuplicateSign, int(common.NoErr.Code)), nil
}

// Check if the node has double sign behavior at a certain block height
func (sc *SlashingContract) checkDuplicateSign(dupType uint8, addr common.Address, blockNumber uint64) ([]byte, error) {
	log.Info("checkDuplicateSign exist", "blockNumber", blockNumber, "addr", hex.EncodeToString(addr.Bytes()), "dupType", dupType)
	txHash, err := sc.Plugin.CheckDuplicateSign(addr, blockNumber, consensus.EvidenceType(dupType), sc.Evm.StateDB)
	var data string

	if nil != err {
		return callResultHandler(sc.Evm, fmt.Sprintf("checkDuplicateSign, duplicateSignBlockNum: %d, addr: %s, dupType: %d",
			blockNumber, addr, dupType), ResultTypeNonNil, data, common.InternalError.Wrap(err.Error())), nil
	}
	if len(txHash) > 0 {
		data = hexutil.Encode(txHash)
	}
	return callResultHandler(sc.Evm, fmt.Sprintf("checkDuplicateSign, duplicateSignBlockNum: %d, addr: %s, dupType: %d",
		blockNumber, addr, dupType), ResultTypeNonNil, data, nil), nil
}
