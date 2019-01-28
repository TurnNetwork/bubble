package vm_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/common/byteutil"
	"github.com/PlatONnetwork/PlatON-Go/common/hexutil"
	"github.com/PlatONnetwork/PlatON-Go/core/vm"
	"github.com/PlatONnetwork/PlatON-Go/crypto/sha3"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
	"math/big"
	"strconv"
	"testing"
)

func TestTicketPoolOverAll(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}

	// CandidateDeposit(nodeId discover.NodeID, owner common.Address, fee uint64, host, port, extra string) ([]byte, error)
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	fmt.Println("setcandidate successfully...")

	// VoteTicket(count uint64, price *big.Int, nodeId discover.NodeID) ([]byte, error)
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// GetCandidateTicketIds(nodeId discover.NodeID) ([]byte, error)
	fmt.Println("GetCandidateTicketIds input==>", "nodeId: ", nodeId.String())
	resByte, err = ticketContract.GetCandidateTicketIds(nodeId)
	if nil != err {
		fmt.Println("GetCandidateTicketIds fail", "err", err)
	}
	fmt.Println("The candidate's ticketId are: ", vm.ResultByte2Json(resByte))

	// GetTicketDetail(ticketId common.Hash) ([]byte, error)
	ticketId := common.HexToHash("e69d8e6dbc1ee87d7fb20600f3fc6744f28b637d43b5a130b2904c30d12e9b30")
	fmt.Println("GetTicketDetail input==>", "ticketId: ", ticketId.String())
	resByte, err = ticketContract.GetTicketDetail(ticketId)
	if nil != err {
		fmt.Println("GetTicketDetail fail", "err", err)
	}
	fmt.Println("ticketInfo is: ", vm.ResultByte2Json(resByte))

	// GetBatchTicketDetail(ticketIds []common.Hash) ([]byte, error)
	ticketIds := []common.Hash{common.HexToHash("e69d8e6dbc1ee87d7fb20600f3fc6744f28b637d43b5a130b2904c30d12e9b30"), common.HexToHash("008674dae3f0c660158fe602589c5505b20e24be4caa8f65c0f92ff372149ccc")}
	input, _ := json.Marshal(ticketIds)
	fmt.Println("GetBatchTicketDetail input==>", "ticketIds: ", string(input))
	resByte, err = ticketContract.GetBatchTicketDetail(ticketIds)
	if nil != err {
		fmt.Println("GetBatchTicketDetail fail", "err", err)
	}
	fmt.Println("ticketInfo is: ", vm.ResultByte2Json(resByte))
}

func TestVoteTicket(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}

	// VoteTicket(count uint64, price *big.Int, nodeId discover.NodeID) ([]byte, error)
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))
}

func TestGetTicketDetail(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// GetTicketDetail(ticketId common.Hash) ([]byte, error)
	ticketId := common.HexToHash("e69d8e6dbc1ee87d7fb20600f3fc6744f28b637d43b5a130b2904c30d12e9b30")
	fmt.Println("GetTicketDetail input==>", "ticketId: ", ticketId.String())
	resByte, err = ticketContract.GetTicketDetail(ticketId)
	if nil != err {
		fmt.Println("GetTicketDetail fail", "err", err)
	}
	if nil == resByte {
		fmt.Println("The ticket info is null")
		return
	}
	fmt.Println("ticketInfo is: ", vm.ResultByte2Json(resByte))
}

func TestGetBatchTicketDetail(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(1)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// GetBatchTicketDetail(ticketIds []common.Hash) ([]byte, error)
	ticketIds := []common.Hash{common.HexToHash("e69d8e6dbc1ee87d7fb20600f3fc6744f28b637d43b5a130b2904c30d12e9b30"), common.HexToHash("008674dae3f0c660158fe602589c5505b20e24be4caa8f65c0f92ff372149ccc")}
	input, _ := json.Marshal(ticketIds)
	fmt.Println("GetBatchTicketDetail input==>", "ticketIds: ", string(input))
	resByte, err = ticketContract.GetBatchTicketDetail(ticketIds)
	if nil != err {
		fmt.Println("GetBatchTicketDetail fail", "err", err)
	}
	if nil == resByte {
		fmt.Println("The batch ticket info is null")
		return
	}
	fmt.Println("ticketInfo is: ", vm.ResultByte2Json(resByte))
}

func TestGetCandidateTicketIds(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// GetCandidateTicketIds(nodeId discover.NodeID) ([]byte, error)
	fmt.Println("GetCandidateTicketIds input==>", "nodeId: ", nodeId.String())
	resByte, err = ticketContract.GetCandidateTicketIds(nodeId)
	if nil != err {
		fmt.Println("GetCandidateTicketIds fail", "err", err)
	}
	if nil == resByte {
		fmt.Println("The candidate's ticket list is null")
		return
	}
	fmt.Println("The candidate's ticketId is: ", vm.ResultByte2Json(resByte))
}

func TestGetBatchCandidateTicketIds(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId1 := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId1: ", nodeId1.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId1, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	fmt.Println("CandidateDeposit1 success")

	nodeId2 := discover.MustHexID("0x11234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner = common.HexToAddress("0x12")
	fee = uint64(8000)
	host = "192.168.9.185"
	port = "16789"
	extra = "{\"nodeName\": \"Platon-Shenzhen\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Cosmic wave\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/sz\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId2: ", nodeId2.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err = candidateContract.CandidateDeposit(nodeId2, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	fmt.Println("CandidateDeposit2 success")

	// CandidateList() ([]byte, error)
	resByte, err := candidateContract.CandidateList()
	if nil != err {
		fmt.Println("CandidateList fail", "err", err)
	}
	if nil == resByte {
		fmt.Println("The candidate list is null")
		return
	}
	fmt.Println("The candidate list is: ", vm.ResultByte2Json(resByte))

	// Vote to Candidate1
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId1: ", nodeId1.String())
	resByte, err = ticketContract.VoteTicket(count, price, nodeId1)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// Vote to Candidate2
	count = uint64(1000)
	price = big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId2: ", nodeId2.String())
	resByte, err = ticketContract.VoteTicket(count, price, nodeId2)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}
	fmt.Println("The list of generated ticketId is: ", vm.ResultByte2Json(resByte))

	// GetBatchCandidateTicketIds(nodeIds []discover.NodeID) ([]byte, error)
	fmt.Println("GetBatchCandidateTicketIds input==>", "nodeIds: ", nodeId1.String(), nodeId2.String())
	var nodeIds []discover.NodeID
	nodeIds = append(append(nodeIds, nodeId1), nodeId2)
	resByte, err = ticketContract.GetBatchCandidateTicketIds(nodeIds)
	if nil != err {
		fmt.Println("GetBatchCandidateTicketIds fail", "err", err)
	}
	if nil == resByte {
		fmt.Println("The candidates's ticket list is null")
		return
	}
	fmt.Println("The candidate's ticketId are: ", vm.ResultByte2Json(resByte))
}

func TestGetCandidateEpoch(t *testing.T) {
	contract := newContract()
	evm := newEvm()

	ticketContract := vm.TicketContract{
		contract,
		evm,
	}
	candidateContract := vm.CandidateContract{
		contract,
		evm,
	}
	nodeId := discover.MustHexID("0x01234567890121345678901123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345")
	owner := common.HexToAddress("0x12")
	fee := uint64(7000)
	host := "192.168.9.184"
	port := "16789"
	extra := "{\"nodeName\": \"Platon-Beijing\", \"nodePortrait\": \"\",\"nodeDiscription\": \"PlatON-Gravitational area\",\"nodeDepartment\": \"JUZIX\",\"officialWebsite\": \"https://www.platon.network/\",\"time\":1546503651190}"
	fmt.Println("CandidateDeposit input==>", "nodeId: ", nodeId.String(), "owner: ", owner.Hex(), "fee: ", fee, "host: ", host, "port: ", port, "extra: ", extra)
	_, err := candidateContract.CandidateDeposit(nodeId, owner, fee, host, port, extra)
	if nil != err {
		fmt.Println("CandidateDeposit fail", "err", err)
	}
	count := uint64(1000)
	price := big.NewInt(1)
	fmt.Println("VoteTicket input==>", "count: ", count, "price: ", price, "nodeId: ", nodeId.String())
	resByte, err := ticketContract.VoteTicket(count, price, nodeId)
	if nil != err {
		fmt.Println("VoteTicket fail", "err", err)
	}

	// GetCandidateEpoch(nodeId discover.NodeID) ([]byte, error)
	fmt.Println("GetCandidateEpoch input==>", "nodeId: ", nodeId.String())
	resByte, err = ticketContract.GetCandidateEpoch(nodeId)
	if nil != err {
		fmt.Println("GetCandidateEpoch fail", "err", err)
	}
	fmt.Println("The candidate's epoch is: ", vm.ResultByte2Json(resByte))

}

func TestGetTicketPrice(t *testing.T) {
	ticketContract := vm.TicketContract{
		newContract(),
		newEvm(),
	}

	// GetTicketPrice() ([]byte, error)
	resByte, err := ticketContract.GetTicketPrice()
	if nil != err {
		fmt.Println("GetTicketPrice fail", "err", err)
	}
	fmt.Println("The ticket price is: ", vm.ResultByte2Json(resByte))
}

func TestTicketPoolEncode(t *testing.T) {
	nodeId := []byte("0x751f4f62fccee84fc290d0c68d673e4b0cc6975a5747d2baccb20f954d59ba3315d7bfb6d831523624d003c8c2d33451129e67c3eef3098f711ef3b3e268fd3c")
	// VoteTicket(count uint64, price *big.Int, nodeId discover.NodeID)
	var VoteTicket [][]byte
	VoteTicket = make([][]byte, 0)
	VoteTicket = append(VoteTicket, byteutil.Uint64ToBytes(1000))
	VoteTicket = append(VoteTicket, []byte("VoteTicket"))
	VoteTicket = append(VoteTicket, byteutil.Uint64ToBytes(100))
	VoteTicket = append(VoteTicket, big.NewInt(1000000000000000000).Bytes())
	VoteTicket = append(VoteTicket, nodeId)
	bufVoteTicket := new(bytes.Buffer)
	err := rlp.Encode(bufVoteTicket, VoteTicket)
	if err != nil {
		fmt.Println(err)
		t.Errorf("VoteTicket encode rlp data fail")
	} else {
		fmt.Println("VoteTicket data rlp: ", hexutil.Encode(bufVoteTicket.Bytes()))
	}

	// GetCandidateTicketIds(nodeId discover.NodeID)
	var GetCandidateTicketIds [][]byte
	GetCandidateTicketIds = make([][]byte, 0)
	GetCandidateTicketIds = append(GetCandidateTicketIds, byteutil.Uint64ToBytes(0xf1))
	GetCandidateTicketIds = append(GetCandidateTicketIds, []byte("GetCandidateTicketIds"))
	GetCandidateTicketIds = append(GetCandidateTicketIds, nodeId)
	bufGetCandidateTicketIds := new(bytes.Buffer)
	err = rlp.Encode(bufGetCandidateTicketIds, GetCandidateTicketIds)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetCandidateTicketIds encode rlp data fail")
	} else {
		fmt.Println("GetCandidateTicketIds data rlp: ", hexutil.Encode(bufGetCandidateTicketIds.Bytes()))
	}

	// GetTicketDetail(ticketId common.Hash)
	ticketId := []byte("0xe0a8900041672aa3f553c0e88a9961ffe2cc318d3fb4cfe3de66e3a1964c919b")
	var GetTicketDetail [][]byte
	GetTicketDetail = make([][]byte, 0)
	GetTicketDetail = append(GetTicketDetail, byteutil.Uint64ToBytes(1004))
	GetTicketDetail = append(GetTicketDetail, []byte("GetTicketDetail"))
	GetTicketDetail = append(GetTicketDetail, ticketId)
	bufGetTicketDetail := new(bytes.Buffer)
	err = rlp.Encode(bufGetTicketDetail, GetTicketDetail)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetTicketDetail encode rlp data fail")
	} else {
		fmt.Println("GetTicketDetail data rlp: ", hexutil.Encode(bufGetTicketDetail.Bytes()))
	}

	// GetBatchTicketDetail(ticketId []common.Hash)
	ticketId1 := "0x3780eb19677a4c69add0fa8151abdac77d550f37585b3e1b06e73561f7197949"
	ticketId2 := "0x4780eb19677a4c69add0fa8151abdac77d550f37585b3e1b06e73561f7197949"
	ticketIds := ticketId1 + ":" + ticketId2
	var GetBatchTicketDetail [][]byte
	GetBatchTicketDetail = make([][]byte, 0)
	GetBatchTicketDetail = append(GetBatchTicketDetail, byteutil.Uint64ToBytes(0xf1))
	GetBatchTicketDetail = append(GetBatchTicketDetail, []byte("GetBatchTicketDetail"))
	GetBatchTicketDetail = append(GetBatchTicketDetail, []byte(ticketIds))
	bufGetBatchTicketDetail := new(bytes.Buffer)
	err = rlp.Encode(bufGetBatchTicketDetail, GetBatchTicketDetail)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetBatchTicketDetail encode rlp data fail")
	} else {
		fmt.Println("GetBatchTicketDetail data rlp: ", hexutil.Encode(bufGetBatchTicketDetail.Bytes()))
	}

	// GetBatchCandidateTicketIds(nodeId []discover.NodeID)
	nodeId1 := "0x1f3a8672348ff6b789e416762ad53e69063138b8eb4d8780101658f24b2369f1a8e09499226b467d8bc0c4e03e1dc903df857eeb3c67733d21b6aaee2840e429"
	nodeId2 := "0x2f3a8672348ff6b789e416762ad53e69063138b8eb4d8780101658f24b2369f1a8e09499226b467d8bc0c4e03e1dc903df857eeb3c67733d21b6aaee2840e429"
	nodeId3 := "0x3f3a8672348ff6b789e416762ad53e69063138b8eb4d8780101658f24b2369f1a8e09499226b467d8bc0c4e03e1dc903df857eeb3c67733d21b6aaee2840e429"
	nodeIds := nodeId1 + ":" + nodeId2 + ":" + nodeId3
	var GetBatchCandidateTicketIds [][]byte
	GetBatchCandidateTicketIds = make([][]byte, 0)
	GetBatchCandidateTicketIds = append(GetBatchCandidateTicketIds, byteutil.Uint64ToBytes(0xf1))
	GetBatchCandidateTicketIds = append(GetBatchCandidateTicketIds, []byte("GetBatchCandidateTicketIds"))
	GetBatchCandidateTicketIds = append(GetBatchCandidateTicketIds, []byte(nodeIds))
	bufGetBatchCandidateTicketIds := new(bytes.Buffer)
	err = rlp.Encode(bufGetBatchCandidateTicketIds, GetBatchCandidateTicketIds)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetBatchCandidateTicketIds encode rlp data fail")
	} else {
		fmt.Println("GetBatchCandidateTicketIds data rlp: ", hexutil.Encode(bufGetBatchCandidateTicketIds.Bytes()))
	}

	// GetPoolRemainder() ([]byte, error)
	var GetPoolRemainder [][]byte
	GetPoolRemainder = make([][]byte, 0)
	GetPoolRemainder = append(GetPoolRemainder, byteutil.Uint64ToBytes(0xf1))
	GetPoolRemainder = append(GetPoolRemainder, []byte("GetPoolRemainder"))
	bufGetPoolRemainder := new(bytes.Buffer)
	err = rlp.Encode(bufGetPoolRemainder, GetPoolRemainder)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetPoolRemainder encode rlp data fail")
	} else {
		fmt.Println("GetPoolRemainder data rlp: ", hexutil.Encode(bufGetPoolRemainder.Bytes()))
	}

	// GetTicketPrice() ([]byte, error)
	var GetTicketPrice [][]byte
	GetTicketPrice = make([][]byte, 0)
	GetTicketPrice = append(GetTicketPrice, byteutil.Uint64ToBytes(0xf1))
	GetTicketPrice = append(GetTicketPrice, []byte("GetTicketPrice"))
	bufGetTicketPrice := new(bytes.Buffer)
	err = rlp.Encode(bufGetTicketPrice, GetTicketPrice)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetTicketPrice encode rlp data fail")
	} else {
		fmt.Println("GetTicketPrice data rlp: ", hexutil.Encode(bufGetTicketPrice.Bytes()))
	}

	// GetCandidateEpoch(nodeId discover.NodeID) ([]byte, error)G
	var GetCandidateEpoch [][]byte
	GetCandidateEpoch = make([][]byte, 0)
	GetCandidateEpoch = append(GetCandidateEpoch, byteutil.Uint64ToBytes(0xf1))
	GetCandidateEpoch = append(GetCandidateEpoch, []byte("GetCandidateEpoch"))
	GetCandidateEpoch = append(GetCandidateEpoch, nodeId)
	bufGetCandidateEpoch := new(bytes.Buffer)
	err = rlp.Encode(bufGetCandidateEpoch, GetCandidateEpoch)
	if err != nil {
		fmt.Println(err)
		t.Errorf("GetCandidateEpoch encode rlp data fail")
	} else {
		fmt.Println("GetCandidateEpoch data rlp: ", hexutil.Encode(bufGetCandidateEpoch.Bytes()))
	}
}

func TestTicketPoolDecode(t *testing.T) {
	//HexString -> []byte
	rlpcode, _ := hex.DecodeString("f8c08800000000000003e88a566f74655469636b6574880000000000000001a00000000000000000000000000000000000000000000000000000000000000000b8803166336138363732333438666636623738396534313637363261643533653639303633313338623865623464383738303130313635386632346232333639663161386530393439393232366234363764386263306334653033653164633930336466383537656562336336373733336432316236616165653238343065343239")
	var source [][]byte
	if err := rlp.Decode(bytes.NewReader(rlpcode), &source); err != nil {
		fmt.Println(err)
		t.Errorf("TestRlpDecode decode rlp data fail")
	}

	for i, v := range source {
		fmt.Println("i: ", i, " v: ", hex.EncodeToString(v))
	}
}

func generateTicketId(txHash common.Hash, index uint64) (common.Hash, error) {
	// generate ticket id
	value := append(txHash.Bytes(), []byte(strconv.Itoa(int(index)))...)
	ticketId := sha3.Sum256(value[:])
	return ticketId, nil
}

func TestGenerateTicketId(t *testing.T) {
	txHash := []byte("2aeb176c6c90b55d59afaa56fcc0af0ede81dfa7a5c45ef89a46f7d3ea1fbaf6")
	ticketId, _ := generateTicketId(byteutil.BytesToHash(txHash), 0)
	fmt.Println(ticketId.String())
}
