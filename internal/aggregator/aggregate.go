package aggregator

import (
	"fmt"

	"github.com/sjy-dv/IZONE/internal/aggregator/vdb"
	"github.com/sjy-dv/IZONE/internal/channel"
)

func WriteMostExecQueriesLog(label, schema, querytext string, count int64) {
	rewrite := fmt.Sprintf("[%s]-%s | %s (%d times)", label, schema, querytext, count)
	txn := vdb.V.Txn(true)
	if err := txn.Insert("auditlogs", vdb.VdbColumn(label, rewrite)); err != nil {
		channel.ErrCh <- err
		return
	}
	txn.Commit()
}
