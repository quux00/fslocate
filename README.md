# fslocate

A Clojure application that indexes files on a filesystem for rapid lookup.  This is a simple replacement for the Unix/Linux locate/updatedb functionality or the old Google Desktop system.

It is launched from the command line, either using leiningen or building a jar.

It runs in the background and runs until it finishes.  It should be run via a cron or [Windows Task Scheduler](http://www.iopus.com/guides/winscheduler.htm). 

## Requirements

fslocate keeps all indexed data in a sqlite database.  It requires SQLite3 to be installed. The database is stored in db/fslocate.db.


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
