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

// getSavings computes the true savings obtainable by a given instruction set by
// computing the optimal instruction selection of each block and the resulting
// cost savings compared to a setup without super instructions.
func getSavings(blocks []BlockStructure, instruction_set InstructionSet, workers int) int64 {

	decoded_set := instruction_set.AsMap()

	in := make(chan BlockStructure, 100)
	out := make(chan int64, 100)
	res := make(chan int64, 1)

	// Start workers processing blocks.
	for i := 0; i < workers; i++ {
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

type InstructionInfo struct {
	instruction SuperInstructionId
	savings     uint64
}

type SelectionProblem struct {
	// The list of possible super instructions and their frequencies
	instructions []InstructionInfo
	// The index of super instructions containing meta information like the actual instruction sequences
	instruction_index SuperInstructionIndex
	// The list of all blocks and their structure information
	block_structure []BlockStructure
	// The maximum number of instructions to be selected
	budget int
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

	solver := runBranchAndBound
	solver = runStagedSolver

	// Run the actual solver.
	best_set, savings := solver(&SelectionProblem{
		instructions:      instructions,
		instruction_index: index,
		block_structure:   block_structure,
		budget:            ctx.Int(BudgetFlag.Name),
	}, ctx.Int(substate.WorkersFlag.Name))

	fmt.Printf("\n----------------------\n")
	fmt.Printf("Best instruction set:\n")
	best_set.Print(&index)
	fmt.Printf("Expected savings: %d\n", savings)

	return nil
}

func runBranchAndBound(selectionProblem *SelectionProblem, workers int) (InstructionSet, int64) {
	instructions := selectionProblem.instructions
	index := selectionProblem.instruction_index
	block_structure := selectionProblem.block_structure
	budget := selectionProblem.budget

	// To prune the search space, we need the instructions to be sorted by their saving potential.
	sort.Slice(instructions, func(i, j int) bool { return instructions[i].savings > instructions[j].savings })

	// Sort list of blocks by size in decreasing order to enable better load balancing in parallel block evaluations.
	sort.Slice(block_structure, func(i, j int) bool { return block_structure[i].structure.rows > block_structure[j].structure.rows })

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
	best_savings := getSavings(block_structure, instruction_set, workers)
	fmt.Printf("Savings of greedy instruction set: %d (%.1f%%)\n", best_savings, (float64(best_savings)/float64(max_savings))*100)

	fmt.Printf("\n=============== Branch & Bound Search ===============\n")

	fmt.Printf("Starting search ...\n")
	steps := 0
	work_list := &WorkList{}

	heap.Push(work_list, Candidate{InstructionSet{}, 0, max_savings})

	for work_list.Len() > 0 {
		cur := heap.Pop(work_list).(Candidate)

		// If by now a better option has been found, skip this one.
		if cur.maximum_potential < best_savings {
			fmt.Printf("Prunning solution with insufficient maximum potential ..\n")
			continue
		}

		steps++
		fmt.Printf("\nStep %d - Work-queue length: %d\n", steps, work_list.Len())

		// Compute saving of current instruction set.
		fmt.Printf("Processing\n")
		cur.instruction_set.Print(&index)
		fmt.Printf("Maximum Potential:  %30d\n", cur.maximum_potential)
		fmt.Printf("Minimum Potential:  %30d\n", cur.minimum_potential)

		savings := getSavings(block_structure, cur.instruction_set, workers)
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
			max_id := SuperInstructionId(0)
			if !cur.instruction_set.Empty() {
				max_id = cur.instruction_set.At(cur.instruction_set.Size() - 1)
			}

			for _, instruction := range instructions {
				if instruction.instruction < max_id {
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
				if maximum_potential > best_savings {
					heap.Push(work_list, Candidate{new_set, minimum_potential, maximum_potential})
				} else {
					// Instuctions are orderd by potential, nothing that follows will be stronger.
					break
				}
			}
		}
	}
	fmt.Printf("Search took %d steps\n", steps)
	return best_instructions, best_savings
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
	a := &(*w)[i]
	b := &(*w)[j]

	if a.minimum_potential > b.minimum_potential {
		return true
	}
	if a.minimum_potential < b.minimum_potential {
		return false
	}

	return a.maximum_potential > b.maximum_potential
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

type SubProblem struct {
	budget    int
	excluding InstructionSet
}

type StagedSolverState struct {
	// The stated instruction selection problem.
	problem *SelectionProblem
	// A record of the costs of every evaluated instruction set.
	eval_data map[InstructionSet]int64
	// The number of workers to be used for parallel operations.
	workers int
	// Solutions for subproblems
	sub_best map[SubProblem]InstructionSet
}

func runStagedSolver(problem *SelectionProblem, workers int) (InstructionSet, int64) {
	state := StagedSolverState{
		problem:   problem,
		eval_data: map[InstructionSet]int64{},
		workers:   workers,
		sub_best:  map[SubProblem]InstructionSet{},
	}

	// To prune the search space, we need the instructions to be sorted by their saving potential.
	sort.Slice(state.problem.instructions, func(i, j int) bool {
		return state.problem.instructions[i].savings > state.problem.instructions[j].savings
	})

	// Sort list of blocks by size in decreasing order to enable better load balancing in parallel block evaluations.
	sort.Slice(state.problem.block_structure, func(i, j int) bool {
		return state.problem.block_structure[i].structure.rows > state.problem.block_structure[j].structure.rows
	})

	// Stage 0 is easy, we can do this in one line.
	fmt.Printf("================ Running Stage %d ================\n", 0)
	state.eval_data[InstructionSet{}] = 0 // The empty instruction set has no savings.
	state.sub_best[SubProblem{budget: 0}] = InstructionSet{}

	// Stage 1 is also easy, it is part of the problem description.
	fmt.Printf("================ Running Stage %d ================\n", 1)
	for _, instruction := range problem.instructions {
		state.eval_data[MakeSingletonSet(instruction.instruction)] = int64(instruction.savings)
	}
	s1_best := MakeSingletonSet(problem.instructions[0].instruction)
	s1_second_best := MakeSingletonSet(problem.instructions[1].instruction)
	state.sub_best[SubProblem{budget: 1}] = s1_best
	state.sub_best[SubProblem{budget: 1, excluding: s1_best}] = s1_second_best

	// Higher stages require more work.
	for i := 2; i <= problem.budget; i++ {
		fmt.Printf("================ Running Stage %d ================\n", 2)

		// Look for the best of the given size.
		best := state.FindBest(SubProblem{budget: i})

		fmt.Printf("Best set for stage %d:\n", i)
		best.Print(&state.problem.instruction_index)

		// If we have reached the full budget, we are done.
		if i == problem.budget {
			break
		}
		/*
			// Find the best solutions excluding every subset of the best solution.
			// Those are sets providing strong bounds for estimating upper bounds for
			// solutions in later stages.
			for _, subset := range best.GetSubsets() {
				if subset.Empty() {
					continue
				}
				next_best := state.FindBest(SubProblem{i, subset})

				fmt.Printf("\n\n=========================\n\n")
				fmt.Printf("Best without\n")
				subset.Print(&state.problem.instruction_index)
				fmt.Printf("is\n")
				next_best.Print(&state.problem.instruction_index)
				fmt.Printf("\n\n=========================\n\n")
			}
		*/
	}

	fmt.Printf("\n=======================\nFull search took %d evaluations\n", len(state.eval_data)-len(state.problem.instructions))

	// Find best encountered solution.
	return state.GetBest()
}

// Attempts to find the best instruction set with the given size. This function assumes
// that a best solution for size-1 has already be found.
func (s *StagedSolverState) FindBest(sub_problem SubProblem) (res InstructionSet) {
	if sub_problem.budget <= 0 {
		return InstructionSet{}
	}

	// Cache the results of this function.
	if val, present := s.sub_best[sub_problem]; present {
		return val
	}
	defer func() {
		s.sub_best[sub_problem] = res
	}()

	size := sub_problem.budget
	excluding := sub_problem.excluding

	// Start by building a Greedy Solution to establish a lower limit on the bound.
	// The greedy set is the best found so far expanded to a set of the requested size.
	greedy_set, _ := s.GetBest()
	greedy_set = Difference(greedy_set, excluding)
	for i := 0; greedy_set.Size() < size && i < len(s.problem.instructions); i++ {
		cur := s.problem.instructions[i].instruction
		if !excluding.Contains(cur) {
			greedy_set = greedy_set.Add(cur)
		}
	}
	best := greedy_set

	fmt.Printf("Evaluating greedy set:\n")
	greedy_set.Print(&s.problem.instruction_index)

	bound := s.Eval(greedy_set)

	// To find the best solutions on this stage we use a branch & bound algorithm.
	worklist := &WorkList{}
	heap.Push(worklist, Candidate{
		instruction_set:   InstructionSet{},
		minimum_potential: 0,
		maximum_potential: s.getUpperBoundForExtraSavings(size, InstructionSet{}, excluding),
	})

	steps := 0
	seen := map[InstructionSet]int{}
	for worklist.Len() > 0 {
		candidate := heap.Pop(worklist).(Candidate)

		// check whether the candidate is still of interest.
		if candidate.maximum_potential < bound {
			continue
		}

		steps++
		fmt.Printf("\n")
		fmt.Printf("Evaluations %d - Stage %d - Step %d - worklist length %d\n", len(s.eval_data)-len(s.problem.instructions), size, steps, worklist.Len())

		// Evaluate current instruction set.
		fmt.Printf("Evaluating\n")
		candidate.instruction_set.Print(&s.problem.instruction_index)

		value := s.Eval(candidate.instruction_set)

		fmt.Printf("Estimated min potential: %30d\n", candidate.minimum_potential)
		fmt.Printf("Actual savings:          %30d\n", value)
		fmt.Printf("Curent best:             %30d\n", bound)
		fmt.Printf("Estimated max potential: %30d\n", candidate.maximum_potential)
		if value > bound {
			fmt.Printf("NEW BEST!\n")
			best = candidate.instruction_set
			bound = value
		}

		// Expand instruction set.
		if candidate.instruction_set.Size() < size {
			for _, instruction := range s.problem.instructions {
				// Do not include excluded instructions.
				if excluding.Contains(instruction.instruction) {
					continue
				}
				// Do not include already present instructions.
				if candidate.instruction_set.Contains(instruction.instruction) {
					continue
				}

				extended := candidate.instruction_set.Add(instruction.instruction)

				// Filter out previously encountered sets.
				if _, present := seen[extended]; present {
					continue
				}
				seen[extended] = 0

				min_potential := value
				max_potential := value + int64(instruction.savings) + s.getUpperBoundForExtraSavings(size, extended, excluding)

				if max_potential > bound {
					heap.Push(worklist, Candidate{
						instruction_set:   extended,
						minimum_potential: min_potential,
						maximum_potential: max_potential,
					})
				} else {
					// Since instructions are ordered by their savings, nothing better will follow.
					break
				}
			}
		}
	}

	return best
}

// Estimates an upper boundary of the maximum extra savings that can be obtained by expanding the given
// instruction set to the provided budget.
func (s *StagedSolverState) getUpperBoundForExtraSavings(budget int, set InstructionSet, excluding InstructionSet) int64 {
	// TODO: improve using savings already computed for non-singleton sets
	// TODO: exclude instructions that are not allowed

	// Empty sets need special handling to avoid recursive dependencies.
	if set.Empty() {
		// We use the best known solution for a one unit smaller budget ..
		best := s.FindBest(SubProblem{budget: budget - 1, excluding: excluding})

		// and add the potential of the most promising instruction not included in the set.
		for _, instruction := range s.problem.instructions {
			if !best.Contains(instruction.instruction) && !excluding.Contains(instruction.instruction) {
				return s.Eval(best) + int64(instruction.savings)
			}
		}
		/*
			var sum int64
			for _, instruction := range s.problem.instructions {
				if set.Size() >= budget {
					return sum
				}
				if !set.Contains(instruction.instruction) {
					set = set.Add(instruction.instruction)
					sum += int64(instruction.savings)
				}
			}
			return sum
		*/
	}

	// See how much extra space there is.
	space := budget - set.Size()

	// See what is the best combination for this extra space.
	/*
		if _, present := s.sub_best[SubProblem{budget: space}]; !present {
			panic(fmt.Sprintf("Subproblem %v not precomuted!", SubProblem{budget: space}))
		}
	*/
	best := s.FindBest(SubProblem{budget: space, excluding: excluding})

	// Check whether there are overlaps.
	overlap := Intersect(best, set)

	// If there are none, the best solution for the remaining space is the tightest boundary we have.
	if overlap.Empty() {
		return s.Eval(best)
	}

	// If there are overlap, look for the best solution without overlap.
	/*
		if _, present := s.sub_best[SubProblem{budget: space, excluding: overlap}]; !present {
			panic(fmt.Sprintf("Subproblem %v not precomuted!", SubProblem{budget: space, excluding: overlap}))
		}
	*/
	best = s.FindBest(SubProblem{budget: space, excluding: Union(overlap, excluding)})
	return s.Eval(best)
}

// Evaluates the actual savings of the givne instruction set. Results are cached.
func (s *StagedSolverState) Eval(set InstructionSet) int64 {
	val, present := s.eval_data[set]
	if present {
		return val
	}
	val = getSavings(s.problem.block_structure, set, s.workers)
	s.eval_data[set] = val
	return val
}

func (s *StagedSolverState) GetBest() (best InstructionSet, max int64) {
	for k, v := range s.eval_data {
		if v > max {
			best = k
			max = v
		}
	}
	return
}
