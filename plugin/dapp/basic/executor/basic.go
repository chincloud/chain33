package executor

import (
	"fmt"
	log "github.com/33cn/chain33/common/log/log15"
	bty "github.com/33cn/chain33/plugin/dapp/basic/types"
	drivers "github.com/33cn/chain33/system/dapp"
	"github.com/33cn/chain33/types"
	"reflect"
	"strings"
)

var clog = log.New("module", "execs.basic")
var driverName = "basic"

// Init norm
func Init(name string, cfg *types.Chain33Config, sub []byte) {
	clog.Debug("register basic execer")
	drivers.Register(cfg, GetName(), newBasic, cfg.GetDappFork(driverName, "Enable"))
	InitExecType()
}

//InitExecType ...
func InitExecType() {
	ety := types.LoadExecutorType(driverName)
	ety.InitFuncList(types.ListMethod(&Basic{}))
}

func CopyBasic(name string, cfg *types.Chain33Config) {
	// 需要先 RegisterDappFork才可以Register dapp
	drivers.Register(cfg, name, newBasic, cfg.GetDappFork(driverName, "Enable"))
	InitExecType()
}

// GetName for norm
func GetName() string {
	return newBasic().GetName()
}

// Basic driver
type Basic struct {
	drivers.DriverBase
}

func newBasic() drivers.Driver {
	n := &Basic{}
	n.SetChild(n)
	n.SetIsFree(true)
	n.SetExecutorType(types.LoadExecutorType(driverName))
	return n
}

func (basic *Basic) GetTxWritesAndReads(tx *types.Transaction) (reads []string, writes []string, err error) {
	action, err := basic.GetExecutorType().DecodePayload(tx)
	var vty int32
	var v reflect.Value
	_, vty, v, err = types.GetActionValue(action, basic.GetFuncMap())
	if err != nil {
		return nil, nil, err
	}
	prefix := fmt.Sprintf("mavl-basic-%s-", basic.GetCurrentExecName())
	if vty == bty.BasicActionRead {
		val := v.Interface().(*bty.Read)
		for _, key := range val.Reads {
			reads = append(reads, prefix+key)
		}
	} else if vty == bty.BasicActionUpdate {
		val := v.Interface().(*bty.Update)
		for _, key := range val.Writes {
			writes = append(writes, prefix+key)
		}
	} else if vty == bty.BasicActionReadModifyWrite {
		val := v.Interface().(*bty.ReadModifyWrite)
		for _, key := range val.Reads {
			reads = append(reads, prefix+key)
		}
		for _, key := range val.Writes {
			writes = append(writes, prefix+key)
		}
	}
	return
}

func (basic *Basic) Allow(tx *types.Transaction, index int) error {
	if len(tx.Execer) == len([]byte(driverName)) && basic.AllowIsSame(tx.Execer) {
		return nil
	}
	if basic.AllowIsUserDot2(tx.Execer) {
		return nil
	}
	return types.ErrNotAllow
}

// GetDriverName for basic
func (basic *Basic) GetDriverName() string {
	return driverName
}

// CheckTx for basic
func (basic *Basic) CheckTx(tx *types.Transaction, index int) error {
	return nil
}

// Key for basic
func (basic *Basic) Key(str string) (key []byte) {
	execName := basic.GetCurrentExecName()
	key = append(key, []byte(fmt.Sprintf("mavl-basic-%s-", execName))...)
	key = append(key, []byte(str)...)
	return key
}

// CheckReceiptExecOk return true to check if receipt ty is ok
func (basic *Basic) CheckReceiptExecOk() bool {
	return true
}

// IsFriend defines a isfriend function
func (basic *Basic) IsFriend(myexec, writekey []byte, othertx *types.Transaction) bool {
	//step1 先判定自己合约的权限
	if !basic.AllowIsSame(myexec) {
		return false
	}
	if strings.HasPrefix(string(writekey), "mavl-basic-user.basic.test") {
		return true
	}
	return false
}

//ExecutorOrder 执行顺序, 如果要使用 ExecLocalSameTime
//那么会同时执行 ExecLocal
//func (basic *Basic) ExecutorOrder() int64 {
//	return drivers.ExecLocalSameTime
//}
