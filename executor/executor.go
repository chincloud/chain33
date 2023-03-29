// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package executor 实现执行器模块基类功能
package executor

//store package store the world - state data
import (
	"fmt"
	"github.com/33cn/chain33/client/api"
	dbm "github.com/33cn/chain33/common/db"
	clog "github.com/33cn/chain33/common/log"
	log "github.com/33cn/chain33/common/log/log15"
	basic "github.com/33cn/chain33/plugin/dapp/basic/executor"
	"github.com/33cn/chain33/pluginmgr"
	"github.com/33cn/chain33/rpc/grpcclient"
	drivers "github.com/33cn/chain33/system/dapp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// register drivers
	"github.com/33cn/chain33/client"
	"github.com/33cn/chain33/queue"
	"github.com/33cn/chain33/types"
	typ "github.com/33cn/chain33/types"
)

var elog = log.New("module", "execs")

// SetLogLevel set log level
func SetLogLevel(level string) {
	clog.SetLogLevel(level)
}

// DisableLog disable log
func DisableLog() {
	elog.SetHandler(log.DiscardHandler())
}

// Executor executor struct
type Executor struct {
	disableExecLocal bool
	disableLocal     bool
	client           queue.Client
	qclient          client.QueueProtocolAPI
	grpccli          types.Chain33Client
	pluginEnable     map[string]bool
	alias            map[string]string
	noneDriverPool   *sync.Pool
	scheduler        *scheduler
}

type RWSet struct {
	Reads  []string
	Writes []string
}

func execInit(cfg *typ.Chain33Config) {
	pluginmgr.InitExec(cfg)
}

var runonce sync.Once

// New new executor
func New(cfg *typ.Chain33Config) *Executor {
	// init executor
	runonce.Do(func() {
		execInit(cfg)
	})
	mcfg := cfg.GetModuleConfig().Exec
	exec := &Executor{}
	exec.pluginEnable = make(map[string]bool)
	exec.disableExecLocal = mcfg.DisableExecLocal
	exec.pluginEnable["stat"] = mcfg.EnableStat
	exec.pluginEnable["mvcc"] = mcfg.EnableMVCC
	exec.pluginEnable["addrindex"] = !mcfg.DisableAddrIndex
	exec.pluginEnable["txindex"] = !mcfg.DisableTxIndex
	exec.pluginEnable["fee"] = !mcfg.DisableFeeIndex
	exec.pluginEnable[addrFeeIndex] = mcfg.EnableAddrFeeIndex
	exec.noneDriverPool = &sync.Pool{
		New: func() interface{} {
			none, err := drivers.LoadDriver("none", 0)
			if err != nil {
				panic("load none driver err")
			}
			return none
		},
	}
	exec.alias = make(map[string]string)
	for _, v := range mcfg.Alias {
		data := strings.Split(v, ":")
		if len(data) != 2 {
			panic("exec.alias config error: " + v)
		}
		if _, ok := exec.alias[data[0]]; ok {
			panic("exec.alias repeat name: " + v)
		}
		if pluginmgr.HasExec(data[0]) {
			panic("exec.alias repeat name with system Exec: " + v)
		}
		exec.alias[data[0]] = data[1]
	}
	exec.scheduler = newScheduler(exec, 10)

	return exec
}

//Wait Executor ready
func (exec *Executor) Wait() {}

// SetQueueClient set client queue, for recv msg
func (exec *Executor) SetQueueClient(qcli queue.Client) {
	exec.client = qcli
	exec.client.Sub("execs")
	var err error
	exec.qclient, err = client.New(qcli, nil)
	if err != nil {
		panic(err)
	}
	types.AssertConfig(exec.client)
	cfg := exec.client.GetConfig()
	if cfg.IsPara() {
		exec.grpccli, err = grpcclient.NewMainChainClient(cfg, "")
		if err != nil {
			panic(err)
		}
	}

	//recv 消息的处理
	go func() {
		for msg := range exec.client.Recv() {
			elog.Debug("exec recv", "msg", msg)
			if msg.Ty == types.EventExecTxList {
				go exec.procExecTxList(msg)
			} else if msg.Ty == types.EventInterBlockExecTxList {
				go exec.scheduler.receiveMsg(msg)
			} else if msg.Ty == types.EventAssistData {
				go exec.scheduler.receiveAssistData(msg)
			} else if msg.Ty == types.EventAddBlock {
				go exec.procExecAddBlock(msg)
			} else if msg.Ty == types.EventDelBlock {
				go exec.procExecDelBlock(msg)
			} else if msg.Ty == types.EventCheckTx {
				go exec.procExecCheckTx(msg)
			} else if msg.Ty == types.EventBlockChainQuery {
				go exec.procExecQuery(msg)
			} else if msg.Ty == types.EventUpgrade {
				//执行升级过程中不允许执行其他的事件，这个事件直接不采用异步执行
				exec.procUpgrade(msg)
			}
		}
	}()
}

func (exec *Executor) procUpgrade(msg *queue.Message) {
	var kvset types.LocalDBSet
	for _, plugin := range pluginmgr.GetExecList() {
		elog.Info("begin upgrade plugin ", "name", plugin)
		kvset1, err := exec.upgradePlugin(plugin)
		if err != nil {
			msg.Reply(exec.client.NewMessage("", types.EventUpgrade, err))
			panic(err)
		}
		if kvset1 != nil && kvset1.KV != nil && len(kvset1.KV) > 0 {
			kvset.KV = append(kvset.KV, kvset1.KV...)
		}
	}
	elog.Info("upgrade plugin success")
	msg.Reply(exec.client.NewMessage("", types.EventUpgrade, &kvset))
}

func (exec *Executor) upgradePlugin(plugin string) (*types.LocalDBSet, error) {
	header, err := exec.qclient.GetLastHeader()
	if err != nil {
		return nil, err
	}
	driver, err := drivers.LoadDriverWithClient(exec.qclient, plugin, header.GetHeight())
	if err == types.ErrUnknowDriver { //已经注册插件，但是没有启动
		elog.Info("upgrade ignore ", "name", plugin)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var localdb dbm.KVDB
	if !exec.disableLocal {
		localdb = NewLocalDB(exec.client, exec.qclient, false)
		defer localdb.(*LocalDB).Close()
		driver.SetLocalDB(localdb)
	}
	//目前升级不允许访问statedb
	driver.SetStateDB(nil)
	driver.SetAPI(exec.qclient)
	driver.SetExecutorAPI(exec.qclient, exec.grpccli)
	driver.SetEnv(header.GetHeight(), header.GetBlockTime(), uint64(header.GetDifficulty()))
	localdb.Begin()
	kvset, err := driver.Upgrade()
	if err != nil {
		localdb.Rollback()
		return nil, err
	}
	localdb.Commit()
	return kvset, nil
}

func (exec *Executor) procExecQuery(msg *queue.Message) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("query panic error", "err", r)
			msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	header, err := exec.qclient.GetLastHeader()
	if err != nil {
		msg.Reply(exec.client.NewMessage("", types.EventBlockChainQuery, err))
		return
	}
	data := msg.GetData().(*types.ChainExecutor)
	driver, err := drivers.LoadDriverWithClient(exec.qclient, data.Driver, header.GetHeight())
	if err != nil {
		msg.Reply(exec.client.NewMessage("", types.EventBlockChainQuery, err))
		return
	}
	if data.StateHash == nil {
		data.StateHash = header.StateHash
	}
	var localdb dbm.KVDB
	if !exec.disableLocal {
		//query 只需要读取localdb
		localdb = NewLocalDB(exec.client, exec.qclient, true)
		defer localdb.(*LocalDB).Close()
		driver.SetLocalDB(localdb)
	}
	opt := &StateDBOption{EnableMVCC: exec.pluginEnable["mvcc"], Height: header.GetHeight()}

	db := NewStateDB(exec.client, data.StateHash, localdb, opt)
	db.(*StateDB).enableMVCC(nil)
	driver.SetStateDB(db)
	driver.SetAPI(exec.qclient)
	driver.SetExecutorAPI(exec.qclient, exec.grpccli)
	driver.SetEnv(header.GetHeight(), header.GetBlockTime(), uint64(header.GetDifficulty()))
	//查询的情况下下，执行器不做严格校验，allow，尽可能的加载执行器，并且做查询

	ret, err := driver.Query(data.FuncName, data.Param)
	if err != nil {
		msg.Reply(exec.client.NewMessage("", types.EventBlockChainQuery, err))
		return
	}
	msg.Reply(exec.client.NewMessage("", types.EventBlockChainQuery, ret))
}

func (exec *Executor) procExecCheckTx(msg *queue.Message) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("check panic error", "err", r)
			msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	datas := msg.GetData().(*types.ExecTxList)
	ctx := &executorCtx{
		stateHash:  datas.StateHash,
		height:     datas.Height,
		blocktime:  datas.BlockTime,
		difficulty: datas.Difficulty,
		mainHash:   datas.MainHash,
		mainHeight: datas.MainHeight,
		parentHash: datas.ParentHash,
	}
	var localdb dbm.KVDB

	if !exec.disableLocal {
		//交易检查只需要读取localdb，只读模式
		localdb = NewLocalDB(exec.client, exec.qclient, true)
		defer localdb.(*LocalDB).Close()
	}
	execute := newExecutor(ctx, exec, localdb, datas.Txs, nil)
	execute.enableMVCC(nil)
	//返回一个列表表示成功还是失败
	result := &types.ReceiptCheckTxList{}
	for i := 0; i < len(datas.Txs); i++ {
		tx := datas.Txs[i]
		index := i
		if datas.IsMempool {
			index = -1
		}
		err := execute.execCheckTx(tx, index)
		if err != nil {
			result.Errs = append(result.Errs, err.Error())
		} else {
			result.Errs = append(result.Errs, "")
		}
	}
	msg.Reply(exec.client.NewMessage("", types.EventReceiptCheckTx, result))
}

// GetStack ...
func GetStack() string {
	var buf [4048]byte
	n := runtime.Stack(buf[:], false)
	return fmt.Sprintf("==> %s\n", string(buf[:n]))
}

func updateMinIndex(keys []string, set *sync.Map, stateLockMap *sync.Map, i int) {
	if len(keys) != 0 {
		for _, r := range keys {
			v, _ := stateLockMap.LoadOrStore(r, &sync.Mutex{})
			if mu, ok := v.(*sync.Mutex); ok {
				mu.Lock()
				if idx, okk := set.Load(r); okk {
					if j, o := idx.(int); o {
						if i < j {
							set.Store(r, i)
						}
					}
				} else {
					set.Store(r, i)
				}
				mu.Unlock()
			}
		}
	}
}

func (exec *Executor) procExecTxListInterBlock(msg *queue.Message, sc *schedulerChan) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("exec tx list panic error", "err", r, "stack", GetStack())
			//msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	datas := msg.GetData().(*types.ExecTxList)
	ctx := &executorCtx{
		stateHash:  datas.StateHash,
		height:     datas.Height,
		blocktime:  datas.BlockTime,
		difficulty: datas.Difficulty,
		mainHash:   datas.MainHash,
		mainHeight: datas.MainHeight,
		parentHash: datas.ParentHash,
	}
	var localdb dbm.KVDB

	if !exec.disableLocal {
		localdb = NewLocalDB(exec.client, exec.qclient, false)
		defer localdb.(*LocalDB).Close()
	}
	execute := newExecutor(ctx, exec, localdb, datas.Txs, nil)
	execute.enableMVCC(nil)
	exec.scheduler.executeMap.Store(int(datas.Height), execute)
	receipts := make([]*types.Receipt, len(datas.Txs))

	initMu := sync.Mutex{}
	needInitRecords := true

	wg := sync.WaitGroup{}
	syncMap := &sync.Map{}
	reads := &sync.Map{}
	writes := &sync.Map{}
	hasParallelTx := false
	rwSetArr := make([]RWSet, len(datas.Txs))
	execFlags := make([]bool, len(datas.Txs))

	isHead := false
	for {
		select {
		case isHead = <-sc.headChan:
			close(sc.txChan)
			initMu.Lock()
			if needInitRecords {
				bt := time.Now()
				for i := 0; i < len(datas.Txs); i++ {
					tx := datas.Txs[i]
					if strings.HasPrefix(string(tx.Execer), "user.basic.test") {
						wg.Add(1)
						go func(j int) {
							defer wg.Done()
							driver := execute.loadDriver(datas.Txs[j], j)
							if driver.GetDriverName() == "basic" {
								basic, ok := driver.(*basic.Basic)
								if ok {
									rs, ws, err := basic.GetTxWritesAndReads(datas.Txs[j])
									if err == nil {
										rwSetArr[j] = RWSet{
											Reads:  rs,
											Writes: ws,
										}
									}
								}
							}
							updateMinIndex(rwSetArr[j].Reads, reads, syncMap, j)
							updateMinIndex(rwSetArr[j].Writes, writes, syncMap, j)
							hasParallelTx = true
						}(i)
						continue
					}
					receipt, err := execute.execTx(exec, tx, true, i)
					if api.IsAPIEnvError(err) {
						//msg.Reply(exec.client.NewMessage("", types.EventReceipts, err))
						return
					}
					if err != nil {
						receipts[i] = types.NewErrReceipt(err)
						continue
					}
					//update local
					receipts[i] = receipt
				}
				elog.Info("First Phase", "Execute Time", time.Since(bt), "Height", datas.Height)
				wg.Wait()
			}
			if !hasParallelTx {
				goto Done
			}
			needInitRecords = false
			initMu.Unlock()
			count := int32(0)
			bt := time.Now()
			for k, _ := range receipts {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					if execFlags[j] {
						atomic.AddInt32(&count, 1)
						//elog.Info("Tx Has Executed", "height", datas.Height, "txIndex", j)
						return
					}
					if hasConflict(j, rwSetArr[j].Writes, writes) {
						receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
						return
					}
					if hasConflict(j, rwSetArr[j].Writes, reads) && hasConflict(j, rwSetArr[j].Reads, writes) {
						receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
					} else {
						receipt, err := execute.execTx(exec, datas.Txs[j], false, j)
						//go exec.scheduler.receiveTxCommitMsg(rwSetArr[j], int(datas.Height))
						if err != nil {
							receipts[j] = types.NewErrReceipt(err)
						}
						receipts[j] = receipt
					}
				}(k)
			}
			wg.Wait()
			elog.Info("Second Phase", "Execute Time", time.Since(bt), "Height", datas.Height)
			elog.Info("Tx Has Executed", "height", datas.Height, "count", count)
			goto Done
		case txChan := <-sc.txChan:
			j := txChan.index
			if !isHead && !needInitRecords {
				if hasConflict(j, rwSetArr[j].Writes, writes) {
					receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
					execFlags[j] = true
				} else {
					if hasConflict(j, rwSetArr[j].Writes, reads) && hasConflict(j, rwSetArr[j].Reads, writes) {
						receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
					} else {
						receipt, err := execute.execTx(exec, datas.Txs[j], false, j)
						//go exec.scheduler.receiveTxCommitMsg(rwSetArr[j], int(datas.Height))
						if err != nil {
							receipts[j] = types.NewErrReceipt(err)
						}
						receipts[j] = receipt
					}
				}
				execFlags[j] = true
			}
		case ad := <-sc.assistChan:
			initMu.Lock()

			if needInitRecords {
				rwSetArr = ad.rwSets
				initWg := sync.WaitGroup{}
				for key, value := range ad.stateMap {
					initWg.Add(1)
					go func(k string, v *types.AssistItem) {
						defer initWg.Done()
						if v.MinRead != -1 {
							reads.Store(k, int(v.MinRead))
						}
						if v.MinWrite != -1 {
							writes.Store(k, int(v.MinWrite))
						}
					}(key, value)
				}
				initWg.Wait()
				needInitRecords = false
				hasParallelTx = true
			}
			initMu.Unlock()
			//if ad.height == 3 {
			//	time.Sleep(time.Duration(3) * time.Second)
			//	if sch, ok := exec.scheduler.schedulerChanMap.Load(0); ok {
			//		sch.(*schedulerChan).headChan <- true
			//	}
			//}
			//default:
			//	fmt.Println(datas.Height)
		}
	}
Done:
	sc.doneChan <- true
	block := &types.Block{
		ParentHash: datas.ParentHash,
		MainHash:   datas.MainHash,
		MainHeight: datas.MainHeight,
		Txs:        datas.Txs,
		BlockTime:  datas.BlockTime,
		Height:     datas.Height,
		Difficulty: uint32(datas.Difficulty),
	}
	reply := exec.client.NewMessage("blockchain", types.EventBlockReceipts, &types.ExecutedBlock{Block: block, Receipts: receipts})
	exec.client.Send(reply, false)
}

func (exec *Executor) procExecTxList(msg *queue.Message) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("exec tx list panic error", "err", r, "stack", GetStack())
			msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	datas := msg.GetData().(*types.ExecTxList)
	ctx := &executorCtx{
		stateHash:  datas.StateHash,
		height:     datas.Height,
		blocktime:  datas.BlockTime,
		difficulty: datas.Difficulty,
		mainHash:   datas.MainHash,
		mainHeight: datas.MainHeight,
		parentHash: datas.ParentHash,
	}
	var localdb dbm.KVDB
	if !exec.disableLocal {
		localdb = NewLocalDB(exec.client, exec.qclient, false)
		defer localdb.(*LocalDB).Close()
	}
	execute := newExecutor(ctx, exec, localdb, datas.Txs, nil)
	execute.enableMVCC(nil)
	var receipts []*types.Receipt
	index := 0

	wg := sync.WaitGroup{}
	syncMap := &sync.Map{}
	reads := &sync.Map{}
	writes := &sync.Map{}
	hasParallelTx := false
	rwSetArr := make([]RWSet, len(datas.Txs))

	for i := 0; i < len(datas.Txs); i++ {
		tx := datas.Txs[i]
		//检查groupcount
		if tx.GroupCount < 0 || tx.GroupCount == 1 || tx.GroupCount > 20 {
			receipts = append(receipts, types.NewErrReceipt(types.ErrTxGroupCount))
			continue
		}
		if tx.GroupCount == 0 {
			if strings.HasPrefix(string(tx.Execer), "user.basic.test") {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					driver := execute.loadDriver(tx, j)
					if driver.GetDriverName() == "basic" {
						basic, ok := driver.(*basic.Basic)
						if ok {
							rs, ws, err := basic.GetTxWritesAndReads(tx)
							if err == nil {
								rwSetArr[j] = RWSet{
									Reads:  rs,
									Writes: ws,
								}
							}
						}
					}
					updateMinIndex(rwSetArr[j].Reads, reads, syncMap, j)
					updateMinIndex(rwSetArr[j].Writes, writes, syncMap, j)
					hasParallelTx = true
					receipts = append(receipts, &types.Receipt{})
				}(index)
				index++
				continue
			}
			receipt, err := execute.execTx(exec, tx, true, index)
			if api.IsAPIEnvError(err) {
				msg.Reply(exec.client.NewMessage("", types.EventReceipts, err))
				return
			}
			if err != nil {
				receipts = append(receipts, types.NewErrReceipt(err))
				continue
			}
			//update local
			receipts = append(receipts, receipt)
			index++
			continue
		}
		//所有tx.GroupCount > 0 的交易都是错误的交易
		if !execute.cfg.IsFork(datas.Height, "ForkTxGroup") {
			receipts = append(receipts, types.NewErrReceipt(types.ErrTxGroupNotSupport))
			continue
		}
		//判断GroupCount 是否会产生越界
		if i+int(tx.GroupCount) > len(datas.Txs) {
			receipts = append(receipts, types.NewErrReceipt(types.ErrTxGroupCount))
			continue
		}
		receiptlist, err := execute.execTxGroup(datas.Txs[i:i+int(tx.GroupCount)], index)
		i = i + int(tx.GroupCount) - 1
		if len(receiptlist) > 0 && len(receiptlist) != int(tx.GroupCount) {
			panic("len(receiptlist) must be equal tx.GroupCount")
		}
		if err != nil {
			if api.IsAPIEnvError(err) {
				msg.Reply(exec.client.NewMessage("", types.EventReceipts, err))
				return
			}
			for n := 0; n < int(tx.GroupCount); n++ {
				receipts = append(receipts, types.NewErrReceipt(err))
			}
			continue
		}
		receipts = append(receipts, receiptlist...)
		index += int(tx.GroupCount)
	}
	wg.Wait()
	if hasParallelTx {
		for k, _ := range receipts {
			wg.Add(1)
			go func(j int, set *RWSet, tx *types.Transaction) {
				defer wg.Done()
				if hasConflict(j, set.Writes, writes) {
					receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
					return
				}
				if hasConflict(j, set.Writes, reads) && hasConflict(j, set.Reads, writes) {
					receipts[j] = types.NewErrReceipt(types.ErrTxAborted)
				} else {
					receipt, err := execute.execTx(exec, tx, false, j)
					if err != nil {
						receipts[j] = types.NewErrReceipt(err)
					}
					receipts[j] = receipt
				}
			}(k, &rwSetArr[k], datas.Txs[k])
		}
		wg.Wait()
	}
	msg.Reply(exec.client.NewMessage("", types.EventReceipts,
		&types.Receipts{Receipts: receipts}))
}

func hasConflict(idx int, keys []string, records *sync.Map) bool {
	for _, key := range keys {
		if min, okk := records.Load(key); okk {
			m, _ := min.(int)
			if idx > m {
				return true
			}
		}
	}
	return false
}

func (exec *Executor) procExecAddBlock(msg *queue.Message) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("add blk panic error", "err", r)
			msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	datas := msg.GetData().(*types.BlockDetail)
	b := datas.Block
	ctx := &executorCtx{
		stateHash:  b.StateHash,
		height:     b.Height,
		blocktime:  b.BlockTime,
		difficulty: uint64(b.Difficulty),
		mainHash:   b.MainHash,
		mainHeight: b.MainHeight,
		parentHash: b.ParentHash,
	}
	var localdb dbm.KVDB
	if !exec.disableLocal {
		localdb = NewLocalDB(exec.client, exec.qclient, false)
		defer localdb.(*LocalDB).Close()
	}
	execute := newExecutor(ctx, exec, localdb, b.Txs, datas.Receipts)
	//因为mvcc 还没有写入，所以目前的mvcc版本是前一个区块的版本
	execute.enableMVCC(datas.PrevStatusHash)
	var kvset types.LocalDBSet
	for _, kv := range datas.KV {
		err := execute.stateDB.Set(kv.Key, kv.Value)
		if err != nil {
			panic(err)
		}
	}
	for name, plugin := range globalPlugins {
		kvs, ok, err := plugin.CheckEnable(execute, exec.pluginEnable[name])
		if err != nil {
			panic(err)
		}
		if !ok {
			continue
		}
		if len(kvs) > 0 {
			kvset.KV = append(kvset.KV, kvs...)
		}
		kvs, err = plugin.ExecLocal(execute, datas)
		if err != nil {
			msg.Reply(exec.client.NewMessage("", types.EventAddBlock, err))
			return
		}
		if len(kvs) > 0 {
			kvset.KV = append(kvset.KV, kvs...)
			for _, kv := range kvs {
				err := execute.localDB.Set(kv.Key, kv.Value)
				if err != nil {
					panic(err)
				}
			}
		}
	}

	if !exec.disableExecLocal {
		for i := 0; i < len(b.Txs); i++ {
			tx := b.Txs[i]
			execute.localDB.(*LocalDB).StartTx()
			kv, err := execute.execLocalTx(tx, datas.Receipts[i], i)
			if err != nil {
				msg.Reply(exec.client.NewMessage("", types.EventAddBlock, err))
				return
			}
			if kv != nil && kv.KV != nil {
				kvset.KV = append(kvset.KV, kv.KV...)
			}
		}
	}
	msg.Reply(exec.client.NewMessage("", types.EventAddBlock, &kvset))
}

func (exec *Executor) procExecDelBlock(msg *queue.Message) {
	//panic 处理
	defer func() {
		if r := recover(); r != nil {
			elog.Error("del blk panic error", "err", r)
			msg.Reply(exec.client.NewMessage("", types.EventReceipts, types.ErrExecPanic))
			return
		}
	}()
	datas := msg.GetData().(*types.BlockDetail)
	b := datas.Block
	ctx := &executorCtx{
		stateHash:  b.StateHash,
		height:     b.Height,
		blocktime:  b.BlockTime,
		difficulty: uint64(b.Difficulty),
		mainHash:   b.MainHash,
		mainHeight: b.MainHeight,
		parentHash: b.ParentHash,
	}
	var localdb dbm.KVDB
	if !exec.disableLocal {
		localdb = NewLocalDB(exec.client, exec.qclient, false)
		defer localdb.(*LocalDB).Close()
	}
	execute := newExecutor(ctx, exec, localdb, b.Txs, nil)
	execute.enableMVCC(nil)
	var kvset types.LocalDBSet
	for _, kv := range datas.KV {
		err := execute.stateDB.Set(kv.Key, kv.Value)
		if err != nil {
			panic(err)
		}
	}
	for name, plugin := range globalPlugins {
		kvs, ok, err := plugin.CheckEnable(execute, exec.pluginEnable[name])
		if err != nil {
			panic(err)
		}
		if !ok {
			continue
		}
		if len(kvs) > 0 {
			kvset.KV = append(kvset.KV, kvs...)
		}
		kvs, err = plugin.ExecDelLocal(execute, datas)
		if err != nil {
			msg.Reply(exec.client.NewMessage("", types.EventAddBlock, err))
			return
		}
		if len(kvs) > 0 {
			kvset.KV = append(kvset.KV, kvs...)
		}
	}
	if !exec.disableExecLocal {
		for i := len(b.Txs) - 1; i >= 0; i-- {
			tx := b.Txs[i]
			kv, err := execute.execDelLocal(tx, datas.Receipts[i], i)
			if err == types.ErrActionNotSupport {
				continue
			}
			if err != nil {
				msg.Reply(exec.client.NewMessage("", types.EventDelBlock, err))
				return
			}
			if kv != nil && kv.KV != nil {
				err := execute.checkPrefix(tx.Execer, kv.KV)
				if err != nil {
					msg.Reply(exec.client.NewMessage("", types.EventDelBlock, err))
					return
				}
				kvset.KV = append(kvset.KV, kv.KV...)
			}
		}
	}
	msg.Reply(exec.client.NewMessage("", types.EventDelBlock, &kvset))
}

// Close close executor
func (exec *Executor) Close() {
	elog.Info("exec module closed")
	if exec.client != nil {
		exec.client.Close()
	}
}
