-- That a part works on one computer because of a change made on it, like a kernel option or a
-- settings file of the owner's. It is the owner's word or their agent's reading, one sentence per
-- computer and check, and it stays until it is taken back. proved is 1 when switching the change
-- off and measuring the part again showed it.

CREATE TABLE needs (
  machine TEXT NOT NULL REFERENCES machines(id),
  check_id TEXT NOT NULL,
  text TEXT NOT NULL,
  how TEXT NOT NULL,            -- owner or agent
  proved INTEGER NOT NULL DEFAULT 0,
  kernel TEXT,
  created TEXT NOT NULL,
  PRIMARY KEY (machine, check_id)
);
