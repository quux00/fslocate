(defproject fslocate "0.1.0-SNAPSHOT"
  :description "File system indexer in Clojure like locate/updatedb on Unix"
  :url "http://example.com/FIXME"
  :license {:name "Eclipse Public License"
            :url "http://www.eclipse.org/legal/epl-v10.html"}
  :dependencies [[org.clojure/clojure "1.5.0-RC16"]
                 [org.clojure/java.jdbc "0.1.1"]
                 [org.xerial/sqlite-jdbc "3.7.2"]
                 [thornydev/go-lightly "0.3.2"]]
  :main thornydev.fslocate.core)
