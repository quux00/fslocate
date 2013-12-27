CREATE TABLE fsentry (
  id   SERIAL PRIMARY KEY,
  path text,
  type char(1),
  toplevel bool
);
CREATE INDEX ON fsentry ((lower(path)));
