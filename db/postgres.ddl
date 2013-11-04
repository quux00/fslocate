CREATE TABLE files (
  id   SERIAL PRIMARY KEY,
  path text,
  type char(1),
  toplevel bool
);
CREATE INDEX ON files ((lower(path)));