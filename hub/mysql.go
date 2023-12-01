package hub

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sjy-dv/IZONE/internal/aggregator"
	"github.com/sjy-dv/IZONE/internal/channel"
	"github.com/sjy-dv/IZONE/pkg/workpool"
)

type mysql struct {
	mu          sync.Mutex
	connections []*mysqlidentifier
}

type mysqlidentifier struct {
	connection *sql.DB
	DBConnector
	timer *time.Ticker
	mysqlCtxChannel
}

type mysqlCtxChannel struct {
	mostExecChannel        chan mostExec
	slowAvgQueryChannel    chan slowAvgQuery
	waitAvgQueryChannel    chan waitAvgQuery
	occurErrorQueryChannel chan occurErrorQuery
}

var mysqlorg *mysql

func MysqlConnector(dbconnector *DBConnector) error {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/performance_schema",
		dbconnector.User, dbconnector.Password, dbconnector.Host, dbconnector.Port))
	if err != nil {
		return err
	}
	mysqlorg.mu.Lock()
	defer mysqlorg.mu.Unlock()
	mysql := &mysqlidentifier{
		connection:  db,
		DBConnector: *dbconnector,
		mysqlCtxChannel: mysqlCtxChannel{
			mostExecChannel:        make(chan mostExec),
			slowAvgQueryChannel:    make(chan slowAvgQuery),
			waitAvgQueryChannel:    make(chan waitAvgQuery),
			occurErrorQueryChannel: make(chan occurErrorQuery),
		},
	}
	mysql.monitoring()
	mysql.realtimePusher()
	mysqlorg.connections = append(mysqlorg.connections, mysql)
	return nil
}

func (identifier *mysqlidentifier) monitoring() {
	identifier.timer = time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-identifier.timer.C:
				identifier.farmingMetrics()
			}
		}
	}()
}

func (identifier *mysqlidentifier) realtimePusher() {
	for {
		select {
		case mec := <-identifier.mostExecChannel:
			if mec.fetch {
				aggregator.Flush(fmt.Sprintf("mec_%s", identifier.Label))
			} else {
				aggregator.WriteMostExecQueriesLog(fmt.Sprintf("mec_%s", identifier.Label), identifier.Label, mec.schemaName, mec.digestText, mec.executionCount)
			}
		case saqc := <-identifier.slowAvgQueryChannel:
			if saqc.fetch {
				aggregator.Flush(fmt.Sprintf("saqc_%s", identifier.Label))
			} else {
				aggregator.WriteSlowAvgQueriesLog(fmt.Sprintf("saqc_%s",
					identifier.Label), identifier.Label, saqc.schemaName,
					saqc.digestText, saqc.avgExecutionTimeMs)
			}
		case waqc := <-identifier.waitAvgQueryChannel:
			if waqc.fetch {
				aggregator.Flush(fmt.Sprintf("waqc_%s", identifier.Label))
			} else {
				aggregator.WriteWaitAvgQueriesLog(fmt.Sprintf("waqc_%s", identifier.Label),
					identifier.Label, waqc.schemaName, waqc.digestText, waqc.totalWaitTimeMs)
			}
		case oeqc := <-identifier.occurErrorQueryChannel:
			if oeqc.fetch {
				aggregator.Flush(fmt.Sprintf("oeqc_%s", identifier.Label))
			} else {
				aggregator.WriteOccurErrorQueriesLog(fmt.Sprintf("oeqc_%s", identifier.Label), identifier.Label,
					oeqc.schemaName, oeqc.digestText, oeqc.errors)
			}
		}
	}
}

func (identifier *mysqlidentifier) farmingMetrics() {
	lightpool := workpool.NewPool(4, 4)
	defer lightpool.Release()
	lightpool.WaitCount(4)
	lightpool.JobQueue <- func() {
		if err := identifier.mostExecutedQueries(); err != nil {
			channel.ErrCh <- err
		}
		defer lightpool.JobDone()
	}
	lightpool.JobQueue <- func() {
		if err := identifier.slowAverageQueries(); err != nil {
			channel.ErrCh <- err
		}
		defer lightpool.JobDone()
	}
	lightpool.JobQueue <- func() {
		if err := identifier.waitAverageQueries(); err != nil {
			channel.ErrCh <- err
		}
		defer lightpool.JobDone()
	}
	lightpool.JobQueue <- func() {
		if err := identifier.occurErrorQueries(); err != nil {
			channel.ErrCh <- err
		}
		defer lightpool.JobDone()
	}
	lightpool.WaitAll()
}

func (identifier *mysqlidentifier) mostExecutedQueries() error {
	rows, err := identifier.connection.Query(`
	SELECT SCHEMA_NAME, DIGEST_TEXT, COUNT_STAR AS EXECUTION_COUNT
	FROM performance_schema.events_statements_summary_by_digest
	ORDER BY EXECUTION_COUNT DESC
	LIMIT 10;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var m mostExec
		if err := rows.Scan(&m.schemaName, &m.digestText, &m.executionCount); err != nil {
			channel.ErrCh <- err
			continue
		}
		identifier.
			mysqlCtxChannel.mostExecChannel <- m
	}
	return nil
}

func (identifier *mysqlidentifier) slowAverageQueries() error {
	rows, err := identifier.connection.Query(`
	SELECT SCHEMA_NAME, DIGEST_TEXT, AVG_TIMER_WAIT/1000000 AS AVG_EXECUTION_TIME_MS
	FROM performance_schema.events_statements_summary_by_digest
	ORDER BY AVG_EXECUTION_TIME_MS DESC
	LIMIT 10;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s slowAvgQuery
		if err := rows.Scan(&s.schemaName, &s.digestText, &s.avgExecutionTimeMs); err != nil {
			channel.ErrCh <- err
			continue
		}
		identifier.mysqlCtxChannel.slowAvgQueryChannel <- s
	}
	return nil
}

func (identifier *mysqlidentifier) waitAverageQueries() error {
	rows, err := identifier.connection.Query(`
	SELECT SCHEMA_NAME, DIGEST_TEXT, SUM_TIMER_WAIT/1000000 AS TOTAL_WAIT_TIME_MS
	FROM performance_schema.events_statements_summary_by_digest
	ORDER BY TOTAL_WAIT_TIME_MS DESC
	LIMIT 10;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var w waitAvgQuery
		if err := rows.Scan(&w.schemaName, &w.digestText, &w.totalWaitTimeMs); err != nil {
			channel.ErrCh <- err
			continue
		}
		identifier.mysqlCtxChannel.waitAvgQueryChannel <- w
	}
	return nil
}

func (identifier *mysqlidentifier) occurErrorQueries() error {
	rows, err := identifier.connection.Query(`
	SELECT SCHEMA_NAME, DIGEST_TEXT, SUM_ERRORS AS ERRORS
	FROM performance_schema.events_statements_summary_by_digest
	WHERE SUM_ERRORS > 0
	ORDER BY ERRORS DESC
	LIMIT 10;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var o occurErrorQuery
		if err := rows.Scan(&o.schemaName, &o.digestText, &o.errors); err != nil {
			channel.ErrCh <- err
			continue
		}
		identifier.mysqlCtxChannel.occurErrorQueryChannel <- o
	}
	return nil
}
