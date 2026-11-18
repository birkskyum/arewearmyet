-- Owners' reports on what works, from probe.sh or an agent working with the owner.
-- A machine is known only by the SHA-256 of a random token kept on it, so a laptop's
-- later runs can replace its earlier ones without any account.

CREATE TABLE machines (
  id TEXT PRIMARY KEY,          -- SHA-256 of the machine's token
  model TEXT,                   -- model id from data/models.yaml, when matched
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  hidden INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE runs (
  id TEXT PRIMARY KEY,          -- like RUN-K7Q2M9XH4P
  machine TEXT NOT NULL REFERENCES machines(id),
  model TEXT,
  compatible TEXT,              -- the device tree's first compatible string
  created TEXT NOT NULL,
  mode TEXT NOT NULL,           -- quick, guided or agent
  kernel TEXT,
  os TEXT,
  packages TEXT,                -- JSON: package versions that matter (kernel, firmware, mesa, systemd)
  hardware TEXT,                -- JSON: maker, product, family, sku, board, bios (no serials)
  agent TEXT,                   -- the agent's name, when one sent it
  hidden INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX runs_model ON runs(model, created);
CREATE INDEX runs_machine ON runs(machine, created);

CREATE TABLE results (
  run TEXT NOT NULL REFERENCES runs(id),
  check_id TEXT NOT NULL,
  state TEXT NOT NULL,          -- works, partial, broken, untested, absent
  how TEXT NOT NULL,            -- measured, owner or agent
  note TEXT,
  evidence TEXT,                -- what the probe saw, short
  PRIMARY KEY (run, check_id)
);

CREATE TABLE fixes (
  id TEXT PRIMARY KEY,
  model TEXT NOT NULL,
  check_id TEXT,
  url TEXT NOT NULL,
  note TEXT,
  machine TEXT,
  created TEXT NOT NULL,
  hidden INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX fixes_model ON fixes(model, created);
