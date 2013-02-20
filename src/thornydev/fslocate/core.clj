(ns thornydev.fslocate.core
  (:refer-clojure :exclude [peek take])
  (:require [clojure.core :as clj]
            [clojure.java.jdbc :as jdbc]
            [clojure.string :as str]
            [clojure.java.io :as io]
            [clojure.set :refer [difference]]
            [thornydev.go-lightly.core :refer :all])
  (:import (java.util.concurrent CountDownLatch)))

;; TODO: once working, replace "file fns" with the https://github.com/Raynes/fs library

(def ^:dynamic *db-spec* {:classname "org.sqlite.JDBC"
                          :subprotocol "sqlite"
                          :subname "db/fsupdate.db"})

(def ^:dynamic *nindexers* 2)

(def query-ch (channel 5000))
(def delete-ch (channel 5000))
(def insert-ch (channel 5000))

;; ---[ DESIGN ]--- ;;
;; 3 threads (as go-lightly routines)
;; 1. indexer: searches the existing data files

;; indexer thread -> one or more
;;  reads conf file to start on fs
;;  grabs all files from that dir and queries db to get all recorded files from that dir
;;  compares: deletes those not present anymore, adds those newly present
;;  => but doesn't add/delete directly. pushes onto a queue or messages the dbupdater thread/routine
;; can have multiple indexer threads
;; possible race condition with dbupdater thread -> may want to have global db lock in memory so
;;  only one thread is reading/writing at a time?

;; dbupdater thread => ONLY ONE
;;  reads from queue of SQL inserts/deletes/updates that get added by indexer threads
;;  performs updates on the db directly

;; resource contention
;;  => indexer threads periodically sleep for 1 minute?
;;     only runs twice per day?

(defn prf [& vals]
  (let [s (apply str (interpose " " (map #(if (nil? %) "nil" %) vals)))]
    (print (str s "\n")) (flush)
    s))

(def latch (CountDownLatch. *nindexers*))

(defn read-conf []
  (str/split-lines (slurp "conf/fslocate.conf")))

;; ---[ database fns ]--- ;;

;; (defn fetch [query]
;;   (jdbc/with-connection *db-spec*
;;     (jdbc/with-query-results res query
;;       (doall res))))


;; (defn in-db? [fname]
;;   (not= 0 (->> fname
;;                (vector "SELECT count(*) as cnt FROM files WHERE path = ?")
;;                fetch
;;                first
;;                :cnt)))

(defn dbdelete
  "fname: string of full path for file/dir"
  [recordset]
  (doseq [r recordset]
    (jdbc/delete-rows :files ["PATH = ? and TYPE = ?" (:path r) (:type r)])))

(defn dbinsert
  "recordset: set of records of form: {:type f|d :path abs-path}
  must be called within a with-connection wrapper"
  [recordset]
  (apply jdbc/insert-records :files recordset))

(defn dbquery
  "dirpath: abs-path to a directory
  must be called within a with-connection wrapper"
  [{:keys [dirpath reply-ch]}]
  (put reply-ch
       (if-let [origdir-rt (jdbc/with-query-results res
                             ["SELECT path FROM files WHERE path = ?" dirpath]
                             (doall res))]
         (flatten
          (cons origdir-rt
                (jdbc/with-query-results res
                  ["SELECT path FROM files WHERE type = ? AND path LIKE ? AND path NOT LIKE ?"
                   \f (str dirpath "/%") (str dirpath "/%/%")]
                  (doall res))))
         false)))

(defn dbhandler []
  (jdbc/with-connection *db-spec*
    ;; TODO: need an atom to check state against for sleeping/pausing/shutting down
    (loop []
      (selectf query-ch  #(dbquery %)
               insert-ch #(dbinsert %)
               delete-ch #(dbdelete %)
               (timeout-channel 2000) #(identity %))
      (recur))))


;; (defn partition-bifurcate [f coll]
;;   (reduce (fn [[vecyes vecno] value]
;;             (if (f value)
;;               [(conj vecyes value) vecno]
;;               [vecyes (conj vecno  value)])) [[] []] coll))

;; LEFT OFF: needs to change
;; needs to go through each subdir returning all files and all dirs (as separate seqs)
;; in a given directory
;; the files will be queried in one query and get back one or more records confirming whether
;;  they are in the db or not
;; if not, they will be queued up for insertion
;; if yes, then ignore (move on)
;; could also add in logic to diff the lists from the fs and the db
;;   -> delete those in the db, not on the fs
;;   -> insert those in the fs, not in the db
;; (defn indexer [dirs update-ch]
;;   (apply prf "List to search:" dirs)
;;   (doseq [sdir dirs]
;;     (let [fdir (io/file sdir)]
;;       (doseq [f (file-seq fdir) :let [fname (.getAbsolutePath f)]]
;;         (when-not (in-db? fname)
;;           (put update-ch fname)
;;           (prf "just queued: " fname)))))
;;   (.countDown latch))

(defn partition-results
  "records should be of form: {:path /usr/local/bin, :type d}
  fs-recs: seq of file system records
  dbrecs: seq of records from db query
  @return: vector pair: [set of records only on the fs, set of records only in the db]"
  [fs-recs db-recs]
  (let [fs-set (set fs-recs)
        db-set (set db-recs)]
    [(difference fs-set db-set) (difference db-set fs-set)]))

(defn sync-list-with-db
  "topdir: (string): directory holding the +files+
  files: (seq/coll of strings): files to sync with the db"
  [topdir files]
  ;; TODO: compare f/d status of each fs entry
  ;; STEP 1: query for all files in db under topdir
  ;; STEP 2: make two sets: 1) on fs, not in db; 2) in db, not on fs;
  ;; STEP 3: insert from db those in #1
  ;; STEP 4: delete from db those in #2
  (let [reply-ch (channel)
        _        (put query-ch {:dirpath topdir :reply-ch reply-ch})
        db-recs  (take reply-ch)
        fs-recs  (cons {:path topdir :type \d} (map #(array-map :path % :type \f) files))
        [fsonly dbonly]  (partition-results fs-recs db-recs)]
    (put insert-ch fsonly)
    (put delete-ch dbonly)))

(defn list-dir
  "List files and directories under path."
  [^String path]
  (map #(str path "/" %) (seq (.list (io/file path)))))

(defn file?
  "Return true if path is a file."
  [path]
  (.isFile (io/file path)))

(defn indexer
  "coll/seq of dirs (as strings) to index with the fslocate db"
  [search-dirs]
  (loop [dirs search-dirs]
    (if-not (seq dirs)
      (.countDown latch)
      (let [[files subdirs] (->> (first dirs)
                                 list-dir
                                 (partition-bifurcate file?))]
        (sync-list-with-db (first dirs) files)
        (recur (conj (rest dirs) subdirs))))))

(defn -main [& args]
  (let [vdirs (read-conf)
        parts (vec (partition-all (/ (count vdirs) *nindexers*) vdirs))]
    (gox (dbhandler))
    (doseq [p parts]
      (gox (indexer2 (vec p))))
    ;; (gox (dbupdater))
    )
  (.await latch)
  (Thread/sleep 1000)
  (stop)
  )
