(ns thornydev.fslocate
  (:refer-clojure :exclude [peek take])
  (:require [clojure.core :as clj]
            [clojure.java.jdbc :as jdbc]
            [clojure.string :as str]
            [clojure.java.io :as io]
            [clojure.set :refer [difference]]
            [thornydev.go-lightly.core :refer :all])
  (:import (java.util.concurrent CountDownLatch))
  (:gen-class))

;; TODO: once working, replace "file fns" with the https://github.com/Raynes/fs library

;; ---[ DESIGN ]--- ;;
;; 2 main flow-controls
;;  1. indexer: searches the existing data files
;;   - 1 or more go-lightly routines - specified in the *nindexers* global var
;;  2. a db handler thread that listens for incoming database execution requests
;;   - it listents to three go-lightly channels: query-ch, delete-ch and insert-ch
;;
;; indexer thread -> one or more
;;  reads conf file to start on fs
;;  grabs all files from that dir and queries db to get all recorded files from that dir
;;  compares: deletes those not present anymore, adds those newly present
;;  => but doesn't add/delete directly. pushes onto a queue or messages the dbupdater thread/routine
;; can have multiple indexer threads
;; possible race condition with dbupdater thread -> may want to have global db lock in memory so
;;  only one thread is reading/writing at a time?

;; resource contention
;;  => indexer threads periodically sleep for 1 minute?
;;     only runs twice per day?

(def sqlite-spec {:classname "org.sqlite.JDBC"
                  :subprotocol "sqlite"
                  :subname "db/fslocate.db"})

(def postgres-spec {:classname "org.postgresql.Driver"
                    :subprotocol "postgresql"
                    :subname "//localhost/fslocate"
                    :user "midpeter444"
                    :password "jiffylube"})

(def ^:dynamic *db-spec* postgres-spec)

(def ^:dynamic *verbose?* false)

(def ^:dynamic *nindexers* 2)

(def ^:dynamic *file-ignore-patterns* [".class"])

(def query-ch  (channel 1000))
(def delete-ch (channel 1000))
(def insert-ch (channel 1000))

(defmacro log [& vals]
  `(when *verbose?*
     (let [s# (apply str (interpose " " (map #(if (nil? %) "nil" %) [~@vals])))]
      (print (str s# "\n")) (flush)
      s#)))

;; use a delay here so that it is not evaluated until the dynamic *nindexers*
;; value is set
(def latch (delay (CountDownLatch. *nindexers*)))

(defn read-conf []
  (->> (slurp "conf/fslocate.conf")
       (str/split-lines)
       (mapv #(str/replace % #"/\s*$" ""))))

;; ---[ database fns ]--- ;;

(defn dbdelete
  "fname: string of full path for file/dir"
  [recordset]
  (log "Deleting" recordset)
  (doseq [r recordset]
    (jdbc/delete-rows :files ["lower(PATH) = ? and TYPE = ?"
                              (str/lower-case (:path r)) (:type r)])))

(defn dbinsert
  "recordset: set of records of form: {:type f|d :path abs-path}
  must be called within a with-connection wrapper"
  [recordset]
  (log "Inserting" recordset)
  (apply jdbc/insert-records :files recordset))

(defn dbquery
  "dirpath: abs-path to a directory
  must be called within a with-connection wrapper"
  [{:keys [dirpath reply-ch]}]
  (put reply-ch
       (if-let [origdir-rt (jdbc/with-query-results res
                             ["SELECT path, type FROM files WHERE path = ?" dirpath]
                             (doall res))]
         (flatten
          (cons origdir-rt
                (jdbc/with-query-results res
                  ["SELECT path, type FROM files WHERE type = ? AND lower(path) LIKE ? AND lower(path) NOT LIKE ?"
                   "f" (str/lower-case (str dirpath "/%")) (str/lower-case (str dirpath "/%/%"))]
                  (doall res))))
         false)))

(defn dbhandler []
  (jdbc/with-connection *db-spec*
    (loop []
      (selectf query-ch  #(dbquery %)
               insert-ch #(dbinsert %)
               delete-ch #(dbdelete %)
               (timeout-channel 2000) #(identity %))
      (recur))))

(defn partition-results
  "records should be of form: {:path /usr/local/bin, :type d}
  fs-recs: seq of file system records
  dbrecs: EITHER: seq of records from db query, OR: a boolean false (meaning the database
          had no records of this directory and its files
  @return: vector pair: [set of records only on the fs, set of records only in the db]"
  [fs-recs db-recs]
  (if db-recs
    (let [fs-set (set fs-recs)
          db-set (set db-recs)]
      [(difference fs-set db-set) (difference db-set fs-set)])
    [(set fs-recs) #{}]))


(defn ignore-file? [fname]
  (re-find #"\.class$" fname))

(defn create-file-records
  "files: (seq/coll of strings): files to sync with the db
  filters out files that meet a do-not-index criterion and
  returns file records of form {:path /path/to/file :type f}"
  [files]
  (->> files
       (filter #(not (ignore-file? %)))
       (map #(array-map :path % :type "f")))
  )

(defn sync-list-with-db
  "topdir: (string): directory holding the +files+
  files: (seq/coll of strings): files to sync with the db"
  [topdir files]
  (let [reply-ch (channel)
        _        (put query-ch {:dirpath topdir :reply-ch reply-ch})
        db-recs  (take reply-ch)
        fs-recs  (cons {:path topdir :type "d"} (create-file-records files))
        [fsonly dbonly]  (partition-results fs-recs db-recs)]
    ;; if there is something to insert or delete, put it on the right channel
    (when (seq fsonly) (put insert-ch fsonly))
    (when (seq dbonly) (put delete-ch dbonly))))

(defn list-dir
  "List files and directories under path."
  [^String path]
  (map #(str path "/" %) (seq (.list (io/file path)))))

(defn file?
  "Return true if path is a file."
  [path]
  (.isFile (io/file path)))

(defn skip-dir?
  "dir: String name of directory
  @return: true if should NOT index this (on the exclusion list)"
  [dir]
  (boolean (re-find #"\.git|\.svn" dir)))

(defn indexer
  "coll/seq of dirs (as strings) to index with the fslocate db"
  [search-dirs]
  (loop [dirs search-dirs]
    (log "Doing" (first dirs))
    (if-not (seq dirs)
      (do (log "before countDown: latch count: " (.getCount @latch))
          (.countDown @latch))
      (if (skip-dir? (first dirs))
        (recur (rest dirs))
        (let [[files subdirs] (->> (first dirs)
                                   list-dir
                                   (partition-bifurcate file?))]
          (sync-list-with-db (first dirs) files)
          (recur (concat (rest dirs) subdirs)))))))

(defn seq-contains? [coll key]
  (boolean (some #{key} coll)))

(defn calc-num-indexers
  "argv: command line arguments from user
  vdirs: vector of all the directories to index (from fslocate.conf)"
  [argv vdirs]
  (let [n (if (seq-contains? argv "-t")
            (get argv (inc (.indexOf argv "-t")))
            (max 3 (Math/ceil (/ (count vdirs) 2.0))))]
    (if (or (nil? n)
            (zero? (Integer/valueOf n)))
      (throw (IllegalStateException. "ERROR: Unable to determine how many threads to run."))
      (Integer/valueOf n))))

(defn lookup-file-ignore-patterns []
  (if (.exists (File. "conf/fslocate.ignore"))
    (->> (slurp "conf/fslocate.ignore")
         (str/split-lines)
         (filter #(not (re-matches #"^\s*$" %)))
         (mapv #(str/replace % #"/\s*$" "")))
    *file-ignore-patterns*
    )
  )

(defn do-fslocate-indexing
  "Executes the primary functionality of the indexing app"
  [argv]
  (let [vdirs (read-conf)]
    (binding [*verbose?* (seq-contains? argv "-v")
              *nindexers* (calc-num-indexers argv vdirs)
              *file-ignore-patterns* (lookup-file-ignore-patterns)]
      (println "Using " *nindexers* " indexing threads")
      (gox (dbhandler))

      (log "count vdirs: " (count vdirs))
      (log "math: " (/ (count vdirs) (double *nindexers*)))
      ;; TODO: this doesn't work -> I need a (partition-into-n *nindexers* vdirs)
      (log "parts: " (vec (partition-all (/ (count vdirs) (double *nindexers*)) vdirs)))

      (doseq [part (vec (partition-all (/ (count vdirs) (double *nindexers*)) vdirs))]
        (gox (indexer (vec part))))
      (.await @latch)
      (Thread/sleep 500)
      (shutdown))))

(defn show-help []
  (println "fslocate indexer takes three command line args:")
  (println "  -t NUM: number of indexing threads")
  (println "  -v    : verbose mode")
  (println "  :h    : this help screen"))

(defn -main
  "Args supported:
  -t NUM : number of indexing threads
  -v     :  verbose
  :h     :  show help"
  [& args]
  (if (seq-contains? args ":h")
    (show-help)
    (do-fslocate-indexing (vec args))))
