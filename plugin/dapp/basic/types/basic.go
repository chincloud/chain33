package types

import "github.com/33cn/chain33/types"

//BasicX name
var BasicX = "basic"

func init() {
	types.AllowUserExec = append(types.AllowUserExec, []byte(BasicX))
	types.RegFork(BasicX, InitFork)
	types.RegExec(BasicX, InitExecutor)
}

//InitFork ...
func InitFork(cfg *types.Chain33Config) {
	cfg.RegisterDappFork(BasicX, "Enable", 0)
}

//InitExecutor ...
func InitExecutor(cfg *types.Chain33Config) {
	types.RegistorExecutor(BasicX, NewType(cfg))
}

// BasicType def
type BasicType struct {
	types.ExecTypeBase
}

// NewType method
func NewType(cfg *types.Chain33Config) *BasicType {
	c := &BasicType{}
	c.SetChild(c)
	c.SetConfig(cfg)
	return c
}

// GetName 获取执行器名称
func (basic *BasicType) GetName() string {
	return BasicX
}

// GetPayload method
func (basic *BasicType) GetPayload() types.Message {
	return &BasicAction{}
}

// GetTypeMap method
func (basic *BasicType) GetTypeMap() map[string]int32 {
	return map[string]int32{
		"Read":            BasicActionRead,
		"Update":          BasicActionUpdate,
		"ReadModifyWrite": BasicActionReadModifyWrite,
	}
}

// GetLogMap method
func (basic *BasicType) GetLogMap() map[int64]*types.LogInfo {
	return map[int64]*types.LogInfo{}
}
