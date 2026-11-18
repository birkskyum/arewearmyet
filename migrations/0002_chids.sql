-- The machine's own hardware IDs (CHIDs), as "type:guid" in a JSON list. They name its model the way a
-- boot image picks its device tree, and for a machine the board doesn't know they are what it needs added.
ALTER TABLE runs ADD COLUMN chids TEXT;
