package model

import "database/sql"

type Override struct {
	Match   string
	Replace string
}

type Config struct {
	Path            string
	Host            string
	IsSmart         bool
	Secret          string
	MysqlServerConn *sql.DB
	CdnOrigin       string
	BucketName      string
	ResultStorage   string
}

type ServerConf struct {
	Host                string
	Port                string
	ThumborHost         string
	ThumborSecret       string
	MysqlServerHost     string
	MysqlServerPort     string
	MysqlServerUsername string
	MysqlServerPassword string
	MysqlServerDatabase string
	CdnOrigin           string
	BucketName          string
	ResultStorage       string
}
