package plugin

import (
	_ "github.com/33cn/chain33/plugin/consensus/init" //consensus init
	_ "github.com/33cn/chain33/plugin/crypto/init"    //crypto init
	_ "github.com/33cn/chain33/plugin/dapp/init"      //dapp init
	_ "github.com/33cn/chain33/plugin/p2p/init"       //p2p init
	_ "github.com/33cn/chain33/plugin/store/init"     //store init
)
