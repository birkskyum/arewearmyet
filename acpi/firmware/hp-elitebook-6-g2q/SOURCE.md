# HP EliteBook 6 G2q

ACPI tables taken out of HP's SoftPaq [sp174817](https://ftp.hp.com/pub/softpaq/sp174501-175000/sp174817.exe), from the BIOS image src/bios/Y68_01050000.bin (board 8F2B, BIOS 01.05), whose ACPI.elf is a raw LZMA stream. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT_SDM8480_d0d5ded8.dat`
- `SSDT_HPQIOL3_a1863660.dat`
- `SSDT_HPQWMI_d8d0214e.dat`
- `SSDT_HWIDTabl_29309a6d.dat`
- `SSDT_RamDisk_7ef53287.dat`
- `SSDT_WMITable_2890e165.dat`
