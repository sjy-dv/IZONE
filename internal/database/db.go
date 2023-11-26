package database

type DBConnector struct {
	User     string
	Password string
	Host     string
	Port     int
	Label    string
}

func Config() {
	mysqlorg = &mysql{}
	mysqlorg.connections = make([]*mysqlidentifier, 0)
}
