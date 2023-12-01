package vdb

import (
	"time"

	"github.com/hashicorp/go-memdb"
)

type auditLog struct {
	Label   string
	log     string
	writeAt time.Time
}

func Cast(obj interface{}) *auditLog {
	return obj.(*auditLog)
}

func Output(log *auditLog) (time.Time, string) {
	return log.writeAt, log.log
}

var V *memdb.MemDB

func Load() error {
	schema := &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"auditlogs": {
				Name: "auditlogs",
				Indexes: map[string]*memdb.IndexSchema{
					"label": {
						Name:    "label",
						Unique:  false,
						Indexer: &memdb.StringFieldIndex{Field: "label"},
					},
				},
			},
		},
	}
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		return err
	}
	V = db
	return nil
}

func VdbColumn(label, log string) *auditLog {
	return &auditLog{
		Label:   label,
		log:     label,
		writeAt: time.Now(),
	}
}
