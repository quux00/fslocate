DROP TABLE IF EXISTS fsentry;

CREATE TABLE fsentry (
  id   SERIAL PRIMARY KEY,
  path text,
  type char(1),
  toplevel bool
);
ALTER TABLE fsentry ADD UNIQUE (path);

CREATE INDEX ON fsentry (path);
CREATE INDEX ON fsentry ((lower(path)));        
