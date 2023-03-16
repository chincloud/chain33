package executor

import (
	bty "github.com/33cn/chain33/plugin/dapp/basic/types"
	"github.com/33cn/chain33/types"
)

// Exec_Read Action
func (basic *Basic) Exec_Read(read *bty.Read, tx *types.Transaction, index int) (*types.Receipt, error) {
	receipt := &types.Receipt{Ty: types.ExecOk, KV: nil, Logs: nil}
	stateDB := basic.GetStateDB()
	for _, key := range read.Reads {
		prefix := basic.Key(key)
		value, err := stateDB.Get(prefix)
		if err != nil {
			continue
		}
		data := bty.Data{}
		types.Decode(value, &data)
	}
	return receipt, nil
}

// Exec_Update Action
func (basic *Basic) Exec_Update(update *bty.Update, tx *types.Transaction, index int) (*types.Receipt, error) {
	receipt := &types.Receipt{Ty: types.ExecOk, KV: nil, Logs: nil}
	for _, key := range update.Writes {
		prefix := basic.Key(key)
		data := bty.Data{Value: 5}

		basicKV := &types.KeyValue{Key: prefix, Value: types.Encode(&data)}
		receipt.KV = append(receipt.KV, basicKV)
	}
	return receipt, nil
}

// Exec_ReadModifyWrite Action
func (basic *Basic) Exec_ReadModifyWrite(update *bty.ReadModifyWrite, tx *types.Transaction, index int) (*types.Receipt, error) {
	receipt := &types.Receipt{Ty: types.ExecOk, KV: nil, Logs: nil}
	stateDB := basic.GetStateDB()
	for _, key := range update.Reads {
		prefix := basic.Key(key)
		value, err := stateDB.Get(prefix)
		if err != nil {
			continue
		}
		data := bty.Data{}
		types.Decode(value, &data)
	}
	for _, key := range update.Writes {
		prefix := basic.Key(key)
		data := bty.Data{}
		value, _ := stateDB.Get(prefix)
		types.Decode(value, &data)

		data.Value = data.Value + 5
		basicKV := &types.KeyValue{Key: prefix, Value: types.Encode(&data)}
		receipt.KV = append(receipt.KV, basicKV)
	}
	return receipt, nil
}
