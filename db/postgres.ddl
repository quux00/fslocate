CREATE TABLE files (
  id   SERIAL PRIMARY KEY,
  path text,
  type char(1)
);
CREATE INDEX ON files ((lower(path)));
