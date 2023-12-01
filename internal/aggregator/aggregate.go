package aggregator

import (
	"fmt"

	"github.com/sjy-dv/IZONE/internal/aggregator/vdb"
	"github.com/sjy-dv/IZONE/internal/channel"
	"github.com/sjy-dv/IZONE/pkg/log"
)

func WriteMostExecQueriesLog(prefix, label, schema, querytext string, count int64) {
	rewrite := fmt.Sprintf("[%s]-%s | %s (%d times)", label, schema, querytext, count)
	txn := vdb.V.Txn(true)
	if err := txn.Insert("auditlogs", vdb.VdbColumn(prefix, rewrite)); err != nil {
		channel.ErrCh <- err
		return
	}
	txn.Commit()
}

func WriteSlowAvgQueriesLog(prefix, label, schema, querytext string, avgexectims int64) {
	rewrite := fmt.Sprintf("Slow Query [%s]-%s | %s (%d ms)", label, schema, querytext, avgexectims)
	txn := vdb.V.Txn(true)
	if err := txn.Insert("auditlogs", vdb.VdbColumn(prefix, rewrite)); err != nil {
		channel.ErrCh <- err
		return
	}
	txn.Commit()
}

func WriteWaitAvgQueriesLog(prefix, label, schema, querytext string, sumtimerwaits int64) {
	rewrite := fmt.Sprintf("Resource Intensive Query [%s]-%s | %s (%d ms)", label, schema, querytext, sumtimerwaits)
	txn := vdb.V.Txn(true)
	if err := txn.Insert("auditlogs", vdb.VdbColumn(prefix, rewrite)); err != nil {
		channel.ErrCh <- err
		return
	}
	txn.Commit()
}

func WriteOccurErrorQueriesLog(prefix, label, schema, querytext string, errorscount int64) {
	rewrite := fmt.Sprintf("Warning Query [%s]-%s | %s (%d times)", label, schema, querytext, errorscount)
	txn := vdb.V.Txn(true)
	if err := txn.Insert("auditlogs", vdb.VdbColumn(prefix, rewrite)); err != nil {
		channel.ErrCh <- err
		return
	}
	txn.Commit()
}

func Flush(label string) {
	txn := vdb.V.Txn(true)
	defer txn.Commit()
	it, err := txn.Get("auditlogs", "label")
	if err != nil {
		channel.ErrCh <- err
		return
	}
	for obj := it.Next(); obj != nil; obj = it.Next() {
		logs := vdb.Cast(obj)
		if logs.Label == label {
			t, l := vdb.Output(logs)
			log.Debugf("[%v]:%s", t, l)
		}
	}
	_, err = txn.DeletePrefix("auditlogs", "label", label)
	if err != nil {
		channel.ErrCh <- err
		return
	}
}
