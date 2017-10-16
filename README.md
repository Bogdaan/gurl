# gurl

Hight perfomance and compact url shortening server.

## Features

* Create short urls (by url hash - https://github.com/cespare/xxhash)
* Store links as k-v (https://github.com/boltdb/bolt)
* Redirect lifetime tracking (can sort redirect by date, or remove old redirects)


```
+---------------------+
|                     |
|         Gurl        |
|                     |
|  +---------------+  |    +-----------------+
|  |               |  |    |                 |
|  |      api      <-------+   You backend   |
|  |               |  |    |                 |
|  +---------------+  |    +--------^--------+
|                     |             |
|                     |             |
|  +---------------+  |    +--------+--------+      +-----------------+
|  |               |  |    |                 |      |                 |
|  |    redirect   <-------+  reverse proxy  <------+       User      |
|  |               |  |    |                 |      |                 |
|  +---------------+  |    +-----------------+      +-----------------+
|                     |
+---------------------+
```

## Why gurl?

You can find lots of solution at [github](https://github.com/topics/url-shortener?o=desc&s=stars) but:

1. It requires remote storage (like mysql or MongoDB) or operate with low-performance storage system (like sqlite, or own file storate).

2. It depdends on Mysql or PostgreSQL features (like AUTO_INCREMENT etc.)

3. It Mix api server and redirect server (same server handle both requests)

4. It marshal / unmarshal JSON

4. It contain complex logic, validation, huge data structures

Gurl devoid these disadvantages.

## Internals

Gurl start two HTTP servers:

+ **api server** - manage links (crete, find, update etc.)
+ **redirect server** - just find link, and send 301 (or 404)

```
$ gurl --help
Usage of gurl:
  -api-address string
    	Control server bind address (default ":7070")
  -database string
    	Database file path (default "links.db")
  -redirect-address string
    	Redirect server bind address (default ":8090")
```

### Storage system

Each short link represents a [12]byte array:

1. 4 byte time preffix, with format "0601" (see [time](https://golang.org/pkg/time/#Time.Format)).
2. 8 byte hash, created by xxhash and encoded in base36

### Api server interface

#### POST link/add

Create one (or more) short urls and returns CSV with hashes. Arguments:

* link - string (requred) - links for shortening, each in new line

```bash
$ curl -X POST -d $'link=http://1.x\nhttp://2.x' http://localhost:7070/link/add
http://1.x,17107z1g4rbg
http://2.x,17101wa6cbnd
```

#### GET link/byHash

Find full link by hash. Arguments:

* hash - string (requred)

```bash
$ curl -X GET http://localhost:7070/link/byHash?hash=17101wa6cbnd
http://2.x
```

#### GET link/list

Find list of hashes (part of sorted list) by arguments:

* start - string (optional)
* end - string (optional)

```bash
$ curl -X GET 'http://localhost:7070/link/list?start=17101wa6cbnd&end=17102wa6cbnd'
17101wa6cbnd,http://2.x
171020y8fgzn,http://x2.x2
17102ewiejoz,http://xx.xx
17102msb5big,http://v.v
```

#### POST hash/remove

Remove hashes. Arguments:

* hash - string (required) - one ore more hashes, each in new line

```bash
$ curl -X POST -d 'hash=17101wa6cbnd' 'http://localhost:7070/hash/remove'
17101wa6cbnd
```

#### POST hash/cleanup

Remove hashes by range (probably day range). Arguments:

* start (optional)
* end (optional)

```bash
$ curl -X POST -d 'start=171020y8fgzn&end=17102msb5big' 'http://localhost:7070/hash/cleanup'
total,3
```

#### GET backup

Build consistent storage backup.

```bash
$ curl -X GET 'http://localhost:7070/backup'
... cut
```

## Related projects

+ [xxhash](https://github.com/cespare/xxhash) - high-quality hashing algorithm that is much faster than anything in the Go standard library
+ [bolt](https://github.com/boltdb/bolt) - pure Go key/value store
+ [gurl-cli](#) - command line util for system admin
