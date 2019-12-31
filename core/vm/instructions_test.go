// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"math/big"
	"testing"

	"github.com/PlatONnetwork/PlatON-Go/common/math"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/params"
)

type twoOperandTest struct {
	x        string
	y        string
	expected string
}

func testTwoOperandOp(t *testing.T, tests []twoOperandTest, opFn func(pc *uint64, interpreter *EVMInterpreter, contract *Contract, memory *Memory, stack *Stack) ([]byte, error)) {
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		pc             = uint64(0)
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	for i, test := range tests {
		x := new(big.Int).SetBytes(common.Hex2Bytes(test.x))
		shift := new(big.Int).SetBytes(common.Hex2Bytes(test.y))
		expected := new(big.Int).SetBytes(common.Hex2Bytes(test.expected))
		stack.push(x)
		stack.push(shift)
		opFn(&pc, evmInterpreter, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(expected) != 0 {
			t.Errorf("Testcase %d, expected  %v, got %v", i, expected, actual)
		}
		// Check pool usage
		// 1.pool is not allowed to contain anything on the stack
		// 2.pool is not allowed to contain the same pointers twice
		if evmInterpreter.intPool.pool.len() > 0 {

			poolvals := make(map[*big.Int]struct{})
			poolvals[actual] = struct{}{}

			for evmInterpreter.intPool.pool.len() > 0 {
				key := evmInterpreter.intPool.get()
				if _, exist := poolvals[key]; exist {
					t.Errorf("Testcase %d, pool contains double-entry", i)
				}
				poolvals[key] = struct{}{}
			}
		}
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func TestByteOp(t *testing.T) {
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	tests := []struct {
		v        string
		th       uint64
		expected *big.Int
	}{
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", 0, big.NewInt(0xAB)},
		{"ABCDEF0908070605040302010000000000000000000000000000000000000000", 1, big.NewInt(0xCD)},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", 0, big.NewInt(0x00)},
		{"00CDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff", 1, big.NewInt(0xCD)},
		{"0000000000000000000000000000000000000000000000000000000000102030", 31, big.NewInt(0x30)},
		{"0000000000000000000000000000000000000000000000000000000000102030", 30, big.NewInt(0x20)},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 32, big.NewInt(0x0)},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 0xFFFFFFFFFFFFFFFF, big.NewInt(0x0)},
	}
	pc := uint64(0)
	for _, test := range tests {
		val := new(big.Int).SetBytes(common.Hex2Bytes(test.v))
		th := new(big.Int).SetUint64(test.th)
		stack.push(val)
		stack.push(th)
		opByte(&pc, evmInterpreter, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(test.expected) != 0 {
			t.Fatalf("Expected  [%v] %v:th byte to be %v, was %v.", test.v, test.th, test.expected, actual)
		}
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func TestSHL(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shl-shift-left
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000002"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "8000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"},
	}
	testTwoOperandOp(t, tests, opSHL)
}

func TestSHR(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#shr-logical-shift-right
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "4000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSHR)
}

func TestSAR(t *testing.T) {
	// Testcases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-145.md#sar-arithmetic-shift-right
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "00", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "01", "c000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"8000000000000000000000000000000000000000000000000000000000000000", "0101", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "00", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "01", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "01", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"4000000000000000000000000000000000000000000000000000000000000000", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "f8", "000000000000000000000000000000000000000000000000000000000000007f"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "fe", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0100", "0000000000000000000000000000000000000000000000000000000000000000"},
	}

	testTwoOperandOp(t, tests, opSAR)
}

func TestSGT(t *testing.T) {
	tests := []twoOperandTest{

		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSgt)
}

func TestSLT(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000001", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"8000000000000000000000000000000000000000000000000000000000000001", "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "8000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffd", "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffb", "0000000000000000000000000000000000000000000000000000000000000001"},
	}
	testTwoOperandOp(t, tests, opSlt)
}

func TestOpAdd(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000008", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000009"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000002"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000003"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "000000000000000000000000000000000000000000000000000000000000000a", "000000000000000000000000000000000000000000000000000000000000000c"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opAdd)
}

func TestOpSub(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		//{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000002"},
		//{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		//{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSub)
}

func TestOpMul(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000002"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opMul)
}

func TestOpDiv(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opDiv)
}

func TestOpSDiv(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSdiv)
}

func TestOpMod(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opMod)
}

func TestOpSmod(t *testing.T) {
	tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	testTwoOperandOp(t, tests, opSmod)
}

func TestOpExp(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(10), v(100)},
		{v(10), v(2), v(1024)},
		{v(100), v(1), v(1)},
		{v(2), v(0), v(0)},
	}
	testTwoOperandOp(t, tests, opExp)
}

func TestOpNot(t *testing.T) {
	/*v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}*/
	/*tests := []twoOperandTest{
		{"0000000000000000000000000000000000000000000000000000000000000001", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000002", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000000", "0000000000000000000000000000000000000000000000000000000000000001"},
	}
	testTwoOperandOp(t, tests, opNot)*/
}

func TestOpLt(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(1), v(1)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(0)},
		{v(-2), v(0), v(1)},
	}
	testTwoOperandOp(t, tests, opLt)
}

func TestOpGt(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(1), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(1)},
		{v(-2), v(0), v(0)},
	}
	testTwoOperandOp(t, tests, opGt)
}

func TestOpSlt(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(1), v(1)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(0)},
		{v(-2), v(0), v(1)},
	}
	testTwoOperandOp(t, tests, opSlt)
}

func TestOpSgt(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(-1), v(0)},
		{v(2), v(1), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(1)},
		{v(-2), v(0), v(0)},
		{common.Bytes2Hex(math.BigPow(2, 256).Bytes()), v(1), v(1)},
		{v(1), common.Bytes2Hex(math.BigPow(2, 256).Bytes()), v(0)},
	}
	testTwoOperandOp(t, tests, opSgt)
}

func TestOpEq(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(1)},
		{v(0), v(0), v(1)},
		{v(1), v(2), v(0)},
		{v(2), v(1), v(0)},
		{v(-2), v(0), v(0)},
		{common.Bytes2Hex(math.BigPow(2, 256).Bytes()), v(1), v(0)},
		{v(1), common.Bytes2Hex(math.BigPow(2, 256).Bytes()), v(0)},
	}
	testTwoOperandOp(t, tests, opEq)
}

func TestOpOr(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(2)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(3)},
		{v(2), v(1), v(3)},
		{v(-2), v(0), v(2)},
	}
	testTwoOperandOp(t, tests, opOr)
}

func TestOpXor(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(3)},
		{v(2), v(1), v(3)},
		{v(-2), v(0), v(2)},
	}
	testTwoOperandOp(t, tests, opXor)
}

func TestOpByte(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(0)},
		{v(200), v(35), v(0)},
		{v(-2), v(0), v(0)},
	}
	testTwoOperandOp(t, tests, opByte)
}

func TestOpAddmod(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []struct {
		x        string
		y        string
		z        string
		expected string
	}{
		{v(2), v(2), v(2), v(0)},
		{v(0), v(0), v(2), v(0)},
		{v(1), v(2), v(2), v(0)},
		{v(200), v(35), v(2), v(37)},
		{v(-2), v(0), v(2), v(0)},
	}
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		pc             = uint64(0)
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	for i, test := range tests {
		x := new(big.Int).SetBytes(common.Hex2Bytes(test.x))
		shift := new(big.Int).SetBytes(common.Hex2Bytes(test.y))
		z := new(big.Int).SetBytes(common.Hex2Bytes(test.z))
		expected := new(big.Int).SetBytes(common.Hex2Bytes(test.expected))
		stack.push(x)
		stack.push(shift)
		stack.push(z)
		opAddmod(&pc, evmInterpreter, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(expected) != 0 {
			t.Errorf("Testcase %d, expected  %v, got %v", i, expected, actual)
		}
		// Check pool usage
		// 1.pool is not allowed to contain anything on the stack
		// 2.pool is not allowed to contain the same pointers twice
		if evmInterpreter.intPool.pool.len() > 0 {

			poolvals := make(map[*big.Int]struct{})
			poolvals[actual] = struct{}{}

			for evmInterpreter.intPool.pool.len() > 0 {
				key := evmInterpreter.intPool.get()
				if _, exist := poolvals[key]; exist {
					t.Errorf("Testcase %d, pool contains double-entry", i)
				}
				poolvals[key] = struct{}{}
			}
		}
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func TestOpMulmod(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []struct {
		x        string
		y        string
		z        string
		expected string
	}{
		{v(2), v(2), v(2), v(0)},
		{v(0), v(0), v(2), v(0)},
		{v(1), v(2), v(2), v(0)},
		{v(200), v(35), v(2), v(70)},
		{v(-2), v(0), v(2), v(0)},
	}
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		pc             = uint64(0)
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	for i, test := range tests {
		x := new(big.Int).SetBytes(common.Hex2Bytes(test.x))
		shift := new(big.Int).SetBytes(common.Hex2Bytes(test.y))
		z := new(big.Int).SetBytes(common.Hex2Bytes(test.z))
		expected := new(big.Int).SetBytes(common.Hex2Bytes(test.expected))
		stack.push(x)
		stack.push(shift)
		stack.push(z)
		opMulmod(&pc, evmInterpreter, nil, nil, stack)
		actual := stack.pop()
		if actual.Cmp(expected) != 0 {
			t.Errorf("Testcase %d, expected  %v, got %v", i, expected, actual)
		}
		// Check pool usage
		// 1.pool is not allowed to contain anything on the stack
		// 2.pool is not allowed to contain the same pointers twice
		if evmInterpreter.intPool.pool.len() > 0 {

			poolvals := make(map[*big.Int]struct{})
			poolvals[actual] = struct{}{}

			for evmInterpreter.intPool.pool.len() > 0 {
				key := evmInterpreter.intPool.get()
				if _, exist := poolvals[key]; exist {
					t.Errorf("Testcase %d, pool contains double-entry", i)
				}
				poolvals[key] = struct{}{}
			}
		}
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func TestOpSHL(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(8)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(4)},
		{v(10), v(10), v(10240)},
		{v(-2), v(0), v(2)},
	}
	testTwoOperandOp(t, tests, opSHL)
}

func TestOpSHR(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(0)},
		{v(266), v(257), v(0)},
		{v(-2), v(0), v(2)},
	}
	testTwoOperandOp(t, tests, opSHR)
}

func TestOpSAR(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(2), v(2), v(0)},
		{v(0), v(0), v(0)},
		{v(1), v(2), v(0)},
		{v(266), v(257), v(0)},
		{v(-2), v(0), v(2)},
	}
	testTwoOperandOp(t, tests, opSAR)
}

// Contains memory data.
func testGlobalOperandOp(t *testing.T, stateDB StateDB, memory *Memory, contract *Contract, tests []twoOperandTest, opFn func(pc *uint64, interpreter *EVMInterpreter, contract *Contract, memory *Memory, stack *Stack) ([]byte, error)) {
	var (
		env            = NewEVM(Context{}, stateDB, params.TestChainConfig, Config{})
		stack          = newstack()
		pc             = uint64(0)
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	for i, test := range tests {
		x := new(big.Int).SetBytes(common.Hex2Bytes(test.x))
		shift := new(big.Int).SetBytes(common.Hex2Bytes(test.y))
		expected := new(big.Int).SetBytes(common.Hex2Bytes(test.expected))
		stack.push(x)
		stack.push(shift)
		opFn(&pc, evmInterpreter, contract, memory, stack)
		actual := stack.pop()
		if actual.Cmp(expected) != 0 {
			t.Errorf("Testcase %d, expected  %v, got %v(%v)", i, expected, actual, common.Bytes2Hex(actual.Bytes()))
		}
		// Check pool usage
		// 1.pool is not allowed to contain anything on the stack
		// 2.pool is not allowed to contain the same pointers twice
		if evmInterpreter.intPool.pool.len() > 0 {

			poolvals := make(map[*big.Int]struct{})
			poolvals[actual] = struct{}{}

			for evmInterpreter.intPool.pool.len() > 0 {
				key := evmInterpreter.intPool.get()
				if _, exist := poolvals[key]; exist {
					t.Errorf("Testcase %d, pool contains double-entry", i)
				}
				poolvals[key] = struct{}{}
			}
		}
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func TestOpSha3(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(8), "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{v(2), v(1), "4535a04e923af75e64a9f6cdfb922004b40beec0649d36cf6ea095b7c4975cae"},
		{v(3), v(2), "7ad37e9ae69046be83354f8de5e8b4814d21075a11ce84f5e52f89733145e87c"},
		{v(4), v(3), "9edfefee6a285de13826a2f33d0056539b801642d4955a202c46835bfcad0c02"},
	}
	memory := NewMemory()
	memory.Resize(8)
	memory.Set(0, 8, []byte{
		0x01, 0x01, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01,
	})
	testGlobalOperandOp(t, nil, memory, nil, tests, opSha3)
}

func TestOpAddress(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), common.Bytes2Hex([]byte("aaa"))},
	}
	c := &Contract{
		self: &MockAddressRef{},
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opAddress)
}

func TestOpOrigin(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), v(0)},
	}
	c := &Contract{
		self: &MockAddressRef{},
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opOrigin)
}

func TestOpCaller(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), common.Bytes2Hex([]byte("aaa"))},
	}
	c := &Contract{
		self:          &MockAddressRef{},
		CallerAddress: common.BytesToAddress([]byte("aaa")),
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opCaller)
}

func TestOpCallValue(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), v(10)},
	}
	c := &Contract{
		self:          &MockAddressRef{},
		CallerAddress: common.BytesToAddress([]byte("aaa")),
		value:         new(big.Int).SetUint64(10),
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opCallValue)
}

func TestOpCallDataLoad(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), "0102030400000000000000000000000000000000000000000000000000000000"},
	}
	c := &Contract{
		self:          &MockAddressRef{},
		CallerAddress: common.BytesToAddress([]byte("aaa")),
		value:         new(big.Int).SetUint64(10),
		Input:         []byte{0x01, 0x02, 0x03, 0x04},
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opCallDataLoad)
}

func TestOpCallDataSize(t *testing.T) {
	v := func(v int64) string {
		b := new(big.Int).SetInt64(v)
		return common.Bytes2Hex(b.Bytes())
	}
	tests := []twoOperandTest{
		{v(0), v(0), v(4)},
	}
	c := &Contract{
		self:          &MockAddressRef{},
		CallerAddress: common.BytesToAddress([]byte("aaa")),
		value:         new(big.Int).SetUint64(10),
		Input:         []byte{0x01, 0x02, 0x03, 0x04},
	}
	testGlobalOperandOp(t, nil, nil, c, tests, opCallDataSize)
}

func opBenchmark(bench *testing.B, op func(pc *uint64, interpreter *EVMInterpreter, contract *Contract, memory *Memory, stack *Stack) ([]byte, error), args ...string) {
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	// convert args
	byteArgs := make([][]byte, len(args))
	for i, arg := range args {
		byteArgs[i] = common.Hex2Bytes(arg)
	}
	pc := uint64(0)
	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		for _, arg := range byteArgs {
			a := new(big.Int).SetBytes(arg)
			stack.push(a)
		}
		op(&pc, evmInterpreter, nil, nil, stack)
		stack.pop()
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func BenchmarkOpAdd64(b *testing.B) {
	x := "ffffffff"
	y := "fd37f3e2bba2c4f"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpAdd128(b *testing.B) {
	x := "ffffffffffffffff"
	y := "f5470b43c6549b016288e9a65629687"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpAdd256(b *testing.B) {
	x := "0802431afcbce1fc194c9eaa417b2fb67dc75a95db0bc7ec6b1c8af11df6a1da9"
	y := "a1f5aac137876480252e5dcac62c354ec0d42b76b0642b6181ed099849ea1d57"

	opBenchmark(b, opAdd, x, y)
}

func BenchmarkOpSub64(b *testing.B) {
	x := "51022b6317003a9d"
	y := "a20456c62e00753a"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpSub128(b *testing.B) {
	x := "4dde30faaacdc14d00327aac314e915d"
	y := "9bbc61f5559b829a0064f558629d22ba"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpSub256(b *testing.B) {
	x := "4bfcd8bb2ac462735b48a17580690283980aa2d679f091c64364594df113ea37"
	y := "97f9b1765588c4e6b69142eb00d20507301545acf3e1238c86c8b29be227d46e"

	opBenchmark(b, opSub, x, y)
}

func BenchmarkOpMul(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMul, x, y)
}

func BenchmarkOpDiv256(b *testing.B) {
	x := "ff3f9014f20db29ae04af2c2d265de17"
	y := "fe7fb0d1f59dfe9492ffbf73683fd1e870eec79504c60144cc7f5fc2bad1e611"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpDiv128(b *testing.B) {
	x := "fdedc7f10142ff97"
	y := "fbdfda0e2ce356173d1993d5f70a2b11"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpDiv64(b *testing.B) {
	x := "fcb34eb3"
	y := "f97180878e839129"
	opBenchmark(b, opDiv, x, y)
}

func BenchmarkOpSdiv(b *testing.B) {
	x := "ff3f9014f20db29ae04af2c2d265de17"
	y := "fe7fb0d1f59dfe9492ffbf73683fd1e870eec79504c60144cc7f5fc2bad1e611"

	opBenchmark(b, opSdiv, x, y)
}

func BenchmarkOpMod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMod, x, y)
}

func BenchmarkOpSmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSmod, x, y)
}

func BenchmarkOpExp(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opExp, x, y)
}

func BenchmarkOpSignExtend(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSignExtend, x, y)
}

func BenchmarkOpLt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opLt, x, y)
}

func BenchmarkOpGt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opGt, x, y)
}

func BenchmarkOpSlt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSlt, x, y)
}

func BenchmarkOpSgt(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opSgt, x, y)
}

func BenchmarkOpEq(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opEq, x, y)
}
func BenchmarkOpEq2(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201fffffffe"
	opBenchmark(b, opEq, x, y)
}
func BenchmarkOpAnd(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opAnd, x, y)
}

func BenchmarkOpOr(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opOr, x, y)
}

func BenchmarkOpXor(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opXor, x, y)
}

func BenchmarkOpByte(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opByte, x, y)
}

func BenchmarkOpAddmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	z := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opAddmod, x, y, z)
}

func BenchmarkOpMulmod(b *testing.B) {
	x := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	y := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"
	z := "ABCDEF090807060504030201ffffffffffffffffffffffffffffffffffffffff"

	opBenchmark(b, opMulmod, x, y, z)
}

func BenchmarkOpSHL(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSHL, x, y)
}
func BenchmarkOpSHR(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSHR, x, y)
}
func BenchmarkOpSAR(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	y := "ff"

	opBenchmark(b, opSAR, x, y)
}
func BenchmarkOpIsZero(b *testing.B) {
	x := "FBCDEF090807060504030201ffffffffFBCDEF090807060504030201ffffffff"
	opBenchmark(b, opIszero, x)
}

func TestOpMstore(t *testing.T) {
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		mem            = NewMemory()
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	mem.Resize(64)
	pc := uint64(0)
	v := "abcdef00000000000000abba000000000deaf000000c0de00100000000133700"
	stack.pushN(new(big.Int).SetBytes(common.Hex2Bytes(v)), big.NewInt(0))
	opMstore(&pc, evmInterpreter, nil, mem, stack)
	if got := common.Bytes2Hex(mem.Get(0, 32)); got != v {
		t.Fatalf("Mstore fail, got %v, expected %v", got, v)
	}
	stack.pushN(big.NewInt(0x1), big.NewInt(0))
	opMstore(&pc, evmInterpreter, nil, mem, stack)
	if common.Bytes2Hex(mem.Get(0, 32)) != "0000000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("Mstore failed to overwrite previous value")
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}

func BenchmarkOpMstore(bench *testing.B) {
	var (
		env            = NewEVM(Context{}, nil, params.TestChainConfig, Config{})
		stack          = newstack()
		mem            = NewMemory()
		evmInterpreter = NewEVMInterpreter(env, env.vmConfig)
	)

	env.interpreter = evmInterpreter
	evmInterpreter.intPool = poolOfIntPools.get()
	mem.Resize(64)
	pc := uint64(0)
	memStart := big.NewInt(0)
	value := big.NewInt(0x1337)

	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		stack.pushN(value, memStart)
		opMstore(&pc, evmInterpreter, nil, mem, stack)
	}
	poolOfIntPools.put(evmInterpreter.intPool)
}
