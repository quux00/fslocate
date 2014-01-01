# fslocate

A Go application that indexes files on a filesystem for rapid lookup.  This is a simple replacement for the Unix/Linux locate/updatedb functionality or the old Google Desktop system.  It has been tested on Linux (Xubuntu) and Windows 7.

It is launched from the command line in order to either index the entries you specify (in a config file) or search for indexed paths that have already been indexed.  Unlike locate/updatedb, you use `fslocate` for both commands.  See Usage for details.

The indexer runs until it finishes with a variable number of indexers.  It can be run via a cron or [Windows Task Scheduler](http://www.iopus.com/guides/winscheduler.htm). 


## Requirements

* Go (tested with 1.1.2)
* PostgreSQL (tested with 9.1)
  * I use the Blake Mizerany's pure Go PostgreSQL driver: https://github.com/bmizerany/pq
* fslocate could also be made to work with SQLite 3
  * I've written the DDL for SQLite and the database code all runs from a single goroutine, so it should be safe to use with SQLite.  The code would have to modified in a few places, a SQLite Go library pulled in and everything recompiled.  If somebody wants that, let me know.

#### PostgreSQL

You will need to create database called `fslocate` and then create the table and indexes in `db/postgres.ddl`.

#### SQLite

The database is stored in db/fslocate.db.  You will need to create the table and index in `db/sqlite.ddl`.  (Again the code isn't ready to work with SQLite yet.)


## Usage

### configuration

fslocate is designed to only "crawl" the parts of the filesystem you want.  Specify absolute paths to the directories you want indexed in the `conf/fslocate.conf` file in the conf directory.  One (absolute path) directory per line.

You can also specify patterns, files and directories you do not want indexed.  Put those in the `conf/fslocate.ignore` files.  See the notes at the top of that file for how the patterns are specified.


### build

After you have the fslocate database set up, next install the Go PostgreSQL driver:

    go get github.com/bmizerany/pq

Then clone this GitHub repo:

    git clone https://github.com/quux00/fslocate.git

or

    git clone git@github.com:quux00/fslocate.git


Assuming you have $GOROOT and $GOPATH properly set up, cd into the fslocate directory and compile and install with:

    go install


### test

To run the tests, you'll need to have a PostgreSQL db called `testfslocate` using the same schema in `db/postgres.ddl`


### edit the config files

In the conf dir, there are three files to edit:

    $ tree conf/
    conf/
    ├── fslocate.conf
    ├── fslocate.ignore
    └── fslocate.indexlist

Put your database username and password in fslocate.conf.

Put a list of dirs and patterns to ignore in fslocate.ignore.  See the note at the top of that file for details.

Put one or more "top level directories" to search.  `fslocate` will not search your whole hard drive by default.  It will only index from the parent directories you specify.


### launch the indexer

To view options:

    fslocate -h
    midpeter444:~/lang/go/projects/src/fslocate$ fslocate -h
    Usage: [-hv] [-t NUM] fslocate search-term | -i
      fslocate <search-term>
      fslocate -i  (run the indexer)
         -t NUM : specify number of indexer threads (default=3)
         -v     : verbose mode
         -h     : show help


To run the indexer:

    fslocate -i    

By default it runs with three indexers (goroutines that scan the filesystem) and one database handler (to do all queries, inserts and deletes).  Currently, you can specify the number of indexers with the `-t` command line option.  The number of db handlers is fixed at 1.


### look up files in the index

    fslocate mysearchterm

Searching is case insensitive.  You can only search for one term at a time.  If a file name has spaces, put quotes around it.

Or you can query the PostgreSQL database directly:

    psql fslocate
    fslocate=> \d fsentry
                                 Table "public.fsentry"
      Column  |     Type     |                      Modifiers                       
    ----------+--------------+------------------------------------------------------
     id       | integer      | not null default nextval('fsentry_id_seq'::regclass)
     path     | text         | 
     type     | character(1) | 
     toplevel | boolean      | 
    Indexes:
        "fsentry_pkey" PRIMARY KEY, btree (id)
        "fsentry_path_key" UNIQUE CONSTRAINT, btree (path)
        "fsentry_lower_idx" btree (lower(path))
        "fsentry_path_idx" btree (path)



## Status

Currently, I haven't tested this on really large filesystems (I currently have about 120,000 entries indexed).  I know one limitation is the channel buffer size of 10,000 will be a limiting factor.  If you run it on some large file system, edit the DIRCHAN_BUFSZ constant to some really big number (currently I have it at 10,000).  On my system with 16GB RAM, fslocate takes about 0.1% of memory while it is indexing, so it very lightlweight. Therefore, increasing this buffer size significantly is no big deal on most modern systems.

Also, there is no way to throttle the code and tell it to go slowly and use less CPU (most PostgreSQL is the one churning away).  That wouldn't be hard to add if people want it.


## License

Copyright © 2013 Michael Peterson

Distributed under the GNU General Public License version 2: [GPLv2](https://www.gnu.org/licenses/gpl-2.0.html).
