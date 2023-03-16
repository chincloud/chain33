// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pbft

import (
	"bufio"
	"flag"
	"fmt"
	basic "github.com/33cn/chain33/plugin/dapp/basic/executor"
	"github.com/33cn/chain33/system/dapp"
	"io"
	"strings"

	//cty "github.com/33cn/chain33/system/dapp/coins/types"
	bty "github.com/33cn/chain33/plugin/dapp/basic/types"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/33cn/chain33/blockchain"
	"github.com/33cn/chain33/common"
	"github.com/33cn/chain33/common/crypto"
	"github.com/33cn/chain33/common/limits"
	"github.com/33cn/chain33/common/log"
	"github.com/33cn/chain33/executor"
	"github.com/33cn/chain33/p2p"
	"github.com/33cn/chain33/queue"
	"github.com/33cn/chain33/store"
	//coins "github.com/33cn/chain33/system/dapp/coins/executor"
	"github.com/33cn/chain33/types"
	"github.com/33cn/chain33/wallet"

	_ "github.com/33cn/chain33/plugin/dapp/init"
	_ "github.com/33cn/chain33/plugin/store/init"
	_ "github.com/33cn/chain33/system"
)

type TxInfo struct {
	To     string
	Reads  []string
	Writes []string
	Type   int
}

var (
	random       *rand.Rand
	transactions []*types.Transaction
	txSize       = 1000
	txs          []TxInfo
)

func init() {
	err := limits.SetLimits()
	if err != nil {
		panic(err)
	}
	random = rand.New(rand.NewSource(types.Now().UnixNano()))
	log.SetLogLevel("info")
}
func TestPbft(t *testing.T) {
	q, chain, p2pnet, s, exec, cs, wallet := initEnvPbft()
	defer chain.Close()
	defer p2pnet.Close()
	defer exec.Close()
	defer s.Close()
	defer cs.Close()
	defer q.Close()
	defer wallet.Close()
	time.Sleep(5 * time.Second)

	initData("./tx_zipf_0.7_ycsb_f.data")
	sendReplyList(q)
	clearTestData()
}

func initData(fileName string) {
	var file *os.File
	var err error
	if file, err = os.Open(fileName); err != nil {
		fmt.Println("打开文件错误：", err)
		return
	}
	defer file.Close()

	r := bufio.NewReader(file)

	txs = make([]TxInfo, 0)
	index := 0
	var tx *TxInfo
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			continue
		}
		if err == io.EOF {
			break
		}
		line = line[:len(line)-1]

		strs := strings.Split(line, " ")
		num, _ := strconv.Atoi(strs[0])
		if num > index {
			if tx != nil {
				SetTxType(tx)
				txs = append(txs, *tx)
			}
			tx = &TxInfo{}
			SetTxWritesOrReads(strs, tx)
			index = num
		} else if index == num {
			SetTxWritesOrReads(strs, tx)
		} else {
			SetTxType(tx)
			txs = append(txs, *tx)

			tx = &TxInfo{}
			SetTxWritesOrReads(strs, tx)
			index = 1
		}
	}
	txs = append(txs, *tx)
}

func SetTxWritesOrReads(strs []string, tx *TxInfo) {
	if strs[1] == "Write" {
		for j := 2; j < len(strs); j++ {
			keys := strings.Split(strs[j], "_")
			n, _ := strconv.Atoi(keys[0])
			tx.To = fmt.Sprintf("user.basic.%03d", n)
			tx.Writes = append(tx.Writes, keys[1])
		}
	} else if strs[1] == "Read" {
		for j := 2; j < len(strs); j++ {
			keys := strings.Split(strs[j], "_")
			n, _ := strconv.Atoi(keys[0])
			tx.To = fmt.Sprintf("user.basic.test%03d", n)
			tx.Reads = append(tx.Reads, keys[1])
		}
	}
}

func SetTxType(tx *TxInfo) {
	flag := 0
	if len(tx.Reads) != 0 {
		flag += 1
	}
	if len(tx.Writes) != 0 {
		flag += 2
	}
	tx.Type = flag
}

func initEnvPbft() (queue.Queue, *blockchain.BlockChain, *p2p.Manager, queue.Module, *executor.Executor, queue.Module, queue.Module) {
	flag.Parse()
	chain33Cfg := types.NewChain33Config(types.ReadFile("chain33.test.toml"))
	var q = queue.New("channel")
	q.SetConfig(chain33Cfg)
	cfg := chain33Cfg.GetModuleConfig()
	cfg.Log.LogFile = ""
	sub := chain33Cfg.GetSubConfig()

	chain := blockchain.New(chain33Cfg)
	chain.SetQueueClient(q.Client())
	exec := executor.New(chain33Cfg)
	exec.SetQueueClient(q.Client())
	chain33Cfg.SetMinFee(0)
	s := store.New(chain33Cfg)
	s.SetQueueClient(q.Client())
	cs := NewPbft(cfg.Consensus, sub.Consensus["pbft"])
	cs.SetQueueClient(q.Client())
	p2pnet := p2p.NewP2PMgr(chain33Cfg)
	p2pnet.SetQueueClient(q.Client())
	walletm := wallet.New(chain33Cfg)
	walletm.SetQueueClient(q.Client())

	for j := 0; j < txSize; j++ {
		name := fmt.Sprintf("user.basic.test%03d", j)
		//addr := dapp.ExecAddress(name)
		//fmt.Println(addr)
		basic.CopyBasic(name, chain33Cfg)
	}

	return q, chain, p2pnet, s, exec, cs, walletm

}

func sendReplyList(q queue.Queue) {
	client := q.Client()
	client.Sub("mempool")
	var count int
	for msg := range client.Recv() {
		if msg.Ty == types.EventTxList {
			count++
			createReplyList(client.GetConfig(), count-1)
			msg.Reply(client.NewMessage("consensus", types.EventReplyTxList,
				&types.ReplyTxList{Txs: transactions}))
			if count == len(txs)/txSize {
				time.Sleep(5 * time.Second)
				break
			}
		} else if msg.Ty == types.EventGetMempoolSize {
			msg.Reply(client.NewMessage("", 0, &types.MempoolSize{}))
		}
	}
}

func getprivkey(key string) crypto.PrivKey {
	cr, err := crypto.Load(types.GetSignName("", types.SECP256K1), -1)
	if err != nil {
		panic(err)
	}
	bkey, err := common.FromHex(key)
	if err != nil {
		panic(err)
	}
	priv, err := cr.PrivKeyFromBytes(bkey)
	if err != nil {
		panic(err)
	}
	return priv
}

func createReplyList(cfg *types.Chain33Config, count int) {

	var result []*types.Transaction
	//for j := 0; j < txSize; j++ {
	//	//tx := &types.Transaction{}
	//	val := &cty.CoinsAction_Transfer{Transfer: &types.AssetsTransfer{Amount: 10}}
	//	action := &cty.CoinsAction{Value: val, Ty: cty.CoinsActionTransfer}
	//	tx := &types.Transaction{Execer: []byte("user.coins.tokenxx"), Payload: types.Encode(action), Fee: 0}
	//	tx.To = "14qViLJfdGaP4EeHnDyJbEGQysnCpwn1gZ"
	//
	//	tx.Nonce = random.Int63()
	//	tx.ChainID = cfg.GetChainID()
	//
	//	tx.Sign(types.SECP256K1, getprivkey("CC38546E9E659D15E6B4893F0AB32A06D103931A8230B0BDE71459D2B27D6944"))
	//	result = append(result, tx)
	//}
	//keys := []string{"test01", "test02", "test03", "test04", "test05", "test06", "test07", "test08", "test09", "test10"}
	for j := 0; j < txSize; j++ {
		//tx := &types.Transaction{}
		idx := count*txSize + j
		tx := &types.Transaction{Execer: []byte(txs[j].To), Payload: types.Encode(CreateAction(idx)), Fee: 0}
		//tx.To = "1Q4NhureJxKNBf71d26B9J3fBQoQcfmez2"
		tx.To = dapp.ExecAddress(txs[j].To)
		tx.Nonce = random.Int63()
		tx.ChainID = cfg.GetChainID()

		//tx.Sign(types.SECP256K1, getprivkey("CC38546E9E659D15E6B4893F0AB32A06D103931A8230B0BDE71459D2B27D6944"))
		result = append(result, tx)
	}
	//result = append(result, tx)
	transactions = result
}

func CreateAction(idx int) *bty.BasicAction {
	tx := txs[idx]
	var action *bty.BasicAction
	if tx.Type == 0 {
		val := &bty.BasicAction_Read{Read: &bty.Read{Reads: tx.Reads}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionRead}
	} else if tx.Type == 1 {
		val := &bty.BasicAction_Update{Update: &bty.Update{Writes: tx.Writes}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionUpdate}
	} else if tx.Type == 3 {
		val := &bty.BasicAction_ReadModifyWrite{ReadModifyWrite: &bty.ReadModifyWrite{Writes: tx.Writes, Reads: tx.Reads}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionReadModifyWrite}
	}
	return action
}

func clearTestData() {
	err := os.RemoveAll("datadir")
	if err != nil {
		fmt.Println("delete datadir have a err:", err.Error())
	}
	err = os.RemoveAll("wallet")
	if err != nil {
		fmt.Println("delete wallet have a err:", err.Error())
	}
	fmt.Println("test data clear successfully!")
}
