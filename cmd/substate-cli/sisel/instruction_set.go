package sisel

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// InstructionSet represents a selected super instruction set. Sets
// are immutable and designed to be used as keys in maps. In particular,
// equivalent sets compare equal.
type InstructionSet struct {
	// The encoded sorted list of included SuperInstruction IDs
	encoded string
}

func MakeSingletonSet(si SuperInstructionId) InstructionSet {
	return InstructionSet{encoded: string([]byte{
		byte(si), byte(si >> 8), byte(si >> 16), byte(si >> 24),
	})}
}

func MakeSet(list []SuperInstructionId) InstructionSet {
	if len(list) == 0 {
		return InstructionSet{}
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	last := list[0] + 1

	buf := bytes.NewBuffer(make([]byte, 0, len(list)*4))
	for _, cur := range list {
		if cur != last {
			binary.Write(buf, binary.LittleEndian, uint32(cur))
		}
		last = cur
	}
	return InstructionSet{buf.String()}
}

func (s *InstructionSet) Size() int {
	return len(s.encoded) / 4
}

func (s *InstructionSet) Empty() bool {
	return len(s.encoded) == 0
}

func (s *InstructionSet) At(pos int) SuperInstructionId {
	return SuperInstructionId(uint32(s.encoded[4*pos]) |
		uint32(s.encoded[4*pos+1])<<8 |
		uint32(s.encoded[4*pos+2])<<16 |
		uint32(s.encoded[4*pos+3])<<24)
}

func (s *InstructionSet) Contains(si SuperInstructionId) bool {
	for i := 0; i < s.Size(); i++ {
		if s.At(i) == si {
			return true
		}
	}
	return false
}

func (a *InstructionSet) ContainsAll(b InstructionSet) bool {
	if b.Empty() {
		return true
	}
	if a.Empty() {
		return false
	}

	// Use intersection algorithm to determine that all elements are present.
	i, j := 0, 0
	for i < a.Size() && j < b.Size() {
		next_a := a.At(i)
		next_b := b.At(j)
		if next_a < next_b {
			i++
			continue
		}
		if next_b < next_a {
			return false
		}
		i++
		j++
	}

	return j >= b.Size()
}

func (s *InstructionSet) Add(si SuperInstructionId) InstructionSet {
	if s.Contains(si) {
		return *s
	}
	return Union(*s, MakeSingletonSet(si))
}

func (s *InstructionSet) Remove(si SuperInstructionId) InstructionSet {
	if !s.Contains(si) {
		return *s
	}
	return Difference(*s, MakeSingletonSet(si))
}

func (s *InstructionSet) GetSubsets() []InstructionSet {
	num := 1 << s.Size()
	res := make([]InstructionSet, num)
	for i := 0; i < num; i++ {
		cur := InstructionSet{}
		for j := 0; j < s.Size(); j++ {
			if i>>j&0x1 > 0 {
				cur = cur.Add(s.At(j))
			}
		}
		res[i] = cur
	}
	return res
}

func Union(a, b InstructionSet) InstructionSet {
	if a.Empty() {
		return b
	}
	if b.Empty() {
		return a
	}
	buf := bytes.NewBuffer(make([]byte, 0, (a.Size()+b.Size())*4))

	// Perform a merge of a sorted list
	i, j := 0, 0
	for i < a.Size() && j < b.Size() {
		next_a := a.At(i)
		next_b := b.At(j)
		if next_a < next_b {
			binary.Write(buf, binary.LittleEndian, next_a)
			i++
		} else if next_a == next_b {
			binary.Write(buf, binary.LittleEndian, next_a)
			i++
			j++
		} else {
			binary.Write(buf, binary.LittleEndian, next_b)
			j++
		}
	}

	// Append rest of a
	for ; i < a.Size(); i++ {
		binary.Write(buf, binary.LittleEndian, a.At(i))
	}

	// Append rest of b
	for ; j < b.Size(); j++ {
		binary.Write(buf, binary.LittleEndian, b.At(j))
	}

	return InstructionSet{buf.String()}
}

func Intersect(a, b InstructionSet) InstructionSet {
	if a.Empty() {
		return a
	}
	if b.Empty() {
		return b
	}
	buf := bytes.NewBuffer(make([]byte, 0, (a.Size()+b.Size())*4))

	// Perform a intersection of a sorted list
	for i, j := 0, 0; i < a.Size() && j < b.Size(); {
		next_a := a.At(i)
		next_b := b.At(j)
		if next_a < next_b {
			i++
			continue
		}
		if next_b < next_a {
			j++
			continue
		}
		binary.Write(buf, binary.LittleEndian, next_a)
		i++
		j++
	}

	return InstructionSet{buf.String()}
}

func Difference(a, b InstructionSet) InstructionSet {
	if a.Empty() {
		return a
	}
	if b.Empty() {
		return a
	}
	buf := bytes.NewBuffer(make([]byte, 0, (a.Size()+b.Size())*4))

	i, j := 0, 0
	for i < a.Size() && j < b.Size() {
		next_a := a.At(i)
		next_b := b.At(j)
		if next_a < next_b {
			binary.Write(buf, binary.LittleEndian, next_a)
			i++
			continue
		}
		if next_b < next_a {
			j++
			continue
		}
		i++
		j++
	}

	// Append rest of a
	for ; i < a.Size(); i++ {
		binary.Write(buf, binary.LittleEndian, a.At(i))
	}

	return InstructionSet{buf.String()}
}

func (s *InstructionSet) AsMap() map[SuperInstructionId]int {
	res, err := decodeInstructionSet(s.encoded)
	if err != nil {
		panic(fmt.Sprintf("instruction set contains invalid set encoding: %v", err))
	}
	return res
}

func (s InstructionSet) String() string {
	builder := strings.Builder{}
	builder.WriteString("{")
	for i := 0; i < s.Size(); i++ {
		if i > 0 {
			builder.WriteString(" ")
		}
		builder.WriteString(fmt.Sprintf("%d", s.At(i)))
	}
	builder.WriteString("}")
	return builder.String()
}

func (s *InstructionSet) Print(index *SuperInstructionIndex) {
	fmt.Printf("Instruction set:\n")
	if s.Empty() {
		fmt.Printf("  <no super instructions>\n")
	}
	for i := 0; i < s.Size(); i++ {
		fmt.Printf("  %v\n", index.Get(s.At(i)))
	}
}

func encodeInstructionSet(set map[SuperInstructionId]int) string {
	list := make([]SuperInstructionId, 0, len(set))
	for k := range set {
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	buf := bytes.NewBuffer(make([]byte, 0, len(set)*4))
	for _, si := range list {
		binary.Write(buf, binary.LittleEndian, uint32(si))
	}
	return buf.String()
}

func decodeInstructionSet(encoded string) (map[SuperInstructionId]int, error) {
	if len(encoded)%4 != 0 {
		return nil, fmt.Errorf("invalid encoded instruction set length: %d not multiple of 4", len(encoded))
	}
	set := InstructionSet{encoded: encoded}
	res := map[SuperInstructionId]int{}
	for i := 0; i < set.Size(); i++ {
		res[set.At(i)] = 0
	}
	return res, nil
}
