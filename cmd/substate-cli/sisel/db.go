package sisel

import (
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/core/vm"
	_ "github.com/mattn/go-sqlite3"
)

type BlockInfo struct {
	Block     Block
	frequency int64
}

func LoadBlocks(db_file string) ([]BlockInfo, error) {
	db, err := sql.Open("sqlite3", db_file)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	fmt.Printf("===========================================================\n")
	fmt.Printf("= WARNING - using block size limit of 100 for development =\n")
	fmt.Printf("===========================================================\n")
	row, err := db.Query("SELECT instructions, sum(frequency) FROM BasicBlockFrequency WHERE length(instructions) <= 100 GROUP BY instructions")
	//row, err := db.Query("SELECT instructions, sum(frequency) FROM BasicBlockFrequency GROUP BY instructions")
	if err != nil {
		return nil, err
	}
	defer row.Close()

	res := []BlockInfo{}
	for row.Next() {
		var instructions string
		var frequency int64
		err := row.Scan(&instructions, &frequency)
		if err != nil {
			return nil, err
		}

		code, err := hex.DecodeString(instructions)
		if err != nil {
			return nil, err
		}

		block := make([]vm.OpCode, 0, len(code))
		for _, opcode := range code {
			block = append(block, vm.OpCode(opcode))
		}

		res = append(res, BlockInfo{Block: block, frequency: frequency})
	}

	return res, nil
}
