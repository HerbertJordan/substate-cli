package sisel

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/vm"
)

// SuperInstruction represents a single (super) instruction with value semantic.
type SuperInstruction struct {
	// this is not really a printable string, but a comparable byte
	// array so that it has value semantic.
	code string
}

func NewSuperInstruction(code []vm.OpCode) SuperInstruction {
	ops := make([]byte, len(code))
	for i, op := range code {
		ops[i] = byte(op)
	}
	return SuperInstruction{code: string(ops)}
}

func (i *SuperInstruction) Size() int {
	return len(i.code)
}

func (i *SuperInstruction) At(pos int) vm.OpCode {
	return vm.OpCode(i.code[pos])
}

func (si SuperInstruction) String() string {
	if len(si.code) == 0 {
		return "<empty>"
	}
	res := ""
	for i := 0; i < si.Size(); i++ {
		if i > 0 {
			res += "_"
		}
		res += fmt.Sprintf("%v", si.At(i))
	}
	return res
}
