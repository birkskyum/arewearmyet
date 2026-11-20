-- What the computer ran that Omarchy didn't install, as JSON: another kernel package, packages with
-- Omarchy's names built by someone else, kernel modules, kernel options and hardware packages of the
-- owner's own, a test device tree, and the owner's words on what they changed. Names only. A result
-- is read against it, since a part that works with the owner's module doesn't work yet for the next owner.
ALTER TABLE runs ADD COLUMN setup TEXT;
