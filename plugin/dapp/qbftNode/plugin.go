// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qbftNode

import (
	"github.com/33cn/chain33/plugin/dapp/qbftNode/commands"
	"github.com/33cn/chain33/plugin/dapp/qbftNode/executor"
	"github.com/33cn/chain33/plugin/dapp/qbftNode/rpc"
	"github.com/33cn/chain33/plugin/dapp/qbftNode/types"
	"github.com/33cn/chain33/pluginmgr"
)

func init() {
	pluginmgr.Register(&pluginmgr.PluginBase{
		Name:     types.QbftNodeX,
		ExecName: executor.GetName(),
		Exec:     executor.Init,
		Cmd:      commands.ValCmd,
		RPC:      rpc.Init,
	})
}
