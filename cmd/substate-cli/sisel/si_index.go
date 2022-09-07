package sisel

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/vm"
)

type SuperInstructionId uint32

const (
	MAX_SI_LENGTH                    = 10
	INVALID       SuperInstructionId = 0
)

type SuperInstructionIndex struct {
	// Maps IDs -> Instructions
	list []SuperInstruction
	// Maps super instructions to IDs
	index map[SuperInstruction]SuperInstructionId
}

func (i *SuperInstructionIndex) Size() int {
	return len(i.list)
}

func (i *SuperInstructionIndex) Add(si SuperInstruction) SuperInstructionId {
	if i.index == nil {
		i.list = []SuperInstruction{NewSuperInstruction([]vm.OpCode{})}
		i.index = map[SuperInstruction]SuperInstructionId{
			i.list[0]: INVALID,
		}
	}
	val, present := i.index[si]
	if !present {
		val = SuperInstructionId(len(i.list))
		i.list = append(i.list, si)
		i.index[si] = val
	}
	return val
}

func (i *SuperInstructionIndex) Get(id SuperInstructionId) *SuperInstruction {
	return &(i.list[id])
}

type BlockStructure struct {
	structure Triangle[SuperInstructionId]
	frequency int64
}

func (b *BlockStructure) Print() {
	PrintMatrix(b.structure)
}

func PrintMatrix[T any](t Triangle[T]) {
	for i := 0; i < t.Rows(); i++ {
		for j := 0; j <= i; j++ {
			fmt.Printf("%v ", t.Get(i, j))
		}
		fmt.Printf("\n")
	}
}

func (b *BlockStructure) GetSavingFor(set map[SuperInstructionId]int) int {
	// Computes the optimal instruction selection for this block given the SI set.
	rows := b.structure.Rows()
	block_length := rows + 2

	getInstruction := func(start, end int) SuperInstructionId {
		length := end - start
		if length < 2 {
			return INVALID
		}
		return b.structure.Get(rows-length+1, start)
	}

	// Use O(N^2) algorithm first to detect branches with super instructions.
	is_affected := NewTriangleMatrix[bool](rows)

	isAffected := func(start, end int) bool {
		length := end - start
		if length < 2 {
			return false
		}
		return is_affected.Get(rows-length+1, start)
	}
	setAffected := func(start, end int) {
		length := end - start
		is_affected.Set(rows-length+1, start, true)
	}

	for i := 0; i <= rows-1; i++ {
		if _, present := set[getInstruction(i, i+2)]; present {
			setAffected(i, i+2)
		}
	}

	for l := 3; l <= block_length; l++ {
		for i := 0; i < block_length-l; i++ {
			si_id := getInstruction(i, i+l)
			// Case 1: If the current range is a super instruction, it is affected.
			if _, present := set[si_id]; present || isAffected(i, i+l-1) || isAffected(i+1, i+l) {
				setAffected(i, i+l)
			}
		}
	}

	if !isAffected(0, rows+1) {
		return 0
	}

	// Compute savings using O(N^3) dynamic programming algorithm
	savings := NewTriangleMatrix[int](rows)

	getSaving := func(start, end int) int {
		length := end - start
		if length < 2 {
			return 0
		}
		return savings.Get(rows-length+1, start)
	}
	setSaving := func(start, end, value int) {
		length := end - start
		savings.Set(rows-length+1, start, value)
	}

	for i := 0; i <= rows-1; i++ {
		saving := 0
		if _, present := set[getInstruction(i, i+2)]; present {
			saving = 1
		}
		setSaving(i, i+2, saving)
	}

	for l := 3; l <= block_length; l++ {
		for i := 0; i < block_length-l; i++ {
			if !isAffected(i, i+l) {
				continue
			}
			// Compute maximal savings for interval [i,(i+l))
			saving := 0
			// Case 1: full interval is a single instruction
			si_id := getInstruction(i, i+l)
			if _, present := set[si_id]; present {
				saving = l - 1
			}
			// Case 2: find ideal split point of range for maximal savings
			for j := 1; j < l; j++ {
				s := getSaving(i, i+j) + getSaving(i+j, i+l)
				if s > saving {
					saving = s
				}
			}
			setSaving(i, i+l, saving)
		}
	}

	return getSaving(0, rows+1)
}

func CreateSiIndex(blocks []BlockInfo) (SuperInstructionIndex, []uint64, []BlockStructure) {
	fmt.Printf("WARNING - using limited SI instruction length!\n")
	index := SuperInstructionIndex{}
	frequency := []uint64{}
	structures := []BlockStructure{}
	for _, block := range blocks {
		rows := len(block.Block) - 1
		structure := BlockStructure{
			structure: NewTriangleMatrix[SuperInstructionId](rows),
			frequency: block.frequency,
		}
		forEachSuperInstruction(block.Block, func(i, j int, si SuperInstruction) {
			id := index.Add(si)
			structure.structure.Set(rows-(j-i)+1, i, id)
			if len(frequency) <= int(id) {
				frequency = append(frequency, make([]uint64, int(id)-len(frequency)+1)...)
			}
			frequency[int(id)] += uint64(block.frequency)
		})
		structures = append(structures, structure)
	}
	return index, frequency, structures
}

func forEachSuperInstruction(block Block, op func(i, j int, si SuperInstruction)) {
	for i := 0; i <= len(block); i++ {
		if i == 0 && block[i] == vm.JUMPDEST {
			continue
		}
		for j := i + 2; j <= len(block); j++ {
			if j-i > MAX_SI_LENGTH {
				break
			}
			op(i, j, NewSuperInstruction(block[i:j]))
		}
	}
}
