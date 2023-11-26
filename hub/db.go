package hub

import "github.com/sjy-dv/IZONE/internal/aggregator/vdb"

type DBConnector struct {
	User     string
	Password string
	Host     string
	Port     int
	Label    string
}

type (
	common struct {
		schemaName string
		digestText string
		fetch      bool
	}

	mostExec struct {
		common
		executionCount int64
	}

	slowAvgQuery struct {
		common
		avgExecutionTimeMs int64
	}

	waitAvgQuery struct {
		common
		totalWaitTimeMs int64
	}

	occurErrorQuery struct {
		common
		errors int64
	}
)

func Config() error {
	mysqlorg = &mysql{}
	mysqlorg.connections = make([]*mysqlidentifier, 0)

	return vdb.Load()
}
