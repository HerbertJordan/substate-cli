package sisel

import (
	"container/heap"
	"fmt"
	"os"
	"runtime/pprof"
	"sort"

	"github.com/Fantom-foundation/substate-cli/cmd/substate-cli/replay"
	"github.com/ethereum/go-ethereum/substate"
	"gopkg.in/urfave/cli.v1"
)

// record-replay: substate-cli code command
var SelectInstrictionsCommand = cli.Command{
	Action: siselAction,
	Name:   "si-selection",
	Usage:  "computes a selection of super instructions",
	Flags: []cli.Flag{
		substate.WorkersFlag,
		substate.SubstateDirFlag,
		replay.CpuProfilingFlag,
		BlockDBFlag,
		BudgetFlag,
	},
	Description: `
The substate-cli si-selection command computes an optimal selection of
super instructions given a database describing basic blocks and their
frequency.

The list of super instructions to be selected are written to the console.
`,
}

// block-db filename
var BlockDBFlag = cli.StringFlag{
	Name:  "blockdb",
	Usage: "SQL lite data base with block frequencies",
	Value: "",
}

var BudgetFlag = cli.IntFlag{
	Name:  "budget",
	Usage: "The maximum number of super instructions to be selected",
	Value: 5,
}

func getSavings(blocks []BlockStructure, instruction_set InstructionSet) int64 {

	decoded_set := instruction_set.AsMap()

	in := make(chan BlockStructure, 100)
	out := make(chan int64, 100)
	res := make(chan int64, 1)

	// Start workers processing blocks.
	for i := 0; i < 12; i++ {
		go func() {
			for block := range in {
				saving := block.GetSavingFor(decoded_set)
				out <- int64(saving) * block.frequency
			}
		}()
	}

	// Start result aggregator.
	go func() {
		var sum int64
		for range blocks {
			sum += <-out
		}
		res <- sum
	}()

	// Send blocks to channel.
	for _, block := range blocks {
		in <- block
	}
	close(in)

	// Wait for result.
	return <-res
}

func getUpperBoundForExtraSaving(instruction_set InstructionSet, instructions []InstructionInfo, budget int) int64 {
	count := instruction_set.Size()
	var res int64
	if count >= budget {
		return res
	}
	for _, cur := range instructions {
		if !instruction_set.Contains(cur.instruction) {
			res += int64(cur.savings)
			count++
			if count >= budget {
				return res
			}
		}
	}
	return res
}

type InstructionInfo struct {
	instruction SuperInstructionId
	savings     uint64
}

type Candidate struct {
	instruction_set   InstructionSet
	minimum_potential int64
	maximum_potential int64
}

// Worklist implements a heap
type WorkList []Candidate

func (w *WorkList) Len() int {
	return len(*w)
}

func (w *WorkList) Less(i, j int) bool {
	// We force a maximum-heap
	/*
		if (*w)[i].instruction_set.Size() > (*w)[j].instruction_set.Size() {
			return true
		}
		if (*w)[i].instruction_set.Size() < (*w)[j].instruction_set.Size() {
			return false
		}
	*/
	a := &(*w)[i]
	b := &(*w)[j]

	if a.minimum_potential > b.minimum_potential {
		return true
	}
	if a.minimum_potential < b.minimum_potential {
		return false
	}

	return a.maximum_potential > b.maximum_potential

	//return (*w)[i].minimum_potential > (*w)[j].minimum_potential
	//return (*w)[i].maximum_potential > (*w)[j].maximum_potential
}

func (w *WorkList) Swap(i, j int) {
	(*w)[i], (*w)[j] = (*w)[j], (*w)[i]
}

func (w *WorkList) Push(x any) {
	*w = append(*w, x.(Candidate))
}

func (w *WorkList) Pop() (res any) {
	res = (*w)[w.Len()-1]
	*w = (*w)[:w.Len()-1]
	return
}

func siselAction(ctx *cli.Context) error {
	// Load basic blocks.
	filename := ctx.String(BlockDBFlag.Name)
	fmt.Printf("Loading block infos from %v ...\n", filename)
	blocks, err := LoadBlocks(filename)
	if err != nil {
		return err
	}
	fmt.Printf("Loaded %d blocks from DB\n", len(blocks))

	// Index super instructions in blocks.
	fmt.Printf("Creating Super Instruction Index ..\n")
	index, frequencies, block_structure := CreateSiIndex(blocks)
	fmt.Printf("Indexed %d super instructions\n", index.Size())

	// Sort list of instructions by saving potential in decreasing order.
	instructions := make([]InstructionInfo, len(frequencies))
	for i := range instructions {
		instructions[i].instruction = SuperInstructionId(i)
		instructions[i].savings = frequencies[i] * uint64(index.Get(SuperInstructionId(i)).Size()-1)
	}
	frequencies = nil
	sort.Slice(instructions, func(i, j int) bool { return instructions[i].savings > instructions[j].savings })

	// Sort list of blocks by size in decreasing order.
	sort.Slice(block_structure, func(i, j int) bool { return block_structure[i].structure.rows > block_structure[j].structure.rows })

	// Start CPU profiling if requested.
	profile_file_name := ctx.String(replay.CpuProfilingFlag.Name)
	if profile_file_name != "" {
		f, err := os.Create(profile_file_name)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Get instruction budget.
	budget := ctx.Int(BudgetFlag.Name)

	fmt.Printf("\n=============== Greedy Search ===============\n")

	// Estimate maximum saving potential
	instruction_set := InstructionSet{}
	max_savings := getUpperBoundForExtraSaving(instruction_set, instructions, budget)
	fmt.Printf("Upper bound of saving potential for %d instructions: %d\n", budget, max_savings)

	// Compute an initial greedy solution.
	fmt.Printf("Computing savings for greedy solution ..\n")
	for i := 0; i < budget; i++ {
		fmt.Printf("  %v - %d\n", index.Get(instructions[i].instruction), instructions[i].savings)
		instruction_set = instruction_set.Add(instructions[i].instruction)
	}

	best_instructions := instruction_set
	best_savings := getSavings(block_structure, instruction_set)
	fmt.Printf("Savings of greedy instruction set: %d (%.1f%%)\n", best_savings, (float64(best_savings)/float64(max_savings))*100)

	fmt.Printf("\n=============== Branch & Bound Search ===============\n")

	fmt.Printf("Starting search ...\n")
	steps := 0
	work_list := &WorkList{}

	heap.Push(work_list, Candidate{InstructionSet{}, 0, max_savings})
	/*
		work_list := []Candidate{
			{MakeEmptyInstructionSet(), max_savings},
		}
	*/

	//for len(work_list) > 0 {
	for work_list.Len() > 0 {
		fmt.Printf("\nWork-queue length: %d\n", work_list.Len())
		/*
			cur := work_list[len(work_list)-1]
			work_list = work_list[:len(work_list)-1]
		*/
		cur := heap.Pop(work_list).(Candidate)

		// If by now a better option has been found, skip this one.
		if cur.maximum_potential < best_savings {
			fmt.Printf("Prunning solution with insufficient maximum potential ..\n")
			continue
		}

		steps++

		// Compute saving of current instruction set.
		fmt.Printf("Processing\n")
		cur.instruction_set.Print(&index)
		fmt.Printf("Maximum Potential:  %30d\n", cur.maximum_potential)
		fmt.Printf("Minimum Potential:  %30d\n", cur.minimum_potential)

		savings := getSavings(block_structure, cur.instruction_set)
		fmt.Printf("Actual savings:     %30d (%.1f%%)\n", savings, (float64(savings)/float64(max_savings))*100)
		if best_savings < savings {
			fmt.Printf("NEW BEST!\n")
			best_savings = savings
			best_instructions = cur.instruction_set
		} else {
			fmt.Printf("Best instruction set so far:\n")
			best_instructions.Print(&index)
			fmt.Printf("Realized savings: %d (%.1f%%)\n", best_savings, (float64(best_savings)/float64(max_savings))*100)
		}

		// Compute extensions
		if cur.instruction_set.Size() < budget {
			max_id := 0
			for instruction := range cur.instruction_set.AsMap() {
				if max_id < int(instruction) {
					max_id = int(instruction)
				}
			}

			for _, instruction := range instructions {
				if int(instruction.instruction) < max_id {
					continue
				}
				if cur.instruction_set.Contains(instruction.instruction) {
					continue
				}

				if instruction.savings == 0 {
					continue
				}

				// Create extended instruction set
				new_set := cur.instruction_set.Add(instruction.instruction)

				// Estimate potential of new solution.
				minimum_potential := savings
				maximum_potential := minimum_potential + int64(instruction.savings) + getUpperBoundForExtraSaving(new_set, instructions, budget)
				/*
					fmt.Printf("New set:\n")
					new_set.Print(&index)
					fmt.Printf("  savings of parent:           %d\n", savings)
					fmt.Printf("  added instruction potential: %d\n", instruction.savings)
					fmt.Printf("  upper bound:                 %d\n", getUpperBoundForExtraSaving(new_set, instructions, budget))
					fmt.Printf("  total potential:             %d\n", potential)
				*/
				if maximum_potential > best_savings {
					//work_list = append(work_list, Candidate{new_set, potential})
					heap.Push(work_list, Candidate{new_set, minimum_potential, maximum_potential})
				} else {
					// Instuctions are orderd by potential, nothing that follows will be stronger.
					break
					//fmt.Printf("Pruned %v, to small potential\n", instruction.instruction)
				}
			}
		}
	}
	fmt.Printf("Search took %d steps\n", steps)
	fmt.Printf("\n----------------------\n")
	fmt.Printf("Best instruction set:\n")
	best_instructions.Print(&index)
	fmt.Printf("Realized savings: %d (%.1f%%)\n", best_savings, (float64(best_savings)/float64(max_savings))*100)

	return nil
}
