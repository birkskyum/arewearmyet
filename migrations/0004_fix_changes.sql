-- What a fix changed, as measured: for each check tested before and after it, its state on both
-- sides, with the kernel and package versions on both sides. This is what ties a change in what
-- works to the pull request that made it.
ALTER TABLE fixes ADD COLUMN changes TEXT;    -- JSON list of {check, from: {state, how, evidence, note}, to: {...}}
ALTER TABLE fixes ADD COLUMN versions TEXT;   -- JSON {before: {kernel, packages}, after: {kernel, packages}}
CREATE INDEX fixes_url ON fixes(url);
