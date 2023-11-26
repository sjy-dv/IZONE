package hub

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

type mysql struct {
	mu          sync.Mutex
	connections []*mysqlidentifier
}

type mysqlidentifier struct {
	connection *sql.DB
	DBConnector
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
	mysqlorg.connections = append(mysqlorg.connections, &mysqlidentifier{
		connection:  db,
		DBConnector: *dbconnector,
	})
	return nil
}
