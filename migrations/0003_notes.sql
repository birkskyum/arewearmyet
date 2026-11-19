-- Notes: what doesn't fit a check's result. A problem that takes a sequence of events to show, a way
-- around one, a pointer to work going on elsewhere, or anything else worth knowing about a model.
-- A note can carry logs, and anyone can answer it, so one owner's agent can pick up where another's stopped.

CREATE TABLE notes (
  id TEXT PRIMARY KEY,          -- like NOTE-K7Q2M9XH4P
  machine TEXT NOT NULL,        -- SHA-256 of the writer's machine token
  parent TEXT,                  -- the note this one answers, or NULL
  model TEXT,                   -- model id; an answer takes its note's
  check_id TEXT,                -- the check it is about, when it is about one
  kind TEXT NOT NULL,           -- problem, workaround, upstream, note or reply
  title TEXT,
  body TEXT NOT NULL,
  links TEXT,                   -- JSON list of links to where the work is
  status TEXT NOT NULL DEFAULT 'open',   -- open or solved, for problems
  kernel TEXT,
  os TEXT,
  packages TEXT,                -- JSON, as in runs
  agent TEXT,
  created TEXT NOT NULL,
  hidden INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX notes_model ON notes(model, created);
CREATE INDEX notes_parent ON notes(parent, created);

CREATE TABLE note_logs (
  note TEXT NOT NULL REFERENCES notes(id),
  name TEXT NOT NULL,           -- like kernel.log
  text TEXT NOT NULL,
  PRIMARY KEY (note, name)
);
