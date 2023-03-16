// Copyright 2018 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Copyright (c) 2010-2016 Yahoo! Inc., 2017 YCSB contributors All rights reserved.
 * <p>
 * Licensed under the Apache License, Version 2.0 (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 * <p>
 * http://www.apache.org/licenses/LICENSE-2.0
 * <p>
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
 * implied. See the License for the specific language governing
 * permissions and limitations under the License. See accompanying
 * LICENSE file.
 */

package db

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magiconair/properties"
	"github.com/pingcap/go-ycsb/pkg/prop"
	"github.com/pingcap/go-ycsb/pkg/ycsb"
)

const (
	simulateDelay         = "basicdb.simulatedelay"
	simulateDelayDefault  = int64(0)
	randomizeDelay        = "basicdb.randomizedelay"
	randomizeDelayDefault = true
)

type contextKey string

const stateKey = contextKey("basicDB")

type basicState struct {
	r *rand.Rand

	buf *bytes.Buffer
}

// BasicDB just prints out the requested operations, instead of doing them against a database
type basicDB struct {
	verbose        bool
	randomizeDelay bool
	toDelay        int64
	txs            []txn
	reads          map[string]int
	writes         map[string]int
}

type txn map[string][]string

func (db *basicDB) delay(ctx context.Context, state *basicState) {
	if db.toDelay == 0 {
		return
	}

	r := state.r
	delayTime := time.Duration(db.toDelay) * time.Millisecond
	if db.randomizeDelay {
		delayTime = time.Duration(r.Int63n(db.toDelay)) * time.Millisecond
		if delayTime == 0 {
			return
		}
	}

	select {
	case <-time.After(delayTime):
	case <-ctx.Done():
	}
}

func (db *basicDB) InitThread(ctx context.Context, _ int, _ int) context.Context {
	state := new(basicState)
	state.r = rand.New(rand.NewSource(time.Now().UnixNano()))
	state.buf = new(bytes.Buffer)

	return context.WithValue(ctx, stateKey, state)
}

func (db *basicDB) CleanupThread(_ context.Context) {

}

func (db *basicDB) Close() error {
	db.output()
	return nil
}

func (db *basicDB) Read(ctx context.Context, table string, key string, fields []string) (map[string][]byte, error) {
	state := ctx.Value(stateKey).(*basicState)

	db.delay(ctx, state)

	if !db.verbose {
		return nil, nil
	}

	num, _ := strconv.Atoi(table)
	if db.txs[num-1] == nil {
		db.txs[num-1] = make(map[string][]string)
	}

	list := make([]string, 0)
	copy(list, fields)
	for _, field := range fields {
		list = append(list, fmt.Sprintf("%s_%s", key[4:], field))
	}
	db.txs[num-1]["Read"] = list
	//buf := state.buf
	//s := fmt.Sprintf("READ %s %s [ ", table, key)
	//buf.WriteString(s)
	//
	//if len(fields) > 0 {
	//	for _, f := range fields {
	//		buf.WriteString(f)
	//		buf.WriteByte(' ')
	//	}
	//} else {
	//	buf.WriteString("<all fields> ")
	//}
	//buf.WriteByte(']')
	//fmt.Println(buf.String())
	//buf.Reset()
	return nil, nil
}

func (db *basicDB) Scan(ctx context.Context, table string, startKey string, count int, fields []string) ([]map[string][]byte, error) {
	state := ctx.Value(stateKey).(*basicState)

	db.delay(ctx, state)

	if !db.verbose {
		return nil, nil
	}

	buf := state.buf
	s := fmt.Sprintf("SCAN %s %s %d [ ", table, startKey, count)
	buf.WriteString(s)

	if len(fields) > 0 {
		for _, f := range fields {
			buf.WriteString(f)
			buf.WriteByte(' ')
		}
	} else {
		buf.WriteString("<all fields> ")
	}
	buf.WriteByte(']')
	fmt.Println(buf.String())
	buf.Reset()
	return nil, nil
}

func (db *basicDB) Update(ctx context.Context, table string, key string, values map[string][]byte) error {
	state := ctx.Value(stateKey).(*basicState)

	db.delay(ctx, state)

	if !db.verbose {
		return nil
	}

	num, _ := strconv.Atoi(table)
	if db.txs[num-1] == nil {
		db.txs[num-1] = make(map[string][]string)
	}

	for k, _ := range values {
		db.txs[num-1]["Write"] = append(db.txs[num-1]["Write"], fmt.Sprintf("%s_%s", key[4:], k))
	}
	//buf := state.buf
	//s := fmt.Sprintf("UPDATE %s %s [ ", table, key)
	//buf.WriteString(s)
	//
	//for key, value := range values {
	//	buf.WriteString(key)
	//	buf.WriteByte('=')
	//	buf.Write(value)
	//	buf.WriteByte(' ')
	//}
	//
	//buf.WriteByte(']')
	//fmt.Println(buf.String())
	//buf.Reset()
	return nil
}

func (db *basicDB) Insert(ctx context.Context, table string, key string, values map[string][]byte) error {
	state := ctx.Value(stateKey).(*basicState)

	db.delay(ctx, state)

	if !db.verbose {
		return nil
	}

	buf := state.buf
	insertRecord(buf, table, key, values)
	return nil
}

func insertRecord(buf *bytes.Buffer, table string, key string, values map[string][]byte) {
	s := fmt.Sprintf("INSERT %s %s [ ", table, key)
	buf.WriteString(s)
	for valueKey, value := range values {
		buf.WriteString(valueKey)
		buf.WriteByte('=')
		buf.Write(value)
		buf.WriteByte(' ')
	}
	buf.WriteByte(']')
	fmt.Println(buf.String())
	buf.Reset()
}

func (db *basicDB) Delete(ctx context.Context, table string, key string) error {
	state := ctx.Value(stateKey).(*basicState)

	db.delay(ctx, state)
	if !db.verbose {
		return nil
	}

	buf := state.buf
	s := fmt.Sprintf("DELETE %s %s", table, key)
	buf.WriteString(s)

	fmt.Println(buf.String())
	buf.Reset()
	return nil
}

func (db *basicDB) output() {
	db.result()
	db.writeFile()

	return
	for idx, txn := range db.txs {
		for key, value := range txn {
			fmt.Print(fmt.Sprintf("%d %s", idx+1, key))
			fmt.Print(" [ ")
			for _, field := range value {
				fmt.Print(field)
				fmt.Print(" ")
			}
			fmt.Println("]")
		}
	}
}

func (db *basicDB) writeFile() {

	fileName := "./tx_zipf_0.7_ycsb_f.data"
	var err error
	var file *os.File

	if exists(fileName) {
		if file, err = os.OpenFile(fileName, os.O_APPEND, 0666); err != nil {
			fmt.Println("打开文件错误：", err)
			return
		}
	} else {
		if file, err = os.Create(fileName); err != nil {
			fmt.Println("创建失败：", err)
			return
		}
	}
	defer file.Close()

	for idx, txn := range db.txs {
		for key, value := range txn {
			line := fmt.Sprintf("%d %s", idx+1, key)
			for _, field := range value {
				line += " " + field
			}
			io.WriteString(file, line+"\n")
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func (db *basicDB) result() {

	abortedTxn := int32(0)
	db.reads = make(map[string]int)
	db.writes = make(map[string]int)
	ids := make(map[int]bool, 0)
	related := make(map[int][]int)
	extraTxn := int32(0)

	xxx := int32(0)

	wg := sync.WaitGroup{}
	for idx, txn := range db.txs {
		//wg.Add(1)
		//func(i int) {
		//defer wg.Done()
		for _, key := range txn["Read"] {
			if db.reads[key] > idx+1 || db.reads[key] == 0 {
				db.reads[key] = idx + 1
			}
		}
		for _, key := range txn["Write"] {
			if db.writes[key] > idx+1 || db.writes[key] == 0 {
				db.writes[key] = idx + 1
			}
		}
		//}(idx)
	}
	//wg.Wait()

	for idx, txn := range db.txs {
		wg.Add(1)
		func(i int) {
			defer wg.Done()
			if hasConflict(i+1, txn["Write"], db.writes) {
				atomic.AddInt32(&abortedTxn, 1)
				return
			}

			if hasConflict(i+1, txn["Read"], db.writes) {
				if hasConflict(i+1, txn["Write"], db.reads) {
					atomic.AddInt32(&abortedTxn, 1)
					ids[i+1] = true
					getRelated(i+1, txn["Write"], db.reads, related)
					return
				}
				atomic.AddInt32(&xxx, 1)
			}

		}(idx)
	}
	wg.Wait()

	fmt.Println(abortedTxn)
	fmt.Println(xxx)
	fmt.Println(len(ids))

	for k, _ := range ids {
		wg.Add(1)
		func(i int) {
			defer wg.Done()
			for _, relatedIdx := range related[i] {
				if ids[relatedIdx] != true {
					return
				}
			}
			atomic.AddInt32(&extraTxn, 1)
		}(k)
	}
	wg.Wait()

	fmt.Println(extraTxn)
}

func hasConflict(idx int, keys []string, records map[string]int) bool {
	for _, key := range keys {
		if records[key] != 0 && records[key] < idx {
			return true
		}
	}
	return false
}

func getRelated(idx int, keys []string, reads map[string]int, related map[int][]int) {
	for _, key := range keys {
		if reads[key] != 0 && reads[key] != idx {
			related[idx] = append(related[idx], reads[key])
		}
	}
}

type basicDBCreator struct{}

func (basicDBCreator) Create(p *properties.Properties) (ycsb.DB, error) {
	db := new(basicDB)

	db.verbose = p.GetBool(prop.Verbose, prop.VerboseDefault)
	db.randomizeDelay = p.GetBool(randomizeDelay, randomizeDelayDefault)
	db.toDelay = p.GetInt64(simulateDelay, simulateDelayDefault)
	db.txs = make([]txn, int(p.GetInt64(prop.OperationCount, 0)))

	return db, nil
}

func init() {
	ycsb.RegisterDBCreator("basic", basicDBCreator{})
}
