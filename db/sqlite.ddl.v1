CREATE TABLE fsentry(
  id integer primary key, 
  path text NOT NULL UNIQUE, 
  type char,
  toplevel TINYINT
);

CREATE INDEX path_idx on fsentry (path);
CREATE INDEX path_lower_idx ON fsentry (path COLLATE NOCASE);
