package executor

import (
	"fmt"
	log "github.com/33cn/chain33/common/log/log15"
	drivers "github.com/33cn/chain33/system/dapp"
	"github.com/33cn/chain33/types"
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

//ExecutorOrder 执行顺序, 如果要使用 ExecLocalSameTime
//那么会同时执行 ExecLocal
func (basic *Basic) ExecutorOrder() int64 {
	return drivers.ExecLocalSameTime
}
