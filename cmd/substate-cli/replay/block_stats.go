package replay

import (
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/substate"
	"gopkg.in/urfave/cli.v1"
)

// record-replay: substate-cli key-stats command
var GetBlockStatsCommand = cli.Command{
	Action:    getBlockStatsAction,
	Name:      "block-stats",
	Usage:     "computes statistical data on a per-block level",
	ArgsUsage: "<blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		substate.WorkersFlag,
		substate.SubstateDirFlag,
		ChainIDFlag,
	},
	Description: `
The substate-cli block-stats command requires two arguments:
<blockNumFirst> <blockNumLast>

<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to be analysed.

Statistics on block properties are printed to the console.
`,
}

// dependsOn determines whether the transaction described by substate b depends
// on the results of the transation described by substate a.
func dependsOn(a, b *substate.Substate) bool {
	/*
		fmt.Printf("Out:\n")
		for key := range a.OutputAlloc {
			fmt.Printf("  %v\n", key)
		}
		fmt.Printf("In:\n")
		for key := range b.InputAlloc {
			fmt.Printf("  %v\n", key)
		}
	*/
	// We consider there to be a dependency as soon as they are referencing some
	// information about a common address.
	for key := range b.InputAlloc {
		_, exists := a.OutputAlloc[key]
		if exists {
			//fmt.Printf(" --- DEPENDENCY!!\n")
			return true
		}
	}
	return false
}

type transaction_statistics struct {
	num_blocks               uint64
	num_transactions         []uint64
	num_parallel_transaction []uint64
	num_max_depth            []uint64
}

func (s *transaction_statistics) Add(num_transactions, num_parallel_transactions, max_depth int) {
	if len(s.num_transactions) <= num_transactions {
		s.num_transactions = append(s.num_transactions, make([]uint64, num_transactions-len(s.num_transactions)+1)...)
	}
	if len(s.num_parallel_transaction) <= num_parallel_transactions {
		s.num_parallel_transaction = append(s.num_parallel_transaction, make([]uint64, num_parallel_transactions-len(s.num_parallel_transaction)+1)...)
	}
	if len(s.num_max_depth) <= max_depth {
		s.num_max_depth = append(s.num_max_depth, make([]uint64, max_depth-len(s.num_max_depth)+1)...)
	}
	s.num_blocks++
	s.num_transactions[num_transactions]++
	s.num_parallel_transaction[num_parallel_transactions]++
	s.num_max_depth[max_depth]++
}

func (s *transaction_statistics) PrintSummary() {
	fmt.Printf("Number of blocks: %d\n", s.num_blocks)
	fmt.Printf("Transaction histogram:\n")
	for i, v := range s.num_transactions {
		fmt.Printf("%d,%d\n", i, v)
	}
	fmt.Printf("Parallel transaction histogram:\n")
	for i, v := range s.num_parallel_transaction {
		fmt.Printf("%d,%d\n", i, v)
	}
	fmt.Printf("Maximum dependency depth histogram:\n")
	for i, v := range s.num_max_depth {
		fmt.Printf("%d,%d\n", i, v)
	}
}

var transaction_mutex = sync.Mutex{}

var transaction_stats = transaction_statistics{
	num_transactions:         []uint64{},
	num_parallel_transaction: []uint64{},
	num_max_depth:            []uint64{},
}

type transaction struct {
	tx       int
	substate *substate.Substate
}

func processBlock(block uint64, transactions map[int]*substate.Substate, pool *substate.SubstateTaskPool) error {
	if len(transactions) == 0 {
		return nil
	}

	// Get sorted list of transaction.
	list := make([]transaction, 0, len(transactions))
	for tx, substate := range transactions {
		list = append(list, transaction{tx: tx, substate: substate})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].tx < list[j].tx })

	// Compute the dependencies of the transactions
	num_dependency_free := 0
	max_depth := 0
	depths := make([]int, len(list))
	for i := 0; i < len(transactions); i++ {
		depth := 0
		for j := 0; j < i; j++ {
			if dependsOn(list[j].substate, list[i].substate) {
				if depths[j]+1 > depth {
					depth = depths[j] + 1
				}
			}
		}
		depths[i] = depth
		if depth == 0 {
			num_dependency_free++
		}
		if depth > max_depth {
			max_depth = depth
		}
	}

	transaction_mutex.Lock()
	defer transaction_mutex.Unlock()
	transaction_stats.Add(len(list), num_dependency_free, max_depth)
	return nil
}

// getBlockStatsAction collects statistical information on blocks and
// prints those to the console.
func getBlockStatsAction(ctx *cli.Context) error {
	var err error

	if len(ctx.Args()) != 2 {
		return fmt.Errorf("substate-cli block-stats command requires exactly 2 arguments")
	}

	chainID = ctx.Int(ChainIDFlag.Name)
	fmt.Printf("chain-id: %v\n", chainID)
	fmt.Printf("git-date: %v\n", gitDate)
	fmt.Printf("git-commit: %v\n", gitCommit)

	first, ferr := strconv.ParseInt(ctx.Args().Get(0), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("substate-cli block-stats: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("substate-cli block-stats: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("substate-cli block-stats: error: first block has larger number than last block")
	}

	substate.SetSubstateFlags(ctx)
	substate.OpenSubstateDBReadOnly()
	defer substate.CloseSubstateDB()

	taskPool := substate.NewSubstateTaskPool("substate-cli storage", nil, uint64(first), uint64(last), ctx)
	taskPool.BlockFunc = processBlock
	err = taskPool.Execute()

	transaction_stats.PrintSummary()
	return err
}
