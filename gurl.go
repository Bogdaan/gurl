package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Bogdaan/gurl/storage"

	"github.com/boltdb/bolt"
	"github.com/cespare/xxhash"
)

// wait for http severs
var wg = &sync.WaitGroup{}

// create ascii hash for link (base36)
func makeBaseHash(link string) []byte {
	return []byte(strconv.FormatUint(xxhash.Sum64String(link), 36))
}

// find redirect by hask (key)
func findRedirect(hash string) string {
	foundLink := ""
	if len(hash) == 0 {
		return foundLink
	}
	storage.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(storage.LinksBucket)
		foundLink = string(b.Get([]byte(hash)))
		return nil
	})
	return foundLink
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

// api server error
type apiError struct {
	Error   error
	Code    int
	Message string
}

// api endpoint function
type apiHandler func(http.ResponseWriter, *http.Request) *apiError

// api server handler
func (fn apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		if err.Error != nil && err.Code >= 500 {
			log.Println(err.Error)
		}
		http.Error(w, err.Message, err.Code)
	}
}

// add redirect list
func apiAddLink(w http.ResponseWriter, r *http.Request) *apiError {
	links := strings.Split(r.PostFormValue("link"), "\n")
	linksLen := len(links)
	if linksLen == 0 {
		return &apiError{nil, 500, "Empty 'link' argument"}
	}

	report := make([][]string, linksLen)
	byteHash := make([]byte, 12)
	// time prefix for keys
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
		return &apiError{err, 500, "Db error"}
	}
	sendReport(&report, w)
	return nil
}

// get link by hash
func apiLinkByHash(w http.ResponseWriter, r *http.Request) *apiError {
	hashPart := r.URL.Query().Get("hash")
	if foundLink := findRedirect(hashPart); foundLink != "" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(foundLink))
		return nil
	}
	return &apiError{nil, 404, "not found"}
}

// get link list
func apiLinkList(w http.ResponseWriter, r *http.Request) *apiError {
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
		return &apiError{err, 500, "Db error"}
	}
	sendReport(&report, w)
	return nil
}

// remove redirect by hash
func apiRemoveHash(w http.ResponseWriter, r *http.Request) *apiError {
	hashes := strings.Split(r.PostFormValue("hash"), "\n")
	hashesLen := len(hashes)
	if hashesLen == 0 {
		return &apiError{nil, 500, "Empty 'hash' argument"}
	}

	report := make([][]string, hashesLen)

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
		return &apiError{err, 500, "Db error"}
	}
	sendReport(&report, w)
	return nil
}

// remove bundle of hashes
func apiCleanupHash(w http.ResponseWriter, r *http.Request) *apiError {
	startID := []byte(r.PostFormValue("start"))
	endID := []byte(r.PostFormValue("end"))

	totalRemoved := 0
	if len(startID) == 0 && len(endID) == 0 {
		return &apiError{nil, 500, "Both empty: 'start' 'end'"}
	}

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
		return &apiError{err, 500, "Db error"}
	}
	report := [][]string{{"total", fmt.Sprintf("%v", totalRemoved)}}
	sendReport(&report, w)
	return nil
}

// Send database backup
func apiBackup(w http.ResponseWriter, r *http.Request) *apiError {
	err := storage.DB.View(func(tx *bolt.Tx) error {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="backup.db"`)
		w.Header().Set("Content-Length", strconv.Itoa(int(tx.Size())))
		_, err := tx.WriteTo(w)
		return err
	})
	if err != nil {
		return &apiError{err, 500, "Db error"}
	}
	return nil
}

// Handle User-wide http requests. Find and create redirect or return 404
func redirectServerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && len(r.URL.Path) >= 13 {
		redirectURL := findRedirect(r.URL.Path[1:13])
		if len(redirectURL) > 0 {
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// Bind server to address, listen inside gorutine
func bindServerTo(address string, srv *http.ServeMux) {
	wg.Add(1)
	go func() {
		log.Printf("Start http server at %s", address)
		err := http.ListenAndServe(address, srv)
		if err != nil {
			log.Fatal("Unable to start server: ", err)
			wg.Done()
		}
	}()
}

// Start and listen two HTTP servers.
func main() {
	var (
		apiAddr = flag.String("api-address", ":7070", "Control server bind address")
		rdrAddr = flag.String("redirect-address", ":8090", "Redirect server bind address")
		dbPath  = flag.String("database", "links.db", "Database file path")
	)
	flag.Parse()

	storage.Init(*dbPath)
	defer storage.DB.Close()

	// redirects
	redirectSrv := http.NewServeMux()
	redirectSrv.HandleFunc("/", redirectServerHandler)
	bindServerTo(*rdrAddr, redirectSrv)

	// api
	apiSrv := http.NewServeMux()
	apiSrv.Handle("/link/add", apiHandler(apiAddLink))
	apiSrv.Handle("/link/byHash", apiHandler(apiLinkByHash))
	apiSrv.Handle("/link/list", apiHandler(apiLinkList))
	apiSrv.Handle("/hash/remove", apiHandler(apiRemoveHash))
	apiSrv.Handle("/hash/cleanup", apiHandler(apiCleanupHash))
	apiSrv.Handle("/backup", apiHandler(apiBackup))
	bindServerTo(*apiAddr, apiSrv)

	wg.Wait()
}
