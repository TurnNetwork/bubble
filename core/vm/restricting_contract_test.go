package vm_test

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/hexutil"
	"github.com/PlatONnetwork/PlatON-Go/core/vm"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
	"github.com/PlatONnetwork/PlatON-Go/x/plugin"
	"github.com/PlatONnetwork/PlatON-Go/x/restricting"
)


type ResultTest struct {
	balance *big.Int
	slash   *big.Int
	staking *big.Int
	debt    *big.Int
	entry   []byte
}


// build input data
func buildRestrictingPlanData() ([]byte, error) {
	var plan  restricting.RestrictingPlan
	var plans = make([]restricting.RestrictingPlan, 5)

	var epoch uint64
	for index := 0; index < len(plans); index++ {
		epoch = uint64(index + 1)
		plan.Epoch = uint64(epoch)
		plan.Amount = big.NewInt(10000000)
		plans[index] = plan
	}

	var params [][]byte
	param0, _ := rlp.EncodeToBytes(common.Uint16ToBytes(4000))  // function_type
	param1, _ := rlp.EncodeToBytes(addrArr[0].Bytes())    // restricting account
	param2, _ := rlp.EncodeToBytes(plans)   // restricting plan

	params = append(params, param0)
	params = append(params, param1)
	params = append(params, param2)

	return rlp.EncodeToBytes(params)
}

func TestRestrictingContract_createRestrictingPlan(t *testing.T) {
	contract := &vm.RestrictingContract{
		Plugin:  plugin.RestrictingInstance(),
		Contract: newContract(common.Big0),
		Evm: newEvm(blockNumber, blockHash, nil),
	}

	input, err := buildRestrictingPlanData()
	if err != nil {
		fmt.Println(err)
		t.Errorf("fail to rlp encode restricting input")
	} else {
		fmt.Println("rlp encode restricting input: ", hexutil.Encode(input))
	}

	if result, err := contract.Run(input); err != nil {
		t.Error(err.Error())
	} else {
		t.Log(string(result))
	}
}


func TestRestrictingContract_getRestrictingInfo(t *testing.T) {
	// build db data for getting info
	stateDb, _, _ := newChainState()
	buildDbRestrictingPlan(t, stateDb)

	contract := &vm.RestrictingContract{
		Plugin:  plugin.RestrictingInstance(),
		Contract: newContract(common.Big0),
		Evm: newEvm(blockNumber, blockHash, stateDb),
	}

	var params [][]byte
	param0, _ := rlp.EncodeToBytes(common.Uint16ToBytes(4100))
	param1, _ := rlp.EncodeToBytes(addrArr[0])
	params = append(params, param0)
	params = append(params, param1)
	input, err := rlp.EncodeToBytes(params)
	if err != nil {
		t.Log(err.Error())
		t.Errorf("fail to rlp encode restricting input")
	} else {
		t.Log("rlp encode restricting input: ", hexutil.Encode(input))
	}

	t.Log("restricting account is", addrArr[0].String())

	if result, err := contract.Run(input); err != nil {
		t.Errorf("getRestrictingInfo returns error! error is: %s", err.Error())
	} else {

		t.Log(string(result))

		var res restricting.Result
		if err = rlp.Decode(bytes.NewBuffer(result), &res); err != nil {
			t.Fatalf("failed to get restricting info as rlp decode result failed, error: %s", err.Error())

		} else {
			t.Logf("%v", res.Balance)
			t.Logf("%v", res.Staking)
			t.Logf("%v", res.Slash)
			t.Logf("%v", res.Debt)
			t.Logf("%v", string(res.Entry))
		}
		t.Log("test pass!")
	}
}