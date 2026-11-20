-- How close a result looked: 'look' when it only says the part is there and its driver came up,
-- 'test' when the part was used. Empty for results from before hw-test said it. A look at a part
-- doesn't replace what a test of the same part found.
ALTER TABLE results ADD COLUMN depth TEXT;
