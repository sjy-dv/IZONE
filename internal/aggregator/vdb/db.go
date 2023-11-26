package vdb

import (
	"time"

	"github.com/hashicorp/go-memdb"
)

type auditLog struct {
	label   string
	log     string
	writeAt time.Time
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
						Unique:  true,
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
		label:   label,
		log:     label,
		writeAt: time.Now(),
	}
}
