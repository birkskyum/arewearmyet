-- When the API last asked GitHub to build the site again, so a burst of results asks once a minute at most
CREATE TABLE rebuild (id INTEGER PRIMARY KEY CHECK (id = 1), asked TEXT NOT NULL);
INSERT INTO rebuild (id, asked) VALUES (1, '1970-01-01T00:00:00.000Z');
