package replay

import (
	"fmt"
	"os"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/substate"
	"gopkg.in/urfave/cli.v1"
)

var EnableSelfOptimizationDbFlag = cli.BoolFlag{
	Name:  "selfoptimize",
	Usage: "enables the self-optimization feature in the simulated storage system",
}

// record-replay: substate-cli storage-sim command
var GetStorageSimCommand = cli.Command{
	Action:    getStorageSimulationAction,
	Name:      "storage-sim",
	Usage:     "simulates storage access patterns on storage implementations",
	ArgsUsage: "<blockNumFirst> <blockNumLast>",
	Flags: []cli.Flag{
		substate.WorkersFlag,
		substate.SubstateDirFlag,
		CpuProfilingFlag,
		EnableSelfOptimizationDbFlag,
		ChainIDFlag,
	},
	Description: `
The substate-cli storage-sim command requires two arguments:
<blockNumFirst> <blockNumLast>

<blockNumFirst> and <blockNumLast> are the first and
last block of the inclusive range of blocks to be analysed.

Statistics on the performance of the simulated storage are printed to the console.
`,
}

type TransactionId struct {
	block uint64
	tx    int
}

// SimulatedStorage defines an interface for a simulated storage to which
// a simulated traffic stream is directed to.
type SimulatedStorage interface {
	Load(addr common.Address, key common.Hash) common.Hash
	Store(addr common.Address, key common.Hash, value common.Hash)

	Start(tx TransactionId)
	End(tx TransactionId)

	PrintSummary()
}

// getStorageSimulationAction simulates the sequential access triggered by the
// sequence of transactions recorded in the substate database.
func getStorageSimulationAction(ctx *cli.Context) error {
	var err error

	if len(ctx.Args()) != 2 {
		return fmt.Errorf("substate-cli replay command requires exactly 2 arguments")
	}

	chainID = ctx.Int(ChainIDFlag.Name)
	fmt.Printf("chain-id: %v\n", chainID)
	fmt.Printf("git-date: %v\n", gitDate)
	fmt.Printf("git-commit: %v\n", gitCommit)

	first, ferr := strconv.ParseInt(ctx.Args().Get(0), 10, 64)
	last, lerr := strconv.ParseInt(ctx.Args().Get(1), 10, 64)
	if ferr != nil || lerr != nil {
		return fmt.Errorf("substate-cli replay: error in parsing parameters: block number not an integer")
	}
	if first < 0 || last < 0 {
		return fmt.Errorf("substate-cli replay: error: block number must be greater than 0")
	}
	if first > last {
		return fmt.Errorf("substate-cli replay: error: first block has larger number than last block")
	}

	if ctx.Bool(ProfileEVMCallFlag.Name) {
		vm.ProfileEVMCall = true
	}
	if ctx.Bool(ProfileEVMOpCodeFlag.Name) {
		vm.ProfileEVMOpCode = true
	}

	substate.SetSubstateFlags(ctx)
	substate.OpenSubstateDBReadOnly()
	defer substate.CloseSubstateDB()

	var store SimulatedStorage
	store = &CountingStorage{}
	store = NewFlatStorage(FlatStorageConfig{
		self_optimize: ctx.Bool(EnableSelfOptimizationDbFlag.Name),
	})
	defer store.PrintSummary()

	// Start CPU profiling if requested.
	profile_file_name := ctx.String(CpuProfilingFlag.Name)
	if profile_file_name != "" {
		f, err := os.Create(profile_file_name)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	num_workers := ctx.Int(substate.WorkersFlag.Name)
	fmt.Printf("substate-cli storage-sim: loading data using %d workers\n", num_workers)
	iter := substate.NewSubstateIterator(uint64(first), num_workers)
	defer iter.Release()
	step := 0
	start := time.Now()
	for iter.Next() {
		transaction := iter.Value()
		if transaction == nil || transaction.Block > uint64(last) {
			return nil
		}
		tx_id := TransactionId{transaction.Block, transaction.Transaction}

		store.Start(tx_id)

		// Simulate read operations of this transaction.
		for addr, account := range transaction.Substate.InputAlloc {
			for key := range account.Storage {
				store.Load(addr, key)
			}
		}

		// Simulate write operations of this transaction.
		for addr, account := range transaction.Substate.OutputAlloc {
			for key, value := range account.Storage {
				store.Store(addr, key, value)
			}
		}

		store.End(tx_id)

		// Some eye candy to show progress.
		step++
		if step%100000 == 0 {
			duration := time.Since(start)
			throughput := float64(step) / duration.Seconds()
			fmt.Printf("Processed block %d, transaction %d, t=%v, %.1f tx/s\n", transaction.Block, transaction.Transaction, duration, throughput)
		}
	}

	return err
}

// --- Demo simulator for basic testing ---

type CountingStorage struct {
	loads  uint64
	stores uint64
}

func (s *CountingStorage) Load(addr common.Address, key common.Hash) common.Hash {
	s.loads++
	return common.Hash{}
}

func (s *CountingStorage) Store(addr common.Address, key common.Hash, value common.Hash) {
	s.stores++
}

func (s *CountingStorage) Start(_ TransactionId) {}
func (s *CountingStorage) End(_ TransactionId)   {}

func (s *CountingStorage) PrintSummary() {
	fmt.Printf("Number of Loads:  %d\n", s.loads)
	fmt.Printf("Number of Stores: %d\n", s.stores)
}

// --- Simulator for flat storage design ---

type addr_id uint32
type key_id uint32
type loc_id uint32

type loc struct {
	addr_id addr_id
	key_id  key_id
}

type FlatStorageConfig struct {
	self_optimize bool
}

type FlatStorage struct {
	config FlatStorageConfig

	addr_index        map[common.Address]addr_id
	key_index         map[common.Hash]key_id
	loc_index         map[loc]loc_id
	reverse_loc_index map[loc_id]loc

	counter       int
	bucket_counts []uint64
	count_lists   [][]uint64
}

func NewFlatStorage(config FlatStorageConfig) *FlatStorage {
	return &FlatStorage{
		config:            config,
		addr_index:        map[common.Address]addr_id{},
		key_index:         map[common.Hash]key_id{},
		loc_index:         map[loc]loc_id{},
		reverse_loc_index: map[loc_id]loc{},
		bucket_counts:     make([]uint64, 0),
		count_lists:       make([][]uint64, 0),
	}
}

func (s *FlatStorage) getAddressId(addr common.Address) addr_id {
	if val, present := s.addr_index[addr]; present {
		return val
	}
	res := addr_id(len(s.addr_index))
	s.addr_index[addr] = res
	return res
}

func (s *FlatStorage) getKeyId(key common.Hash) key_id {
	if val, present := s.key_index[key]; present {
		return val
	}
	res := key_id(len(s.key_index))
	s.key_index[key] = res
	return res
}

func (s *FlatStorage) getLocationId(addr common.Address, key common.Hash) loc_id {
	loc := loc{s.getAddressId(addr), s.getKeyId(key)}
	if val, present := s.loc_index[loc]; present {
		return val
	}
	res := loc_id(len(s.loc_index))
	s.loc_index[loc] = res
	s.reverse_loc_index[res] = loc
	return res
}

func (s *FlatStorage) access(pos loc_id) {
	bucket := pos / (1 << 15) // Assuming ~1MiB pages with 32 byte values
	if len(s.bucket_counts) < int(bucket+1) {
		s.bucket_counts = append(s.bucket_counts, make([]uint64, int(bucket)-len(s.bucket_counts)+1)...)
	}
	s.bucket_counts[bucket]++

	if s.config.self_optimize {
		// Swap location with parent
		loc := s.reverse_loc_index[pos]
		parent_pos := pos / 2
		parent_loc := s.reverse_loc_index[parent_pos]

		s.loc_index[parent_loc] = pos
		s.reverse_loc_index[pos] = parent_loc

		s.loc_index[loc] = parent_pos
		s.reverse_loc_index[parent_pos] = loc

		// Register swap as an access.
		bucket = parent_pos / (1 << 15)
		s.bucket_counts[bucket]++
	}

	// Collect statistics.
	s.counter++
	if s.counter%1000000 == 0 {
		s.count_lists = append(s.count_lists, s.bucket_counts)
		s.bucket_counts = make([]uint64, 0)
	}

}

func (s *FlatStorage) Load(addr common.Address, key common.Hash) common.Hash {
	s.access(s.getLocationId(addr, key))
	return common.Hash{}
}

func (s *FlatStorage) Store(addr common.Address, key common.Hash, value common.Hash) {
	s.access(s.getLocationId(addr, key))
}

func (s *FlatStorage) Start(_ TransactionId) {}
func (s *FlatStorage) End(_ TransactionId)   {}

func (s *FlatStorage) PrintSummary() {
	max_length := 0
	for _, list := range s.count_lists {
		if len(list) > max_length {
			max_length = len(list)
		}
	}
	fmt.Printf("Access Counts:\n")
	for _, list := range s.count_lists {
		//fmt.Printf("%d", i)
		for _, count := range list {
			fmt.Printf(" %d", count)
		}
		for i := 0; i < max_length-len(list); i++ {
			fmt.Printf(" 0")
		}
		fmt.Printf("\n")
	}
}
