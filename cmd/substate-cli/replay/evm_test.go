package replay

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/Fantom-foundation/go-opera/evmcore"
	"github.com/Fantom-foundation/go-opera/opera"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/lfvm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/substate"
)

// runTransaction runs a single transaction through the EVM.
func runTransaction(vm_impl string, transaction *substate.Transaction) error {

	inputAlloc := transaction.Substate.InputAlloc
	inputEnv := transaction.Substate.Env
	inputMessage := transaction.Substate.Message

	var (
		vmConfig    vm.Config
		chainConfig *params.ChainConfig
	)

	vmConfig = opera.DefaultVMConfig
	vmConfig.NoBaseFee = true

	chainConfig = params.AllEthashProtocolChanges
	chainConfig.ChainID = big.NewInt(int64(chainID))
	chainConfig.LondonBlock = new(big.Int).SetUint64(37534833)
	chainConfig.BerlinBlock = new(big.Int).SetUint64(37455223)

	var hashError error
	getHash := func(num uint64) common.Hash {
		if inputEnv.BlockHashes == nil {
			hashError = fmt.Errorf("getHash(%d) invoked, no blockhashes provided", num)
			return common.Hash{}
		}
		h, ok := inputEnv.BlockHashes[num]
		if !ok {
			hashError = fmt.Errorf("getHash(%d) invoked, blockhash for that block not provided", num)
		}
		return h
	}

	statedb := MakeInMemoryStateDB(&inputAlloc)

	// Apply Message
	var (
		gaspool = new(evmcore.GasPool)
		txHash  = common.Hash{0x02}
		txIndex = transaction.Transaction
	)

	gaspool.AddGas(inputEnv.GasLimit)
	blockCtx := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		Coinbase:    inputEnv.Coinbase,
		BlockNumber: new(big.Int).SetUint64(inputEnv.Number),
		Time:        new(big.Int).SetUint64(inputEnv.Timestamp),
		Difficulty:  inputEnv.Difficulty,
		GasLimit:    inputEnv.GasLimit,
		GetHash:     getHash,
	}
	// If currentBaseFee is defined, add it to the vmContext.
	if inputEnv.BaseFee != nil {
		blockCtx.BaseFee = new(big.Int).Set(inputEnv.BaseFee)
	}

	msg := inputMessage.AsMessage()

	vmConfig.Tracer = nil
	vmConfig.Debug = false
	vmConfig.InterpreterImpl = vm_impl
	statedb.Prepare(txHash, txIndex)

	txCtx := evmcore.NewEVMTxContext(msg)

	evm := vm.NewEVM(blockCtx, txCtx, statedb, chainConfig, vmConfig)

	_, err := evmcore.ApplyMessage(evm, msg, gaspool)

	if err != nil {
		return err
	}
	if hashError != nil {
		return hashError
	}
	return nil
}

var benchmark_transactions []*substate.Transaction

const (
	substate_dir = "/media/herbert/Data/fantom/substate/substate.fantom"
	start_block  = 5000000
	end_block    = start_block + 10000
	num_workers  = 4
)

func loadTransactions() []*substate.Transaction {
	if benchmark_transactions != nil {
		return benchmark_transactions
	}

	fmt.Printf("Loading transactions ...\n")
	substate.SetSubstateDirectory(substate_dir)
	substate.OpenSubstateDBReadOnly()
	defer substate.CloseSubstateDB()

	transactions := []*substate.Transaction{}
	iter := substate.NewSubstateIterator(start_block, num_workers)
	defer iter.Release()
	for iter.Next() {
		transaction := iter.Value()
		if transaction.Block > end_block {
			break
		}
		transactions = append(transactions, transaction)
	}
	fmt.Printf("Loading transactions Finished\n")
	fmt.Printf("Loaded %d transactions\n", len(transactions))
	benchmark_transactions = transactions
	return transactions
}

func runBenchmarkEvm(b *testing.B, impl string) {
	transactions := loadTransactions()
	lfvm.ResetCodeCache()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, t := range transactions {
			runTransaction(impl, t)
		}
	}
}

func BenchmarkGeth(b *testing.B) {
	runBenchmarkEvm(b, "geth")
}

func BenchmarkLfvm(b *testing.B) {
	runBenchmarkEvm(b, "lfvm")
}

func BenchmarkLfvmWithSuperInstructions(b *testing.B) {
	runBenchmarkEvm(b, "lfvm-si")
}
