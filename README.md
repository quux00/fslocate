**[Requirements](#requirements)** |
**[Usage - Build and Test](#usage1)** |
**[Usage - Run](#usage2)** |
**[Status](#status)** |
**[License](#license)** |

# fslocate

A Go application that indexes file names (not content) on a filesystem for rapid lookup.  This is a simple replacement for the Unix/Linux locate/updatedb functionality or the old Google Desktop system.  It has been tested on Linux (Xubuntu) and Windows 7.

It is launched from the command line in order to either index the entries you specify (in a config file) or search for indexed paths that have already been indexed.  Unlike locate/updatedb, you use `fslocate` for both commands.  See Usage for details.

The indexer runs until it finishes with a variable number of indexers.  It can be run via a cron or [Windows Task Scheduler](http://www.iopus.com/guides/winscheduler.htm). 


<a name="requirements"></a>
## Requirements

* Go (tested with 1.1.2 through 1.4.2)


### Implementation

This is now the default implementation.  All records are written to a plaintext file with record separators. This is the "boyer" format.

Versions 0.5 and 1.0 also had code to run this with PostgreSQL.  That code has been removed from this version to simplify it, since the text database file is fast enough for my purposes.  You can get the previous versions from the git history (tags are `v0.5` and `v1.0`).

<a name="usage1"></a>
## Usage - Build and Test

### configuration

fslocate is designed to only index the parts of the filesystem you want.  Specify absolute paths to the directories you want indexed in the `conf/fslocate.indexlist` file in the conf directory.  One (absolute path) directory per line.

You can also specify patterns, files and directories you do not want indexed.  Put those in the `conf/fslocate.ignore` files.  See the notes at the top of that file for how the patterns are specified.


### build

Then clone this GitHub repo:

    git clone https://github.com/quux00/fslocate.git

or

    git clone git@github.com:quux00/fslocate.git


Assuming you have $GOROOT and $GOPATH properly set up, cd into the fslocate directory and compile and install with:

    go install


### edit the config files

In the conf dir, there are three files to edit:

    $ tree conf/
    conf/
    ├── fslocate.ignore
    └── fslocate.indexlist

Put one or more "top level directories" to search.  `fslocate` will **not** search your whole hard drive by default.  It will only index from the parent directories you specify.

Put a list of dirs and patterns to ignore in `fslocate.ignore`.  See the note at the top of that file for details.

Put your database username and password in `fslocate.conf` (only needed if using PostgreSQL as your database).

## create a db directory
Create an empty `db` directory (in the fslocate directory; it will be a sibling directory to `conf`). The output of `fslocate -i` will be stored here.

<a name="usage2"></a>
## Usage - Run

You need to run `fslocate` from the a directory with the `conf` directory (see above) in the current path.  I recommend creating a shell script like so:

    #!/bin/bash
    d=`pwd`
    cd $GOPATH/src/fslocate
    $GOPATH/bin/fslocate $@
    cd $d

Call it `fslocate.sh` and create an alias to it:

    alias fslocate=$HOME/bin/fslocate.sh



### launch the indexer

To view options:

    $ fslocate -h
    Usage: [-hv] [-t NUM] fslocate search-term | -i
      fslocate <search-term>
      fslocate -i  (run the indexer)
         -v     : verbose mode
         -h     : show help


To run the indexer:

    fslocate -i    

By default it runs with three indexers (goroutines that scan the filesystem) and one database handler (to do all queries, inserts and deletes).  Currently, you can specify the number of indexers with the `-t` command line option.  The number of db handlers is fixed at 1.


### look up files in the index

    fslocate mysearchterm

Searching is case insensitive.  You can only search for one term at a time.  If a file name has spaces, put quotes around it.

----

<a name="status"></a>
## Status

Currently, I haven't tested this on really large filesystems (I currently have about 120,000 entries indexed).  I know one limitation is the `dirChan` channel buffer size of 10,000 (in `indexer.go`) will be a limiting factor.  If you run it on some large file system, edit the DIRCHAN_BUFSZ constant to some really big number.  On my system with 16GB RAM, fslocate takes about 0.1% of memory while it is indexing, so it very lightweight. Thus, increasing this buffer size significantly is no big deal on most modern systems.

Also, there is no way to throttle the code and tell it to go slowly and use less CPU or disk IO.  That wouldn't be hard to add if people want it.


<a name="license"></a>
## License

Copyright © 2013 Michael Peterson

Distributed under the GNU General Public License version 2: [GPLv2](https://www.gnu.org/licenses/gpl-2.0.html).
