# Dynabook XD5

ACPI tables taken out of Dynabook's BIOS 1.13 [bios_v113.exe](https://dynabook.com/assistpc/download/modify/dynabook/xd5/ap/bios/bios_v113.exe), whose image holds two ACPI sets, so there are two DSDTs (a and b). They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-xd5-a.dat`
- `DSDT-xd5-b.dat`
