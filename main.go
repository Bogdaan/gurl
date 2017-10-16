package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"gurl/storage"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/cespare/xxhash"
)

// wait for http severs
var wg = &sync.WaitGroup{}

// create ascii hash for link
func makeBaseHash(link string) []byte {
	return []byte(strconv.FormatUint(xxhash.Sum64String(link), 36))
}

// find redirect by hask (key)
func findRedirect(hash string) string {
	foundLink := ""
	storage.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(storage.LinksBucket)
		foundLink = string(b.Get([]byte(hash)))
		return nil
	})
	return foundLink
}

// Send database backup
func dbBackupHandler(w http.ResponseWriter, req *http.Request) {
	err := storage.DB.View(func(tx *bolt.Tx) error {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="backup.db"`)
		w.Header().Set("Content-Length", strconv.Itoa(int(tx.Size())))
		_, err := tx.WriteTo(w)
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Send csv report table
func sendReport(data *[][]string, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	csvW := csv.NewWriter(w)
	for _, i := range *data {
		if len(i) > 0 && len(i[0]) > 0 {
			csvW.Write(i)
		}
	}
	csvW.Flush()
}

// Handle api server requests
func ctrlServerHandler(w http.ResponseWriter, r *http.Request) {
	endpointName := r.Method + ":" + string(r.URL.Path[1:])

	switch endpointName {
	// add redirect list
	case "POST:link/add":
		links := strings.Split(r.PostFormValue("link"), "\n")
		report := make([][]string, len(links))

		// time prefix for keys
		byteHash := make([]byte, 12)
		copy(byteHash, []byte((time.Now()).Format("0601")))

		err := storage.DB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(storage.LinksBucket)
			for _, link := range links {
				if len(link) == 0 {
					continue
				}
				copy(byteHash[4:], makeBaseHash(link))
				b.Put(byteHash, []byte(link))
				report = append(report,
					[]string{link, fmt.Sprintf("%s", byteHash)})
			}
			return nil
		})
		if err != nil {
			log.Println("rdb transaction error ", err)
		}
		sendReport(&report, w)
		return

	// get link by hash
	case "GET:link/byHash":
		hashPart := r.URL.Query().Get("hash")
		if hashPart != "" {
			w.Header().Set("Content-Type", "text/plain")
			if foundLink := findRedirect(hashPart); foundLink != "" {
				w.Write([]byte(foundLink))
			}
			return
		}
		return

	// get link list
	case "GET:link/list":
		startID := []byte(r.URL.Query().Get("start"))
		endID := []byte(r.URL.Query().Get("end"))

		reportLimit := 30
		report := make([][]string, reportLimit)

		err := storage.DB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(storage.LinksBucket)
			c := b.Cursor()

			if len(startID) == 0 {
				startID, _ = c.First()
			}

			isNoEnd := len(endID) == 0
			for k, v := c.Seek(startID); k != nil && reportLimit > 0 && (isNoEnd || bytes.Compare(k, endID) <= 0); k, v = c.Next() {
				report = append(report, []string{
					fmt.Sprintf("%s", k), fmt.Sprintf("%s", v)})
				reportLimit--
			}
			return nil
		})
		if err != nil {
			log.Println("rdb transaction error ", err)
		}
		sendReport(&report, w)
		return

	// remove redirect by hash
	case "POST:hash/remove":
		hashes := strings.Split(r.PostFormValue("hash"), "\n")
		report := make([][]string, len(hashes))

		err := storage.DB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(storage.LinksBucket)
			for _, hash := range hashes {
				if len(hash) == 0 {
					continue
				}
				b.Delete([]byte(hash))
				report = append(report, []string{hash})
			}
			return nil
		})
		if err != nil {
			log.Println("rdb transaction error ", err)
		}
		sendReport(&report, w)
		return

	// remove previous hashes
	case "POST:hash/cleanup":
		startID := []byte(r.PostFormValue("start"))
		endID := []byte(r.PostFormValue("end"))
		totalRemoved := 0

		err := storage.DB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(storage.LinksBucket)
			c := b.Cursor()

			if len(startID) == 0 {
				startID, _ = c.First()
			}
			for k, _ := c.Seek(startID); k != nil && bytes.Compare(k, endID) <= 0; k, _ = c.Next() {
				b.Delete(k)
				totalRemoved++
			}
			return nil
		})
		if err != nil {
			log.Println("rdb transaction error ", err)
		}
		report := [][]string{{"total", fmt.Sprintf("%v", totalRemoved)}}
		sendReport(&report, w)
		return

	// download rdb copy
	case "GET:backup":
		dbBackupHandler(w, r)
		return

	}
	w.WriteHeader(http.StatusNotFound)
}

// Handle User-wide http requests. Find and create redirect or return 404
func redirectServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && len(r.URL.Path) > 20 {
		redirectURL := findRedirect(r.URL.Path[1:])
		if len(redirectURL) > 0 {
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// Create http.Server instance, listen and serve request in gorutine
func createHTTPServer(listenOn string, rqHandler http.HandlerFunc) *http.ServeMux {
	srv := http.NewServeMux()
	srv.HandleFunc("/", rqHandler)

	wg.Add(1)
	go func() {
		log.Printf("Start http server at %s", listenOn)
		err := http.ListenAndServe(listenOn, srv)
		if err != nil {
			log.Fatal("Unable to start server: ", err)
			wg.Done()
		}
	}()

	return srv
}

// Start and listen two HTTP servers.
func main() {
	var (
		c2cAddr = flag.String("api-address", ":7070", "Control server bind address")
		rdrAddr = flag.String("redirect-address", ":8090", "Redirect server bind address")
		dbPath  = flag.String("database", "links.db", "Database file path")
	)
	flag.Parse()

	storage.Init(*dbPath)
	defer storage.DB.Close()

	createHTTPServer(*c2cAddr, ctrlServerHandler)
	createHTTPServer(*rdrAddr, redirectServerHandler)

	wg.Wait()
}
