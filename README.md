# fslocate

A Clojure application that indexes files on a filesystem for rapid lookup.  This is a simple replacement for the Unix/Linux locate/updatedb functionality or the old Google Desktop system.

It is launched from the command line, either using leiningen or building a jar.

It runs in the background and runs until it finishes.  It should be run via a cron or [Windows Task Scheduler](http://www.iopus.com/guides/winscheduler.htm). 

## Requirements

*Note*: this is probably not something you will want to install. First, it is totally uncessary on any Unix/Linus with locate and updatedb and there are probably better options for Windows.  To use what I've got here, you'll need to have:

* Java 6 or 7
* Clojure 1.5
* PostgreSQL 9 or SQLite 3
* Go

An insanely heavyweight requirement for this type of thing.  I wrote to use on my Windows machine, to explore Go concurrency in Clojure (using my go-lightly library), and get some experience with using Go with a database.

fslocate keeps all indexed data in a SQLite or a PostgreSQL database.  Thus, it requires SQLite3 or PostgreSQL 9 to be installed.

*Note*: right now I don't have a Go client that can access a SQLite db.  Once I figure out how to use the SQLite drivers for Go, I'll add that.

#### SQLite

The database is stored in db/fslocate.db.  You will need to create the table and index in `db/sqlite.ddl`.

#### PostgreSQL

You will need to create an fslocate database and then create the table and index in `db/postgres.ddl`.

## Usage

### configuration

It is designed to only "crawl" the parts of the filesystem you want.  Specify absolute paths to the directories you want indexed in the fslocate.conf file in the conf directory.  One (absolute path) directory per line.

### launch the indexer

To run the indexer, just launch:

    lein run

or

    java -jar fslocate-uberjar.jar

### look up files in the index

You can query the sqlite database directly:

sqlite3 db/fslocate.db

I will be building a 'locate' app later.


## License

Copyright © 2013 Michael Peterson

Distributed under the Eclipse Public License, the same as Clojure.
