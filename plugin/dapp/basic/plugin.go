package basic

import (
	"github.com/33cn/chain33/plugin/dapp/basic/executor"
	"github.com/33cn/chain33/plugin/dapp/basic/types"
	"github.com/33cn/chain33/pluginmgr"
)

func init() {
	pluginmgr.Register(&pluginmgr.PluginBase{
		Name:     types.BasicX,
		ExecName: executor.GetName(),
		Exec:     executor.Init,
		Cmd:      nil,
		RPC:      nil,
	})
}
