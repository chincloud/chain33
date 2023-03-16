package benchmarks

import (
	"context"
	"fmt"
	_ "github.com/33cn/chain33/benchmarks/db"
	wl "github.com/33cn/chain33/benchmarks/workload"
	"github.com/magiconair/properties"
	"github.com/pingcap/go-ycsb/pkg/client"
	"github.com/pingcap/go-ycsb/pkg/measurement"
	_ "github.com/pingcap/go-ycsb/pkg/measurement"
	"github.com/pingcap/go-ycsb/pkg/prop"
	"github.com/pingcap/go-ycsb/pkg/util"
	"github.com/pingcap/go-ycsb/pkg/ycsb"
	"net/http"
	"strconv"
	"testing"
	"time"
)

var (
	propertyFiles []string
	//propertyValues []string
	tableName string

	globalContext context.Context

	globalDB       ycsb.DB
	globalWorkload ycsb.Workload
	globalProps    *properties.Properties
)

func TestYCSB(t *testing.T) {

	globalContext = context.Background()
	propertyFiles = append(propertyFiles, "./workload/workloadf")

	initialGlobal("basic", func() {

		globalProps.Set(prop.DoTransactions, "true")
		//globalProps.Set(prop.Command, "run")

		globalProps.Set(prop.ThreadCount, strconv.Itoa(4))
		globalProps.Set(prop.Target, strconv.Itoa(10000))
		globalProps.Set(prop.Verbose, "true")
		globalProps.Set(prop.InsertOrder, "sequence")
		//globalProps.Set(prop.WriteAllFields, "true")
		globalProps.Set(prop.ReadAllFields, "false")
		//globalProps.Set(prop.LogInterval, strconv.Itoa(10000))

	})

	fmt.Println("***************** properties *****************")
	for key, value := range globalProps.Map() {
		fmt.Printf("\"%s\"=\"%s\"\n", key, value)
	}
	fmt.Println("**********************************************")

	c := client.NewClient(globalProps, globalWorkload, globalDB)
	start := time.Now()

	globalContext = context.WithValue(globalContext, wl.TxnSeqKey, &wl.TxnSeqState{Sequence: new(int32)})
	c.Run(globalContext)

	fmt.Printf("Run finished, takes %s\n", time.Now().Sub(start))
	measurement.Output()

	if globalDB != nil {
		globalDB.Close()
	}

	if globalWorkload != nil {
		globalWorkload.Close()
	}
}

func initialGlobal(dbName string, onProperties func()) {
	globalProps = properties.NewProperties()
	if len(propertyFiles) > 0 {
		globalProps = properties.MustLoadFiles(propertyFiles, properties.UTF8, false)
	}

	//for _, prop := range propertyValues {
	//	seps := strings.SplitN(prop, "=", 2)
	//	if len(seps) != 2 {
	//		log.Fatalf("bad property: `%s`, expected format `name=value`", prop)
	//	}
	//	globalProps.Set(seps[0], seps[1])
	//}

	if onProperties != nil {
		onProperties()
	}

	addr := globalProps.GetString(prop.DebugPprof, prop.DebugPprofDefault)
	go func() {
		http.ListenAndServe(addr, nil)
	}()

	measurement.InitMeasure(globalProps)

	if len(tableName) == 0 {
		tableName = globalProps.GetString(prop.TableName, prop.TableNameDefault)
	}

	workloadName := globalProps.GetString(prop.Workload, "core")
	workloadCreator := ycsb.GetWorkloadCreator(workloadName)

	var err error
	if globalWorkload, err = workloadCreator.Create(globalProps); err != nil {
		util.Fatalf("create workload %s failed %v", workloadName, err)
	}

	dbCreator := ycsb.GetDBCreator(dbName)
	if dbCreator == nil {
		util.Fatalf("%s is not registered", dbName)
	}
	if globalDB, err = dbCreator.Create(globalProps); err != nil {
		util.Fatalf("create db %s failed %v", dbName, err)
	}
	globalDB = client.DbWrapper{globalDB}
}
