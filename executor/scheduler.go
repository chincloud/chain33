package executor

import (
	"github.com/33cn/chain33/common/db"
	basic "github.com/33cn/chain33/plugin/dapp/basic/executor"
	"github.com/33cn/chain33/queue"
	"github.com/33cn/chain33/types"
	"sync"
	"sync/atomic"
	"time"
)

type scheduler struct {
	exec               *Executor
	executingQueue     chan *queue.Message
	executingQueueSize int
	schedulerChanMap   *sync.Map
	txListMap          *sync.Map
	currentHeight      int
	executeMap         *sync.Map
	stateMap           *sync.Map
	stateMuMap         *sync.Map
	heightTxMap        *sync.Map
	count              int32
}

type schedulerChan struct {
	headChan   chan bool
	txChan     chan *transIndex
	assistChan chan *assistData
	doneChan   chan bool
}

func newSchedulerChan() *schedulerChan {
	return &schedulerChan{
		headChan:   make(chan bool, 1),
		txChan:     make(chan *transIndex, 5000),
		assistChan: make(chan *assistData, 1),
		doneChan:   make(chan bool, 1),
	}
}

type assistData struct {
	stateMap map[string]*types.AssistItem
	height   int
	rwSets   []RWSet
}

type transIndex struct {
	index   int
	rwSet   RWSet
	height  int
	depends int32
}

type txScheduler struct {
	ti *transIndex
	sc *schedulerChan
}

func newScheduler(exec *Executor, executingQueueSize int) *scheduler {
	s := &scheduler{
		exec:               exec,
		executingQueueSize: executingQueueSize,
		executingQueue:     make(chan *queue.Message, 128),
		schedulerChanMap:   &sync.Map{},
		txListMap:          &sync.Map{},
		executeMap:         &sync.Map{},
		stateMap:           &sync.Map{},
		stateMuMap:         &sync.Map{},
		heightTxMap:        &sync.Map{},
		count:              int32(0),
	}

	go func() {
		for {
			msg := <-s.executingQueue
			data := msg.Data.(*types.ExecTxList)
			height := int(data.Height)
			if c, ok := s.schedulerChanMap.Load(height); ok {
				//等待队头区块执行完毕
				sc := c.(*schedulerChan)
				sc.headChan <- true
				<-sc.doneChan
				s.currentHeight = height + 1
				go func() {
					if xxx, okk := s.heightTxMap.Load(int32(height)); okk {
						tsMap := xxx.(*sync.Map)
						wg := sync.WaitGroup{}
						tsMap.Range(func(key, value any) bool {
							wg.Add(1)
							go func(xx *txScheduler) {
								defer wg.Done()
								if xx.ti.height > s.currentHeight {
									xx.sc.txChan <- xx.ti
								}
								tsMap.Delete(xx)
							}(key.(*txScheduler))
							return true
						})

						wg.Wait()
						s.heightTxMap.Delete(int32(height))
					}
				}()

				//下个区块已经到达执行队列队头
				//if next, okk := s.schedulerChanMap.Load(height + 1); okk {
				//	nc := next.(*schedulerChan)
				//	nc.headChan <- true
				//}
				s.schedulerChanMap.Delete(height)
			}
		}
	}()
	return s
}

func (s *scheduler) receiveMsg(msg *queue.Message) {
	data := msg.Data.(*types.ExecTxList)
	height := int(data.Height)
	s.txListMap.Store(height, data)

	sc := newSchedulerChan()
	s.schedulerChanMap.Store(height, sc)
	s.executingQueue <- msg
	//if data.Height == 0 {
	//	sc.headChan <- true
	//}
	go s.exec.procExecTxListInterBlock(msg, sc)
}

func (s *scheduler) receiveAssistData(msg *queue.Message) {
	ad := msg.Data.(*types.AssistData)
	data := &assistData{
		stateMap: ad.StateMap,
		height:   int(ad.Height),
	}
	if data.height <= s.currentHeight {
		return
	}
	if v, ok := s.txListMap.Load(data.height); ok {
		txList := v.(*types.ExecTxList)
		data.rwSets = make([]RWSet, len(txList.Txs))
		wg := sync.WaitGroup{}
		count := int32(0)

		var execute *executor
		if e, okk := s.executeMap.Load(data.height); okk {
			execute = e.(*executor)
		} else {
			ctx := &executorCtx{
				stateHash:  txList.StateHash,
				height:     txList.Height,
				blocktime:  txList.BlockTime,
				difficulty: txList.Difficulty,
				mainHash:   txList.MainHash,
				mainHeight: txList.MainHeight,
				parentHash: txList.ParentHash,
			}
			var localdb db.KVDB
			if !s.exec.disableLocal {
				localdb = NewLocalDB(s.exec.client, s.exec.qclient, false)
				defer localdb.(*LocalDB).Close()
			}
			execute = newExecutor(ctx, s.exec, localdb, txList.Txs, nil)
			execute.enableMVCC(nil)
		}

		for i, _ := range txList.Txs {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				driver := execute.loadDriver(txList.Txs[idx], idx)
				if driver.GetDriverName() == "basic" {
					basic, o := driver.(*basic.Basic)
					if o {
						rs, ws, err := basic.GetTxWritesAndReads(txList.Txs[idx])
						if err == nil {
							atomic.AddInt32(&count, 1)
							data.rwSets[idx] = RWSet{
								Reads:  rs,
								Writes: ws,
							}
						}
					}
				}
			}(i)
		}

		wg.Wait()
		var sc *schedulerChan
		scc, okk := s.schedulerChanMap.Load(data.height)
		if okk {
			sc = scc.(*schedulerChan)
			sc.assistChan <- data
		}

		time.Sleep(time.Duration(3*data.height) * time.Millisecond)
		//TODO 生成调度数据
		bt := time.Now()
		for idx, _ := range txList.Txs {
			wg.Add(1)
			go func(i int) {
				wg.Done()
				transIdx := &transIndex{
					index:   i,
					rwSet:   data.rwSets[i],
					height:  data.height,
					depends: 0,
				}

				if ver, ojbk := ad.VersionMap[int32(i)]; ojbk {
					transIdx.depends = int32(data.height) - ver
				}

				height := transIdx.depends
				if height == 0 && data.height > s.executingQueueSize {
					height = int32(data.height - s.executingQueueSize)
				}

				actual, _ := s.heightTxMap.LoadOrStore(height, &sync.Map{})
				ts := &txScheduler{
					ti: transIdx,
					sc: sc,
				}
				actual.(*sync.Map).Store(ts, true)

			}(idx)
		}
		wg.Wait()
		elog.Info("Scheduler", "Height", data.height, "SpentTime", time.Since(bt))
	}
}

func (s *scheduler) receiveTxCommitMsg(set RWSet, height int) {
	for _, key := range set.Writes {
		if m, ok := s.stateMap.Load(key); ok {
			stateMu, _ := s.stateMuMap.LoadOrStore(key, &sync.Mutex{})
			stateMu.(*sync.Mutex).Lock()
			if arr, okk := m.(*sync.Map).Load(height); okk {
				for _, txIdx := range arr.([]*transIndex) {
					depends := atomic.AddInt32(&txIdx.depends, -1)
					if depends == 0 {
						if scc, o := s.schedulerChanMap.Load(txIdx.height); o {
							scc.(*schedulerChan).txChan <- txIdx
						}
					}
				}
			}
			stateMu.(*sync.Mutex).Unlock()
			//循环删除无效调度数据
			//for h := height - 1; ; h-- {
			//	if _, okk := m.(*sync.Map).Load(h); okk {
			//		m.(*sync.Map).Delete(h)
			//	} else {
			//		break
			//	}
			//}
		}
	}
}
