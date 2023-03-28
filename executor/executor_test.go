// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package executor

import (
	"bufio"
	"encoding/hex"
	"fmt"
	log "github.com/33cn/chain33/common/log/log15"
	"github.com/33cn/chain33/common/merkle"
	basic "github.com/33cn/chain33/plugin/dapp/basic/executor"
	bty "github.com/33cn/chain33/plugin/dapp/basic/types"
	_ "github.com/33cn/chain33/plugin/dapp/init"
	cty "github.com/33cn/chain33/system/dapp/coins/types"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/33cn/chain33/client"
	"github.com/33cn/chain33/client/api"
	"github.com/33cn/chain33/common/address"
	"github.com/33cn/chain33/queue"
	"github.com/33cn/chain33/store"
	_ "github.com/33cn/chain33/system"
	"github.com/33cn/chain33/system/dapp"
	drivers "github.com/33cn/chain33/system/dapp"
	"github.com/33cn/chain33/types"
	"github.com/33cn/chain33/util"
	"github.com/stretchr/testify/assert"
)

var (
	chLog               = log.New("module", "blockchain")
	zeroHash            [32]byte
	lastBlock           *types.Block
	blocks              []*WaitConnectBlock
	txs                 []TxInfo
	blockExecutedTimes  []time.Time
	random              *rand.Rand
	transactions        []*types.Transaction
	txSize              = 1000
	bt                  time.Time
	assistDataMap       = make(map[int]*types.AssistData)
	stateLastVersionMap = make(map[string]int)
	schedulerQueueSize  = 4
	count               int
	size                int
)

type TxInfo struct {
	To     string
	Reads  []string
	Writes []string
	Type   int
}

type WaitConnectBlock struct {
	StateRoot chan []byte
	Detail    *types.BlockDetail
}

func initEnv(cfgstring string) (*Executor, queue.Queue) {
	random = rand.New(rand.NewSource(types.Now().UnixNano()))
	cfg := types.NewChain33Config(cfgstring)
	q := queue.New("channel")
	q.SetConfig(cfg)
	exec := New(cfg)
	exec.client = q.Client()
	exec.qclient, _ = client.New(exec.client, nil)
	return exec, q
}

func TestIsModule(t *testing.T) {
	var qmodule queue.Module = &Executor{}
	assert.NotNil(t, qmodule)
}

func TestExecutorGetTxGroup(t *testing.T) {
	exec, _ := initEnv(types.GetDefaultCfgstring())
	cfg := exec.client.GetConfig()
	execInit(nil)
	var txs []*types.Transaction
	addr2, priv2 := util.Genaddress()
	addr3, priv3 := util.Genaddress()
	addr4, _ := util.Genaddress()
	genkey := util.TestPrivkeyList[0]
	txs = append(txs, util.CreateCoinsTx(cfg, genkey, addr2, types.DefaultCoinPrecision))
	txs = append(txs, util.CreateCoinsTx(cfg, priv2, addr3, types.DefaultCoinPrecision))
	txs = append(txs, util.CreateCoinsTx(cfg, priv3, addr4, types.DefaultCoinPrecision))
	//执行三笔交易: 全部正确
	txgroup, err := types.CreateTxGroup(txs, cfg.GetMinTxFeeRate())
	if err != nil {
		t.Error(err)
		return
	}
	//重新签名
	txgroup.SignN(0, types.SECP256K1, genkey)
	txgroup.SignN(1, types.SECP256K1, priv2)
	txgroup.SignN(2, types.SECP256K1, priv3)
	txs = txgroup.GetTxs()
	ctx := &executorCtx{
		stateHash:  nil,
		height:     1,
		blocktime:  time.Now().Unix(),
		difficulty: 1,
		mainHash:   nil,
		parentHash: nil,
	}
	execute := newExecutor(ctx, exec, nil, txs, nil)
	e := execute.loadDriver(txs[0], 0)
	execute.setEnv(e)
	txs2 := e.GetTxs()
	assert.Equal(t, txs2, txgroup.GetTxs())
	for i := 0; i < len(txs); i++ {
		txg, err := e.GetTxGroup(i)
		assert.Nil(t, err)
		assert.Equal(t, txg, txgroup.GetTxs())
	}
	_, err = e.GetTxGroup(len(txs))
	assert.Equal(t, err, types.ErrTxGroupIndex)

	//err tx group list
	txs[0].Header = nil
	execute = newExecutor(ctx, exec, nil, txs, nil)
	e = execute.loadDriver(txs[0], 0)
	execute.setEnv(e)
	_, err = e.GetTxGroup(len(txs) - 1)
	assert.Equal(t, err, types.ErrTxGroupFormat)
}

//gen 1万币需要 2s，主要是签名的花费
func BenchmarkGenRandBlock(b *testing.B) {
	exec, _ := initEnv(types.GetDefaultCfgstring())
	cfg := exec.client.GetConfig()
	_, key := util.Genaddress()
	for i := 0; i < b.N; i++ {
		util.CreateNoneBlock(cfg, key, 10000)
	}
}

func TestLoadDriver(t *testing.T) {
	d, err := drivers.LoadDriver("none", 0)
	if err != nil {
		t.Error(err)
	}

	if d.GetName() != "none" {
		t.Error(d.GetName())
	}
}

func TestKeyAllow(t *testing.T) {
	exect, _ := initEnv(types.GetDefaultCfgstring())
	execInit(nil)
	key := []byte("mavl-coins-bty-exec-1wvmD6RNHzwhY4eN75WnM6JcaAvNQ4nHx:19xXg1WHzti5hzBRTUphkM8YmuX6jJkoAA")
	exec := []byte("retrieve")
	tx1 := "0a05636f696e73120e18010a0a1080c2d72f1a036f746520a08d0630f1cdebc8f7efa5e9283a22313271796f6361794e46374c7636433971573461767873324537553431664b536676"
	tx11, _ := hex.DecodeString(tx1)
	var tx12 types.Transaction
	types.Decode(tx11, &tx12)
	tx12.Execer = exec
	ctx := &executorCtx{
		stateHash:  nil,
		height:     1,
		blocktime:  time.Now().Unix(),
		difficulty: 1,
		mainHash:   nil,
		parentHash: nil,
	}
	execute := newExecutor(ctx, exect, nil, nil, nil)
	if !isAllowKeyWrite(execute, key, exec, &tx12, 0) {
		t.Error("retrieve can modify exec")
	}
}

func TestKeyAllow_evm(t *testing.T) {
	exect, _ := initEnv(types.GetDefaultCfgstring())
	execInit(nil)
	key := []byte("mavl-coins-bty-exec-1GacM93StrZveMrPjXDoz5TxajKa9LM5HG:19EJVYexvSn1kZ6MWiKcW14daXsPpdVDuF")
	exec := []byte("user.evm.0xc79c9113a71c0a4244e20f0780e7c13552f40ee30b05998a38edb08fe617aaa5")
	tx1 := "0a05636f696e73120e18010a0a1080c2d72f1a036f746520a08d0630f1cdebc8f7efa5e9283a22313271796f6361794e46374c7636433971573461767873324537553431664b536676"
	tx11, _ := hex.DecodeString(tx1)
	var tx12 types.Transaction
	types.Decode(tx11, &tx12)
	tx12.Execer = exec
	ctx := &executorCtx{
		stateHash:  nil,
		height:     1,
		blocktime:  time.Now().Unix(),
		difficulty: 1,
		mainHash:   nil,
		parentHash: nil,
	}
	execute := newExecutor(ctx, exect, nil, nil, nil)
	if !isAllowKeyWrite(execute, key, exec, &tx12, 0) {
		t.Error("user.evm.hash can modify exec")
	}
	//assert.Nil(t, t)
}

func TestKeyLocalAllow(t *testing.T) {
	exec, _ := initEnv(types.GetDefaultCfgstring())
	cfg := exec.client.GetConfig()
	err := isAllowLocalKey(cfg, []byte("token"), []byte("LODB-token-"))
	assert.Equal(t, err, types.ErrLocalKeyLen)
	err = isAllowLocalKey(cfg, []byte("token"), []byte("LODB_token-a"))
	assert.Equal(t, err, types.ErrLocalPrefix)
	err = isAllowLocalKey(cfg, []byte("token"), []byte("LODB-token-a"))
	assert.Nil(t, err)
	err = isAllowLocalKey(cfg, []byte(""), []byte("LODB--a"))
	assert.Equal(t, err, types.ErrLocalPrefix)
	err = isAllowLocalKey(cfg, []byte("exec"), []byte("LODB-execaa"))
	assert.Equal(t, err, types.ErrLocalPrefix)
	err = isAllowLocalKey(cfg, []byte("exec"), []byte("-exec------aa"))
	assert.Equal(t, err, types.ErrLocalPrefix)
	err = isAllowLocalKey(cfg, []byte("paracross"), []byte("LODB-user.p.para.paracross-xxxx"))
	assert.Equal(t, err, types.ErrLocalPrefix)
	err = isAllowLocalKey(cfg, []byte("user.p.para.paracross"), []byte("LODB-user.p.para.paracross-xxxx"))
	assert.Nil(t, err)
	err = isAllowLocalKey(cfg, []byte("user.p.para.user.wasm.abc"), []byte("LODB-user.p.para.user.wasm.abc-xxxx"))
	assert.Nil(t, err)
	err = isAllowLocalKey(cfg, []byte("user.p.para.paracross"), []byte("LODB-paracross-xxxx"))
	assert.Nil(t, err)
}

func init() {
	types.AllowUserExec = append(types.AllowUserExec, []byte("demo"), []byte("demof"))
}

var testRegOnce sync.Once

func Register(cfg *types.Chain33Config) {
	testRegOnce.Do(func() {
		drivers.Register(cfg, "demo", newdemoApp, 1)
		drivers.Register(cfg, "demof", newdemofApp, 1)
	})
}

//ErrEnvAPI 测试
type demoApp struct {
	*drivers.DriverBase
}

func newdemoApp() drivers.Driver {
	demo := &demoApp{DriverBase: &drivers.DriverBase{}}
	demo.SetChild(demo)
	return demo
}

func (demo *demoApp) GetDriverName() string {
	return "demo"
}

func (demo *demoApp) Exec(tx *types.Transaction, index int) (receipt *types.Receipt, err error) {
	return nil, queue.ErrQueueTimeout
}

func (demo *demoApp) Upgrade() (*types.LocalDBSet, error) {
	db := demo.GetLocalDB()
	db.Set([]byte("LODB-demo-a"), []byte("t1"))
	db.Set([]byte("LODB-demo-b"), []byte("t2"))
	var kvset types.LocalDBSet
	kvset.KV = []*types.KeyValue{
		{Key: []byte("LODB-demo-a"), Value: []byte("t1")},
		{Key: []byte("LODB-demo-b"), Value: []byte("t2")},
	}
	return &kvset, nil
}

type demofApp struct {
	*drivers.DriverBase
}

func newdemofApp() drivers.Driver {
	demo := &demofApp{DriverBase: &drivers.DriverBase{}}
	demo.SetChild(demo)
	return demo
}

func (demo *demofApp) GetDriverName() string {
	return "demof"
}

func (demo *demofApp) Exec(tx *types.Transaction, index int) (receipt *types.Receipt, err error) {
	return nil, queue.ErrQueueTimeout
}

func (demo *demofApp) Upgrade() (kvset *types.LocalDBSet, err error) {
	return nil, types.ErrInvalidParam
}

func TestExecutorErrAPIEnv(t *testing.T) {
	exec, q := initEnv(types.GetDefaultCfgstring())
	types.AllowUserExec = append(types.AllowUserExec, []byte("demo"))
	exec.disableLocal = true
	cfg := exec.client.GetConfig()
	cfg.SetMinFee(0)
	Register(cfg)
	execInit(cfg)

	store := store.New(cfg)
	store.SetQueueClient(q.Client())
	defer store.Close()

	var txs []*types.Transaction
	genkey := util.TestPrivkeyList[0]
	txs = append(txs, util.CreateTxWithExecer(cfg, genkey, "demo"))
	txlist := &types.ExecTxList{
		StateHash:  nil,
		Height:     1,
		BlockTime:  time.Now().Unix(),
		Difficulty: 1,
		MainHash:   nil,
		MainHeight: 1,
		ParentHash: nil,
		Txs:        txs,
	}
	msg := queue.NewMessage(0, "", 1, txlist)
	exec.procExecTxList(msg)
	_, err := exec.client.WaitTimeout(msg, 100*time.Second)
	fmt.Println(err)
	assert.Equal(t, true, api.IsAPIEnvError(err))
}
func TestCheckTx(t *testing.T) {
	exec, q := initEnv(types.ReadFile("../cmd/chain33/chain33.test.toml"))
	cfg := exec.client.GetConfig()

	store := store.New(cfg)
	store.SetQueueClient(q.Client())
	defer store.Close()

	addr, priv := util.Genaddress()

	tx := util.CreateCoinsTx(cfg, priv, addr, types.DefaultCoinPrecision)
	tx.Execer = []byte("user.xxx")
	tx.To = address.ExecAddress("user.xxx")
	tx.Fee = 2 * types.DefaultCoinPrecision
	tx.Sign(types.SECP256K1, priv)

	var txs []*types.Transaction
	txs = append(txs, tx)
	ctx := &executorCtx{
		stateHash:  nil,
		height:     0,
		blocktime:  time.Now().Unix(),
		difficulty: 1,
		mainHash:   nil,
		parentHash: nil,
	}
	execute := newExecutor(ctx, exec, nil, txs, nil)
	err := execute.execCheckTx(tx, 0)
	assert.Equal(t, err, types.ErrNoBalance)
}

func TestExecutorUpgradeMsg(t *testing.T) {
	exec, q := initEnv(types.GetDefaultCfgstring())
	cfg := exec.client.GetConfig()
	cfg.SetMinFee(0)
	Register(cfg)
	execInit(cfg)

	exec.SetQueueClient(q.Client())
	client := q.Client()
	msg := client.NewMessage("execs", types.EventUpgrade, nil)
	err := client.Send(msg, true)
	assert.Nil(t, err)
}

func TestExecutorPluginUpgrade(t *testing.T) {
	exec, q := initEnv(types.GetDefaultCfgstring())
	cfg := exec.client.GetConfig()
	cfg.SetMinFee(0)
	Register(cfg)
	execInit(cfg)

	go func() {
		client := q.Client()
		client.Sub("blockchain")
		for msg := range client.Recv() {
			if msg.Ty == types.EventLocalNew {
				msg.Reply(client.NewMessage("", types.EventHeader, &types.Int64{Data: 100}))
			} else {
				msg.Reply(client.NewMessage("", types.EventHeader, &types.Header{Height: 100}))
			}
		}
	}()

	kvset, err := exec.upgradePlugin("demo")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(kvset.GetKV()))
	kvset, err = exec.upgradePlugin("demof")
	assert.NotNil(t, err)
	assert.Nil(t, kvset)
}

func TestBlockInterConcurrent(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU() / 4)
	exec, q := initEnv(types.GetDefaultCfgstring())
	exec.SetQueueClient(exec.client)
	cfg := exec.client.GetConfig()
	cfg.SetMinFee(0)
	initData("tx_0.3_ycsb_f_100000.data")
	generateAssistData()
	execInit(cfg)
	for j := 0; j < len(txs); j++ {
		name := fmt.Sprintf("user.basic.test%05d", j)
		//addr := dapp.ExecAddress(name)
		//fmt.Println(addr)
		basic.CopyBasic(name, cfg)
	}
	//basic.CopyBasic("basic", cfg)
	s := store.New(cfg)
	s.SetQueueClient(q.Client())

	client := q.Client()
	client.Sub("blockchain")

	wg := &sync.WaitGroup{}
	lastBlock = Genesis(cfg, client)
	blocks = append(blocks, &WaitConnectBlock{StateRoot: make(chan []byte, 1)})
	wg.Add(1)
	ExecTx(client, zeroHash[:], lastBlock)
	blockExecutedTimes = make([]time.Time, len(txs)/txSize+1)
	bt = time.Now()
	for i := 0; i < len(txs)/txSize; i++ {
		var newblock types.Block
		newblock.ParentHash = lastBlock.Hash(cfg)
		newblock.Height = lastBlock.Height + 1
		var preStateRoot []byte
		if lastBlock.StateHash != nil {
			preStateRoot = lastBlock.StateHash
		}
		createTransactionList(cfg, i)
		newblock.Txs = transactions

		//需要首先对交易进行排序
		if cfg.IsFork(newblock.Height, "ForkRootHash") {
			newblock.Txs = types.TransactionSort(newblock.Txs)
		}
		newblock.TxHash = merkle.CalcMerkleRoot(cfg, newblock.Height, newblock.Txs)
		newblock.BlockTime = types.Now().Unix()
		blocks = append(blocks, &WaitConnectBlock{StateRoot: make(chan []byte, 1)})
		wg.Add(1)
		ExecTx(client, preStateRoot, &newblock)
		if i > 1 {
			SendAssistData(client, assistDataMap[i+1])
		}
		lastBlock = &newblock
	}

	//time.Sleep(time.Duration(200) * time.Millisecond)
	//for i := 0; i < len(txs)/txSize; i++ {
	//	if i > 1 {
	//		SendAssistData(client, assistDataMap[i+1])
	//	}
	//}
	fmt.Println(size)

	go func() {
		for msg := range client.Recv() {
			if msg.Ty == types.EventBlockReceipts {
				executedBlock := msg.GetData().(*types.ExecutedBlock)
				go ConnectBlock(cfg, client, executedBlock, wg)
			} else if msg.Ty == types.EventLocalNew {
				//chainCfg := cfg.GetModuleConfig().BlockChain
				//tx := db.NewLocalDB(dbm.NewDB("blockchain", chainCfg.Driver, chainCfg.DbPath, chainCfg.DbCache), msg.GetData().(bool))
				//id := common.StorePointer(tx)
				msg.Reply(client.NewMessage("", types.EventLocalNew, &types.Int64{Data: 1}))
			}
		}
	}()
	wg.Wait()
	max := bt
	for _, tt := range blockExecutedTimes {
		if tt.After(max) {
			max = tt
		}
	}
	chLog.Info("InterBlockConcurrent", "ExecTime", max.Sub(bt))
}

// CreateGenesisTx get genesis tx
func CreateGenesisTx(cfg *types.Chain33Config) (ret []*types.Transaction) {
	var tx types.Transaction
	tx.Execer = []byte(cfg.GetCoinExec())
	var genesis string
	if cfg.GetModuleConfig().Consensus.Genesis != "" {
		genesis = cfg.GetModuleConfig().Consensus.Genesis
	}
	tx.To = genesis
	//gen payload
	g := &cty.CoinsAction_Genesis{}
	g.Genesis = &types.AssetsGenesis{}
	g.Genesis.Amount = 1e8 * cfg.GetCoinPrecision()
	tx.Payload = types.Encode(&cty.CoinsAction{Value: g, Ty: cty.CoinsActionGenesis})
	ret = append(ret, &tx)
	return
}

func Genesis(cfg *types.Chain33Config, client queue.Client) *types.Block {
	newblock := &types.Block{}
	newblock.Height = 0

	if cfg.GetModuleConfig().Consensus.GenesisBlockTime > 0 {
		newblock.BlockTime = cfg.GetModuleConfig().Consensus.GenesisBlockTime
	}
	// TODO: 下面这些值在创世区块中赋值nil，是否合理？
	newblock.ParentHash = zeroHash[:]
	tx := CreateGenesisTx(cfg)
	newblock.Txs = tx
	newblock.TxHash = merkle.CalcMerkleRoot(cfg, newblock.Height, newblock.Txs)
	if newblock.Height == 0 {
		types.AssertConfig(client)
		newblock.Difficulty = cfg.GetP(0).PowLimitBits
	}
	return newblock
}

func ConnectBlock(cfg *types.Chain33Config, client queue.Client, executedBlock *types.ExecutedBlock, wg *sync.WaitGroup) {

	defer wg.Done()

	block := executedBlock.Block
	height := executedBlock.Block.Height

	blockExecutedTimes[height] = time.Now()
	prevStateRoot := zeroHash[:]
	if height > 0 {
		prevStateRoot = <-blocks[height-1].StateRoot
	}

	//prevStateRoot := executedBlock.PrevStatusHash
	receipts := executedBlock.Receipts
	config := client.GetConfig()
	cacheTxs := types.TxsToCache(block.Txs)

	kvset := make([]*types.KeyValue, 0, len(receipts))
	rdata := make([]*types.ReceiptData, 0, len(receipts)) //save to db receipt log
	//删除无效的交易
	var deltxs []*types.Transaction
	index := 0
	for i, receipt := range receipts {
		if receipt.Ty == types.ExecErr {
			errTx := block.Txs[i]
			//ulog.Error("exec tx err", "err", receipt, "txhash", common.ToHex(errTx.Hash()))
			deltxs = append(deltxs, errTx)
			continue
		}
		block.Txs[index] = block.Txs[i]
		cacheTxs[index] = cacheTxs[i]
		index++
		rdata = append(rdata, &types.ReceiptData{Ty: receipt.Ty, Logs: receipt.Logs})
		kvset = append(kvset, receipt.KV...)
	}
	block.Txs = block.Txs[:index]
	cacheTxs = cacheTxs[:index]

	//此时需要区分主链和平行链
	if config.IsPara() {
		height = block.MainHeight
	}

	var txHash []byte
	if !config.IsFork(height, "ForkRootHash") {
		txHash = merkle.CalcMerkleRootCache(cacheTxs)
	} else {
		txHash = merkle.CalcMerkleRoot(config, height, types.TransactionSort(block.Txs))
	}

	// 存在错误交易, 或共识生成区块时未设置TxHash, 需要重新赋值
	block.TxHash = txHash
	//ulog.Debug("PreExecBlock", "CalcMerkleRootCache", types.Since(beg))

	kvset = DelDupKey(kvset)
	stateHash, err := ExecKVMemSet(client, prevStateRoot, block.Height, kvset, false, false)
	if err != nil {
		return
	}

	block.StateHash = stateHash
	var detail types.BlockDetail
	detail.Block = block
	detail.Receipts = rdata

	if len(kvset) > 0 {
		detail.KV = kvset
	}
	detail.PrevStatusHash = prevStateRoot

	blocks[height].Detail = &detail
	blocks[height].StateRoot <- prevStateRoot
	chLog.Info("ProcAddBlockMsg", "height", detail.GetBlock().GetHeight(),
		"txCount", len(detail.GetBlock().GetTxs()))
	if height != 0 {
		detail.Block.ParentHash = blocks[height-1].Detail.Block.Hash(cfg)
	}
}

func ExecTx(client queue.Client, prevStateRoot []byte, block *types.Block) {
	list := &types.ExecTxList{
		StateHash:  prevStateRoot,
		ParentHash: block.ParentHash,
		MainHash:   block.MainHash,
		MainHeight: block.MainHeight,
		Txs:        block.Txs,
		BlockTime:  block.BlockTime,
		Height:     block.Height,
		Difficulty: uint64(block.Difficulty),
		IsMempool:  false,
	}
	msg := client.NewMessage("execs", types.EventInterBlockExecTxList, list)
	client.Send(msg, false)
}

func SendAssistData(client queue.Client, data *types.AssistData) {
	size += len(types.Encode(data))
	msg := client.NewMessage("execs", types.EventAssistData, data)
	client.Send(msg, false)
}

//func PreExecBlock(client queue.Client, prevStateRoot []byte, block *types.Block) {
//	//发送执行交易给execs模块
//	//通过consensus module 再次检查
//
//	// exec tx routine
//	对区块的正确性保持乐观，交易查重和执行并行处理，提高效率
//	ExecTx(client, prevStateRoot, block)
//	//ulog.Info("PreExecBlock", "height", block.GetHeight(), "ExecTx", types.Since(beg))
//}

func DelDupKey(kvs []*types.KeyValue) []*types.KeyValue {
	dupindex := make(map[string]int)
	n := 0
	for _, kv := range kvs {
		if index, ok := dupindex[string(kv.Key)]; ok {
			//重复的key 替换老的key
			kvs[index] = kv
		} else {
			dupindex[string(kv.Key)] = n
			kvs[n] = kv
			n++
		}
	}
	return kvs[0:n]
}

//ExecKVMemSet : send kv values to memory store and set it in db
func ExecKVMemSet(client queue.Client, prevStateRoot []byte, height int64, kvset []*types.KeyValue, sync bool, upgrade bool) ([]byte, error) {
	set := &types.StoreSet{StateHash: prevStateRoot, KV: kvset, Height: height}
	setwithsync := &types.StoreSetWithSync{Storeset: set, Sync: sync, Upgrade: upgrade}

	msg := client.NewMessage("store", types.EventStoreMemSet, setwithsync)
	err := client.Send(msg, true)
	if err != nil {
		return nil, err
	}
	resp, err := client.Wait(msg)
	if err != nil {
		return nil, err
	}
	hash := resp.GetData().(*types.ReplyHash)
	return hash.GetHash(), nil
}

func generateAssistData() {
	for i := 0; i < len(txs)/txSize; i++ {
		reads := make(map[string]int)
		writes := make(map[string]int)
		keys := make(map[string]bool)
		data := &types.AssistData{
			StateMap: make(map[string]*types.AssistItem),
			Height:   int32(i + 1),
		}
		start := i * txSize
		for j := 0; j < txSize; j++ {
			prefix := fmt.Sprintf("mavl-basic-%s-", txs[start+j].To)
			updateMin(txs[start+j].Reads, reads, j, prefix)
			updateMin(txs[start+j].Writes, writes, j, prefix)
		}
		count = 0
		for j := 0; j < txSize; j++ {
			prefix := fmt.Sprintf("mavl-basic-%s-", txs[start+j].To)
			for _, key := range txs[start+j].Reads {
				keys[prefix+key] = true
				if _, ok := data.StateMap[prefix+key]; !ok {
					lastVersion := stateLastVersionMap[prefix+key]
					gap := i - lastVersion
					if gap >= schedulerQueueSize {
						gap = 0
					}
					data.StateMap[prefix+key] = &types.AssistItem{LastVersion: int32(gap)}
				}
			}
			for _, key := range txs[start+j].Writes {
				keys[prefix+key] = true
				if _, ok := data.StateMap[prefix+key]; !ok {
					lastVersion := stateLastVersionMap[prefix+key]
					gap := i - lastVersion
					if gap >= schedulerQueueSize {
						gap = 0
					}
					data.StateMap[prefix+key] = &types.AssistItem{LastVersion: int32(gap)}
				}
				stateLastVersionMap[prefix+key] = i
			}
			if conflict(j, txs[start+j].Writes, writes, prefix) {
				continue
			}
			if !conflict(j, txs[start+j].Writes, reads, prefix) || !conflict(j, txs[start+j].Reads, writes, prefix) {
				count++
			}
		}
		for key, _ := range keys {
			if k, ok := reads[key]; ok {
				data.StateMap[key].MinRead = int32(k)
			} else {
				data.StateMap[key].MinRead = -1
			}

			if k, ok := writes[key]; ok {
				data.StateMap[key].MinWrite = int32(k)
			} else {
				data.StateMap[key].MinWrite = -1
			}
		}
		assistDataMap[i+1] = data
	}
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
	SetTxType(tx)
	txs = append(txs, *tx)
}

func SetTxWritesOrReads(strs []string, tx *TxInfo) {
	if strs[1] == "Write" {
		for j := 2; j < len(strs); j++ {
			keys := strings.Split(strs[j], "_")
			n, _ := strconv.Atoi(keys[0])
			tx.To = fmt.Sprintf("user.basic.test%05d", n)
			tx.Writes = append(tx.Writes, keys[1])
		}
	} else if strs[1] == "Read" {
		for j := 2; j < len(strs); j++ {
			keys := strings.Split(strs[j], "_")
			n, _ := strconv.Atoi(keys[0])
			tx.To = fmt.Sprintf("user.basic.test%05d", n)
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

func createTransactionList(cfg *types.Chain33Config, count int) {

	var result []*types.Transaction

	for j := 0; j < txSize; j++ {
		//tx := &types.Transaction{}
		idx := count*txSize + j
		tx := &types.Transaction{Execer: []byte(txs[idx].To), Payload: types.Encode(CreateAction(idx)), Fee: 0}
		//tx.To = "1Q4NhureJxKNBf71d26B9J3fBQoQcfmez2"
		tx.To = dapp.ExecAddress(txs[idx].To)
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
	if tx.Type == 1 {
		val := &bty.BasicAction_Read{Read: &bty.Read{Reads: tx.Reads}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionRead}
	} else if tx.Type == 2 {
		val := &bty.BasicAction_Update{Update: &bty.Update{Writes: tx.Writes}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionUpdate}
	} else if tx.Type == 3 {
		val := &bty.BasicAction_ReadModifyWrite{ReadModifyWrite: &bty.ReadModifyWrite{Writes: tx.Writes, Reads: tx.Reads}}
		action = &bty.BasicAction{Value: val, Ty: bty.BasicActionReadModifyWrite}
	}
	return action
}

func updateMin(keys []string, records map[string]int, i int, prefix string) {
	for _, r := range keys {
		if idx, ok := records[prefix+r]; ok {
			if i < idx {
				records[prefix+r] = i
			}
		} else {
			records[prefix+r] = i
		}
	}
}

func conflict(idx int, keys []string, records map[string]int, prefix string) bool {
	for _, key := range keys {
		if min, okk := records[prefix+key]; okk {
			if idx > min {
				return true
			}
		}
	}
	return false
}
