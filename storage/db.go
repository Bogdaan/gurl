package storage

import (
	"log"
	"time"

	"github.com/boltdb/bolt"
)

// LinksBucket contain redirects (link hash) -> (link)
var LinksBucket = []byte("links")

// DB - bolt database instance
var DB *bolt.DB

// Init database connection and structure
func Init(pathToFile string) {
	var err error
	DB, err = bolt.Open(pathToFile, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		log.Fatalf("Db connection: %s", err)
	}

	DB.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists(LinksBucket)
		return nil
	})
}
