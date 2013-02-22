CREATE TABLE files (id integer primary key, path text, type char);
CREATE INDEX path_idx on files (path);
